package stream

const (
	// OpenAI finish reasons not defined in openai package
	openaiFinishReasonToolCalls = "tool_calls"

	// OpenAI extra field names that map to Anthropic content blocks
	OpenaiFieldReasoningContent = "reasoning_content"

	// OpenAI tool call ID max length (40 characters per OpenAI API spec)
	maxToolCallIDLength = 40
)
