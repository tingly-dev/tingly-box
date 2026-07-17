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
	"slices"
	"strings"
	"unicode/utf8"

	"go.opentelemetry.io/otel/attribute"
	"go.opentelemetry.io/otel/metric"
	semconv "go.opentelemetry.io/otel/semconv/v1.37.0"
	"go.opentelemetry.io/otel/semconv/v1.37.0/genaiconv"
)

// Gateway-specific metric attributes. These have no gen_ai equivalent; they
// live in the tingly.* namespace instead of squatting on a standard one.
// This is the single home for these keys — pkg/otel aliases them for spans
// (tracker cannot import pkg/otel, which imports tracker).
var (
	AttrScenario     = attribute.Key("tingly.scenario")
	AttrProviderUUID = attribute.Key("tingly.provider.uuid")
	AttrRuleUUID     = attribute.Key("tingly.rule.uuid")
	AttrStreaming    = attribute.Key("tingly.streaming")
	AttrUserTier     = attribute.Key("tingly.user.tier")
)

// gen_ai.token.type values. input/output are the spec enum (from semconv);
// the enum is open, so the gateway extends it for cache and system token
// accounting.
var (
	tokenTypeInput     = string(genaiconv.TokenTypeInput)
	tokenTypeOutput    = string(genaiconv.TokenTypeOutput)
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
	// Defaults to "chat" when empty. Callers MUST pass a bounded set of
	// values — every distinct operation mints permanent timeseries.
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
	tokenUsage        genaiconv.ClientTokenUsage
	operationDuration genaiconv.ClientOperationDuration
}

// NewTokenTracker creates a new TokenTracker with the provided meter.
// The genaiconv constructors supply the spec-exact instrument names, units
// and descriptions.
func NewTokenTracker(meter metric.Meter) (*TokenTracker, error) {
	tt := &TokenTracker{}

	var err error

	tt.tokenUsage, err = genaiconv.NewClientTokenUsage(meter,
		metric.WithExplicitBucketBoundaries(tokenBoundaries...))
	if err != nil {
		return nil, err
	}

	tt.operationDuration, err = genaiconv.NewClientOperationDuration(meter,
		metric.WithExplicitBucketBoundaries(durationBoundaries...))
	if err != nil {
		return nil, err
	}

	return tt, nil
}

// RecordUsage records token usage and duration for one request.
func (tt *TokenTracker) RecordUsage(ctx context.Context, opts UsageOptions) {
	operation := opts.Operation
	if operation == "" {
		operation = string(genaiconv.OperationNameChat)
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
	commonAttrs := make([]attribute.KeyValue, 0, 10)
	commonAttrs = append(commonAttrs,
		semconv.GenAIOperationNameKey.String(operation),
		semconv.GenAIProviderNameKey.String(opts.Provider),
		semconv.GenAIResponseModelKey.String(strings.Clone(opts.Model)),
		semconv.GenAIRequestModelKey.String(strings.Clone(opts.RequestModel)),
		AttrScenario.String(opts.Scenario),
		AttrProviderUUID.String(opts.ProviderUUID),
		AttrStreaming.Bool(opts.Streamed),
	)
	if opts.RuleUUID != "" {
		commonAttrs = append(commonAttrs, AttrRuleUUID.String(opts.RuleUUID))
	}
	if opts.UserTier != "" {
		commonAttrs = append(commonAttrs, AttrUserTier.String(opts.UserTier))
	}
	// NOTE: latency is deliberately NOT an attribute. It is near-unique per
	// request, so every request would permanently allocate a new data point
	// (and pin its attribute strings, see above) on every instrument below.
	// This exact line was bisected as the #1255 leak: with it, the tb2→tb1
	// e2e retains 823KB/request forever; without it, 0.5KB/request.
	// Latency is the duration histogram VALUE instead.

	// Token usage, split by gen_ai.token.type. Each record builds its own
	// attribute set from a fresh copy of commonAttrs (never append onto the
	// shared backing array — a second append would clobber the first's
	// token_type element).
	tt.recordTokens(ctx, commonAttrs, tokenTypeInput, opts.InputTokens)
	tt.recordTokens(ctx, commonAttrs, tokenTypeOutput, opts.OutputTokens)
	tt.recordTokens(ctx, commonAttrs, tokenTypeCacheRead, opts.CacheInputTokens)
	tt.recordTokens(ctx, commonAttrs, tokenTypeSystem, opts.SystemTokens)

	// Operation duration; the histogram count doubles as the request count,
	// per the spec. error.type is attached only for genuine failures —
	// client cancellations (Status "canceled") routinely happen mid-stream
	// in LLM UIs and must not trip error-rate alerts computed from this
	// metric ("data points with error.type present / total").
	durAttrs := commonAttrs
	if opts.Status == "error" {
		errType := opts.ErrorCode
		if errType == "" {
			errType = string(genaiconv.ErrorTypeOther)
		}
		durAttrs = append(slices.Clone(commonAttrs),
			semconv.ErrorTypeKey.String(strings.Clone(truncateErrorType(errType))))
	}
	tt.operationDuration.RecordSet(ctx, float64(opts.LatencyMs)/1000.0,
		attribute.NewSet(durAttrs...))
}

// recordTokens records count tokens of the given type, skipping zero counts
// so absent token kinds don't allocate empty timeseries.
func (tt *TokenTracker) recordTokens(ctx context.Context, commonAttrs []attribute.KeyValue, tokenType string, count int) {
	if count <= 0 {
		return
	}
	attrs := append(slices.Clone(commonAttrs), semconv.GenAITokenTypeKey.String(tokenType))
	tt.tokenUsage.RecordSet(ctx, int64(count), attribute.NewSet(attrs...))
}

// truncateErrorType bounds an error.type value to maxErrorTypeAttrLen bytes
// without splitting a multi-byte UTF-8 rune (OTLP requires valid UTF-8; a
// poisoned attribute value could make strict collectors reject every
// subsequent export of the retained timeseries).
func truncateErrorType(s string) string {
	if len(s) <= maxErrorTypeAttrLen {
		return s
	}
	s = s[:maxErrorTypeAttrLen]
	for len(s) > 0 && !utf8.ValidString(s) {
		s = s[:len(s)-1]
	}
	return s
}
