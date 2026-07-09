package db

import (
	"sort"
	"strconv"
	"time"

	"github.com/sirupsen/logrus"
	"gorm.io/gorm"
)

// Daily pre-aggregation for usage statistics.
//
// Dashboard queries over long ranges (7/30/90 days) used to GROUP BY over the
// raw usage_records table on every load, which degrades linearly with record
// count. Instead, completed UTC days (matching SQLite's date(timestamp)
// bucketing already used by the raw queries) are aggregated once into
// usage_daily, and range queries combine:
//
//	[start .. firstDay)   raw scan (partial leading day, at most ~1 day)
//	[firstDay .. lastDay) usage_daily lookup (cheap, few rows per day)
//	[lastDay .. end]      raw scan (today's partial day + grace window)
//
// Aggregation is lazy: triggered by the first query that needs a day, tracked
// in memory, and persisted in usage_daily itself. Days are only aggregated
// once they are dailyAggGrace past midnight so late-finishing requests whose
// timestamps fall just before midnight are still captured.

const (
	dailyDateLayout = "2006-01-02"
	// dailyAggGrace delays aggregation of a finished day so requests that
	// started before midnight but were recorded after it are not missed.
	dailyAggGrace = time.Hour
	// minDailySpan is the minimum number of complete days a query must cover
	// before the pre-aggregation path is worth taking over a raw scan.
	minDailySpan = 2 * 24 * time.Hour
	// dstScanPad widens the per-day raw scan window so rows whose stored
	// wall-clock offset differs from the current one (DST shifts) are still
	// found; the exact date(timestamp) guard filters out the excess.
	dstScanPad = 2 * time.Hour
)

// utcDayStart truncates a time to the start of its UTC day.
func utcDayStart(t time.Time) time.Time {
	u := t.UTC()
	return time.Date(u.Year(), u.Month(), u.Day(), 0, 0, 0, 0, time.UTC)
}

// dailyWindow returns the [firstDay, lastDayEx) range of complete UTC days
// within [start, end] that are eligible for pre-aggregation as of now.
func dailyWindow(start, end, now time.Time) (time.Time, time.Time) {
	firstDay := utcDayStart(start)
	if !firstDay.Equal(start) {
		firstDay = firstDay.Add(24 * time.Hour)
	}
	lastDayEx := utcDayStart(end)
	if cap := utcDayStart(now.Add(-dailyAggGrace)); cap.Before(lastDayEx) {
		lastDayEx = cap
	}
	return firstDay, lastDayEx
}

// ensureDailyAggregates lazily builds usage_daily rows for every day in
// [firstDay, lastDayEx) that has not been aggregated yet.
func (us *UsageStore) ensureDailyAggregates(firstDay, lastDayEx time.Time) error {
	us.aggMu.Lock()
	defer us.aggMu.Unlock()

	if !us.aggLoaded {
		var days []string
		us.mu.RLock()
		err := us.db.Model(&UsageDailyRecord{}).Distinct("date").Pluck("date", &days).Error
		us.mu.RUnlock()
		if err != nil {
			return err
		}
		us.aggregatedDays = make(map[string]bool, len(days))
		for _, d := range days {
			us.aggregatedDays[d] = true
		}
		us.aggLoaded = true
	}

	for day := firstDay; day.Before(lastDayEx); day = day.Add(24 * time.Hour) {
		key := day.Format(dailyDateLayout)
		if us.aggregatedDays[key] {
			continue
		}
		if _, err := us.aggregateDay(key, day); err != nil {
			return err
		}
		us.aggregatedDays[key] = true
	}
	return nil
}

