package db

import (
	"testing"
	"time"

	"github.com/stretchr/testify/require"
)

func TestRequestBillingSurvivesDailyAggregation(t *testing.T) {
	store, err := NewUsageStore(t.TempDir())
	require.NoError(t, err)
	defer store.Close()
	day := utcDayStart(time.Now()).Add(-6 * 24 * time.Hour)
	for i := 0; i < 5; i++ {
		record := &UsageRecord{ProviderUUID: "deepseek", ProviderName: "DeepSeek", Model: "deepseek-tools/search", Scenario: "webtools", Timestamp: day.Add(time.Duration(i)*24*time.Hour + time.Hour).In(time.Local), EventType: "requests", Currency: "CNY", UnitPrice: 0.005}
		if i < 3 {
			record.BillableRequests = 1
			record.RequestCostCNY = 0.005
		}
		if i == 4 {
			record.Status = "error"
		}
		require.NoError(t, store.RecordUsage(record))
	}
	legacy := &UsageRecord{ProviderUUID: "deepseek", ProviderName: "DeepSeek", Model: "chat", Timestamp: day.In(time.Local), InputTokens: 12, OutputTokens: 8, Streamed: true, LatencyMs: 120}
	require.NoError(t, store.RecordUsage(legacy))
	require.Equal(t, "tokens", legacy.EventType)
	query := UsageStatsQuery{EventType: "requests", GroupBy: "provider", StartTime: day.In(time.Local), EndTime: day.Add(5 * 24 * time.Hour).In(time.Local), Limit: 100}
	buckets, err := store.rawAggBuckets(query, true)
	raw := make([]AggregatedStat, 0, len(buckets))
	for _, bucket := range buckets {
		raw = append(raw, bucket.toAggregatedStat())
	}
	require.NoError(t, err)
	require.Len(t, raw, 1)
	daily, handled, err := store.aggregatedStatsFromRollups(query)
	require.NoError(t, err)
	require.True(t, handled)
	require.Len(t, daily, 1)
	for _, stats := range [][]AggregatedStat{raw, daily} {
		require.EqualValues(t, 3, stats[0].BillableRequests)
		require.InDelta(t, 0.015, stats[0].RequestCostCNY, 0.0000001)
		require.Zero(t, stats[0].TotalTokens, "tool billing must not include model tokens")
		require.EqualValues(t, 5, stats[0].RequestCount)
	}
	// Default model stats must not include any tool traffic.
	modelQuery := query
	modelQuery.EventType = ""
	modelStats, err := store.GetAggregatedStats(modelQuery)
	require.NoError(t, err)
	require.Len(t, modelStats, 1)
	require.EqualValues(t, 1, modelStats[0].RequestCount)
	require.EqualValues(t, 12, modelStats[0].AvgInputTokens)
	require.EqualValues(t, 8, modelStats[0].AvgOutputTokens)
	require.EqualValues(t, 120, modelStats[0].AvgLatencyMs)
	require.EqualValues(t, 1, modelStats[0].StreamedRate)
	require.Zero(t, modelStats[0].ErrorCount)
	require.Zero(t, modelStats[0].RequestCostCNY)
	modelSeries, err := store.GetTimeSeries("hour", query.StartTime, query.EndTime, nil)
	require.NoError(t, err)
	require.Len(t, modelSeries, 1)
	require.EqualValues(t, 1, modelSeries[0].RequestCount)
	performance, err := store.GetPerformanceSummary(query.StartTime, query.EndTime, nil)
	require.NoError(t, err)
	require.EqualValues(t, 1, performance.Completion.SampleCount)
	// Exercise both partial raw edge days and the pre-aggregated middle days.
	edge := query
	edge.StartTime = day.Add(30 * time.Minute).In(time.Local)
	edge.EndTime = day.Add(4*24*time.Hour + 2*time.Hour).In(time.Local)
	edgeDaily, handled, err := store.aggregatedStatsFromRollups(edge)
	require.NoError(t, err)
	require.True(t, handled)
	require.Len(t, edgeDaily, 1)
	require.EqualValues(t, 3, edgeDaily[0].BillableRequests)
	require.InDelta(t, 0.015, edgeDaily[0].RequestCostCNY, 0.0000001)
	require.Zero(t, edgeDaily[0].TotalTokens)

	series, err := store.GetTimeSeries("day", query.StartTime, query.EndTime, map[string]string{"event_type": "requests"})
	require.NoError(t, err)
	var cost float64
	var count int64
	for _, point := range series {
		cost += point.RequestCostCNY
		count += point.BillableRequests
	}
	require.InDelta(t, 0.015, cost, 0.0000001)
	require.EqualValues(t, 3, count)
	// The current rollup architecture also serves hourly and scenario-only
	// queries; request billing must survive both tiers and raw edge merges.
	for _, interval := range []string{"hour", "day"} {
		series, err := store.GetTimeSeries(interval, edge.StartTime, edge.EndTime, map[string]string{"scenario": "webtools", "event_type": "requests"})
		require.NoError(t, err)
		var cost float64
		var count int64
		for _, point := range series {
			cost += point.RequestCostCNY
			count += point.BillableRequests
		}
		require.InDelta(t, 0.015, cost, 0.0000001, interval)
		require.EqualValues(t, 3, count, interval)
	}
	scenarioQuery := edge
	scenarioQuery.GroupBy = "scenario"
	scenarioQuery.Scenario = "webtools"
	scenarios, handled, err := store.aggregatedStatsFromRollups(scenarioQuery)
	require.NoError(t, err)
	require.True(t, handled)
	require.Len(t, scenarios, 1)
	require.EqualValues(t, 3, scenarios[0].BillableRequests)
	require.InDelta(t, 0.015, scenarios[0].RequestCostCNY, 0.0000001)
	records, total, err := store.GetRecords(query.StartTime, query.EndTime, map[string]string{"model": "deepseek-tools/search"}, 100, 0)
	require.NoError(t, err)
	require.EqualValues(t, 5, total)
	for _, record := range records {
		require.Equal(t, "requests", record.EventType)
		require.Equal(t, "CNY", record.Currency)
		require.Equal(t, 0.005, record.UnitPrice)
	}
}

