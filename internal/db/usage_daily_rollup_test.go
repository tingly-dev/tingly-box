package db

import (
	"gorm.io/gorm"
	"math"
	"sort"
	"strings"
	"testing"
	"time"
)

// seededUsageStore is a store holding a few thousand deterministic records
// over the dashboard seed's 100 days (several agents, providers, users).
func seededUsageStore(t *testing.T, n int) *UsageStore {
	t.Helper()
	store := newUsageStoreForTest(t)
	seedDashboardUsage(t, store, n)
	return store
}

// comparableStat keeps the fields a rollup-vs-raw comparison can rely on:
// provider/model/user are not dimensions of a group_by=scenario result.
func comparableStat(s AggregatedStat) AggregatedStat {
	return AggregatedStat{
		Key: s.Key, Scenario: s.Scenario,
		RequestCount: s.RequestCount, TotalTokens: s.TotalTokens,
		InputTokens: s.InputTokens, OutputTokens: s.OutputTokens,
		CacheReadTokens: s.CacheReadTokens, CacheWriteTokens: s.CacheWriteTokens,
		ReasoningTokens: s.ReasoningTokens, SystemTokens: s.SystemTokens,
		AvgLatencyMs: s.AvgLatencyMs, ErrorCount: s.ErrorCount,
		StreamedCount: s.StreamedCount,
	}
}

func rawStats(t *testing.T, store *UsageStore, q UsageStatsQuery) []AggregatedStat {
	t.Helper()
	buckets, err := store.rawAggBuckets(q, true)
	if err != nil {
		t.Fatal(err)
	}
	out := make([]AggregatedStat, len(buckets))
	for i, b := range buckets {
		out[i] = b.toAggregatedStat()
	}
	return out
}

func sortStats(s []AggregatedStat) {
	sort.Slice(s, func(i, j int) bool { return s[i].Key < s[j].Key })
}

// TestScenarioStatsFromDailyMatchRaw checks the usage_daily_scenario path
// (group_by=scenario, scenario filter) against a raw usage_records scan.
func TestScenarioStatsFromDailyMatchRaw(t *testing.T) {
	store := seededUsageStore(t, 6000)
	for _, days := range []int{3, 30, 90} {
		start, end := dayRange(days)
		for name, q := range map[string]UsageStatsQuery{
			"group scenario":          {GroupBy: "scenario"},
			"group daily + scenario":  {GroupBy: "daily", Scenario: "codex"},
			"group scenario + filter": {GroupBy: "scenario", Scenario: "claude_code"},
			"group scenario + user":   {GroupBy: "scenario", UserID: "user-0"}, // raw fallback
			"group model + scenario":  {GroupBy: "model", Scenario: "codex"},   // raw fallback
			"group scenario + status": {GroupBy: "scenario", Status: "error"},  // raw fallback
			"group scenario + model":  {GroupBy: "scenario", Model: "model-0"}, // raw fallback
			"group provider+scenario": {GroupBy: "provider", Scenario: "openai"},
		} {
			q.StartTime, q.EndTime, q.Limit = start, end, 1000
			got, err := store.GetAggregatedStats(q)
			if err != nil {
				t.Fatalf("%dd %s: %v", days, name, err)
			}
			want := rawStats(t, store, q)
			sortStats(got)
			sortStats(want)
			if len(got) != len(want) {
				t.Fatalf("%dd %s: %d buckets, raw has %d", days, name, len(got), len(want))
			}
			for i := range got {
				g, w := comparableStat(got[i]), comparableStat(want[i])
				if q.GroupBy != "scenario" && q.GroupBy != "daily" {
					// non-scenario groupings clear Scenario on the daily path
					// only; the merged-vs-raw difference is irrelevant here.
					g.Scenario, w.Scenario = "", ""
				}
				if g.Key != w.Key || g.RequestCount != w.RequestCount || g.TotalTokens != w.TotalTokens ||
					g.InputTokens != w.InputTokens || g.OutputTokens != w.OutputTokens ||
					g.CacheReadTokens != w.CacheReadTokens || g.CacheWriteTokens != w.CacheWriteTokens ||
					g.ReasoningTokens != w.ReasoningTokens || g.ErrorCount != w.ErrorCount ||
					g.StreamedCount != w.StreamedCount || math.Abs(g.AvgLatencyMs-w.AvgLatencyMs) > 1e-6 {
					t.Fatalf("%dd %s bucket %q:\n got  %+v\n want %+v", days, name, g.Key, g, w)
				}
			}
		}
	}
}

