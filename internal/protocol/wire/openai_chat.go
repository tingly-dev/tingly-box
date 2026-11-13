package wire

import "encoding/json"

// Chat Completions stream DTOs preserve the minimal outbound JSON shape emitted by this proxy.
// Keep these fields checked against openai-go Chat Completions stream types when updating the SDK.
// Every SDK-required chunk key is present; optional ones (choice logprobs,
// system_fingerprint, obfuscation) are deliberately omitted. service_tier rides
// on every chunk as the real API does; moderation only on the final one.
type ChatStreamChunk struct {
	ID          string             `json:"id"`
	Object      string             `json:"object"`
	Created     int64              `json:"created"`
	Model       string             `json:"model"`
	Choices     []ChatStreamChoice `json:"choices"`
	Usage       *ChatStreamUsage   `json:"usage,omitempty"`
	ServiceTier *string            `json:"service_tier"`
	Moderation  json.RawMessage    `json:"moderation,omitempty"`
}

type ChatStreamChoice struct {
	Index        int             `json:"index"`
	Delta        ChatStreamDelta `json:"delta"`
	FinishReason *string         `json:"finish_reason"`
}

type ChatStreamDelta struct {
	Role      string               `json:"role,omitempty"`
	Content   string               `json:"content,omitempty"`
	Refusal   string               `json:"refusal,omitempty"`
	ToolCalls []ChatStreamToolCall `json:"tool_calls,omitempty"`
	// ReasoningContent carries extended-thinking text (DeepSeek-style extension;
	// not part of the official OpenAI Chat schema).
	ReasoningContent string `json:"reasoning_content,omitempty"`
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

// ChatStreamPromptTokenDetails breaks down prompt token categories on a
// streaming usage chunk. cache_write_tokens is only emitted by gpt-5.6+ and
// stays omitted for providers that never report it, so downstream can tell
// "no writes" (explicit 0) apart from "channel does not report writes".
type ChatStreamPromptTokenDetails struct {
	CachedTokens     int64 `json:"cached_tokens"`
	CacheWriteTokens int64 `json:"cache_write_tokens,omitempty"`
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
// service_tier and moderation are always emitted, null when the upstream did
// not report them. system_fingerprint is left out: it identifies an OpenAI
// backend configuration and has no honest value for a converted response.
type ChatCompletionWire struct {
	ID          string                     `json:"id"`
	Object      string                     `json:"object"`
	Created     int64                      `json:"created"`
	Model       string                     `json:"model"`
	Choices     []ChatCompletionChoiceWire `json:"choices"`
	Usage       ChatCompletionUsageWire    `json:"usage"`
	ServiceTier *string                    `json:"service_tier"`
	Moderation  json.RawMessage            `json:"moderation"`
}

// ToMap serializes to a generic map for callers that apply runtime transforms.
func (r ChatCompletionWire) ToMap() map[string]any {
	raw, _ := json.Marshal(r)
	var m map[string]any
	_ = json.Unmarshal(raw, &m)
	return m
}

// ChatCompletionChoiceWire is a single choice in the OpenAI Chat Completions response.
// Logprobs is always null: the proxy never requests or translates logprobs.
type ChatCompletionChoiceWire struct {
	Index        int                       `json:"index"`
	Message      ChatCompletionMessageWire `json:"message"`
	FinishReason string                    `json:"finish_reason"`
	Logprobs     interface{}               `json:"logprobs"`
}

// ChatCompletionMessageWire is the message inside a choice.
// content and refusal are required-but-nullable; see MarshalJSON. The optional
// annotations/audio keys are left out — nothing upstream maps to them.
type ChatCompletionMessageWire struct {
	Role             string                       `json:"role"`
	Content          string                       `json:"content"`
	Refusal          string                       `json:"refusal"`
	ToolCalls        []ChatCompletionToolCallWire `json:"tool_calls,omitempty"`
	ReasoningContent string                       `json:"reasoning_content,omitempty"`
}

// MarshalJSON always emits content and refusal, as the real API does: refusal
// is null unless set, and content is null only when the turn is tool calls or a
// refusal (an empty text answer stays "").
func (m ChatCompletionMessageWire) MarshalJSON() ([]byte, error) {
	type alias ChatCompletionMessageWire
	var content, refusal *string
	if m.Content != "" || (len(m.ToolCalls) == 0 && m.Refusal == "") {
		content = &m.Content
	}
	if m.Refusal != "" {
		refusal = &m.Refusal
	}
	return json.Marshal(struct {
		alias
		Content *string `json:"content"`
		Refusal *string `json:"refusal"`
	}{alias(m), content, refusal})
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
// prompt_tokens = TOTAL (uncached + cached + written); cached_tokens and
// cache_write_tokens are reported, disjoint subsets of it.
type ChatCompletionUsageWire struct {
	PromptTokens            int64                            `json:"prompt_tokens"`
	CompletionTokens        int64                            `json:"completion_tokens"`
	TotalTokens             int64                            `json:"total_tokens"`
	PromptTokensDetails     *ChatCompletionPromptDetailsWire `json:"prompt_tokens_details,omitempty"`
	CompletionTokensDetails *ChatCompletionOutputDetailsWire `json:"completion_tokens_details,omitempty"`
}

// ChatCompletionPromptDetailsWire breaks down prompt token categories.
// cached_tokens and cache_write_tokens are disjoint subsets of prompt_tokens.
type ChatCompletionPromptDetailsWire struct {
	CachedTokens     int64 `json:"cached_tokens"`
	CacheWriteTokens int64 `json:"cache_write_tokens,omitempty"`
}

// ChatCompletionOutputDetailsWire breaks down completion token categories.
type ChatCompletionOutputDetailsWire struct {
	ReasoningTokens int64 `json:"reasoning_tokens"`
}
