package tracker

import (
	"context"
	"strings"
	"testing"

	"go.opentelemetry.io/otel/attribute"
	"go.opentelemetry.io/otel/sdk/metric"
	"go.opentelemetry.io/otel/sdk/metric/metricdata"
)

func newTestTracker(t *testing.T) (*TokenTracker, *metric.ManualReader) {
	t.Helper()
	reader := metric.NewManualReader()
	meterProvider := metric.NewMeterProvider(metric.WithReader(reader))
	tracker, err := NewTokenTracker(meterProvider.Meter("test"))
	if err != nil {
		t.Fatalf("Failed to create token tracker: %v", err)
	}
	return tracker, reader
}

func collect(t *testing.T, reader *metric.ManualReader) map[string]metricdata.Metrics {
	t.Helper()
	var rm metricdata.ResourceMetrics
	if err := reader.Collect(context.Background(), &rm); err != nil {
		t.Fatalf("Failed to collect metrics: %v", err)
	}
	out := map[string]metricdata.Metrics{}
	for _, sm := range rm.ScopeMetrics {
		for _, m := range sm.Metrics {
			out[m.Name] = m
		}
	}
	return out
}

func TestNewTokenTracker(t *testing.T) {
	tracker, _ := newTestTracker(t)
	if tracker.tokenUsage == nil {
		t.Error("tokenUsage histogram should be initialized")
	}
	if tracker.operationDuration == nil {
		t.Error("operationDuration histogram should be initialized")
	}
}

func TestRecordUsage_StandardMetricShape(t *testing.T) {
	tracker, reader := newTestTracker(t)

	tracker.RecordUsage(context.Background(), UsageOptions{
		Provider:         "openai",
		ProviderUUID:     "provider-123",
		Model:            "gpt-4-0613",
		RequestModel:     "gpt-4",
		RuleUUID:         "rule-456",
		Scenario:         "openai",
		InputTokens:      100,
		OutputTokens:     50,
		CacheInputTokens: 30,
		Streamed:         true,
		Status:           "success",
		LatencyMs:        250,
		UserTier:         "enterprise",
	})

	metrics := collect(t, reader)

	// Token usage: one histogram, split by gen_ai.token.type.
	tu, ok := metrics["gen_ai.client.token.usage"]
	if !ok {
		t.Fatalf("missing gen_ai.client.token.usage; got %v", keys(metrics))
	}
	tuHist, ok := tu.Data.(metricdata.Histogram[int64])
	if !ok {
		t.Fatalf("token.usage should be Histogram[int64], got %T", tu.Data)
	}
	byType := map[string]int64{}
	for _, dp := range tuHist.DataPoints {
		typ, _ := dp.Attributes.Value("gen_ai.token.type")
		byType[typ.AsString()] = dp.Sum

		if v, _ := dp.Attributes.Value("gen_ai.provider.name"); v.AsString() != "openai" {
			t.Errorf("gen_ai.provider.name = %q, want openai", v.AsString())
		}
		if v, _ := dp.Attributes.Value("gen_ai.request.model"); v.AsString() != "gpt-4" {
			t.Errorf("gen_ai.request.model = %q, want gpt-4", v.AsString())
		}
		if v, _ := dp.Attributes.Value("gen_ai.response.model"); v.AsString() != "gpt-4-0613" {
			t.Errorf("gen_ai.response.model = %q, want gpt-4-0613", v.AsString())
		}
		if v, _ := dp.Attributes.Value("gen_ai.operation.name"); v.AsString() != "chat" {
			t.Errorf("gen_ai.operation.name = %q, want chat (default)", v.AsString())
		}
		if v, _ := dp.Attributes.Value("tingly.scenario"); v.AsString() != "openai" {
			t.Errorf("tingly.scenario = %q, want openai", v.AsString())
		}
	}
	want := map[string]int64{"input": 100, "output": 50, "cache_read": 30}
	for typ, sum := range want {
		if byType[typ] != sum {
			t.Errorf("token.usage[%s] = %d, want %d", typ, byType[typ], sum)
		}
	}
	if _, ok := byType["system"]; ok {
		t.Error("system token type should be absent when SystemTokens is 0")
	}

	// Duration: seconds histogram, count doubles as request count, no
	// error.type on success.
	od, ok := metrics["gen_ai.client.operation.duration"]
	if !ok {
		t.Fatalf("missing gen_ai.client.operation.duration; got %v", keys(metrics))
	}
	odHist, ok := od.Data.(metricdata.Histogram[float64])
	if !ok {
		t.Fatalf("operation.duration should be Histogram[float64], got %T", od.Data)
	}
	if len(odHist.DataPoints) != 1 {
		t.Fatalf("expected 1 duration data point, got %d", len(odHist.DataPoints))
	}
	dp := odHist.DataPoints[0]
	if dp.Count != 1 {
		t.Errorf("duration count = %d, want 1 (doubles as request count)", dp.Count)
	}
	if dp.Sum != 0.25 {
		t.Errorf("duration sum = %v s, want 0.25 (250ms converted to seconds)", dp.Sum)
	}
	if _, ok := dp.Attributes.Value("error.type"); ok {
		t.Error("error.type must be absent on success")
	}

	if len(metrics) != 2 {
		t.Errorf("expected exactly 2 instruments, got %v", keys(metrics))
	}
}

