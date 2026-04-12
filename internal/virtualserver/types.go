package virtualserver

import (
	"encoding/json"

	"github.com/anthropics/anthropic-sdk-go"
	"github.com/openai/openai-go/v3"
	"github.com/tingly-dev/tingly-box/internal/protocol"
	"github.com/tingly-dev/tingly-box/internal/virtualmodel"
)

// ChatCompletionRequest is an OpenAI-compatible chat completion request.
type ChatCompletionRequest = protocol.OpenAIChatCompletionRequest

// Response types aliased from OpenAI SDK
type (
	ChatCompletionResponse       = openai.ChatCompletion
	Choice                       = openai.ChatCompletionChoice
	Message                      = openai.ChatCompletionMessage
	ToolCall                     = openai.ChatCompletionMessageFunctionToolCall
	FunctionCall                 = openai.ChatCompletionMessageFunctionToolCallFunction
	Usage                        = openai.CompletionUsage
	ChatCompletionStreamResponse = openai.ChatCompletionChunk
	StreamChoice                 = openai.ChatCompletionChunkChoice
	Delta                        = openai.ChatCompletionChunkChoiceDelta
)

// OpenAIModelsResponse is the OpenAI models list response format.
type OpenAIModelsResponse struct {
	Object string               `json:"object"`
	Data   []virtualmodel.Model `json:"data"`
}

// Request types aliased from protocol / Anthropic SDK
type (
	AnthropicMessageRequest = protocol.AnthropicBetaMessagesRequest
	AnthropicMessage        = anthropic.BetaMessageParam
	AnthropicTool           = anthropic.BetaToolParam
)

// AnthropicMessageResponse is an Anthropic-compatible message response.
type AnthropicMessageResponse struct {
	ID         string             `json:"id"`
	Type       string             `json:"type"`
	Role       string             `json:"role"`
	Model      string             `json:"model"`
	Content    []AnthropicContent `json:"content"`
	StopReason string             `json:"stop_reason"`
	Usage      AnthropicUsage     `json:"usage"`
}

// AnthropicContent is a content block in an Anthropic response.
type AnthropicContent struct {
	Type  string          `json:"type"`
	Text  string          `json:"text,omitempty"`
	ID    string          `json:"id,omitempty"`
	Name  string          `json:"name,omitempty"`
	Input json.RawMessage `json:"input,omitempty"`
}

// AnthropicUsage holds token usage in Anthropic format.
type AnthropicUsage struct {
	InputTokens  int64 `json:"input_tokens"`
	OutputTokens int64 `json:"output_tokens"`
}

// AnthropicStreamEvent is a streaming event in Anthropic format.
type AnthropicStreamEvent struct {
	Type    string                    `json:"type"`
	Message *AnthropicMessageResponse `json:"message,omitempty"`
	Index   int                       `json:"index,omitempty"`
	Delta   *AnthropicDelta           `json:"delta,omitempty"`
	Usage   *AnthropicUsage           `json:"usage,omitempty"`
}

// AnthropicDelta is a delta in an Anthropic streaming response.
type AnthropicDelta struct {
	Type       string `json:"type,omitempty"`
	Text       string `json:"text,omitempty"`
	StopReason string `json:"stop_reason,omitempty"`
}
