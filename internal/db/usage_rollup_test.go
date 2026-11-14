package db

import (
	"fmt"
	"math"
	"reflect"
	"testing"
	"time"
)

func TestPlanRollupSegments(t *testing.T) {
	now := time.Date(2026, 11, 14, 15, 30, 0, 0, time.UTC) // eligible hours end at 15:00
	at := func(day, hour, min int) time.Time { return time.Date(2026, 11, day, hour, min, 0, 0, time.UTC) }
	type seg struct {
		tier     string
		from, to time.Time
	}
	describe := func(segs []rollupSegment) []seg {
		out := make([]seg, len(segs))
		for i, s := range segs {
			name := "raw"
			if s.tier != nil {
				name = s.tier.name
			}
			out[i] = seg{name, s.from, s.to}
		}
		return out
	}
	ns := time.Nanosecond

	for _, tc := range []struct {
		name       string
		start, end time.Time
		hourOnly   bool
		want       []seg
	}{
		{"under one complete hour is raw", at(14, 14, 30), at(14, 15, 30), false, nil},
		{"hour aligned window, hours only", at(14, 8, 0), at(14, 15, 0), false, []seg{{"hour", at(14, 8, 0), at(14, 15, 0)}}},
		{"unaligned edges become raw", at(14, 8, 20), at(14, 15, 30), false, []seg{
			{"raw", at(14, 8, 20), at(14, 9, 0).Add(-ns)},
			{"hour", at(14, 9, 0), at(14, 15, 0)},
			{"raw", at(14, 15, 0), at(14, 15, 30)},
		}},
		{"end past the grace window stays raw", at(14, 8, 0), at(14, 16, 0), false, []seg{
			{"hour", at(14, 8, 0), at(14, 15, 0)},
			{"raw", at(14, 15, 0), at(14, 16, 0)},
		}},
		{"two whole days use the day tier", at(10, 6, 0), at(13, 0, 0), false, []seg{
			{"hour", at(10, 6, 0), at(11, 0, 0)},
			{"day", at(11, 0, 0), at(13, 0, 0)},
		}},
		{"day tier flanked by hours", at(10, 6, 0), at(14, 15, 0), false, []seg{
			{"hour", at(10, 6, 0), at(11, 0, 0)},
			{"day", at(11, 0, 0), at(14, 0, 0)},
			{"hour", at(14, 0, 0), at(14, 15, 0)},
		}},
		{"one whole day is not worth the day tier", at(11, 0, 0), at(13, 0, 0).Add(-time.Hour), false, []seg{
			{"hour", at(11, 0, 0), at(12, 23, 0)},
		}},
		{"hour-only queries never use days", at(10, 6, 0), at(14, 15, 0), true, []seg{
			{"hour", at(10, 6, 0), at(14, 15, 0)},
		}},
	} {
		t.Run(tc.name, func(t *testing.T) {
			got := planRollupSegments(tc.start, tc.end, now, tc.hourOnly)
			if tc.want == nil {
				if got != nil {
					t.Fatalf("got %v, want a raw scan", describe(got))
				}
				return
			}
			if !reflect.DeepEqual(describe(got), tc.want) {
				t.Fatalf("got  %v\nwant %v", describe(got), tc.want)
			}
		})
	}
}

// TestPlanRollupGraceWindow: an hour becomes eligible only hourAggGrace after
// it ended.
func TestPlanRollupGraceWindow(t *testing.T) {
	start := time.Date(2026, 11, 14, 8, 0, 0, 0, time.UTC)
	end := time.Date(2026, 11, 14, 15, 30, 0, 0, time.UTC)
	lastHourBuilt := func(now time.Time) time.Time {
		segs := planRollupSegments(start, end, now, false)
		return segs[0].to
	}
	within := time.Date(2026, 11, 14, 15, 0, 0, 0, time.UTC).Add(hourAggGrace - time.Second)
	if got, want := lastHourBuilt(within), time.Date(2026, 11, 14, 14, 0, 0, 0, time.UTC); !got.Equal(want) {
		t.Errorf("inside the grace window the 14:00 hour must not be built: ends at %v, want %v", got, want)
	}
	past := time.Date(2026, 11, 14, 15, 0, 0, 0, time.UTC).Add(hourAggGrace)
	if got, want := lastHourBuilt(past), time.Date(2026, 11, 14, 15, 0, 0, 0, time.UTC); !got.Equal(want) {
		t.Errorf("past the grace window the 14:00 hour is built: ends at %v, want %v", got, want)
	}
}

