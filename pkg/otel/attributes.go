package otel

import "go.opentelemetry.io/otel/attribute"

// OTel GenAI semantic convention attributes (Development status, tracked at
// https://github.com/open-telemetry/semantic-conventions-genai). Adopted
// wholesale while this package has no telemetry consumers yet — going
// straight to the standard namespace avoids a migration later.
var (
	// AttrGenAIOperationName is the operation kind: "chat", "embeddings",
	// "text_completion", "generate_content", "execute_tool", ...
	AttrGenAIOperationName = attribute.Key("gen_ai.operation.name")

	// AttrGenAIProviderName identifies the provider: "openai", "anthropic",
	// "aws.bedrock", "gcp.vertex_ai", ...
	AttrGenAIProviderName = attribute.Key("gen_ai.provider.name")

	// AttrGenAIRequestModel is the model requested by the client.
	AttrGenAIRequestModel = attribute.Key("gen_ai.request.model")

	// AttrGenAIResponseModel is the model that actually served the request.
	AttrGenAIResponseModel = attribute.Key("gen_ai.response.model")

	// AttrGenAITokenType distinguishes token kinds on the token.usage metric.
	// Spec values: "input", "output". This gateway extends the open enum with
	// "cache_read" and "system" (see tracker.RecordUsage).
	AttrGenAITokenType = attribute.Key("gen_ai.token.type")

	// AttrGenAIUsageInputTokens / AttrGenAIUsageOutputTokens carry token
	// usage on inference spans.
	AttrGenAIUsageInputTokens  = attribute.Key("gen_ai.usage.input_tokens")
	AttrGenAIUsageOutputTokens = attribute.Key("gen_ai.usage.output_tokens")

	// AttrErrorType is the standard OTel error.type attribute, set on the
	// operation.duration metric and on failed spans.
	AttrErrorType = attribute.Key("error.type")
)

// Gateway-specific attributes. These have no gen_ai equivalent; they live in
// the tingly.* namespace instead of squatting on a standard one.
var (
	// AttrTinglyScenario is the API scenario ("openai", "anthropic",
	// "claude_code", ...) — which protocol surface the request entered on.
	AttrTinglyScenario = attribute.Key("tingly.scenario")

	// AttrTinglyProviderUUID identifies the configured provider instance.
	AttrTinglyProviderUUID = attribute.Key("tingly.provider.uuid")

	// AttrTinglyRuleUUID identifies the load balancer rule used.
	AttrTinglyRuleUUID = attribute.Key("tingly.rule.uuid")

	// AttrTinglyStreaming indicates whether the request was streaming.
	AttrTinglyStreaming = attribute.Key("tingly.streaming")

	// AttrTinglyUserTier is a low-cardinality user class for enterprise
	// observability.
	AttrTinglyUserTier = attribute.Key("tingly.user.tier")
)
