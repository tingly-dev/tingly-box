package otel

import (
	"github.com/tingly-dev/tingly-box/internal/otel/tracker"
	"go.opentelemetry.io/otel/attribute"
	semconv "go.opentelemetry.io/otel/semconv/v1.37.0"
)

// OTel GenAI semantic convention attribute keys, aliased from the official
// semconv package so a version bump tracks spec renames (the conventions are
// still Development status — gen_ai.system already became
// gen_ai.provider.name once).
var (
	AttrGenAIOperationName     = semconv.GenAIOperationNameKey
	AttrGenAIProviderName      = semconv.GenAIProviderNameKey
	AttrGenAIRequestModel      = semconv.GenAIRequestModelKey
	AttrGenAIResponseModel     = semconv.GenAIResponseModelKey
	AttrGenAIUsageInputTokens  = semconv.GenAIUsageInputTokensKey
	AttrGenAIUsageOutputTokens = semconv.GenAIUsageOutputTokensKey
	AttrErrorType              = semconv.ErrorTypeKey

	// Standard HTTP attributes on the root span: what was asked of the gateway.
	AttrHTTPRequestMethod = semconv.HTTPRequestMethodKey
	AttrHTTPRoute         = semconv.HTTPRouteKey
	AttrURLPath           = semconv.URLPathKey
)

// Gateway-specific attributes, aliased from tracker (their single home) so
// metrics and spans are guaranteed to emit identical key strings.
var (
	AttrTinglyScenario     = tracker.AttrScenario
	AttrTinglyProviderUUID = tracker.AttrProviderUUID
	AttrTinglyRuleUUID     = tracker.AttrRuleUUID
	AttrTinglyStreaming    = tracker.AttrStreaming
)

// Span-only gateway attributes. These carry per-request (near-unique) values
// and therefore MUST NOT be used as metric attributes — spans are released
// after export, cumulative metric series are not (see the cardinality rules
// in .design/otel.md §4).
var (
	AttrTinglyRequestID       = attribute.Key("tingly.request_id")
	AttrTinglyLBServiceID     = attribute.Key("tingly.lb.service_id")
	AttrTinglyLBTactic        = attribute.Key("tingly.lb.tactic")
	AttrTinglyFailoverAttempt = attribute.Key("tingly.failover.attempt")
	// The protocol the client spoke to the gateway and the one the gateway
	// spoke to the provider, as "<vendor>.<api>" (openai.chat, anthropic.messages).
	AttrTinglyProtocolInbound  = attribute.Key("tingly.protocol.inbound")
	AttrTinglyProtocolUpstream = attribute.Key("tingly.protocol.upstream")
	AttrHTTPResponseStatus     = semconv.HTTPResponseStatusCodeKey
)