// statsEqual compares a rollup-served stat with the raw scan of the same
// query, ignoring dimensions the grouping does not define.
func statsEqual(g, w AggregatedStat) bool {
	return g.Key == w.Key && g.RequestCount == w.RequestCount && g.TotalTokens == w.TotalTokens &&
		g.InputTokens == w.InputTokens && g.OutputTokens == w.OutputTokens &&
		g.CacheReadTokens == w.CacheReadTokens && g.CacheWriteTokens == w.CacheWriteTokens &&
		g.ReasoningTokens == w.ReasoningTokens && g.SystemTokens == w.SystemTokens &&
		g.ErrorCount == w.ErrorCount && g.StreamedCount == w.StreamedCount &&
		math.Abs(g.AvgLatencyMs-w.AvgLatencyMs) < 1e-6
}

// hourWindows are windows of every shape the planner handles, relative to now.
// Bounds are server-local, as in production: usage_records.timestamp is a
// string compared lexicographically, so a bound in another zone would make the
// raw reference scan itself inexact.
func hourWindows(now time.Time) map[string][2]time.Time {
	top := now.Truncate(time.Hour)
	utcMidnight := utcDayStart(now).In(time.Local)
	windows := map[string][2]time.Time{
		"last 6h unaligned":      {now.Add(-6*time.Hour - 17*time.Minute), now},
		"aligned 8h block":       {top.Add(-30 * time.Hour), top.Add(-22 * time.Hour)},
		"half-hour start":        {top.Add(-50*time.Hour - 30*time.Minute), now.Add(-time.Hour)},
		"3 days":                 {top.Add(-72 * time.Hour), now},
		"7 whole days":           {utcMidnight.AddDate(0, 0, -7), utcMidnight.AddDate(0, 0, 1)},
		"under an hour":          {now.Add(-20 * time.Minute), now},
		"90 days, odd start/end": {now.Add(-90*24*time.Hour - 41*time.Minute), now.Add(-7 * time.Minute)},
	}
	for name, w := range windows {
		windows[name] = [2]time.Time{w[0].In(time.Local), w[1].In(time.Local)}
	}
	return windows
}

// TestHourRollupStatsMatchRaw: every grouping the rollups serve, over every
// window shape, equals a raw scan.
func TestHourRollupStatsMatchRaw(t *testing.T) {
	store := seededUsageStore(t, 8000)
	now := time.Now()
	for wname, w := range hourWindows(now) {
		for _, q := range []UsageStatsQuery{
			{GroupBy: "model"},
			{GroupBy: "provider"},
			{GroupBy: "user"},
			{GroupBy: "daily"},
			{GroupBy: "hourly"},
			{GroupBy: "scenario"},
			{GroupBy: "model", Provider: "prov-0", UserID: "user-1"},
			{GroupBy: "hourly", Scenario: "codex"},
			{GroupBy: "daily", Scenario: "claude_code"},
		} {
			q.StartTime, q.EndTime, q.Limit = w[0], w[1], 1000
			name := fmt.Sprintf("%s/%s/%s%s", wname, q.GroupBy, q.Provider, q.Scenario)
			got, err := store.GetAggregatedStats(q)
			if err != nil {
				t.Fatalf("%s: %v", name, err)
			}
			want := rawStats(t, store, q)
			sortStats(got)
			sortStats(want)
			if len(got) != len(want) {
				t.Fatalf("%s: %d buckets, raw has %d", name, len(got), len(want))
			}
			for i := range got {
				g, w := got[i], want[i]
				if !statsEqual(g, w) {
					t.Fatalf("%s bucket %q:\n got  %+v\n want %+v", name, g.Key, g, w)
				}
			}
		}
	}
}