func TestRecordUsage_ErrorType(t *testing.T) {
	tracker, reader := newTestTracker(t)
	ctx := context.Background()

	tracker.RecordUsage(ctx, UsageOptions{
		Provider: "anthropic", Model: "claude-3-opus", RequestModel: "claude-3",
		Scenario: "anthropic", Status: "error", ErrorCode: "rate_limit", LatencyMs: 50,
	})
	tracker.RecordUsage(ctx, UsageOptions{
		Provider: "google", Model: "gemini-pro", RequestModel: "gemini",
		Scenario: "openai", Status: "canceled", LatencyMs: 10,
	})

	od := collect(t, reader)["gen_ai.client.operation.duration"]
	hist := od.Data.(metricdata.Histogram[float64])
	got := map[string]bool{}
	for _, dp := range hist.DataPoints {
		v, ok := dp.Attributes.Value("error.type")
		if !ok {
			t.Error("error.type should be set on failed operations")
			continue
		}
		got[v.AsString()] = true
	}
	if !got["rate_limit"] {
		t.Error("error.type should carry the error code (rate_limit)")
	}
	if !got["canceled"] {
		t.Error("error.type should fall back to the status (canceled) when no code")
	}
}

func TestRecordUsage_ZeroTokens(t *testing.T) {
	tracker, reader := newTestTracker(t)

	// Zero tokens: duration (request count) still records, token usage doesn't.
	tracker.RecordUsage(context.Background(), UsageOptions{
		Provider: "openai", Model: "gpt-3.5-turbo", RequestModel: "gpt-3.5-turbo",
		Scenario: "openai", Status: "success", LatencyMs: 0,
	})

	metrics := collect(t, reader)
	if _, ok := metrics["gen_ai.client.token.usage"]; ok {
		t.Error("token.usage should have no data points when all token counts are 0")
	}
	od, ok := metrics["gen_ai.client.operation.duration"]
	if !ok {
		t.Fatal("operation.duration must record even for zero-token requests")
	}
	if hist := od.Data.(metricdata.Histogram[float64]); hist.DataPoints[0].Count != 1 {
		t.Error("duration count should be 1")
	}
}

func TestRecordUsage_MultipleRequests(t *testing.T) {
	tracker, reader := newTestTracker(t)
	ctx := context.Background()

	for i := 0; i < 5; i++ {
		tracker.RecordUsage(ctx, UsageOptions{
			Provider: "openai", Model: "gpt-4", RequestModel: "gpt-4",
			Scenario: "openai", InputTokens: 100, OutputTokens: 50,
			Status: "success", LatencyMs: 200,
		})
	}

	od := collect(t, reader)["gen_ai.client.operation.duration"]
	hist := od.Data.(metricdata.Histogram[float64])
	var count uint64
	for _, dp := range hist.DataPoints {
		count += dp.Count
	}
	if count != 5 {
		t.Errorf("total duration count = %d, want 5 (request count)", count)
	}
}

// TestRecordUsage_NoHighCardinalityAttributes guards the fix for #1255: the
// per-request latency value must not be a metric attribute (each unique value
// would permanently allocate a new data point per instrument), and the
// error.type attribute must be bounded in length.
func TestRecordUsage_NoHighCardinalityAttributes(t *testing.T) {
	tracker, reader := newTestTracker(t)

	longError := strings.Repeat("x", 4096)
	tracker.RecordUsage(context.Background(), UsageOptions{
		Provider: "openai", ProviderUUID: "provider-123",
		Model: "gpt-4", RequestModel: "gpt-4", Scenario: "openai",
		InputTokens: 100, OutputTokens: 50, Streamed: true,
		Status: "error", ErrorCode: longError, LatencyMs: 1234,
	})

	checkAttrs := func(metricName string, attrs attribute.Set) {
		iter := attrs.Iter()
		for iter.Next() {
			kv := iter.Attribute()
			if strings.Contains(string(kv.Key), "latency") {
				t.Errorf("%s: latency must not be a metric attribute (unbounded cardinality)", metricName)
			}
		}
		if v, ok := attrs.Value("error.type"); ok {
			if len(v.AsString()) > maxErrorTypeAttrLen {
				t.Errorf("%s: error.type exceeds %d chars (len=%d)", metricName, maxErrorTypeAttrLen, len(v.AsString()))
			}
		}
	}
	for name, m := range collect(t, reader) {
		switch data := m.Data.(type) {
		case metricdata.Histogram[int64]:
			for _, dp := range data.DataPoints {
				checkAttrs(name, dp.Attributes)
			}
		case metricdata.Histogram[float64]:
			for _, dp := range data.DataPoints {
				checkAttrs(name, dp.Attributes)
			}
		}
	}
}

func keys(m map[string]metricdata.Metrics) []string {
	out := make([]string, 0, len(m))
	for k := range m {
		out = append(out, k)
	}
	return out
}
