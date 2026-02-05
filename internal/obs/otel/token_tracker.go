package otel

import (
	"context"

	"go.opentelemetry.io/otel/attribute"
	"go.opentelemetry.io/otel/metric"
)

// UsageOptions contains the options for recording token usage.
type UsageOptions struct {
	// Provider is the name of the LLM provider (e.g., "openai", "anthropic")
	Provider string

	// ProviderUUID is the unique identifier of the provider
	ProviderUUID string

	// Model is the actual model used (not the requested model)
	Model string

	// RequestModel is the original model name requested by the user
	RequestModel string

	// RuleUUID is the load balancer rule UUID
	RuleUUID string

	// Scenario is the API scenario (e.g., "openai", "anthropic", "claude_code")
	Scenario string

	// InputTokens is the number of input/prompt tokens consumed
	InputTokens int

	// OutputTokens is the number of output/completion tokens consumed
	OutputTokens int

	// Streamed indicates whether this was a streaming request
	Streamed bool

	// Status is the request status - "success", "error", or "canceled"
	Status string

	// ErrorCode is the error code if status is not "success"
	ErrorCode string

	// LatencyMs is the request processing time in milliseconds
	LatencyMs int
}

// TokenTracker provides a unified interface for tracking token usage
// using OpenTelemetry metrics.
type TokenTracker struct {
	inputTokens     metric.Int64Counter
	outputTokens    metric.Int64Counter
	totalTokens     metric.Int64Counter
	requestCount    metric.Int64Counter
	requestDuration metric.Float64Histogram
	requestError    metric.Int64Counter
}

// NewTokenTracker creates a new TokenTracker with the provided meter.
func NewTokenTracker(meter metric.Meter) (*TokenTracker, error) {
	tt := &TokenTracker{}

	var err error

	// Token usage counters
	tt.inputTokens, err = meter.Int64Counter(
		"llm.token.usage",
		metric.WithDescription("LLM token usage by type (input/output)"),
		metric.WithUnit("{token}"),
	)
	if err != nil {
		return nil, err
	}

	tt.outputTokens, err = meter.Int64Counter(
		"llm.token.usage",
		metric.WithDescription("LLM token usage by type (input/output)"),
		metric.WithUnit("{token}"),
	)
	if err != nil {
		return nil, err
	}

	tt.totalTokens, err = meter.Int64Counter(
		"llm.token.total",
		metric.WithDescription("Total LLM tokens consumed (input + output)"),
		metric.WithUnit("{token}"),
	)
	if err != nil {
		return nil, err
	}

	// Request counter
	tt.requestCount, err = meter.Int64Counter(
		"llm.request.count",
		metric.WithDescription("Number of LLM requests"),
		metric.WithUnit("{request}"),
	)
	if err != nil {
		return nil, err
	}

	// Request duration histogram
	tt.requestDuration, err = meter.Float64Histogram(
		"llm.request.duration",
		metric.WithDescription("LLM request duration in milliseconds"),
		metric.WithUnit("ms"),
	)
	if err != nil {
		return nil, err
	}

	// Error counter
	tt.requestError, err = meter.Int64Counter(
		"llm.request.errors",
		metric.WithDescription("Number of LLM request errors"),
		metric.WithUnit("{error}"),
	)
	if err != nil {
		return nil, err
	}

	return tt, nil
}

// RecordUsage records token usage with the provided options.
func (tt *TokenTracker) RecordUsage(ctx context.Context, opts UsageOptions) {
	// Build common attributes
	commonAttrs := []attribute.KeyValue{
		AttrLLMProvider.String(opts.Provider),
		AttrLLMProviderUUID.String(opts.ProviderUUID),
		AttrLLMModel.String(opts.Model),
		AttrLLMRequestModel.String(opts.RequestModel),
		AttrLLMScenario.String(opts.Scenario),
		AttrLLMStreaming.Bool(opts.Streamed),
		AttrLLMResponseStatus.String(opts.Status),
	}

	if opts.RuleUUID != "" {
		commonAttrs = append(commonAttrs, AttrLLMRuleUUID.String(opts.RuleUUID))
	}

	if opts.ErrorCode != "" {
		commonAttrs = append(commonAttrs, AttrLLMErrorCode.String(opts.ErrorCode))
	}

	// Record input tokens
	if opts.InputTokens > 0 {
		inputAttrs := append(commonAttrs, AttrLLMTokenType.String("input"))
		tt.inputTokens.Add(ctx, int64(opts.InputTokens), metric.WithAttributes(inputAttrs...))
	}

	// Record output tokens
	if opts.OutputTokens > 0 {
		outputAttrs := append(commonAttrs, AttrLLMTokenType.String("output"))
		tt.outputTokens.Add(ctx, int64(opts.OutputTokens), metric.WithAttributes(outputAttrs...))
	}

	// Record total tokens
	totalTokens := opts.InputTokens + opts.OutputTokens
	if totalTokens > 0 {
		tt.totalTokens.Add(ctx, int64(totalTokens), metric.WithAttributes(commonAttrs...))
	}

	// Record request count
	tt.requestCount.Add(ctx, 1, metric.WithAttributes(commonAttrs...))

	// Record request duration
	if opts.LatencyMs > 0 {
		tt.requestDuration.Record(ctx, float64(opts.LatencyMs), metric.WithAttributes(commonAttrs...))
	}

	// Record error if status is not success
	if opts.Status == "error" {
		tt.requestError.Add(ctx, 1, metric.WithAttributes(commonAttrs...))
	}
}
