package wire

import "encoding/json"

// Chat Completions stream DTOs preserve the minimal outbound JSON shape emitted by this proxy.
// Keep these fields checked against openai-go Chat Completions stream types when updating the SDK.
type ChatStreamChunk struct {
	ID      string             `json:"id"`
	Object  string             `json:"object"`
	Created int64              `json:"created"`
	Model   string             `json:"model"`
	Choices []ChatStreamChoice `json:"choices"`
	Usage   *ChatStreamUsage   `json:"usage,omitempty"`
}

type ChatStreamChoice struct {
	Index        int             `json:"index"`
	Delta        ChatStreamDelta `json:"delta"`
	FinishReason *string         `json:"finish_reason"`
}

type ChatStreamDelta struct {
	Role      string               `json:"role,omitempty"`
	Content   string               `json:"content,omitempty"`
	ToolCalls []ChatStreamToolCall `json:"tool_calls,omitempty"`
}

type ChatStreamToolCall struct {
	Index    int                    `json:"index"`
	ID       string                 `json:"id,omitempty"`
	Type     string                 `json:"type,omitempty"`
	Function ChatStreamToolFunction `json:"function"`
}

type ChatStreamToolFunction struct {
	Name      string  `json:"name,omitempty"`
	Arguments *string `json:"arguments,omitempty"`
}

type ChatStreamUsage struct {
	PromptTokens            int64                         `json:"prompt_tokens"`
	CompletionTokens        int64                         `json:"completion_tokens"`
	TotalTokens             int64                         `json:"total_tokens"`
	PromptTokensDetails     *ChatStreamPromptTokenDetails `json:"prompt_tokens_details,omitempty"`
	CompletionTokensDetails *ChatStreamOutputTokenDetails `json:"completion_tokens_details,omitempty"`
}

type ChatStreamPromptTokenDetails struct {
	CachedTokens int64 `json:"cached_tokens"`
}

type ChatStreamOutputTokenDetails struct {
	ReasoningTokens int64 `json:"reasoning_tokens"`
}

type ChatStreamErrorChunk struct {
	Error ChatStreamError `json:"error"`
}

type ChatStreamError struct {
	Message string `json:"message"`
	Type    string `json:"type"`
	Code    string `json:"code"`
}

// ChatCompletionWire is the OpenAI Chat Completions response wire format.
type ChatCompletionWire struct {
	ID      string                    `json:"id"`
	Object  string                    `json:"object"`
	Created int64                     `json:"created"`
	Model   string                    `json:"model"`
	Choices []ChatCompletionChoiceWire `json:"choices"`
	Usage   ChatCompletionUsageWire   `json:"usage"`
}

// ToMap serializes to a generic map for callers that apply runtime transforms.
func (r ChatCompletionWire) ToMap() map[string]interface{} {
	raw, _ := json.Marshal(r)
	var m map[string]interface{}
	_ = json.Unmarshal(raw, &m)
	return m
}

// ChatCompletionChoiceWire is a single choice in the OpenAI Chat Completions response.
type ChatCompletionChoiceWire struct {
	Index        int                      `json:"index"`
	Message      ChatCompletionMessageWire `json:"message"`
	FinishReason string                   `json:"finish_reason"`
}

// ChatCompletionMessageWire is the message inside a choice.
type ChatCompletionMessageWire struct {
	Role             string                      `json:"role"`
	Content          string                      `json:"content,omitempty"`
	ToolCalls        []ChatCompletionToolCallWire `json:"tool_calls,omitempty"`
	ReasoningContent string                      `json:"reasoning_content,omitempty"`
}

// ChatCompletionToolCallWire is a single tool call inside a message.
type ChatCompletionToolCallWire struct {
	ID       string                     `json:"id"`
	Type     string                     `json:"type"`
	Function ChatCompletionFunctionWire `json:"function"`
}

// ChatCompletionFunctionWire carries the function name and JSON-encoded arguments.
type ChatCompletionFunctionWire struct {
	Name      string `json:"name"`
	Arguments string `json:"arguments"`
}

// ChatCompletionUsageWire is the usage block in the OpenAI Chat Completions response.
// prompt_tokens = TOTAL (uncached + cached); cached_tokens is a reported subset.
type ChatCompletionUsageWire struct {
	PromptTokens        int64                            `json:"prompt_tokens"`
	CompletionTokens    int64                            `json:"completion_tokens"`
	TotalTokens         int64                            `json:"total_tokens"`
	PromptTokensDetails *ChatCompletionPromptDetailsWire `json:"prompt_tokens_details,omitempty"`
}

// ChatCompletionPromptDetailsWire breaks down prompt token categories.
type ChatCompletionPromptDetailsWire struct {
	CachedTokens int64 `json:"cached_tokens"`
}
