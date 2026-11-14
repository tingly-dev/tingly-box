package db

import (
	"strings"
	"time"

	"gorm.io/gorm"
)

// Aggregation is lazy: triggered by the first query that needs a bucket, and
// tracked in the rollup tables themselves (a bucket with no traffic gets a
// zero-count marker row), so concurrent requests never contend on a lock just
// to ask what is built, and the answer cannot go stale across a restart.

const (
	// rollupSumColumns are the additive columns every rollup table carries.
	rollupSumColumns = `request_count, total_tokens, input_tokens, output_tokens,
		cache_input_tokens, cache_write_tokens, reasoning_tokens, system_tokens,
		error_count, streamed_count, latency_sum_ms`

	// rawSumSelect computes rollupSumColumns from usage_records rows.
	rawSumSelect = `COUNT(*),
		COALESCE(SUM(total_tokens), 0),
		COALESCE(SUM(input_tokens), 0),
		COALESCE(SUM(output_tokens), 0),
		COALESCE(SUM(cache_input_tokens), 0),
		COALESCE(SUM(cache_write_tokens), 0),
		COALESCE(SUM(reasoning_tokens), 0),
		COALESCE(SUM(system_tokens), 0),
		COALESCE(SUM(CASE WHEN status = 'error' THEN 1 ELSE 0 END), 0),
		COALESCE(SUM(CASE WHEN streamed = true THEN 1 ELSE 0 END), 0),
		COALESCE(SUM(latency_ms), 0)`

	// rollupSumSelect re-sums rollupSumColumns from finer rollup rows.
	rollupSumSelect = `COALESCE(SUM(request_count), 0),
		COALESCE(SUM(total_tokens), 0),
		COALESCE(SUM(input_tokens), 0),
		COALESCE(SUM(output_tokens), 0),
		COALESCE(SUM(cache_input_tokens), 0),
		COALESCE(SUM(cache_write_tokens), 0),
		COALESCE(SUM(reasoning_tokens), 0),
		COALESCE(SUM(system_tokens), 0),
		COALESCE(SUM(error_count), 0),
		COALESCE(SUM(streamed_count), 0),
		COALESCE(SUM(latency_sum_ms), 0)`

	// maxAggregateRun bounds how many days one aggregation transaction covers.
	maxAggregateRun = 7 * 24 * time.Hour
)

// ensureSegments builds whatever the tier segments of a plan still lack.
func (us *UsageStore) ensureSegments(segs []rollupSegment) error {
	for _, seg := range segs {
		if seg.tier == nil {
			continue
		}
		if err := us.ensureBuckets(seg.tier, seg.from, seg.to); err != nil {
			return err
		}
	}
	return nil
}

// ensureBuckets builds the tier's buckets in [from, to) that do not exist
// yet, per contiguous run of at most maxAggregateRun with one grouped scan
// each (a cold 90-day window is a few dozen statements, not thousands, and the
// write lock is held for a week of data at a time rather than the window).
func (us *UsageStore) ensureBuckets(tier *rollupTier, from, to time.Time) error {
	us.mu.RLock()
	missing, err := missingBuckets(us.db, tier, from, to)
	us.mu.RUnlock()
	if err != nil || len(missing) == 0 {
		return err
	}
	for _, run := range contiguousRuns(missing, tier.step, maxAggregateRun) {
		if err := us.buildBuckets(tier, run[0], run[len(run)-1].Add(tier.step)); err != nil {
			return err
		}
	}
	return nil
}

