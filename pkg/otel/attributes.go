package otel

import (
	semconv "go.opentelemetry.io/otel/semconv/v1.37.0"

	"github.com/tingly-dev/tingly-box/pkg/otel/tracker"
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
)

// Gateway-specific attributes, aliased from tracker (their single home) so
// metrics and spans are guaranteed to emit identical key strings.
var (
	AttrTinglyScenario     = tracker.AttrScenario
	AttrTinglyProviderUUID = tracker.AttrProviderUUID
	AttrTinglyRuleUUID     = tracker.AttrRuleUUID
	AttrTinglyStreaming    = tracker.AttrStreaming
	AttrTinglyUserTier     = tracker.AttrUserTier
)
