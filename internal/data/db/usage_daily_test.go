package db

import (
	"fmt"
	"testing"
	"time"
)

// seedUsageRecords writes a deterministic mix of records spread over the past
// `days` days (several providers/models/users, errors, streamed, cache).
func seedUsageRecords(t *testing.T, store *UsageStore, days int) int {
	t.Helper()

	providers := []struct{ uuid, name string }{
		{"prov-a", "Provider A"},
		{"prov-b", "Provider B"},
	}
	models := []string{"model-x", "model-y", "model-z"}
	users := []string{"admin", "alice"}

	count := 0
	now := time.Now()
	for d := 0; d < days; d++ {
		for h := 0; h < 24; h += 5 {
			for i, p := range providers {
				model := models[(d+h+i)%len(models)]
				user := users[(d+i)%len(users)]
				status := "success"
				if (d+h)%10 == 0 {
					status = "error"
				}
				rec := &UsageRecord{
					ProviderUUID:     p.uuid,
					ProviderName:     p.name,
					Model:            model,
					Scenario:         "default",
					UserID:           user,
					Timestamp:        now.Add(-time.Duration(d)*24*time.Hour - time.Duration(h)*time.Hour),
					InputTokens:      100 + d*10 + h,
					OutputTokens:     50 + h,
					CacheInputTokens: 20 * i,
					SystemTokens:     5,
					Status:           status,
					LatencyMs:        200 + h*3,
					Streamed:         h%2 == 0,
				}
				if err := store.RecordUsage(rec); err != nil {
					t.Fatalf("RecordUsage failed: %v", err)
				}
				count++
			}
		}
	}
	return count
}

func statsKey(s AggregatedStat) string {
	return fmt.Sprintf("%s|%s|%s|%s", s.Key, s.ProviderUUID, s.Model, s.UserID)
}

// TestAggregatedStatsDailyMatchesRaw verifies the usage_daily merged path
// returns the same numbers as a raw usage_records scan.
func TestAggregatedStatsDailyMatchesRaw(t *testing.T) {
	sm, err := NewStoreManager(t.TempDir())
	if err != nil {
		t.Fatalf("NewStoreManager failed: %v", err)
	}
	defer sm.Close()
	store := sm.Usage()
	seedUsageRecords(t, store, 8)

	now := time.Now()
	for _, groupBy := range []string{"model", "provider", "user", "daily"} {
		query := UsageStatsQuery{
			GroupBy:   groupBy,
			StartTime: now.Add(-7 * 24 * time.Hour),
			EndTime:   now,
			Limit:     100,
			SortBy:    "total_tokens",
			SortOrder: "desc",
		}

		merged, handled, err := store.aggregatedStatsFromDaily(query)
		if err != nil {
			t.Fatalf("[%s] aggregatedStatsFromDaily failed: %v", groupBy, err)
		}
		if !handled {
			t.Fatalf("[%s] expected daily path to handle a 7-day query", groupBy)
		}

		buckets, err := store.rawAggBuckets(query, true)
		if err != nil {
			t.Fatalf("[%s] rawAggBuckets failed: %v", groupBy, err)
		}
		raw := make(map[string]AggregatedStat, len(buckets))
		totalRawRequests := int64(0)
		for _, b := range buckets {
			normalizeStatBucket(groupBy, &b)
			raw[statsKey(b.toAggregatedStat())] = b.toAggregatedStat()
			totalRawRequests += b.RequestCount
		}

		if len(merged) != len(raw) {
			t.Fatalf("[%s] group count mismatch: merged=%d raw=%d", groupBy, len(merged), len(raw))
		}
		for _, m := range merged {
			r, ok := raw[statsKey(m)]
			if !ok {
				t.Fatalf("[%s] merged group %q missing from raw results", groupBy, statsKey(m))
			}
			if m.RequestCount != r.RequestCount ||
				m.TotalTokens != r.TotalTokens ||
				m.InputTokens != r.InputTokens ||
				m.OutputTokens != r.OutputTokens ||
				m.CacheInputTokens != r.CacheInputTokens ||
				m.SystemTokens != r.SystemTokens ||
				m.ErrorCount != r.ErrorCount ||
				m.StreamedCount != r.StreamedCount {
				t.Fatalf("[%s] group %q mismatch:\nmerged=%+v\nraw=%+v", groupBy, statsKey(m), m, r)
			}
			if diff := m.AvgLatencyMs - r.AvgLatencyMs; diff > 0.001 || diff < -0.001 {
				t.Fatalf("[%s] group %q avg latency mismatch: merged=%f raw=%f", groupBy, statsKey(m), m.AvgLatencyMs, r.AvgLatencyMs)
			}
		}
	}
}

