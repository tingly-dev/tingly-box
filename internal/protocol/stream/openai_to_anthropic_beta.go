package stream

import (
	"github.com/anthropics/anthropic-sdk-go"
	"github.com/openai/openai-go/v3"
)

// NewOpenAIChatToAnthropicBetaConverter creates the transport-free beta stream
// state machine. The caller owns driving and closing the supplied stream.
func NewOpenAIChatToAnthropicBetaConverter(stream OpenAIChatStream, responseModel string, req *openai.ChatCompletionNewParams) StreamConverter {
	return newOpenAIToAnthropicConverter(stream, responseModel, req, nil, mapOpenAIFinishReasonToAnthropicBeta)
}

// mapOpenAIFinishReasonToAnthropicBeta converts OpenAI finish_reason to Anthropic beta stop_reason
func mapOpenAIFinishReasonToAnthropicBeta(finishReason string) string {
	switch finishReason {
	case string(openai.CompletionChoiceFinishReasonStop):
		return string(anthropic.BetaStopReasonEndTurn)
	case string(openai.CompletionChoiceFinishReasonLength):
		return string(anthropic.BetaStopReasonMaxTokens)
	case openaiFinishReasonToolCalls, openaiFinishReasonFunctionCall:
		return string(anthropic.BetaStopReasonToolUse)
	case string(openai.CompletionChoiceFinishReasonContentFilter):
		// MENTION: we may use `refusal` but it works badly - then we use end turn as normal
		return string(anthropic.BetaStopReasonEndTurn)
	default:
		return string(anthropic.BetaStopReasonEndTurn)
	}
}