// TestScenarioStatsDropProviderNoise: the raw path leaks an arbitrary
// provider/model/user into a group_by=scenario bucket (bare SQLite columns);
// the rollup path must report only the dimension that was grouped.
func TestScenarioStatsDropProviderNoise(t *testing.T) {
	store := seededUsageStore(t, 3000)
	start, end := dayRange(30)
	stats, err := store.GetAggregatedStats(UsageStatsQuery{GroupBy: "scenario", StartTime: start, EndTime: end, Limit: 100})
	if err != nil {
		t.Fatal(err)
	}
	if len(stats) == 0 {
		t.Fatal("no scenario buckets")
	}
	for _, s := range stats {
		if s.Scenario == "" || s.Key != s.Scenario || s.ProviderUUID != "" || s.Model != "" || s.UserID != "" {
			t.Fatalf("scenario bucket carries other dimensions: %+v", s)
		}
	}
}

func TestScenarioTimeSeriesFromDailyMatchesRaw(t *testing.T) {
	store := seededUsageStore(t, 6000)
	for _, days := range []int{3, 30, 90, 365} {
		start, end := dayRange(days)
		for _, filters := range []map[string]string{
			{"scenario": "codex"},
			{"scenario": "nope"},
			{"scenario": "codex", "model": "model-0"}, // raw fallback
			{"user_id": "user-1"},
			nil,
		} {
			got, err := store.GetTimeSeries("day", start, end, filters)
			if err != nil {
				t.Fatal(err)
			}
			want, err := store.rawTimeSeries("day", start, end, filters)
			if err != nil {
				t.Fatal(err)
			}
			if len(got) != len(want) {
				t.Fatalf("%dd %v: %d buckets, raw has %d", days, filters, len(got), len(want))
			}
			for i := range got {
				g, w := got[i], want[i]
				if g.Timestamp != w.Timestamp || g.RequestCount != w.RequestCount || g.TotalTokens != w.TotalTokens ||
					g.InputTokens != w.InputTokens || g.OutputTokens != w.OutputTokens ||
					g.CacheReadTokens != w.CacheReadTokens || g.ErrorCount != w.ErrorCount ||
					math.Abs(g.AvgLatencyMs-w.AvgLatencyMs) > 1e-6 {
					t.Fatalf("%dd %v bucket %d:\n got  %+v\n want %+v", days, filters, i, g, w)
				}
			}
		}
	}
}

// TestQuietDaysAreAggregatedOnce: a long window over mostly-empty history must
// not re-aggregate its quiet days on every request (the 365-day heatmap used
// to issue ~530 statements per load), and the marker rows must not surface
// in any result.
func TestQuietDaysAreAggregatedOnce(t *testing.T) {
	store := seededUsageStore(t, 600) // 100 days, ~6/day: many quiet days
	counter := &queryCounter{}
	store.db.Config.Logger = counter
	start, end := dayRange(365)

	first, err := store.GetTimeSeries("day", start, end, nil)
	if err != nil {
		t.Fatal(err)
	}
	counter.reset()
	second, err := store.GetTimeSeries("day", start, end, nil)
	if err != nil {
		t.Fatal(err)
	}
	if n := counter.count(); n > 4 {
		t.Fatalf("repeat 365-day timeseries issued %d statements, want <= 4", n)
	}
	if len(first) != len(second) {
		t.Fatalf("repeat result differs: %d vs %d buckets", len(first), len(second))
	}
	for _, b := range second {
		if b.RequestCount == 0 {
			t.Fatalf("empty-day marker leaked into the time series: %+v", b)
		}
	}

	stats, err := store.GetAggregatedStats(UsageStatsQuery{GroupBy: "model", StartTime: start, EndTime: end, Limit: 1000})
	if err != nil {
		t.Fatal(err)
	}
	for _, s := range stats {
		if s.Key == "" || s.RequestCount == 0 {
			t.Fatalf("empty-day marker leaked into stats: %+v", s)
		}
	}
	var total int64
	for _, s := range stats {
		total += s.RequestCount
	}
	if total != 600 {
		t.Fatalf("stats over the full window count %d requests, want 600", total)
	}
}

