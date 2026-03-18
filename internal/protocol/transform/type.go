package transform

// TargetAPIStyle represents the target API style for protocol conversion
type TargetAPIStyle string

const (
	// TargetAPIStyleOpenAIChat converts requests to OpenAI Chat Completions format
	TargetAPIStyleOpenAIChat TargetAPIStyle = "openai_chat"

	// TargetAPIStyleOpenAIResponses converts requests to OpenAI Responses API format
	TargetAPIStyleOpenAIResponses TargetAPIStyle = "openai_responses"

	// TargetAPIStyleAnthropicV1 converts requests to Anthropic v1 Messages API format
	TargetAPIStyleAnthropicV1 TargetAPIStyle = "anthropic_v1"

	// TargetAPIStyleAnthropicBeta converts requests to Anthropic v1beta Messages API format
	TargetAPIStyleAnthropicBeta TargetAPIStyle = "anthropic_beta"

	// TargetAPIStyleGoogle converts requests to Google Gemini API format
	TargetAPIStyleGoogle TargetAPIStyle = "google"
)
