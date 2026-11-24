package db

import (
	"fmt"
	"math/rand"
	"os"
	"sort"
	"strconv"
	"testing"
	"time"
)

// Dashboard read-path benchmark over a seeded, realistically sized
// usage_records table. Mirrors what the dashboard pages request (see
// frontend/src/hooks/useDashboardData.ts and friends): local-midnight windows
// ending tomorrow-midnight for day ranges, group_by=model + timeseries for
// the stat cards/charts, group_by=scenario for the agent filter, the
// 365-day heatmap, the requests table (limit 500) and the performance
// summary.
//
// Run:
//
//	go test ./internal/db -run '^$' -bench DashboardRead -benchtime 5x
//	TB_DASH_REPORT=1 go test ./internal/db -run TestDashboardReport -v
//
// TB_DASH_ROWS (default 300000) sets the seeded row count.

var dashSeedDays = 100

func dashboardRows() int {
	if v, err := strconv.Atoi(os.Getenv("TB_DASH_ROWS")); err == nil && v > 0 {
		return v
	}
	return 300000
}

// skew draws from [0, n) biased toward 0, so a few providers/models/users/
// agents carry most traffic (as in real use) instead of a uniform cross
// product that would defeat any per-day rollup.
func skew(rng *rand.Rand, n int) int {
	r := rng.Float64()
	return int(float64(n) * r * r)
}

// seedDashboardUsage inserts n deterministic records spread over the last
// dashSeedDays days (plus a heavier "today"), across 6 providers x 24 models,
// 5 scenarios, 8 users and mixed status/streaming.
func seedDashboardUsage(tb testing.TB, store *UsageStore, n int) {
	tb.Helper()
	rng := rand.New(rand.NewSource(42))
	scenarios := []string{"claude_code", "codex", "opencode", "anthropic", "openai"}
	now := time.Now()
	todayAge := int64(now.Sub(utcDayStart(now.Local())))
	records := make([]UsageRecord, n)
	for i := range records {
		var age time.Duration
		if i%25 == 0 { // ~4% of traffic lands today
			age = time.Duration(rng.Int63n(todayAge))
		} else {
			age = time.Duration(rng.Int63n(int64(time.Duration(dashSeedDays) * 24 * time.Hour)))
		}
		p := skew(rng, 6)
		in, out := 200+rng.Intn(4000), 50+rng.Intn(1500)
		status := "success"
		if rng.Intn(20) == 0 {
			status = "error"
		}
		records[i] = UsageRecord{
			ProviderUUID:     fmt.Sprintf("prov-%d", p),
			ProviderName:     fmt.Sprintf("Provider %d", p),
			Model:            fmt.Sprintf("model-%d", p*4+skew(rng, 4)),
			Scenario:         scenarios[skew(rng, len(scenarios))],
			RuleUUID:         fmt.Sprintf("rule-%d", rng.Intn(12)),
			UserID:           fmt.Sprintf("user-%d", skew(rng, 8)),
			RequestModel:     "req-model",
			Timestamp:        now.Add(-age),
			InputTokens:      in,
			OutputTokens:     out,
			TotalTokens:      in + out,
			CacheReadTokens:  rng.Intn(2000),
			CacheWriteTokens: rng.Intn(300),
			ReasoningTokens:  rng.Intn(200),
			Status:           status,
			LatencyMs:        300 + rng.Intn(9000),
			TTFTMs:           100 + rng.Intn(1500),
			Streamed:         rng.Intn(3) != 0,
		}
	}
	// Production inserts are chronological, so ids follow time and a range
	// scan reads adjacent pages. Seeding in time order keeps that property.
	sort.Slice(records, func(i, j int) bool { return records[i].Timestamp.Before(records[j].Timestamp) })

	const batch = 500
	tx := store.db.Begin()
	if tx.Error != nil {
		tb.Fatal(tx.Error)
	}
	for i := 0; i < n; i += batch {
		if err := tx.Create(records[i:min(i+batch, n)]).Error; err != nil {
			tb.Fatal(err)
		}
	}
	if err := tx.Commit().Error; err != nil {
		tb.Fatal(err)
	}
}

