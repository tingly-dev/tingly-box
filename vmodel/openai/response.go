package openai

// VModelResponse describes what the virtual model wants to respond for OpenAI Chat.
type VModelResponse struct {
	Content      string
	ToolCalls    []VToolCall
	FinishReason string
}
