package tracker

import (
	"context"
	"testing"

	"go.opentelemetry.io/otel/sdk/metric"
	"go.opentelemetry.io/otel/sdk/metric/metricdata"
)

func TestNewTokenTracker(t *testing.T) {
	// Create a meter provider for testing
	meterProvider := metric.NewMeterProvider()
	meter := meterProvider.Meter("test")

	// Create token tracker
	tracker, err := NewTokenTracker(meter)
	if err != nil {
		t.Fatalf("Failed to create token tracker: %v", err)
	}

	if tracker == nil {
		t.Fatal("Token tracker should not be nil")
	}

	if tracker.inputTokens == nil {
		t.Error("inputTokens counter should be initialized")
	}

	if tracker.outputTokens == nil {
		t.Error("outputTokens counter should be initialized")
	}

	if tracker.totalTokens == nil {
		t.Error("totalTokens counter should be initialized")
	}

	if tracker.requestCount == nil {
		t.Error("requestCount counter should be initialized")
	}

	if tracker.requestDuration == nil {
		t.Error("requestDuration histogram should be initialized")
	}

	if tracker.requestError == nil {
		t.Error("requestError counter should be initialized")
	}
}

func TestRecordUsage_Success(t *testing.T) {
	// Create a meter provider with reader for testing
	reader := metric.NewManualReader()
	meterProvider := metric.NewMeterProvider(metric.WithReader(reader))
	meter := meterProvider.Meter("test")

	tracker, err := NewTokenTracker(meter)
	if err != nil {
		t.Fatalf("Failed to create token tracker: %v", err)
	}

	// Record usage
	opts := UsageOptions{
		Provider:     "openai",
		ProviderUUID: "provider-123",
		Model:        "gpt-4",
		RequestModel: "gpt-4",
		RuleUUID:     "rule-456",
		Scenario:     "openai",
		InputTokens:  100,
		OutputTokens: 50,
		Streamed:     true,
		Status:       "success",
		LatencyMs:    250,
		UserTier:     "enterprise",
	}

	ctx := context.Background()
	tracker.RecordUsage(ctx, opts)

	// Verify metrics were recorded
	var rm metricdata.ResourceMetrics
	if err := reader.Collect(ctx, &rm); err != nil {
		t.Fatalf("Failed to collect metrics: %v", err)
	}

	// Check that we have scope metrics
	if len(rm.ScopeMetrics) == 0 {
		t.Fatal("No scope metrics recorded")
	}
}

func TestRecordUsage_WithError(t *testing.T) {
	reader := metric.NewManualReader()
	meterProvider := metric.NewMeterProvider(metric.WithReader(reader))
	meter := meterProvider.Meter("test")

	tracker, err := NewTokenTracker(meter)
	if err != nil {
		t.Fatalf("Failed to create token tracker: %v", err)
	}

	opts := UsageOptions{
		Provider:     "anthropic",
		ProviderUUID: "provider-789",
		Model:        "claude-3-opus",
		RequestModel: "claude-3",
		Scenario:     "anthropic",
		InputTokens:  200,
		OutputTokens: 100,
		Streamed:     false,
		Status:       "error",
		ErrorCode:    "rate_limit",
		LatencyMs:    50,
	}

	ctx := context.Background()
	tracker.RecordUsage(ctx, opts)

	// Verify metrics were recorded
	var rm metricdata.ResourceMetrics
	if err := reader.Collect(ctx, &rm); err != nil {
		t.Fatalf("Failed to collect metrics: %v", err)
	}
}

func TestRecordUsage_WithCanceledStatus(t *testing.T) {
	reader := metric.NewManualReader()
	meterProvider := metric.NewMeterProvider(metric.WithReader(reader))
	meter := meterProvider.Meter("test")

	tracker, err := NewTokenTracker(meter)
	if err != nil {
		t.Fatalf("Failed to create token tracker: %v", err)
	}

	opts := UsageOptions{
		Provider:     "google",
		ProviderUUID: "provider-abc",
		Model:        "gemini-pro",
		RequestModel: "gemini",
		Scenario:     "openai",
		InputTokens:  50,
		OutputTokens: 0,
		Streamed:     true,
		Status:       "canceled",
		LatencyMs:    10,
	}

	ctx := context.Background()
	tracker.RecordUsage(ctx, opts)

	var rm metricdata.ResourceMetrics
	if err := reader.Collect(ctx, &rm); err != nil {
		t.Fatalf("Failed to collect metrics: %v", err)
	}
}

func TestRecordUsage_ZeroTokens(t *testing.T) {
	reader := metric.NewManualReader()
	meterProvider := metric.NewMeterProvider(metric.WithReader(reader))
	meter := meterProvider.Meter("test")

	tracker, err := NewTokenTracker(meter)
	if err != nil {
		t.Fatalf("Failed to create token tracker: %v", err)
	}

	// Record with zero tokens - should still record request count
	opts := UsageOptions{
		Provider:     "openai",
		ProviderUUID: "provider-xyz",
		Model:        "gpt-3.5-turbo",
		RequestModel: "gpt-3.5-turbo",
		Scenario:     "openai",
		InputTokens:  0,
		OutputTokens: 0,
		Streamed:     false,
		Status:       "success",
		LatencyMs:    0,
	}

	ctx := context.Background()
	tracker.RecordUsage(ctx, opts)

	var rm metricdata.ResourceMetrics
	if err := reader.Collect(ctx, &rm); err != nil {
		t.Fatalf("Failed to collect metrics: %v", err)
	}
}

func TestRecordUsage_MultipleRequests(t *testing.T) {
	reader := metric.NewManualReader()
	meterProvider := metric.NewMeterProvider(metric.WithReader(reader))
	meter := meterProvider.Meter("test")

	tracker, err := NewTokenTracker(meter)
	if err != nil {
		t.Fatalf("Failed to create token tracker: %v", err)
	}

	ctx := context.Background()

	// Record multiple requests
	for i := 0; i < 5; i++ {
		opts := UsageOptions{
			Provider:     "openai",
			ProviderUUID: "provider-123",
			Model:        "gpt-4",
			RequestModel: "gpt-4",
			Scenario:     "openai",
			InputTokens:  100 * (i + 1),
			OutputTokens: 50 * (i + 1),
			Streamed:     i%2 == 0,
			Status:       "success",
			LatencyMs:    200 * (i + 1),
		}
		tracker.RecordUsage(ctx, opts)
	}

	var rm metricdata.ResourceMetrics
	if err := reader.Collect(ctx, &rm); err != nil {
		t.Fatalf("Failed to collect metrics: %v", err)
	}
}

func TestUsageOptions_AllFields(t *testing.T) {
	// Verify all fields in UsageOptions are properly defined
	opts := UsageOptions{
		Provider:     "test-provider",
		ProviderUUID: "test-uuid",
		Model:        "test-model",
		RequestModel: "test-request-model",
		RuleUUID:     "test-rule",
		Scenario:     "test-scenario",
		InputTokens:  100,
		OutputTokens: 50,
		Streamed:     true,
		Status:       "success",
		ErrorCode:    "",
		LatencyMs:    250,
		UserTier:     "enterprise",
	}

	if opts.Provider != "test-provider" {
		t.Errorf("Provider mismatch: %s", opts.Provider)
	}

	if opts.InputTokens != 100 {
		t.Errorf("InputTokens mismatch: %d", opts.InputTokens)
	}

	if opts.OutputTokens != 50 {
		t.Errorf("OutputTokens mismatch: %d", opts.OutputTokens)
	}

	if opts.Streamed != true {
		t.Error("Streamed should be true")
	}
}