// buildBuckets builds what is still missing of the tier's buckets [from, to) in
// one transaction. It re-checks under the write lock: a concurrent request may
// have built the same buckets while this one waited.
func (us *UsageStore) buildBuckets(tier *rollupTier, from, to time.Time) error {
	us.mu.Lock()
	defer us.mu.Unlock()
	return us.db.Transaction(func(tx *gorm.DB) error {
		// Hours first, whichever tier was asked for: a day is the sum of its
		// hours, and building both in this transaction means the two tiers can
		// never disagree.
		hours, err := missingBuckets(tx, hourTier, from, to)
		if err != nil {
			return err
		}
		for _, run := range contiguousRuns(hours, time.Hour, maxAggregateRun) {
			if err := buildHours(tx, run[0], run[len(run)-1].Add(time.Hour)); err != nil {
				return err
			}
		}
		if tier != dayTier {
			return nil
		}
		days, err := missingBuckets(tx, dayTier, from, to)
		if err != nil || len(days) == 0 {
			return err
		}
		return deriveDays(tx, from, to)
	})
}

// missingBuckets returns the tier's bucket starts in [from, to) with no rows.
func missingBuckets(db *gorm.DB, tier *rollupTier, from, to time.Time) ([]time.Time, error) {
	var have []string
	err := db.Table(tier.table).
		Where(tier.keyCol+" >= ? AND "+tier.keyCol+" < ?", tier.key(from), tier.key(to)).
		Distinct(tier.keyCol).Pluck(tier.keyCol, &have).Error
	if err != nil {
		return nil, err
	}
	haveSet := make(map[string]bool, len(have))
	for _, k := range have {
		haveSet[k] = true
	}
	var missing []time.Time
	for at := from; at.Before(to); at = at.Add(tier.step) {
		if !haveSet[tier.key(at)] {
			missing = append(missing, at)
		}
	}
	return missing, nil
}

// contiguousRuns splits ascending bucket starts into runs of adjacent buckets,
// each spanning at most maxSpan.
func contiguousRuns(buckets []time.Time, step, maxSpan time.Duration) [][]time.Time {
	var runs [][]time.Time
	for _, b := range buckets {
		if n := len(runs); n > 0 {
			run := runs[n-1]
			if b.Equal(run[len(run)-1].Add(step)) && b.Add(step).Sub(run[0]) <= maxSpan {
				runs[n-1] = append(run, b)
				continue
			}
		}
		runs = append(runs, []time.Time{b})
	}
	return runs
}

// clearBuckets deletes both tables of a tier for the keys in [fromKey, toKey).
func clearBuckets(tx *gorm.DB, tier *rollupTier, fromKey, toKey string) error {
	for _, table := range []string{tier.table, tier.scenarioTable} {
		if err := tx.Exec("DELETE FROM "+table+" WHERE "+tier.keyCol+" >= ? AND "+tier.keyCol+" < ?", fromKey, toKey).Error; err != nil {
			return err
		}
	}
	return nil
}

// buildHours rebuilds usage_hourly and usage_hourly_scenario for the UTC hours
// [from, to) from the raw records.
func buildHours(tx *gorm.DB, from, to time.Time) error {
	fromKey, toKey := hourTier.key(from), hourTier.key(to)
	if err := clearBuckets(tx, hourTier, fromKey, toKey); err != nil {
		return err
	}
	// The timestamp range prunes via the index; the strftime key guard is
	// exact. Bind times converted to server-local so the driver formats them
	// with the same offset the records were stored with.
	scanStart := from.Add(-dstScanPad).In(time.Local)
	scanEnd := to.Add(dstScanPad).In(time.Local)
	const keyed = `FROM (SELECT strftime('%Y-%m-%d %H', timestamp) AS k, * FROM usage_records
		WHERE timestamp >= ? AND timestamp < ?) WHERE k >= ? AND k < ?`

	// provider_name is not part of the unique key (a provider renamed
	// mid-hour must not split the group into two conflicting rows), so MAX()
	// picks one.
	if err := tx.Exec(`
		INSERT INTO usage_hourly (hour, provider_uuid, provider_name, model, user_id, `+rollupSumColumns+`)
		SELECT k, COALESCE(provider_uuid, ''), COALESCE(MAX(provider_name), ''), COALESCE(model, ''), COALESCE(user_id, ''),
			`+rawSumSelect+`
		`+keyed+`
		GROUP BY k, provider_uuid, model, user_id`, scanStart, scanEnd, fromKey, toKey).Error; err != nil {
		return err
	}
	if err := tx.Exec(`
		INSERT INTO usage_hourly_scenario (hour, scenario, `+rollupSumColumns+`)
		SELECT k, COALESCE(scenario, ''),
			`+rawSumSelect+`
		`+keyed+`
		GROUP BY k, scenario`, scanStart, scanEnd, fromKey, toKey).Error; err != nil {
		return err
	}
	return markEmptyBuckets(tx, hourTier, from, to)
}