// TestHourRollupSeriesMatchRaw: the hourly series is the peak/valley view --
// each bucket must equal the raw scan, for every window shape and filter.
func TestHourRollupSeriesMatchRaw(t *testing.T) {
	store := seededUsageStore(t, 8000)
	now := time.Now()
	for wname, w := range hourWindows(now) {
		for _, interval := range []string{"hour", "day"} {
			for _, filters := range []map[string]string{nil, {"provider_uuid": "prov-0"}, {"scenario": "codex"}, {"user_id": "user-2", "model": "model-1"}} {
				name := fmt.Sprintf("%s/%s/%v", wname, interval, filters)
				got, err := store.GetTimeSeries(interval, w[0], w[1], filters)
				if err != nil {
					t.Fatalf("%s: %v", name, err)
				}
				want, err := store.rawTimeSeries(interval, w[0], w[1], filters)
				if err != nil {
					t.Fatal(err)
				}
				if len(got) != len(want) {
					t.Fatalf("%s: %d buckets, raw has %d", name, len(got), len(want))
				}
				for i := range got {
					g, w := got[i], want[i]
					if g.Timestamp != w.Timestamp || g.RequestCount != w.RequestCount || g.TotalTokens != w.TotalTokens ||
						g.InputTokens != w.InputTokens || g.OutputTokens != w.OutputTokens ||
						g.CacheReadTokens != w.CacheReadTokens || g.CacheWriteTokens != w.CacheWriteTokens ||
						g.ReasoningTokens != w.ReasoningTokens || g.SystemTokens != w.SystemTokens ||
						g.ErrorCount != w.ErrorCount || math.Abs(g.AvgLatencyMs-w.AvgLatencyMs) > 1e-6 {
						t.Fatalf("%s bucket %d:\n got  %+v\n want %+v", name, i, g, w)
					}
				}
			}
		}
	}
}

// TestDayRollupIsSumOfHours: the two tiers can never disagree, for the main
// and the scenario tables alike.
func TestDayRollupIsSumOfHours(t *testing.T) {
	store := seededUsageStore(t, 6000)
	start, end := dayRange(40)
	if _, err := store.GetAggregatedStats(UsageStatsQuery{GroupBy: "model", StartTime: start, EndTime: end, Limit: 10}); err != nil {
		t.Fatal(err)
	}
	for _, pair := range [][2]string{{"usage_daily", "usage_hourly"}, {"usage_daily_scenario", "usage_hourly_scenario"}} {
		var mismatched int64
		err := store.db.Raw(fmt.Sprintf(`
			SELECT COUNT(*) FROM (
				SELECT date, SUM(request_count) r, SUM(total_tokens) t, SUM(error_count) e, SUM(latency_sum_ms) l FROM %s GROUP BY date
			) d LEFT JOIN (
				SELECT substr(hour, 1, 10) date, SUM(request_count) r, SUM(total_tokens) t, SUM(error_count) e, SUM(latency_sum_ms) l FROM %s GROUP BY substr(hour, 1, 10)
			) h USING (date)
			WHERE h.r IS NULL OR d.r != h.r OR d.t != h.t OR d.e != h.e OR d.l != h.l`, pair[0], pair[1])).Row().Scan(&mismatched)
		if err != nil || mismatched != 0 {
			t.Fatalf("%s vs %s: %d days differ (%v)", pair[0], pair[1], mismatched, err)
		}
	}
}

// TestCurrentHourIsNeverAggregated: an hour is built only after it ended plus
// the grace window, so records still arriving never miss the rollup.
func TestCurrentHourIsNeverAggregated(t *testing.T) {
	store := newUsageStoreForTest(t)
	now := time.Now()
	if err := store.RecordUsage(&UsageRecord{ProviderUUID: "p", ProviderName: "P", Model: "m", Scenario: "s", Timestamp: now, Status: "success", LatencyMs: 10}); err != nil {
		t.Fatal(err)
	}
	if _, err := store.GetTimeSeries("hour", now.Add(-5*time.Hour), now.Add(time.Hour), nil); err != nil {
		t.Fatal(err)
	}
	var built int64
	if err := store.db.Model(&UsageHourlyRecord{}).Where("hour >= ?", hourTier.key(now.Add(-hourAggGrace))).Count(&built).Error; err != nil {
		t.Fatal(err)
	}
	if built != 0 {
		t.Fatalf("%d hour rows exist for the current/grace hour", built)
	}
	got, err := store.GetTimeSeries("hour", now.Add(-5*time.Hour), now.Add(time.Hour), nil)
	if err != nil {
		t.Fatal(err)
	}
	var total int64
	for _, b := range got {
		total += b.RequestCount
	}
	if total != 1 {
		t.Fatalf("series counts %d requests, want the 1 recorded this hour", total)
	}
}