func TestRequestBillingRollupMigrationSeparatesSameDimensions(t *testing.T) {
	dir := t.TempDir()
	store, err := NewUsageStore(dir)
	require.NoError(t, err)
	day := utcDayStart(time.Now()).Add(-5 * 24 * time.Hour).In(time.Local)
	for _, event := range []string{"tokens", "requests"} {
		record := &UsageRecord{ProviderUUID: "p", ProviderName: "Provider", Model: "shared-model", UserID: "u", Scenario: "same", Timestamp: day.Add(time.Hour), EventType: event, LatencyMs: 10}
		if event == "tokens" {
			record.InputTokens = 12
			record.OutputTokens = 8
			record.Streamed = true
		} else {
			record.BillableRequests = 1
			record.RequestCostCNY = .005
			record.LatencyMs = 1000
		}
		require.NoError(t, store.RecordUsage(record))
	}
	// Simulate pre-event-dimension cached rollups. Raw events must survive.
	for _, tier := range []*rollupTier{hourTier, dayTier} {
		for _, table := range []string{tier.table, tier.scenarioTable} {
			require.NoError(t, store.db.Exec("DROP TABLE "+table).Error)
			require.NoError(t, store.db.Exec("CREATE TABLE "+table+" ("+tier.keyCol+" TEXT, request_count INTEGER)").Error)
			require.NoError(t, store.db.Exec("INSERT INTO "+table+" VALUES (?, 2)", tier.key(day)).Error)
		}
	}
	require.NoError(t, store.Close())
	store, err = NewUsageStore(dir)
	require.NoError(t, err)
	defer store.Close()
	for _, event := range []string{"tokens", "requests"} {
		for _, group := range []string{"model", "provider", "scenario", "daily", "hourly"} {
			q := UsageStatsQuery{EventType: event, GroupBy: group, StartTime: day, EndTime: day.Add(4 * 24 * time.Hour), Limit: 100}
			stats, handled, err := store.aggregatedStatsFromRollups(q)
			require.NoError(t, err)
			require.True(t, handled)
			require.Len(t, stats, 1)
			require.EqualValues(t, 1, stats[0].RequestCount)
			if event == "tokens" {
				require.EqualValues(t, 20, stats[0].TotalTokens)
				require.Zero(t, stats[0].RequestCostCNY)
			} else {
				require.Zero(t, stats[0].TotalTokens)
				require.InDelta(t, .005, stats[0].RequestCostCNY, 1e-9)
			}
		}
	}
}
