package db

import (
	"fmt"
	"sort"
	"strconv"
	"time"

	"github.com/sirupsen/logrus"
)

// Reads served from the rollup tiers (see usage_rollup_plan.go for the
// segment plan and usage_rollup_build.go for how buckets get built).

// rollupSegmentRange returns the inclusive raw-scan bounds of a segment. Bounds
// that are the caller's own are passed through untouched; derived ones are
// converted to server-local so the driver formats them with the offset the
// records were stored with.
func rollupSegmentRange(seg rollupSegment, start, end time.Time) (time.Time, time.Time) {
	from, to := seg.from, seg.to
	if !from.Equal(start) {
		from = from.In(time.Local)
	}
	if !to.Equal(end) {
		to = to.In(time.Local)
	}
	return from, to
}

// mergeBucket adds b into dst under key.
func mergeBucket(dst map[string]*aggBucket, key string, b aggBucket) {
	cur, ok := dst[key]
	if !ok {
		dst[key] = &b
		return
	}
	cur.RequestCount += b.RequestCount
	cur.TotalTokens += b.TotalTokens
	cur.InputTokens += b.InputTokens
	cur.OutputTokens += b.OutputTokens
	cur.CacheReadTokens += b.CacheReadTokens
	cur.CacheWriteTokens += b.CacheWriteTokens
	cur.ReasoningTokens += b.ReasoningTokens
	cur.SystemTokens += b.SystemTokens
	cur.ErrorCount += b.ErrorCount
	cur.StreamedCount += b.StreamedCount
	cur.LatencySum += b.LatencySum
}

// aggregatedStatsFromRollups serves GetAggregatedStats from the rollup tiers
// when the query shape allows it. Returns handled=false to fall back to the
// raw scan.
func (us *UsageStore) aggregatedStatsFromRollups(q UsageStatsQuery) ([]AggregatedStat, bool, error) {
	switch q.GroupBy {
	case "model", "provider", "user", "daily", "hourly", "scenario":
	default:
		return nil, false, nil
	}
	// Dimensions/filters not present in the rollups require the raw table.
	if q.RuleUUID != "" || q.TeamID != "" || q.Status != "" {
		return nil, false, nil
	}
	// Scenario lives only in the *_scenario tables, which carry no
	// provider/model/user, so a query that needs scenario together with one of
	// those goes to the raw table.
	if (q.GroupBy == "scenario" || q.Scenario != "") &&
		(q.Provider != "" || q.Model != "" || q.UserID != "" || (q.GroupBy != "scenario" && q.GroupBy != "daily" && q.GroupBy != "hourly")) {
		return nil, false, nil
	}
	if q.StartTime.IsZero() || q.EndTime.IsZero() {
		return nil, false, nil
	}

	segs := planRollupSegments(q.StartTime, q.EndTime, time.Now(), q.GroupBy == "hourly")
	if segs == nil {
		return nil, false, nil
	}
	if err := us.ensureSegments(segs); err != nil {
		logrus.WithError(err).Warn("usage: rollup aggregation failed; falling back to raw scan")
		return nil, false, nil
	}

	merged := make(map[string]*aggBucket)
	for _, seg := range segs {
		var buckets []aggBucket
		var err error
		if seg.tier == nil {
			edge := q
			edge.StartTime, edge.EndTime = rollupSegmentRange(seg, q.StartTime, q.EndTime)
			buckets, err = us.rawAggBuckets(edge, false)
		} else {
			buckets, err = us.tierStatBuckets(q, seg.tier, seg.from, seg.to)
		}
		if err != nil {
			return nil, true, err
		}
		for _, b := range buckets {
			mergeBucket(merged, statsMergeKey(q.GroupBy, b), b)
		}
	}

	list := make([]aggBucket, 0, len(merged))
	for _, b := range merged {
		normalizeStatBucket(q.GroupBy, b)
		list = append(list, *b)
	}
	sortAggBuckets(list, q.SortBy, q.SortOrder)
	if q.Limit > 0 && len(list) > q.Limit {
		list = list[:q.Limit]
	}

	stats := make([]AggregatedStat, len(list))
	for i, b := range list {
		stats[i] = b.toAggregatedStat()
	}
	return stats, true, nil
}