// reference implementation of GetPerformanceSummary before it streamed rows.
func referencePerformanceSummary(t *testing.T, us *UsageStore, start, end time.Time, filters map[string]string) PerformanceSummary {
	t.Helper()
	var records []UsageRecord
	if err := us.db.Model(&UsageRecord{}).
		Select("latency_ms", "ttft_ms", "output_tokens", "streamed").
		Where("status = ?", "success").
		Scopes(withinTimeRange(start, end), withColumnFilters(filters)).
		Find(&records).Error; err != nil {
		t.Fatal(err)
	}
	var completion, ttft, tps []float64
	for _, r := range records {
		if r.LatencyMs > 0 {
			completion = append(completion, float64(r.LatencyMs))
		}
		if !r.Streamed {
			continue
		}
		if r.TTFTMs > 0 {
			ttft = append(ttft, float64(r.TTFTMs))
		}
		if v := TokensPerSecond(r.OutputTokens, r.LatencyMs, r.TTFTMs); v > 0 {
			tps = append(tps, v)
		}
	}
	return PerformanceSummary{
		TTFT:       summarizePerformanceMetric(ttft),
		TPS:        summarizePerformanceMetric(tps),
		Completion: summarizePerformanceMetric(completion),
	}
}

func TestPerformanceSummaryMatchesReference(t *testing.T) {
	store := seededUsageStore(t, 5000)
	store.perfCacheTTL = 0
	start, end := dayRange(30)
	for _, filters := range []map[string]string{
		nil,
		{"provider_uuid": "prov-0"},
		{"scenario": "codex", "user_id": "user-0"},
		{"model": "model-none"},
	} {
		got, err := store.GetPerformanceSummary(start, end, filters)
		if err != nil {
			t.Fatal(err)
		}
		if want := referencePerformanceSummary(t, store, start, end, filters); got != want {
			t.Fatalf("filters %v:\n got  %+v\n want %+v", filters, got, want)
		}
	}
}

func TestPerformanceSummaryCache(t *testing.T) {
	store := newUsageStoreForTest(t)
	now := time.Now()
	record := func(latency int) {
		t.Helper()
		if err := store.RecordUsage(&UsageRecord{ProviderUUID: "p", ProviderName: "P", Model: "m", Scenario: "s", Timestamp: now.Add(-time.Minute), Status: "success", LatencyMs: latency}); err != nil {
			t.Fatal(err)
		}
	}
	record(100)
	start, end := now.Add(-time.Hour), now.Add(time.Hour)

	first, err := store.GetPerformanceSummary(start, end, nil)
	if err != nil || first.Completion.SampleCount != 1 {
		t.Fatalf("first = %+v, %v", first, err)
	}
	record(300)
	cached, _ := store.GetPerformanceSummary(start, end, nil)
	if cached.Completion.SampleCount != 1 {
		t.Fatalf("within the TTL the cached summary should be served, got %+v", cached.Completion)
	}
	if other, _ := store.GetPerformanceSummary(start, end, map[string]string{"model": "m"}); other.Completion.SampleCount != 2 {
		t.Fatalf("a different filter set must not share the entry, got %+v", other.Completion)
	}
	if _, err := store.DeleteOlderThan(now.Add(-2 * time.Hour)); err != nil {
		t.Fatal(err)
	}
	if fresh, _ := store.GetPerformanceSummary(start, end, nil); fresh.Completion.SampleCount != 2 {
		t.Fatalf("a delete must drop cached summaries, got %+v", fresh.Completion)
	}

	store.perfCacheTTL = 0
	record(500)
	if live, _ := store.GetPerformanceSummary(start, end, nil); live.Completion.SampleCount != 3 {
		t.Fatalf("TTL 0 disables the cache, got %+v", live.Completion)
	}
}

