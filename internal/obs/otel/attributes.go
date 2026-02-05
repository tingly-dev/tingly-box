package otel

import "go.opentelemetry.io/otel/attribute"

// Semantic convention attributes following OpenLLMetry and OpenTelemetry standards
// These attributes are used to annotate metrics with consistent, meaningful labels.

var (
	// AttrLLMProvider identifies the LLM provider (e.g., "openai", "anthropic", "google")
	AttrLLMProvider = attribute.Key("llm.provider")

	// AttrLLMModel identifies the actual model used (e.g., "gpt-4", "claude-3-opus")
	AttrLLMModel = attribute.Key("llm.model")

	// AttrLLMRequestModel identifies the model requested by the user
	AttrLLMRequestModel = attribute.Key("llm.request.model")

	// AttrLLMTokenType identifies the type of token (input/output)
	AttrLLMTokenType = attribute.Key("llm.token_type")

	// AttrLLMScenario identifies the API scenario (e.g., "openai", "anthropic", "claude_code")
	AttrLLMScenario = attribute.Key("llm.scenario")

	// AttrLLMStreaming indicates whether the request was streaming
	AttrLLMStreaming = attribute.Key("llm.streaming")

	// AttrLLMResponseStatus indicates the response status (success, error, canceled)
	AttrLLMResponseStatus = attribute.Key("llm.response.status")

	// AttrLLMErrorCode contains the error code if status is error
	AttrLLMErrorCode = attribute.Key("llm.error.code")

	// AttrLLMRuleUUID identifies the load balancer rule used
	AttrLLMRuleUUID = attribute.Key("llm.rule.uuid")

	// AttrLLMProviderUUID identifies the provider UUID
	AttrLLMProviderUUID = attribute.Key("llm.provider.uuid")
)
