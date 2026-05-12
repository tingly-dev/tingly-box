package stream

const (
	// OpenAI finish reasons not defined in openai package
	openaiFinishReasonToolCalls    = "tool_calls"
	openaiFinishReasonFunctionCall = "function_call"
)

// OpenAI extra field names that map to Anthropic content blocks
const OpenaiFieldReasoningContent = "reasoning_content"

// OpenAI tool call ID max length (40 characters per OpenAI API spec)
const maxToolCallIDLength = 40