// TestRetentionRebuildsBoundaryHour: after the retention delete cuts through
// an hour and a day, both tiers are rebuilt from what is left.
func TestRetentionRebuildsBoundaryHour(t *testing.T) {
	store := seededUsageStore(t, 8000)
	start, end := dayRange(60)
	query := func(groupBy string) []AggregatedStat {
		t.Helper()
		got, err := store.GetAggregatedStats(UsageStatsQuery{GroupBy: groupBy, StartTime: start, EndTime: end, Limit: 1000})
		if err != nil {
			t.Fatal(err)
		}
		sortStats(got)
		return got
	}
	query("hourly")
	query("daily")

	cutoff := time.Now().Add(-20*24*time.Hour - 37*time.Minute) // mid-hour, mid-day
	if _, err := store.DeleteOlderThan(cutoff); err != nil {
		t.Fatal(err)
	}
	for _, groupBy := range []string{"hourly", "daily", "model"} {
		got := query(groupBy)
		want := rawStats(t, store, UsageStatsQuery{GroupBy: groupBy, StartTime: start, EndTime: end, Limit: 1000})
		sortStats(want)
		if len(got) != len(want) {
			t.Fatalf("%s after delete: %d buckets, raw has %d", groupBy, len(got), len(want))
		}
		for i := range got {
			if !statsEqual(got[i], want[i]) {
				t.Fatalf("%s after delete, bucket %q:\n got  %+v\n want %+v", groupBy, got[i].Key, got[i], want[i])
			}
		}
	}
}

// TestAggregateToDailyRebuilds: the explicit rebuild picks up rows written
// after the day was aggregated, in both tiers.
func TestAggregateToDailyRebuilds(t *testing.T) {
	store := newUsageStoreForTest(t)
	day := utcDayStart(time.Now().AddDate(0, 0, -3))
	add := func(h int) {
		t.Helper()
		if err := store.RecordUsage(&UsageRecord{ProviderUUID: "p", ProviderName: "P", Model: "m", Scenario: "s", UserID: "u",
			Timestamp: day.Add(time.Duration(h)*time.Hour + 10*time.Minute).In(time.Local), InputTokens: 10, Status: "success"}); err != nil {
			t.Fatal(err)
		}
	}
	add(3)
	if n, err := store.AggregateToDaily(day); err != nil || n != 1 {
		t.Fatalf("first aggregate = %d rows, %v", n, err)
	}
	add(9) // late row for an already aggregated day
	if n, err := store.AggregateToDaily(day); err != nil || n != 1 {
		t.Fatalf("rebuild = %d rows, %v", n, err)
	}
	var reqs, hours int64
	store.db.Model(&UsageDailyRecord{}).Where("date = ?", dayTier.key(day)).Select("COALESCE(SUM(request_count), 0)").Row().Scan(&reqs)
	store.db.Model(&UsageHourlyRecord{}).Where("hour >= ? AND hour < ? AND request_count > 0", hourTier.key(day), hourTier.key(day.Add(24*time.Hour))).Count(&hours)
	if reqs != 2 || hours != 2 {
		t.Fatalf("after rebuild: day counts %d requests over %d hours, want 2 over 2", reqs, hours)
	}
}

// TestUpgradeKeepsDailyRowsAndFillsHours: a database upgraded from the
// day-only rollup has usage_daily rows and no hour rows. Answers stay equal to
// raw, and the hour tier fills in lazily.
func TestUpgradeKeepsDailyRowsAndFillsHours(t *testing.T) {
	store := seededUsageStore(t, 6000)
	start, end := dayRange(30)
	q := UsageStatsQuery{GroupBy: "model", StartTime: start, EndTime: end, Limit: 1000}
	if _, err := store.GetAggregatedStats(q); err != nil {
		t.Fatal(err)
	}
	for _, table := range []string{"usage_hourly", "usage_hourly_scenario"} {
		if err := store.db.Exec("DELETE FROM " + table).Error; err != nil {
			t.Fatal(err)
		}
	}
	var days int64
	store.db.Model(&UsageDailyRecord{}).Count(&days)
	if days == 0 {
		t.Fatal("setup: no usage_daily rows")
	}

	got, err := store.GetAggregatedStats(q)
	if err != nil {
		t.Fatal(err)
	}
	want := rawStats(t, store, q)
	sortStats(got)
	sortStats(want)
	if len(got) != len(want) {
		t.Fatalf("%d buckets, raw has %d", len(got), len(want))
	}
	for i := range got {
		if !statsEqual(got[i], want[i]) {
			t.Fatalf("bucket %q:\n got  %+v\n want %+v", got[i].Key, got[i], want[i])
		}
	}
	// Only the edge hours were rebuilt; the days kept their rows.
	var hours int64
	store.db.Model(&UsageHourlyRecord{}).Count(&hours)
	if hours == 0 || hours > 2*24*60 {
		t.Fatalf("hour rows after the first post-upgrade query = %d, want only the edge hours", hours)
	}
}