// TestAggregatedStatsDailyWithFilters checks provider/model/user filters take
// the daily path and match raw results.
func TestAggregatedStatsDailyWithFilters(t *testing.T) {
	sm, err := NewStoreManager(t.TempDir())
	if err != nil {
		t.Fatalf("NewStoreManager failed: %v", err)
	}
	defer sm.Close()
	store := sm.Usage()
	seedUsageRecords(t, store, 6)

	now := time.Now()
	query := UsageStatsQuery{
		GroupBy:   "model",
		StartTime: now.Add(-5 * 24 * time.Hour),
		EndTime:   now,
		Provider:  "prov-a",
		UserID:    "admin",
		Limit:     100,
		SortBy:    "total_tokens",
		SortOrder: "desc",
	}

	merged, handled, err := store.aggregatedStatsFromDaily(query)
	if err != nil {
		t.Fatalf("aggregatedStatsFromDaily failed: %v", err)
	}
	if !handled {
		t.Fatal("expected daily path to handle filtered query")
	}

	buckets, err := store.rawAggBuckets(query, true)
	if err != nil {
		t.Fatalf("rawAggBuckets failed: %v", err)
	}
	if len(merged) != len(buckets) {
		t.Fatalf("group count mismatch: merged=%d raw=%d", len(merged), len(buckets))
	}
	var mergedTotal, rawTotal int64
	for _, m := range merged {
		mergedTotal += m.RequestCount
	}
	for _, b := range buckets {
		rawTotal += b.RequestCount
	}
	if mergedTotal != rawTotal {
		t.Fatalf("request totals mismatch: merged=%d raw=%d", mergedTotal, rawTotal)
	}
}

// TestTimeSeriesDailyMatchesRaw verifies day-interval time series from the
// daily path match the raw scan bucket-for-bucket.
func TestTimeSeriesDailyMatchesRaw(t *testing.T) {
	sm, err := NewStoreManager(t.TempDir())
	if err != nil {
		t.Fatalf("NewStoreManager failed: %v", err)
	}
	defer sm.Close()
	store := sm.Usage()
	seedUsageRecords(t, store, 8)

	now := time.Now()
	start := now.Add(-7 * 24 * time.Hour)

	merged, handled, err := store.timeSeriesFromDaily("day", start, now, nil)
	if err != nil {
		t.Fatalf("timeSeriesFromDaily failed: %v", err)
	}
	if !handled {
		t.Fatal("expected daily path to handle a 7-day day-interval query")
	}

	raw, err := store.rawTimeSeries("day", start, now, nil)
	if err != nil {
		t.Fatalf("rawTimeSeries failed: %v", err)
	}

	if len(merged) != len(raw) {
		t.Fatalf("bucket count mismatch: merged=%d raw=%d", len(merged), len(raw))
	}
	for i := range merged {
		m, r := merged[i], raw[i]
		if m.Timestamp != r.Timestamp ||
			m.RequestCount != r.RequestCount ||
			m.TotalTokens != r.TotalTokens ||
			m.InputTokens != r.InputTokens ||
			m.OutputTokens != r.OutputTokens ||
			m.CacheInputTokens != r.CacheInputTokens ||
			m.ErrorCount != r.ErrorCount {
			t.Fatalf("bucket %d mismatch:\nmerged=%+v\nraw=%+v", i, m, r)
		}
	}

	// The daily table must now be populated for completed days.
	var dailyRows int64
	if err := store.db.Model(&UsageDailyRecord{}).Count(&dailyRows).Error; err != nil {
		t.Fatalf("counting usage_daily rows failed: %v", err)
	}
	if dailyRows == 0 {
		t.Fatal("expected usage_daily to be populated after daily-path query")
	}
}