type dashCase struct {
	name string
	run  func(*UsageStore) error
}

// localMidnight / dayRange reproduce the frontend's buildTimeParams.
func localMidnight(t time.Time) time.Time {
	return time.Date(t.Year(), t.Month(), t.Day(), 0, 0, 0, 0, time.Local)
}

func dayRange(days int) (time.Time, time.Time) {
	today := localMidnight(time.Now())
	return today.AddDate(0, 0, -(days - 1)), today.AddDate(0, 0, 1)
}

func dashboardCases() []dashCase {
	stats := func(groupBy string, days int, mod func(*UsageStatsQuery)) func(*UsageStore) error {
		return func(s *UsageStore) error {
			start, end := dayRange(days)
			q := UsageStatsQuery{GroupBy: groupBy, StartTime: start, EndTime: end, Limit: 1000, SortBy: "total_tokens", SortOrder: "desc"}
			if mod != nil {
				mod(&q)
			}
			_, err := s.GetAggregatedStats(q)
			return err
		}
	}
	series := func(interval string, days int, filters map[string]string) func(*UsageStore) error {
		return func(s *UsageStore) error {
			start, end := dayRange(days)
			_, err := s.GetTimeSeries(interval, start, end, filters)
			return err
		}
	}
	return []dashCase{
		{"stats/model/today", func(s *UsageStore) error {
			now := time.Now()
			_, err := s.GetAggregatedStats(UsageStatsQuery{GroupBy: "model", StartTime: localMidnight(now), EndTime: now, Limit: 1000})
			return err
		}},
		{"stats/provider/last24h", func(s *UsageStore) error {
			now := time.Now()
			_, err := s.GetAggregatedStats(UsageStatsQuery{GroupBy: "provider", StartTime: now.Add(-24 * time.Hour), EndTime: now, Limit: 1000})
			return err
		}},
		// A peak/valley window: one whole-hour 8h block yesterday (UTC 00-08).
		{"stats/provider/valley8h-yesterday", func(s *UsageStore) error {
			from := utcDayStart(time.Now().AddDate(0, 0, -1))
			_, err := s.GetAggregatedStats(UsageStatsQuery{GroupBy: "provider", StartTime: from, EndTime: from.Add(8 * time.Hour), Limit: 1000})
			return err
		}},
		{"stats/hourly/7d", stats("hourly", 7, nil)},
		{"stats/model/30d", stats("model", 30, nil)},
		{"stats/model/90d", stats("model", 90, nil)},
		{"stats/user/30d", stats("user", 30, nil)},
		{"stats/provider/30d", stats("provider", 30, nil)},
		{"stats/scenario/30d", stats("scenario", 30, nil)},
		{"stats/model/30d+scenario", stats("model", 30, func(q *UsageStatsQuery) { q.Scenario = "claude_code" })},
		{"timeseries/minute/today", func(s *UsageStore) error {
			now := time.Now()
			_, err := s.GetTimeSeries("minute", localMidnight(now), now, nil)
			return err
		}},
		{"timeseries/hour/today", func(s *UsageStore) error {
			now := time.Now()
			_, err := s.GetTimeSeries("hour", localMidnight(now), now, nil)
			return err
		}},
		{"timeseries/hour/7d", series("hour", 7, nil)},
		{"timeseries/hour/30d", series("hour", 30, nil)},
		{"timeseries/hour/30d+provider", series("hour", 30, map[string]string{"provider_uuid": "prov-0"})},
		{"timeseries/day/30d", series("day", 30, nil)},
		{"timeseries/day/90d", series("day", 90, nil)},
		{"timeseries/day/heatmap365", series("day", 365, nil)},
		{"timeseries/day/30d+scenario", series("day", 30, map[string]string{"scenario": "codex"})},
		{"records/30d/limit500", func(s *UsageStore) error {
			start, end := dayRange(30)
			_, _, err := s.GetRecords(start, end, nil, 500, 0)
			return err
		}},
		{"performance/30d", func(s *UsageStore) error {
			start, end := dayRange(30)
			_, err := s.GetPerformanceSummary(start, end, nil)
			return err
		}},
		{"performance/30d (TTL cache hit)", func(s *UsageStore) error {
			s.perfCacheTTL = defaultPerfCacheTTL
			defer func() { s.perfCacheTTL = 0 }()
			start, end := dayRange(30)
			_, err := s.GetPerformanceSummary(start, end, nil)
			return err
		}},
	}
}

