package stream

// Chat Completions stream DTOs preserve the minimal outbound JSON shape emitted by this proxy.
// Keep these fields checked against openai-go Chat Completions stream types when updating the SDK.
type chatCompletionStreamChunk struct {
	ID      string                       `json:"id"`
	Object  string                       `json:"object"`
	Created int64                        `json:"created"`
	Model   string                       `json:"model"`
	Choices []chatCompletionStreamChoice `json:"choices"`
	Usage   *chatCompletionStreamUsage   `json:"usage,omitempty"`
}

type chatCompletionStreamChoice struct {
	Index        int                       `json:"index"`
	Delta        chatCompletionStreamDelta `json:"delta"`
	FinishReason *string                   `json:"finish_reason"`
}

type chatCompletionStreamDelta struct {
	Role      string                         `json:"role,omitempty"`
	Content   string                         `json:"content,omitempty"`
	ToolCalls []chatCompletionStreamToolCall `json:"tool_calls,omitempty"`
}

type chatCompletionStreamToolCall struct {
	Index    int                              `json:"index"`
	ID       string                           `json:"id,omitempty"`
	Type     string                           `json:"type,omitempty"`
	Function chatCompletionStreamToolFunction `json:"function"`
}

type chatCompletionStreamToolFunction struct {
	Name      string  `json:"name,omitempty"`
	Arguments *string `json:"arguments,omitempty"`
}

type chatCompletionStreamUsage struct {
	PromptTokens            int64                                   `json:"prompt_tokens"`
	CompletionTokens        int64                                   `json:"completion_tokens"`
	TotalTokens             int64                                   `json:"total_tokens"`
	PromptTokensDetails     *chatCompletionStreamPromptTokenDetails `json:"prompt_tokens_details,omitempty"`
	CompletionTokensDetails *chatCompletionStreamOutputTokenDetails `json:"completion_tokens_details,omitempty"`
}

type chatCompletionStreamPromptTokenDetails struct {
	CachedTokens int64 `json:"cached_tokens"`
}

type chatCompletionStreamOutputTokenDetails struct {
	ReasoningTokens int64 `json:"reasoning_tokens"`
}

type chatCompletionStreamErrorChunk struct {
	Error chatCompletionStreamError `json:"error"`
}

type chatCompletionStreamError struct {
	Message string `json:"message"`
	Type    string `json:"type"`
	Code    string `json:"code"`
}