// TestPublicAPIsUseDailyPathTransparently exercises the public entry points
// end to end (routing decision included).
func TestPublicAPIsUseDailyPathTransparently(t *testing.T) {
	sm, err := NewStoreManager(t.TempDir())
	if err != nil {
		t.Fatalf("NewStoreManager failed: %v", err)
	}
	defer sm.Close()
	store := sm.Usage()
	total := seedUsageRecords(t, store, 4)

	now := time.Now()
	stats, err := store.GetAggregatedStats(UsageStatsQuery{
		GroupBy:   "model",
		StartTime: now.Add(-30 * 24 * time.Hour),
		EndTime:   now,
		Limit:     100,
		SortBy:    "total_tokens",
		SortOrder: "desc",
	})
	if err != nil {
		t.Fatalf("GetAggregatedStats failed: %v", err)
	}
	var got int64
	for _, s := range stats {
		got += s.RequestCount
	}
	if got != int64(total) {
		t.Fatalf("GetAggregatedStats total requests = %d, want %d", got, total)
	}

	series, err := store.GetTimeSeries("day", now.Add(-30*24*time.Hour), now, nil)
	if err != nil {
		t.Fatalf("GetTimeSeries failed: %v", err)
	}
	got = 0
	for _, b := range series {
		got += b.RequestCount
	}
	if got != int64(total) {
		t.Fatalf("GetTimeSeries total requests = %d, want %d", got, total)
	}
}

// TestDeleteOlderThanPurgesDailyAggregates ensures raw deletion keeps the
// daily table consistent and resets the in-memory aggregation cache.
func TestDeleteOlderThanPurgesDailyAggregates(t *testing.T) {
	sm, err := NewStoreManager(t.TempDir())
	if err != nil {
		t.Fatalf("NewStoreManager failed: %v", err)
	}
	defer sm.Close()
	store := sm.Usage()
	seedUsageRecords(t, store, 6)

	now := time.Now()
	// Populate the daily table.
	if _, _, err := store.timeSeriesFromDaily("day", now.Add(-5*24*time.Hour), now, nil); err != nil {
		t.Fatalf("timeSeriesFromDaily failed: %v", err)
	}

	cutoff := now.Add(-3 * 24 * time.Hour)
	if _, err := store.DeleteOlderThan(cutoff); err != nil {
		t.Fatalf("DeleteOlderThan failed: %v", err)
	}

	var stale int64
	if err := store.db.Model(&UsageDailyRecord{}).
		Where("date < ?", cutoff.UTC().Format(dailyDateLayout)).
		Count(&stale).Error; err != nil {
		t.Fatalf("counting stale daily rows failed: %v", err)
	}
	if stale != 0 {
		t.Fatalf("expected daily aggregates before cutoff to be deleted, found %d", stale)
	}

	// Query again after deletion: totals must match a raw scan.
	mergedSeries, handled, err := store.timeSeriesFromDaily("day", now.Add(-5*24*time.Hour), now, nil)
	if err != nil || !handled {
		t.Fatalf("timeSeriesFromDaily after delete: handled=%v err=%v", handled, err)
	}
	rawSeries, err := store.rawTimeSeries("day", now.Add(-5*24*time.Hour), now, nil)
	if err != nil {
		t.Fatalf("rawTimeSeries failed: %v", err)
	}
	var mergedTotal, rawTotal int64
	for _, b := range mergedSeries {
		mergedTotal += b.RequestCount
	}
	for _, b := range rawSeries {
		rawTotal += b.RequestCount
	}
	if mergedTotal != rawTotal {
		t.Fatalf("post-delete totals mismatch: merged=%d raw=%d", mergedTotal, rawTotal)
	}
}

// TestGetRecordsTotalWithoutFullCount checks the COUNT-skip fast path.
func TestGetRecordsTotalWithoutFullCount(t *testing.T) {
	sm, err := NewStoreManager(t.TempDir())
	if err != nil {
		t.Fatalf("NewStoreManager failed: %v", err)
	}
	defer sm.Close()
	store := sm.Usage()
	total := seedUsageRecords(t, store, 1)

	start := time.Now().Add(-2 * 24 * time.Hour)
	end := time.Now()

	// Page smaller than result set: COUNT path.
	records, gotTotal, err := store.GetRecords(start, end, nil, 3, 0)
	if err != nil {
		t.Fatalf("GetRecords failed: %v", err)
	}
	if len(records) != 3 || gotTotal != int64(total) {
		t.Fatalf("paged GetRecords = (%d records, total %d), want (3, %d)", len(records), gotTotal, total)
	}

	// Page larger than result set: fast path total.
	records, gotTotal, err = store.GetRecords(start, end, nil, 500, 0)
	if err != nil {
		t.Fatalf("GetRecords failed: %v", err)
	}
	if len(records) != total || gotTotal != int64(total) {
		t.Fatalf("full GetRecords = (%d records, total %d), want (%d, %d)", len(records), gotTotal, total, total)
	}
}