// TestUsageDailyRebuiltForScenarioTable: a usage_daily that predates
// usage_daily_scenario is dropped on migration, otherwise its already
// aggregated days would never get scenario rows.
func TestUsageDailyRebuiltForScenarioTable(t *testing.T) {
	store := seededUsageStore(t, 1500)
	start, end := dayRange(30)
	if _, err := store.GetAggregatedStats(UsageStatsQuery{GroupBy: "model", StartTime: start, EndTime: end, Limit: 10}); err != nil {
		t.Fatal(err)
	}
	m := store.db.Migrator()
	if err := m.DropTable(&UsageDailyScenarioRecord{}); err != nil { // simulate the older schema
		t.Fatal(err)
	}
	if err := migrateUsageTables(store.db); err != nil {
		t.Fatal(err)
	}
	var rows int64
	if err := store.db.Model(&UsageDailyRecord{}).Count(&rows).Error; err != nil || rows != 0 {
		t.Fatalf("usage_daily rows after migration = %d (%v), want 0 (rebuilt lazily)", rows, err)
	}
	got, err := store.GetAggregatedStats(UsageStatsQuery{GroupBy: "scenario", StartTime: start, EndTime: end, Limit: 100})
	if err != nil || len(got) == 0 {
		t.Fatalf("scenario stats after rebuild = %v, %v", got, err)
	}
}

// TestUnusedUsageIndexesDropped: indexes older versions created and no query
// uses are removed from existing databases, and a fresh one never has them.
func TestUnusedUsageIndexesDropped(t *testing.T) {
	store := newUsageStoreForTest(t)
	m := store.db.Migrator()
	for _, name := range droppedUsageIndexes {
		if m.HasIndex(&UsageRecord{}, name) {
			t.Errorf("fresh database has dropped index %s", name)
		}
	}
	// Recreate what a pre-upgrade database carried.
	for stmt, name := range map[string]string{
		"CREATE INDEX idx_timestamp_scenario ON usage_records(timestamp, scenario)":  "idx_timestamp_scenario",
		"CREATE INDEX idx_scenario ON usage_records(scenario)":                       "idx_scenario",
		"CREATE INDEX idx_usage_records_status ON usage_records(status)":             "idx_usage_records_status",
		"CREATE INDEX idx_usage_records_total_tokens ON usage_records(total_tokens)": "idx_usage_records_total_tokens",
	} {
		if err := store.db.Exec(stmt).Error; err != nil {
			t.Fatal(err)
		}
		if !m.HasIndex(&UsageRecord{}, name) {
			t.Fatalf("setup: %s not created", name)
		}
	}
	if err := migrateUsageTables(store.db); err != nil {
		t.Fatal(err)
	}
	for _, name := range droppedUsageIndexes {
		if m.HasIndex(&UsageRecord{}, name) {
			t.Errorf("index %s survived migration", name)
		}
	}
	if !m.HasIndex(&UsageRecord{}, "idx_timestamp") {
		t.Error("idx_timestamp must remain")
	}
}

// TestRangeQueriesUseTimestampIndex pins the planner choice that makes the
// raw edge scans and GetRecords/GetPerformanceSummary cost O(window) rather
// than O(history): with filters present, SQLite must still walk idx_timestamp.
func TestRangeQueriesUseTimestampIndex(t *testing.T) {
	store := seededUsageStore(t, 2000)
	start, end := dayRange(30)
	q := store.db.Model(&UsageRecord{}).
		Where("+status = ?", "success").
		Scopes(withinTimeRange(start, end), withColumnFilters(map[string]string{"scenario": "codex", "provider_uuid": "prov-0", "user_id": "user-0"}))
	var plan []string
	stmt := q.Session(&gorm.Session{DryRun: true}).Find(&[]UsageRecord{}).Statement
	rows, err := store.db.Raw("EXPLAIN QUERY PLAN "+stmt.SQL.String(), stmt.Vars...).Rows()
	if err != nil {
		t.Fatal(err)
	}
	defer rows.Close()
	for rows.Next() {
		var id, parent, unused int
		var detail string
		if err := rows.Scan(&id, &parent, &unused, &detail); err != nil {
			t.Fatal(err)
		}
		plan = append(plan, detail)
	}
	if len(plan) != 1 || !strings.Contains(plan[0], "USING INDEX idx_timestamp") {
		t.Fatalf("plan = %v, want a search on idx_timestamp", plan)
	}
}