// tierStatBuckets aggregates one tier's rows over [from, to) with the
// grouping/filters of the given query.
func (us *UsageStore) tierStatBuckets(q UsageStatsQuery, tier *rollupTier, from, to time.Time) ([]aggBucket, error) {
	us.mu.RLock()
	defer us.mu.RUnlock()

	table, filters := tier.table, q.dailyFilterMap()
	if q.GroupBy == "scenario" || q.Scenario != "" {
		// Only reachable without provider/model/user dimensions (see
		// aggregatedStatsFromRollups).
		table, filters = tier.scenarioTable, map[string]string{}
		if q.Scenario != "" {
			filters["scenario"] = q.Scenario
		}
	}

	dayKey := tier.keyCol
	if tier == hourTier {
		dayKey = "substr(hour, 1, 10)"
	}
	const noDims = "'' as provider_uuid, '' as provider_name, '' as model, '' as user_id, '' as scenario"
	var keyExpr, groupBy string
	switch q.GroupBy {
	case "provider":
		keyExpr = "provider_uuid as key, provider_uuid, MAX(provider_name) as provider_name, '' as model, '' as user_id, '' as scenario"
		groupBy = "provider_uuid"
	case "user":
		keyExpr = "user_id as key, '' as provider_uuid, '' as provider_name, '' as model, user_id, '' as scenario"
		groupBy = "user_id"
	case "scenario":
		keyExpr = "scenario as key, '' as provider_uuid, '' as provider_name, '' as model, '' as user_id, scenario"
		groupBy = "scenario"
	case "daily":
		keyExpr = dayKey + " as key, " + noDims
		groupBy = dayKey
	case "hourly":
		// Only planned against the hour tier.
		keyExpr = "hour || ':00:00' as key, " + noDims
		groupBy = "hour"
	default: // model
		keyExpr = "model as key, provider_uuid, MAX(provider_name) as provider_name, model, '' as user_id, '' as scenario"
		groupBy = "provider_uuid, model"
	}

	var buckets []aggBucket
	err := us.db.Table(table).
		Where(tier.keyCol+" >= ? AND "+tier.keyCol+" < ? AND request_count > 0", tier.key(from), tier.key(to)).
		Scopes(withColumnFilters(filters)).
		Select(keyExpr + `,
		COALESCE(SUM(request_count), 0) as request_count,
		COALESCE(SUM(total_tokens), 0) as total_tokens,
		COALESCE(SUM(input_tokens), 0) as input_tokens,
		COALESCE(SUM(output_tokens), 0) as output_tokens,
		COALESCE(SUM(cache_input_tokens), 0) as cache_read_tokens,
		COALESCE(SUM(cache_write_tokens), 0) as cache_write_tokens,
		COALESCE(SUM(reasoning_tokens), 0) as reasoning_tokens,
		COALESCE(SUM(system_tokens), 0) as system_tokens,
		COALESCE(SUM(error_count), 0) as error_count,
		COALESCE(SUM(streamed_count), 0) as streamed_count,
		COALESCE(SUM(latency_sum_ms), 0) as latency_sum`).
		Group(groupBy).
		Scan(&buckets).Error
	return buckets, err
}

// statsMergeKey identifies the group a bucket belongs to when combining
// results from different tiers and raw edge scans.
func statsMergeKey(groupBy string, b aggBucket) string {
	switch groupBy {
	case "provider":
		return b.ProviderUUID
	case "user":
		return b.UserID
	case "scenario":
		return b.Scenario
	case "daily", "hourly":
		return b.Key // YYYY-MM-DD / YYYY-MM-DD HH:00:00
	default: // model
		return b.ProviderUUID + "\x00" + b.Model
	}
}

