package tracker

import (
	"context"
	"slices"
	"strings"

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
)

// maxErrorCodeAttrLen caps the llm.error.code attribute value. Every distinct
// attribute set becomes a data point the cumulative metrics SDK retains for
// the lifetime of the process, so unbounded error strings (which may embed
// upstream response bodies) would leak memory one timeseries at a time.
// Callers should already pass a bounded classification; this is a guard.
const maxErrorCodeAttrLen = 64

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
	// Build common attributes.
	//
	// Attribute values are retained for the lifetime of the process by the
	// cumulative metrics SDK. Values that originate from the parsed request
	// body — model, request model, and error text — can be substrings ALIASING
	// the multi-megabyte gjson copy of the entire body (SDK JSON decoding
	// slices rather than copies), so a retained attribute would pin that whole
	// buffer forever (the #1255 leak: ~0.8MB pinned per request, attributed to
	// gjson.ParseBytes in heap profiles). strings.Clone detaches exactly those
	// values; the remaining attributes are config- or enum-owned strings that
	// cannot alias a request buffer.
	commonAttrs := []attribute.KeyValue{
		attrLLMProvider.String(opts.Provider),
		attrLLMProviderUUID.String(opts.ProviderUUID),
		attrLLMModel.String(strings.Clone(opts.Model)),
		attrLLMRequestModel.String(strings.Clone(opts.RequestModel)),
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
		code := opts.ErrorCode
		if len(code) > maxErrorCodeAttrLen {
			// The truncated slice still aliases the original backing array, so
			// the clone below is what actually bounds retained memory.
			code = code[:maxErrorCodeAttrLen]
		}
		commonAttrs = append(commonAttrs, attrLLMErrorCode.String(strings.Clone(code)))
	}
	// NOTE: latency is deliberately NOT an attribute. It is near-unique per
	// request, so every request would permanently allocate a new data point
	// (and pin its attribute strings, see above) on every instrument below.
	// This exact line was bisected as the #1255 leak: with it, the tb2→tb1
	// e2e retains 823KB/request forever; without it, 0.5KB/request.
	// Latency is recorded as the requestDuration histogram VALUE instead.

	// Build the attribute set once and reuse it for every instrument below;
	// metric.WithAttributes would re-sort and re-deduplicate the same slice
	// on each call. NewSet may reorder commonAttrs, which is fine — sets are
	// order-insensitive and the values are unchanged.
	commonOpt := metric.WithAttributeSet(attribute.NewSet(commonAttrs...))

	// Record input tokens
	if opts.InputTokens > 0 {
		tt.inputTokens.Add(ctx, int64(opts.InputTokens), commonOpt)
	}

	// Record output tokens
	if opts.OutputTokens > 0 {
		tt.outputTokens.Add(ctx, int64(opts.OutputTokens), commonOpt)
	}

	// Record total tokens
	totalTokens := opts.InputTokens + opts.OutputTokens
	if totalTokens > 0 {
		tt.totalTokens.Add(ctx, int64(totalTokens), commonOpt)
	}

	// Record cache tokens. slices.Clone before append: two appends off the
	// same commonAttrs base would otherwise share its backing array and the
	// second could overwrite the first's token_type element.
	if opts.CacheInputTokens > 0 {
		cacheAttrs := append(slices.Clone(commonAttrs), attrLLMTokenType.String("cache"))
		tt.cacheInputTokens.Add(ctx, int64(opts.CacheInputTokens), metric.WithAttributeSet(attribute.NewSet(cacheAttrs...)))
	}

	// Record system tokens
	if opts.SystemTokens > 0 {
		systemAttrs := append(slices.Clone(commonAttrs), attrLLMTokenType.String("system"))
		tt.systemTokens.Add(ctx, int64(opts.SystemTokens), metric.WithAttributeSet(attribute.NewSet(systemAttrs...)))
	}

	// Record request count
	tt.requestCount.Add(ctx, 1, commonOpt)

	// Record request duration
	if opts.LatencyMs > 0 {
		tt.requestDuration.Record(ctx, float64(opts.LatencyMs), commonOpt)
	}

	// Record error if status is "error"
	if opts.Status == "error" {
		tt.requestError.Add(ctx, 1, commonOpt)
	}
}