// aggregateDay (re)builds the usage_daily rows for one UTC day.
func (us *UsageStore) aggregateDay(key string, dayStart time.Time) (int64, error) {
	us.mu.Lock()
	defer us.mu.Unlock()

	// The timestamp range prunes via the index; the date() guard is exact.
	// Bind times converted to server-local so the driver formats them with
	// the same offset the records were stored with.
	scanStart := dayStart.Add(-dstScanPad).In(time.Local)
	scanEnd := dayStart.Add(24*time.Hour + dstScanPad).In(time.Local)

	var rows int64
	err := us.db.Transaction(func(tx *gorm.DB) error {
		if err := tx.Where("date = ?", key).Delete(&UsageDailyRecord{}).Error; err != nil {
			return err
		}
		res := tx.Exec(`
			INSERT INTO usage_daily (date, provider_uuid, provider_name, model, user_id,
				request_count, total_tokens, input_tokens, output_tokens,
				cache_input_tokens, system_tokens, error_count, streamed_count, latency_sum_ms)
			SELECT ?, COALESCE(provider_uuid, ''), COALESCE(provider_name, ''), COALESCE(model, ''), COALESCE(user_id, ''),
				COUNT(*),
				COALESCE(SUM(total_tokens), 0),
				COALESCE(SUM(input_tokens), 0),
				COALESCE(SUM(output_tokens), 0),
				COALESCE(SUM(cache_input_tokens), 0),
				COALESCE(SUM(system_tokens), 0),
				COALESCE(SUM(CASE WHEN status = 'error' THEN 1 ELSE 0 END), 0),
				COALESCE(SUM(CASE WHEN streamed = true THEN 1 ELSE 0 END), 0),
				COALESCE(SUM(latency_ms), 0)
			FROM usage_records
			WHERE timestamp >= ? AND timestamp < ? AND date(timestamp) = ?
			GROUP BY provider_uuid, provider_name, model, user_id
		`, key, scanStart, scanEnd, key)
		if res.Error != nil {
			return res.Error
		}
		rows = res.RowsAffected
		return nil
	})
	return rows, err
}

