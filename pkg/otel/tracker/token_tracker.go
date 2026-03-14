package tracker

import (
	"context"

	"go.opentelemetry.io/otel/attribute"
	"go.opentelemetry.io/otel/metric"
)

// Attribute keys for token usage tracking
// Note: Attribute names follow the original internal/obs/otel convention for compatibility
var (
	attrLLMProvider       = attribute.Key("llm.provider")
	attrLLMProviderUUID   = attribute.Key("llm.provider.uuid")
	attrLLMModel          = attribute.Key("llm.model")
	attrLLMRequestModel   = attribute.Key("llm.request.model")
	attrLLMTokenType      = attribute.Key("llm.token_type") // Underscore for backward compatibility
	attrLLMScenario       = attribute.Key("llm.scenario")
	attrLLMStreaming      = attribute.Key("llm.streaming")
	attrLLMResponseStatus = attribute.Key("llm.response.status")
	attrLLMErrorCode      = attribute.Key("llm.error.code")
	attrLLMRuleUUID       = attribute.Key("llm.rule.uuid")
	attrLLMUserTier       = attribute.Key("llm.user.tier")
	attrLLMLatencyMs      = attribute.Key("llm.latency.ms")
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

	// InputTokens is the number of input/prompt tokens consumed (excluding cache)
	InputTokens int

	// OutputTokens is the number of output/completion tokens consumed
	OutputTokens int

	// CacheInputTokens is the number of cache-related tokens consumed
	CacheInputTokens int

	// SystemTokens represents tokens consumed by system-level operations
	SystemTokens int

	// Streamed indicates whether this was a streaming request
	Streamed bool

	// Status is the request status - "success", "error", or "canceled"
	Status string

	// ErrorCode is the error code if status is not "success"
	ErrorCode string

	// LatencyMs is the request processing time in milliseconds
	LatencyMs int

	// UserTier is a low-cardinality class for enterprise observability.
	UserTier string
}

// TokenTracker provides a unified interface for tracking token usage
// using OpenTelemetry metrics.
type TokenTracker struct {
	inputTokens      metric.Int64Counter
	outputTokens     metric.Int64Counter
	totalTokens      metric.Int64Counter
	cacheInputTokens metric.Int64Counter
	systemTokens     metric.Int64Counter
	requestCount     metric.Int64Counter
	requestDuration  metric.Float64Histogram
	requestError     metric.Int64Counter
}

// NewTokenTracker creates a new TokenTracker with the provided meter.
func NewTokenTracker(meter metric.Meter) (*TokenTracker, error) {
	tt := &TokenTracker{}

	var err error

	// Token usage counters - input tokens
	tt.inputTokens, err = meter.Int64Counter(
		"llm.token.usage.input",
		metric.WithDescription("LLM input/prompt token usage"),
		metric.WithUnit("{token}"),
	)
	if err != nil {
		return nil, err
	}

	// Token usage counters - output tokens
	tt.outputTokens, err = meter.Int64Counter(
		"llm.token.usage.output",
		metric.WithDescription("LLM output/completion token usage"),
		metric.WithUnit("{token}"),
	)
	if err != nil {
		return nil, err
	}

	// Total tokens counter
	tt.totalTokens, err = meter.Int64Counter(
		"llm.token.total",
		metric.WithDescription("Total LLM tokens consumed (input + output)"),
		metric.WithUnit("{token}"),
	)
	if err != nil {
		return nil, err
	}

	// Cache token counters
	tt.cacheInputTokens, err = meter.Int64Counter(
		"llm.token.cache.input",
		metric.WithDescription("LLM cache-related input token usage"),
		metric.WithUnit("{token}"),
	)
	if err != nil {
		return nil, err
	}

	// System tokens counter
	tt.systemTokens, err = meter.Int64Counter(
		"llm.token.system",
		metric.WithDescription("LLM tokens consumed by system operations"),
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
		attrLLMProvider.String(opts.Provider),
		attrLLMProviderUUID.String(opts.ProviderUUID),
		attrLLMModel.String(opts.Model),
		attrLLMRequestModel.String(opts.RequestModel),
		attrLLMScenario.String(opts.Scenario),
		attrLLMStreaming.Bool(opts.Streamed),
		attrLLMResponseStatus.String(opts.Status),
	}

	if opts.RuleUUID != "" {
		commonAttrs = append(commonAttrs, attrLLMRuleUUID.String(opts.RuleUUID))
	}
	if opts.UserTier != "" {
		commonAttrs = append(commonAttrs, attrLLMUserTier.String(opts.UserTier))
	}
	if opts.ErrorCode != "" {
		commonAttrs = append(commonAttrs, attrLLMErrorCode.String(opts.ErrorCode))
	}
	if opts.LatencyMs > 0 {
		commonAttrs = append(commonAttrs, attrLLMLatencyMs.Int(opts.LatencyMs))
	}

	// Record input tokens
	if opts.InputTokens > 0 {
		tt.inputTokens.Add(ctx, int64(opts.InputTokens), metric.WithAttributes(commonAttrs...))
	}

	// Record output tokens
	if opts.OutputTokens > 0 {
		tt.outputTokens.Add(ctx, int64(opts.OutputTokens), metric.WithAttributes(commonAttrs...))
	}

	// Record total tokens
	totalTokens := opts.InputTokens + opts.OutputTokens
	if totalTokens > 0 {
		tt.totalTokens.Add(ctx, int64(totalTokens), metric.WithAttributes(commonAttrs...))
	}

	// Record cache tokens
	if opts.CacheInputTokens > 0 {
		cacheAttrs := append(commonAttrs, attrLLMTokenType.String("cache"))
		tt.cacheInputTokens.Add(ctx, int64(opts.CacheInputTokens), metric.WithAttributes(cacheAttrs...))
	}

	// Record system tokens
	if opts.SystemTokens > 0 {
		systemAttrs := append(commonAttrs, attrLLMTokenType.String("system"))
		tt.systemTokens.Add(ctx, int64(opts.SystemTokens), metric.WithAttributes(systemAttrs...))
	}

	// Record request count
	tt.requestCount.Add(ctx, 1, metric.WithAttributes(commonAttrs...))

	// Record request duration
	if opts.LatencyMs > 0 {
		tt.requestDuration.Record(ctx, float64(opts.LatencyMs), metric.WithAttributes(commonAttrs...))
	}

	// Record error if status is "error"
	if opts.Status == "error" {
		tt.requestError.Add(ctx, 1, metric.WithAttributes(commonAttrs...))
	}
}
