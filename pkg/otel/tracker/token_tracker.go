// Package tracker records LLM token usage and request duration as
// OpenTelemetry metrics following the GenAI semantic conventions
// (https://github.com/open-telemetry/semantic-conventions-genai):
//
//   - gen_ai.client.token.usage       histogram {token}, split by gen_ai.token.type
//   - gen_ai.client.operation.duration histogram s, errors via error.type
//
// Request counts and error counts are intentionally NOT separate counters:
// the duration histogram's count is the request count, and error.type on it
// classifies failures — that is the standard shape.
package tracker

import (
	"context"
	"strings"

	"go.opentelemetry.io/otel/attribute"
	"go.opentelemetry.io/otel/metric"
)

// Metric attribute keys. Standard gen_ai.* / error.type keys plus the
// gateway's own tingly.* dimensions (kept in a separate namespace).
var (
	attrOperationName = attribute.Key("gen_ai.operation.name")
	attrProviderName  = attribute.Key("gen_ai.provider.name")
	attrRequestModel  = attribute.Key("gen_ai.request.model")
	attrResponseModel = attribute.Key("gen_ai.response.model")
	attrTokenType     = attribute.Key("gen_ai.token.type")
	attrErrorType     = attribute.Key("error.type")

	attrScenario     = attribute.Key("tingly.scenario")
	attrProviderUUID = attribute.Key("tingly.provider.uuid")
	attrRuleUUID     = attribute.Key("tingly.rule.uuid")
	attrStreaming    = attribute.Key("tingly.streaming")
	attrUserTier     = attribute.Key("tingly.user.tier")
)

// gen_ai.token.type values. "input"/"output" are spec-defined; the enum is
// open, so the gateway extends it for cache and system token accounting.
const (
	tokenTypeInput     = "input"
	tokenTypeOutput    = "output"
	tokenTypeCacheRead = "cache_read"
	tokenTypeSystem    = "system"
)

// maxErrorTypeAttrLen caps the error.type attribute value. Every distinct
// attribute set becomes a data point the cumulative metrics SDK retains for
// the lifetime of the process, so unbounded error strings (which may embed
// upstream response bodies) would leak memory one timeseries at a time.
// Callers should already pass a bounded classification; this is a guard.
const maxErrorTypeAttrLen = 64

// Spec-advised histogram bucket boundaries.
var (
	durationBoundaries = []float64{0.01, 0.02, 0.04, 0.08, 0.16, 0.32, 0.64, 1.28, 2.56, 5.12, 10.24, 20.48, 40.96, 81.92}
	tokenBoundaries    = []float64{1, 4, 16, 64, 256, 1024, 4096, 16384, 65536, 262144, 1048576, 4194304}
)

// UsageOptions contains the options for recording token usage.
type UsageOptions struct {
	// Operation is the gen_ai.operation.name ("chat", "embeddings", ...).
	// Defaults to "chat" when empty.
	Operation string

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

// TokenTracker records token usage and operation duration using the
// OpenTelemetry GenAI client metrics.
type TokenTracker struct {
	tokenUsage        metric.Int64Histogram
	operationDuration metric.Float64Histogram
}

// NewTokenTracker creates a new TokenTracker with the provided meter.
func NewTokenTracker(meter metric.Meter) (*TokenTracker, error) {
	tt := &TokenTracker{}

	var err error

	tt.tokenUsage, err = meter.Int64Histogram(
		"gen_ai.client.token.usage",
		metric.WithDescription("Number of input and output tokens used per GenAI request"),
		metric.WithUnit("{token}"),
		metric.WithExplicitBucketBoundaries(tokenBoundaries...),
	)
	if err != nil {
		return nil, err
	}

	tt.operationDuration, err = meter.Float64Histogram(
		"gen_ai.client.operation.duration",
		metric.WithDescription("Duration of GenAI client operations"),
		metric.WithUnit("s"),
		metric.WithExplicitBucketBoundaries(durationBoundaries...),
	)
	if err != nil {
		return nil, err
	}

	return tt, nil
}

// RecordUsage records token usage and duration for one request.
func (tt *TokenTracker) RecordUsage(ctx context.Context, opts UsageOptions) {
	operation := opts.Operation
	if operation == "" {
		operation = "chat"
	}

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
		attrOperationName.String(operation),
		attrProviderName.String(opts.Provider),
		attrResponseModel.String(strings.Clone(opts.Model)),
		attrRequestModel.String(strings.Clone(opts.RequestModel)),
		attrScenario.String(opts.Scenario),
		attrProviderUUID.String(opts.ProviderUUID),
		attrStreaming.Bool(opts.Streamed),
	}
	if opts.RuleUUID != "" {
		commonAttrs = append(commonAttrs, attrRuleUUID.String(opts.RuleUUID))
	}
	if opts.UserTier != "" {
		commonAttrs = append(commonAttrs, attrUserTier.String(opts.UserTier))
	}
	// NOTE: latency is deliberately NOT an attribute. It is near-unique per
	// request, so every request would permanently allocate a new data point
	// (and pin its attribute strings, see above) on every instrument below.
	// This exact line was bisected as the #1255 leak: with it, the tb2→tb1
	// e2e retains 823KB/request forever; without it, 0.5KB/request.
	// Latency is the duration histogram VALUE instead.

	// Token usage, split by gen_ai.token.type. Each type gets its own
	// attribute set built once (metric.WithAttributeSet avoids the re-sort
	// metric.WithAttributes would do per call).
	tt.recordTokens(ctx, commonAttrs, tokenTypeInput, opts.InputTokens)
	tt.recordTokens(ctx, commonAttrs, tokenTypeOutput, opts.OutputTokens)
	tt.recordTokens(ctx, commonAttrs, tokenTypeCacheRead, opts.CacheInputTokens)
	tt.recordTokens(ctx, commonAttrs, tokenTypeSystem, opts.SystemTokens)

	// Operation duration; failures are classified by error.type. The
	// histogram count doubles as the request count, per the spec.
	durAttrs := commonAttrs
	if opts.Status != "" && opts.Status != "success" {
		errType := opts.ErrorCode
		if errType == "" {
			errType = opts.Status // e.g. "canceled", or bare "error"
		}
		if len(errType) > maxErrorTypeAttrLen {
			// The truncated slice still aliases the original backing array, so
			// the clone below is what actually bounds retained memory.
			errType = errType[:maxErrorTypeAttrLen]
		}
		// Clone: never append onto commonAttrs' backing array (shared above).
		durAttrs = append(append([]attribute.KeyValue{}, commonAttrs...),
			attrErrorType.String(strings.Clone(errType)))
	}
	tt.operationDuration.Record(ctx, float64(opts.LatencyMs)/1000.0,
		metric.WithAttributeSet(attribute.NewSet(durAttrs...)))
}

// recordTokens records count tokens of the given type, skipping zero counts
// so absent token kinds don't allocate empty timeseries.
func (tt *TokenTracker) recordTokens(ctx context.Context, commonAttrs []attribute.KeyValue, tokenType string, count int) {
	if count <= 0 {
		return
	}
	attrs := append(append([]attribute.KeyValue{}, commonAttrs...), attrTokenType.String(tokenType))
	tt.tokenUsage.Record(ctx, int64(count), metric.WithAttributeSet(attribute.NewSet(attrs...)))
}