// deriveDays rebuilds usage_daily and usage_daily_scenario for the UTC days
// [from, to) by summing their hour rows. The hours must already exist.
func deriveDays(tx *gorm.DB, from, to time.Time) error {
	fromKey, toKey := dayTier.key(from), dayTier.key(to)
	if err := clearBuckets(tx, dayTier, fromKey, toKey); err != nil {
		return err
	}
	hourFrom, hourTo := hourTier.key(from), hourTier.key(to)
	if err := tx.Exec(`
		INSERT INTO usage_daily (date, provider_uuid, provider_name, model, user_id, `+rollupSumColumns+`)
		SELECT substr(hour, 1, 10), provider_uuid, MAX(provider_name), model, user_id,
			`+rollupSumSelect+`
		FROM usage_hourly WHERE hour >= ? AND hour < ? AND request_count > 0
		GROUP BY substr(hour, 1, 10), provider_uuid, model, user_id`, hourFrom, hourTo).Error; err != nil {
		return err
	}
	if err := tx.Exec(`
		INSERT INTO usage_daily_scenario (date, scenario, `+rollupSumColumns+`)
		SELECT substr(hour, 1, 10), scenario,
			`+rollupSumSelect+`
		FROM usage_hourly_scenario WHERE hour >= ? AND hour < ? AND request_count > 0
		GROUP BY substr(hour, 1, 10), scenario`, hourFrom, hourTo).Error; err != nil {
		return err
	}
	return markEmptyBuckets(tx, dayTier, from, to)
}

// markEmptyBuckets writes the zero-count marker rows that record "this
// completed bucket was aggregated and had no traffic": without them a quiet
// stretch of a long window (the 365-day heatmap) would be re-aggregated on
// every request. Every dimension is empty and every count zero; readers skip
// rows with request_count = 0 so a marker never surfaces as a bucket (a real
// group cannot collide with it: it is only written for a bucket with no row).
func markEmptyBuckets(tx *gorm.DB, tier *rollupTier, from, to time.Time) error {
	missing, err := missingBuckets(tx, tier, from, to)
	if err != nil || len(missing) == 0 {
		return err
	}
	values := make([]string, len(missing))
	args := make([]any, len(missing))
	for i, at := range missing {
		values[i] = "(?, '', '', '', '', 0, 0, 0, 0)"
		args[i] = tier.key(at)
	}
	return tx.Exec("INSERT INTO "+tier.table+
		" ("+tier.keyCol+", provider_uuid, provider_name, model, user_id, request_count, total_tokens, input_tokens, output_tokens) VALUES "+
		strings.Join(values, ", "), args...).Error
}

// clearRollupsThrough drops every rollup bucket touching records at or before
// cutoff, so the retention delete cannot leave an aggregate that still counts
// a deleted record. The boundary buckets are rebuilt from the remaining raw
// rows on the next query.
func clearRollupsThrough(tx *gorm.DB, cutoff time.Time) error {
	for _, tier := range []*rollupTier{hourTier, dayTier} {
		for _, table := range []string{tier.table, tier.scenarioTable} {
			if err := tx.Exec("DELETE FROM "+table+" WHERE "+tier.keyCol+" <= ?", tier.key(cutoff)).Error; err != nil {
				return err
			}
		}
	}
	return nil
}