// normalizeStatBucket clears fields that are not dimensions of the grouping,
// so merged results don't leak arbitrary per-row values. Key is already set
// correctly by the SQL aliasing in every source path and preserved by
// mergeBucket, so only the non-dimension fields need clearing here.
func normalizeStatBucket(groupBy string, b *aggBucket) {
	switch groupBy {
	case "provider":
		b.Model, b.Scenario, b.UserID = "", "", ""
	case "user":
		b.ProviderUUID, b.ProviderName, b.Model, b.Scenario = "", "", "", ""
	case "scenario":
		b.ProviderUUID, b.ProviderName, b.Model, b.UserID = "", "", "", ""
	case "daily", "hourly":
		b.ProviderUUID, b.ProviderName, b.Model, b.Scenario, b.UserID = "", "", "", "", ""
	default: // model
		b.Scenario, b.UserID = "", ""
	}
}

func sortAggBuckets(list []aggBucket, sortBy, sortOrder string) {
	metric := func(b aggBucket) float64 {
		switch sortBy {
		case "request_count":
			return float64(b.RequestCount)
		case "avg_latency":
			return avgFloat(float64(b.LatencySum), b.RequestCount)
		default: // total_tokens
			return float64(b.TotalTokens)
		}
	}
	asc := sortOrder == "asc"
	sort.SliceStable(list, func(i, j int) bool {
		if asc {
			return metric(list[i]) < metric(list[j])
		}
		return metric(list[i]) > metric(list[j])
	})
}

// timeSeriesFromRollups serves hour- and day-interval time series from the
// rollup tiers when the query shape allows it. Returns handled=false to fall
// back to raw.
func (us *UsageStore) timeSeriesFromRollups(interval string, start, end time.Time, filters map[string]string) ([]TimeSeriesData, bool, error) {
	if (interval != "day" && interval != "hour") || start.IsZero() || end.IsZero() {
		return nil, false, nil
	}
	for k := range filters {
		switch k {
		case "provider_uuid", "model", "user_id":
		case "scenario":
			// The scenario tables have no other dimension to filter on.
			if len(filters) > 1 {
				return nil, false, nil
			}
		default:
			return nil, false, nil
		}
	}

	segs := planRollupSegments(start, end, time.Now(), interval == "hour")
	if segs == nil {
		return nil, false, nil
	}
	if err := us.ensureSegments(segs); err != nil {
		logrus.WithError(err).Warn("usage: rollup aggregation failed; falling back to raw scan")
		return nil, false, nil
	}

	merged := make(map[string]*aggBucket)
	for _, seg := range segs {
		var buckets []aggBucket
		var err error
		if seg.tier == nil {
			from, to := rollupSegmentRange(seg, start, end)
			buckets, err = us.rawSeriesBuckets(interval, from, to, filters)
		} else {
			buckets, err = us.tierSeriesBuckets(seg.tier, interval, seg.from, seg.to, filters)
		}
		if err != nil {
			return nil, true, err
		}
		for _, b := range buckets {
			mergeBucket(merged, b.Key, b)
		}
	}
	return seriesFromBuckets(merged), true, nil
}

// seriesFromBuckets orders merged buckets by their Unix-second key.
func seriesFromBuckets(merged map[string]*aggBucket) []TimeSeriesData {
	type keyed struct {
		unix int64
		b    *aggBucket
	}
	list := make([]keyed, 0, len(merged))
	for k, b := range merged {
		unix, _ := strconv.ParseInt(k, 10, 64)
		list = append(list, keyed{unix, b})
	}
	sort.Slice(list, func(i, j int) bool { return list[i].unix < list[j].unix })
	data := make([]TimeSeriesData, len(list))
	for i, kb := range list {
		b := kb.b
		data[i] = TimeSeriesData{
			Timestamp:        b.Key,
			RequestCount:     b.RequestCount,
			TotalTokens:      b.TotalTokens,
			InputTokens:      b.InputTokens,
			OutputTokens:     b.OutputTokens,
			CacheReadTokens:  b.CacheReadTokens,
			CacheWriteTokens: b.CacheWriteTokens,
			ReasoningTokens:  b.ReasoningTokens,
			SystemTokens:     b.SystemTokens,
			ErrorCount:       b.ErrorCount,
			AvgLatencyMs:     avgFloat(float64(b.LatencySum), b.RequestCount),
		}
	}
	return data
}

