package stream

import (
	"errors"

	"github.com/anthropics/anthropic-sdk-go"
	"github.com/openai/openai-go/v3"
)

// OpenAIToAnthropicToolCall captures a complete tool call assembled from OpenAI stream chunks.
type OpenAIToAnthropicToolCall struct {
	ID        string
	Name      string
	Arguments string
}

// OpenAIToAnthropicMCPHooks provides optional hooks for MCP-aware stream handling.
type OpenAIToAnthropicMCPHooks struct {
	ShouldSuppressTool func(name string) bool
	OnToolCallsFinal   func(calls []OpenAIToAnthropicToolCall) error
}

var ErrMCPStreamContinue = errors.New("mcp stream should continue")

// NewOpenAIChatToAnthropicV1Converter creates the transport-free V1 stream
// state machine. The caller owns driving and closing the supplied stream.
func NewOpenAIChatToAnthropicV1Converter(stream OpenAIChatStream, responseModel string, req *openai.ChatCompletionNewParams) StreamConverter {
	return newOpenAIToAnthropicConverter(stream, responseModel, req, nil, mapOpenAIFinishReasonToAnthropic)
}

// mapOpenAIFinishReasonToAnthropic converts OpenAI finish_reason to Anthropic stop_reason
func mapOpenAIFinishReasonToAnthropic(finishReason string) string {
	switch finishReason {
	case string(openai.CompletionChoiceFinishReasonStop):
		return anthropicStopReasonEndTurn
	case string(openai.CompletionChoiceFinishReasonLength):
		return anthropicStopReasonMaxTokens
	case openaiFinishReasonToolCalls:
		return anthropicStopReasonToolUse
	case string(openai.CompletionChoiceFinishReasonContentFilter):
		// MENTION: we may use `refusal` but it works badly - then we use end turn as normal
		return string(anthropic.StopReasonEndTurn)
	default:
		return anthropicStopReasonEndTurn
	}
}