// aggregatedStatsFromDaily serves GetAggregatedStats from usage_daily when the
// query shape allows it. Returns handled=false to fall back to the raw scan.
func (us *UsageStore) aggregatedStatsFromDaily(q UsageStatsQuery) ([]AggregatedStat, bool, error) {
	switch q.GroupBy {
	case "model", "provider", "user", "daily":
	default:
		return nil, false, nil
	}
	// Dimensions/filters not present in usage_daily require the raw table.
	if q.Scenario != "" || q.RuleUUID != "" || q.Status != "" {
		return nil, false, nil
	}
	if q.StartTime.IsZero() || q.EndTime.IsZero() {
		return nil, false, nil
	}

	firstDay, lastDayEx := dailyWindow(q.StartTime, q.EndTime, time.Now())
	if lastDayEx.Sub(firstDay) < minDailySpan {
		return nil, false, nil
	}
	if err := us.ensureDailyAggregates(firstDay, lastDayEx); err != nil {
		logrus.WithError(err).Warn("usage: daily aggregation failed; falling back to raw scan")
		return nil, false, nil
	}

	merged := make(map[string]*aggBucket)
	mergeInto := func(buckets []aggBucket) {
		for i := range buckets {
			b := buckets[i]
			k := statsMergeKey(q.GroupBy, b)
			if cur, ok := merged[k]; ok {
				cur.RequestCount += b.RequestCount
				cur.TotalTokens += b.TotalTokens
				cur.InputTokens += b.InputTokens
				cur.OutputTokens += b.OutputTokens
				cur.CacheInputTokens += b.CacheInputTokens
				cur.SystemTokens += b.SystemTokens
				cur.ErrorCount += b.ErrorCount
				cur.StreamedCount += b.StreamedCount
				cur.LatencySum += b.LatencySum
			} else {
				merged[k] = &b
			}
		}
	}

	// Complete days from the pre-aggregation table.
	daily, err := us.dailyStatBuckets(q, firstDay, lastDayEx)
	if err != nil {
		return nil, true, err
	}
	mergeInto(daily)

	// Partial edge days from the raw table.
	if q.StartTime.Before(firstDay) {
		edge := q
		edge.EndTime = firstDay.Add(-time.Nanosecond).In(time.Local)
		buckets, err := us.rawAggBuckets(edge, false)
		if err != nil {
			return nil, true, err
		}
		mergeInto(buckets)
	}
	if q.EndTime.After(lastDayEx) {
		edge := q
		edge.StartTime = lastDayEx.In(time.Local)
		buckets, err := us.rawAggBuckets(edge, false)
		if err != nil {
			return nil, true, err
		}
		mergeInto(buckets)
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

// dailyStatBuckets aggregates usage_daily rows over [firstDay, lastDayEx)
// with the grouping/filters of the given query.
func (us *UsageStore) dailyStatBuckets(q UsageStatsQuery, firstDay, lastDayEx time.Time) ([]aggBucket, error) {
	us.mu.RLock()
	defer us.mu.RUnlock()

	var keyExpr, groupBy string
	switch q.GroupBy {
	case "provider":
		keyExpr = "provider_uuid as key, provider_uuid, provider_name, '' as model, '' as user_id"
		groupBy = "provider_uuid, provider_name"
	case "user":
		keyExpr = "user_id as key, '' as provider_uuid, '' as provider_name, '' as model, user_id"
		groupBy = "user_id"
	case "daily":
		keyExpr = "date as key, '' as provider_uuid, '' as provider_name, '' as model, '' as user_id"
		groupBy = "date"
	default: // model
		keyExpr = "model as key, provider_uuid, provider_name, model, '' as user_id"
		groupBy = "provider_uuid, provider_name, model"
	}

	db := us.db.Model(&UsageDailyRecord{}).
		Where("date >= ? AND date < ?", firstDay.Format(dailyDateLayout), lastDayEx.Format(dailyDateLayout))
	if q.Provider != "" {
		db = db.Where("provider_uuid = ?", q.Provider)
	}
	if q.Model != "" {
		db = db.Where("model = ?", q.Model)
	}
	if q.UserID != "" {
		db = db.Where("user_id = ?", q.UserID)
	}

	var buckets []aggBucket
	if err := db.Select(keyExpr + `,
		COALESCE(SUM(request_count), 0) as request_count,
		COALESCE(SUM(total_tokens), 0) as total_tokens,
		COALESCE(SUM(input_tokens), 0) as input_tokens,
		COALESCE(SUM(output_tokens), 0) as output_tokens,
		COALESCE(SUM(cache_input_tokens), 0) as cache_input_tokens,
		COALESCE(SUM(system_tokens), 0) as system_tokens,
		COALESCE(SUM(error_count), 0) as error_count,
		COALESCE(SUM(streamed_count), 0) as streamed_count,
		COALESCE(SUM(latency_sum_ms), 0) as latency_sum`).
		Group(groupBy).
		Scan(&buckets).Error; err != nil {
		return nil, err
	}
	return buckets, nil
}

// statsMergeKey identifies the group a bucket belongs to when combining
// usage_daily results with raw edge scans.
func statsMergeKey(groupBy string, b aggBucket) string {
	switch groupBy {
	case "provider":
		return b.ProviderUUID
	case "user":
		return b.UserID
	case "daily":
		return b.Key // YYYY-MM-DD
	default: // model
		return b.ProviderUUID + "\x00" + b.Model
	}
}

// normalizeStatBucket clears fields that are not dimensions of the grouping,
// so merged results don't leak arbitrary per-row values.
func normalizeStatBucket(groupBy string, b *aggBucket) {
	switch groupBy {
	case "provider":
		b.Key = b.ProviderUUID
		b.Model, b.Scenario, b.UserID = "", "", ""
	case "user":
		b.Key = b.UserID
		b.ProviderUUID, b.ProviderName, b.Model, b.Scenario = "", "", "", ""
	case "daily":
		b.ProviderUUID, b.ProviderName, b.Model, b.Scenario, b.UserID = "", "", "", "", ""
	default: // model
		b.Key = b.Model
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

// timeSeriesFromDaily serves day-interval time series from usage_daily when
// the query shape allows it. Returns handled=false to fall back to raw.
func (us *UsageStore) timeSeriesFromDaily(interval string, start, end time.Time, filters map[string]string) ([]TimeSeriesData, bool, error) {
	if interval != "day" || start.IsZero() || end.IsZero() {
		return nil, false, nil
	}
	for k := range filters {
		switch k {
		case "provider_uuid", "model", "user_id":
		default:
			return nil, false, nil
		}
	}

	firstDay, lastDayEx := dailyWindow(start, end, time.Now())
	if lastDayEx.Sub(firstDay) < minDailySpan {
		return nil, false, nil
	}
	if err := us.ensureDailyAggregates(firstDay, lastDayEx); err != nil {
		logrus.WithError(err).Warn("usage: daily aggregation failed; falling back to raw scan")
		return nil, false, nil
	}

	var out []TimeSeriesData

	// Partial leading day from the raw table.
	if start.Before(firstDay) {
		lead, err := us.rawTimeSeries("day", start, firstDay.Add(-time.Nanosecond).In(time.Local), filters)
		if err != nil {
			return nil, true, err
		}
		out = append(out, lead...)
	}

	// Complete days from the pre-aggregation table.
	body, err := us.dailyTimeSeries(firstDay, lastDayEx, filters)
	if err != nil {
		return nil, true, err
	}
	out = append(out, body...)

	// Partial trailing days (today + grace window) from the raw table.
	if end.After(lastDayEx) {
		tail, err := us.rawTimeSeries("day", lastDayEx.In(time.Local), end, filters)
		if err != nil {
			return nil, true, err
		}
		out = append(out, tail...)
	}

	sort.SliceStable(out, func(i, j int) bool {
		a, _ := strconv.ParseInt(out[i].Timestamp, 10, 64)
		b, _ := strconv.ParseInt(out[j].Timestamp, 10, 64)
		return a < b
	})
	return out, true, nil
}

// dailyTimeSeries returns one bucket per date from usage_daily.
func (us *UsageStore) dailyTimeSeries(firstDay, lastDayEx time.Time, filters map[string]string) ([]TimeSeriesData, error) {
	us.mu.RLock()
	defer us.mu.RUnlock()

	db := us.db.Model(&UsageDailyRecord{}).
		Where("date >= ? AND date < ?", firstDay.Format(dailyDateLayout), lastDayEx.Format(dailyDateLayout))
	for k, v := range filters {
		db = db.Where(k+" = ?", v)
	}

	type row struct {
		Date             string
		RequestCount     int64
		TotalTokens      int64
		InputTokens      int64
		OutputTokens     int64
		CacheInputTokens int64
		SystemTokens     int64
		ErrorCount       int64
		LatencySum       int64
	}
	var rows []row
	if err := db.Select(`date,
		COALESCE(SUM(request_count), 0) as request_count,
		COALESCE(SUM(total_tokens), 0) as total_tokens,
		COALESCE(SUM(input_tokens), 0) as input_tokens,
		COALESCE(SUM(output_tokens), 0) as output_tokens,
		COALESCE(SUM(cache_input_tokens), 0) as cache_input_tokens,
		COALESCE(SUM(system_tokens), 0) as system_tokens,
		COALESCE(SUM(error_count), 0) as error_count,
		COALESCE(SUM(latency_sum_ms), 0) as latency_sum`).
		Group("date").
		Order("date ASC").
		Scan(&rows).Error; err != nil {
		return nil, err
	}

	data := make([]TimeSeriesData, 0, len(rows))
	for _, r := range rows {
		day, err := time.ParseInLocation(dailyDateLayout, r.Date, time.UTC)
		if err != nil {
			continue
		}
		data = append(data, TimeSeriesData{
			// Unix seconds of the UTC day start, matching the raw
			// strftime('%s', ...) bucket encoding.
			Timestamp:        strconv.FormatInt(day.Unix(), 10),
			RequestCount:     r.RequestCount,
			TotalTokens:      r.TotalTokens,
			InputTokens:      r.InputTokens,
			OutputTokens:     r.OutputTokens,
			CacheInputTokens: r.CacheInputTokens,
			SystemTokens:     r.SystemTokens,
			ErrorCount:       r.ErrorCount,
			AvgLatencyMs:     avgFloat(float64(r.LatencySum), r.RequestCount),
		})
	}
	return data, nil
}