// dailyFilterColumns is the subset of usageFilterColumns the main rollup
// tables carry as columns -- rule_uuid, team_id and status are not columns
// there, which is exactly why aggregatedStatsFromRollups and
// timeSeriesFromRollups refuse a query that sets them and fall back to the raw
// table. (scenario lives in the *_scenario tables.)
var dailyFilterColumns = map[string]struct{}{
	"provider_uuid": {},
	"provider_name": {},
	"model":         {},
	"user_id":       {},
}

// validateDailyFilterColumns rejects any filter key the rollup tables have no
// column for. The callers above already gate on the same set, so reaching this
// is a bug rather than user input -- but an unchecked key would be
// interpolated into SQL, so it is checked here too.
func validateDailyFilterColumns(filters map[string]string) error {
	for key := range filters {
		if _, ok := dailyFilterColumns[key]; !ok {
			return fmt.Errorf("usage rollup: unsupported filter column %q", key)
		}
	}
	return nil
}

// dailyFilterMap is UsageStatsQuery.filterMap restricted to the dimensions
// the main rollup tables carry.
func (q UsageStatsQuery) dailyFilterMap() map[string]string {
	filters := make(map[string]string, 3)
	for column, value := range map[string]string{
		"provider_uuid": q.Provider,
		"model":         q.Model,
		"user_id":       q.UserID,
	} {
		if value != "" {
			filters[column] = value
		}
	}
	return filters
}

// tierSeriesBuckets returns one bucket per interval from a rollup tier. Keys
// are Unix seconds of the bucket start, matching the raw strftime('%s', ...)
// encoding.
func (us *UsageStore) tierSeriesBuckets(tier *rollupTier, interval string, from, to time.Time, filters map[string]string) ([]aggBucket, error) {
	us.mu.RLock()
	defer us.mu.RUnlock()

	table := tier.table
	if scenario, ok := filters["scenario"]; ok {
		// timeSeriesFromRollups only lets scenario through on its own.
		table, filters = tier.scenarioTable, map[string]string{"scenario": scenario}
	} else if err := validateDailyFilterColumns(filters); err != nil {
		return nil, err
	}

	// A day series over the hour tier folds hours into their day.
	bucketExpr, layout := tier.keyCol, tier.layout
	if tier == hourTier && interval == "day" {
		bucketExpr, layout = "substr(hour, 1, 10)", dailyDateLayout
	}

	var rows []aggBucket
	if err := us.db.Table(table).
		Where(tier.keyCol+" >= ? AND "+tier.keyCol+" < ? AND request_count > 0", tier.key(from), tier.key(to)).
		Scopes(withColumnFilters(filters)).
		Select(bucketExpr + ` as key,
		COALESCE(SUM(request_count), 0) as request_count,
		COALESCE(SUM(total_tokens), 0) as total_tokens,
		COALESCE(SUM(input_tokens), 0) as input_tokens,
		COALESCE(SUM(output_tokens), 0) as output_tokens,
		COALESCE(SUM(cache_input_tokens), 0) as cache_read_tokens,
		COALESCE(SUM(cache_write_tokens), 0) as cache_write_tokens,
		COALESCE(SUM(reasoning_tokens), 0) as reasoning_tokens,
		COALESCE(SUM(system_tokens), 0) as system_tokens,
		COALESCE(SUM(error_count), 0) as error_count,
		COALESCE(SUM(latency_sum_ms), 0) as latency_sum`).
		Group(bucketExpr).
		Scan(&rows).Error; err != nil {
		return nil, err
	}

	out := rows[:0]
	for _, r := range rows {
		at, err := time.ParseInLocation(layout, r.Key, time.UTC)
		if err != nil {
			continue
		}
		r.Key = strconv.FormatInt(at.Unix(), 10)
		out = append(out, r)
	}
	return out, nil
}
