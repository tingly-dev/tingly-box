package db

import (
	"github.com/stretchr/testify/require"
	"testing"
	"time"
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
	legacy := &UsageRecord{ProviderUUID: "deepseek", ProviderName: "DeepSeek", Model: "chat", Timestamp: day.In(time.Local), InputTokens: 12, OutputTokens: 8}
	require.NoError(t, store.RecordUsage(legacy))
	require.Equal(t, "tokens", legacy.EventType)
	query := UsageStatsQuery{GroupBy: "provider", StartTime: day.In(time.Local), EndTime: day.Add(5 * 24 * time.Hour).In(time.Local), Limit: 100}
	buckets, err := store.rawAggBuckets(query, true)
	raw := make([]AggregatedStat, 0, len(buckets))
	for _, bucket := range buckets {
		raw = append(raw, bucket.toAggregatedStat())
	}
	require.NoError(t, err)
	require.Len(t, raw, 1)
	daily, handled, err := store.aggregatedStatsFromDaily(query)
	require.NoError(t, err)
	require.True(t, handled)
	require.Len(t, daily, 1)
	for _, stats := range [][]AggregatedStat{raw, daily} {
		require.EqualValues(t, 3, stats[0].BillableRequests)
		require.InDelta(t, 0.015, stats[0].RequestCostCNY, 0.0000001)
		require.EqualValues(t, 20, stats[0].TotalTokens, "request billing must not invent tokens")
		require.EqualValues(t, 6, stats[0].RequestCount)
	}
	// Exercise both partial raw edge days and the pre-aggregated middle days.
	edge := query
	edge.StartTime = day.Add(30 * time.Minute).In(time.Local)
	edge.EndTime = day.Add(4*24*time.Hour + 2*time.Hour).In(time.Local)
	edgeDaily, handled, err := store.aggregatedStatsFromDaily(edge)
	require.NoError(t, err)
	require.True(t, handled)
	require.Len(t, edgeDaily, 1)
	require.EqualValues(t, 3, edgeDaily[0].BillableRequests)
	require.InDelta(t, 0.015, edgeDaily[0].RequestCostCNY, 0.0000001)
	require.Zero(t, edgeDaily[0].TotalTokens)

	series, err := store.GetTimeSeries("day", query.StartTime, query.EndTime, nil)
	require.NoError(t, err)
	var cost float64
	var count int64
	for _, point := range series {
		cost += point.RequestCostCNY
		count += point.BillableRequests
	}
	require.InDelta(t, 0.015, cost, 0.0000001)
	require.EqualValues(t, 3, count)
	records, total, err := store.GetRecords(query.StartTime, query.EndTime, map[string]string{"model": "deepseek-tools/search"}, 100, 0)
	require.NoError(t, err)
	require.EqualValues(t, 5, total)
	for _, record := range records {
		require.Equal(t, "requests", record.EventType)
		require.Equal(t, "CNY", record.Currency)
		require.Equal(t, 0.005, record.UnitPrice)
	}
}