// dashboardLoad is the Summary-tab page load: stat cards + chart + agent
// filter options + performance panel.
func dashboardLoad(days int) func(*UsageStore) error {
	byName := map[string]func(*UsageStore) error{}
	for _, c := range dashboardCases() {
		byName[c.name] = c.run
	}
	names := []string{fmt.Sprintf("stats/model/%dd", days), fmt.Sprintf("timeseries/day/%dd", days), fmt.Sprintf("stats/scenario/%dd", 30), "performance/30d"}
	return func(s *UsageStore) error {
		for _, n := range names {
			if err := byName[n](s); err != nil {
				return err
			}
		}
		return nil
	}
}

func newSeededDashboardStore(tb testing.TB) (*UsageStore, *queryCounter) {
	tb.Helper()
	store := newUsageStoreForTest(tb)
	seedDashboardUsage(tb, store, dashboardRows())
	store.perfCacheTTL = 0 // measure the query, not the cache
	counter := &queryCounter{}
	store.db.Config.Logger = counter
	// Warm: builds usage_daily for every complete day (the one-off cold cost).
	for _, c := range dashboardCases() {
		if err := c.run(store); err != nil {
			tb.Fatalf("%s: %v", c.name, err)
		}
	}
	return store, counter
}

func BenchmarkDashboardRead(b *testing.B) {
	store, counter := newSeededDashboardStore(b)
	cases := append(dashboardCases(), dashCase{"PAGE/summary/30d", dashboardLoad(30)}, dashCase{"PAGE/summary/90d", dashboardLoad(90)})
	for _, c := range cases {
		b.Run(c.name, func(b *testing.B) {
			counter.reset()
			b.ResetTimer()
			for i := 0; i < b.N; i++ {
				if err := c.run(store); err != nil {
					b.Fatal(err)
				}
			}
			b.ReportMetric(float64(int64(counter.count()))/float64(b.N), "queries/op")
		})
	}
}

// TestDashboardReport prints median latency and query count per case;
// opt-in because it seeds a large table.
func TestDashboardReport(t *testing.T) {
	if os.Getenv("TB_DASH_REPORT") == "" {
		t.Skip("set TB_DASH_REPORT=1")
	}
	store := newUsageStoreForTest(t)
	seedDashboardUsage(t, store, dashboardRows())
	store.perfCacheTTL = 0 // measure the query, not the cache
	counter := &queryCounter{}
	store.db.Config.Logger = counter

	// Cold: the very first dashboard query after upgrade/restart builds
	// usage_daily for the whole window.
	counter.reset()
	start := time.Now()
	if err := dashboardCasesByName("stats/model/90d")(store); err != nil {
		t.Fatal(err)
	}
	t.Logf("COLD first stats/model/90d: %v, %d queries (rows=%d)", time.Since(start).Round(time.Millisecond), int64(counter.count()), dashboardRows())

	cases := append(dashboardCases(), dashCase{"PAGE/summary/30d", dashboardLoad(30)}, dashCase{"PAGE/summary/90d", dashboardLoad(90)})
	const runs = 7
	fmt.Printf("\n%-34s %12s %8s\n", "case", "median", "queries")
	for _, c := range cases {
		_ = c.run(store) // warm
		durs := make([]time.Duration, runs)
		counter.reset()
		for i := range durs {
			s := time.Now()
			if err := c.run(store); err != nil {
				t.Fatal(err)
			}
			durs[i] = time.Since(s)
		}
		sort.Slice(durs, func(i, j int) bool { return durs[i] < durs[j] })
		fmt.Printf("%-34s %12v %8d\n", c.name, durs[runs/2].Round(10*time.Microsecond), int64(counter.count())/runs)
	}
}

func dashboardCasesByName(name string) func(*UsageStore) error {
	for _, c := range dashboardCases() {
		if c.name == name {
			return c.run
		}
	}
	panic("no case " + name)
}
