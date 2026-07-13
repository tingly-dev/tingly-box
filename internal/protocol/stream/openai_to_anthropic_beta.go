package stream

import (
	"context"
	"errors"
	"net/http"

	"github.com/anthropics/anthropic-sdk-go"
	"github.com/openai/openai-go/v3"
	openaistream "github.com/openai/openai-go/v3/packages/ssestream"
	"github.com/sirupsen/logrus"

	"github.com/tingly-dev/tingly-box/internal/protocol"
)

// NewOpenAIChatToAnthropicBetaConverter creates the transport-free beta stream
// state machine. The caller owns driving and closing the supplied stream.
func NewOpenAIChatToAnthropicBetaConverter(stream OpenAIChatStream, responseModel string, req *openai.ChatCompletionNewParams) StreamConverter {
	return newOpenAIToAnthropicConverter(stream, responseModel, req, nil, mapOpenAIFinishReasonToAnthropicBeta)
}

// HandleOpenAIToAnthropicBetaStream processes OpenAI streaming events and converts them to Anthropic beta format.
// Returns UsageStat containing token usage information for tracking.
func HandleOpenAIToAnthropicBetaStream(hc *protocol.HandleContext, req *openai.ChatCompletionNewParams, stream *openaistream.Stream[openai.ChatCompletionChunk], responseModel string) (*protocol.TokenUsage, error) {
	return handleOpenAIToAnthropicBetaStream(hc, req, stream, responseModel, nil)
}

// HandleOpenAIToAnthropicBetaStreamWithMCPHooks enables MCP-aware tool suppression/finalization during conversion.
func HandleOpenAIToAnthropicBetaStreamWithMCPHooks(
	hc *protocol.HandleContext,
	req *openai.ChatCompletionNewParams,
	stream *openaistream.Stream[openai.ChatCompletionChunk],
	responseModel string,
	hooks *OpenAIToAnthropicMCPHooks,
) (*protocol.TokenUsage, error) {
	return handleOpenAIToAnthropicBetaStream(hc, req, stream, responseModel, hooks)
}

func handleOpenAIToAnthropicBetaStream(
	hc *protocol.HandleContext,
	req *openai.ChatCompletionNewParams,
	stream *openaistream.Stream[openai.ChatCompletionChunk],
	responseModel string,
	hooks *OpenAIToAnthropicMCPHooks,
) (*protocol.TokenUsage, error) {
	c := hc.GinContext
	logrus.WithContext(c.Request.Context()).Debug("Starting OpenAI to Anthropic beta streaming response handler")
	defer func() {
		if r := recover(); r != nil {
			logrus.WithContext(c.Request.Context()).Errorf("Panic in OpenAI to Anthropic beta streaming handler: %v", r)
			if c.Writer != nil {
				c.Writer.WriteHeader(http.StatusInternalServerError)
				c.Writer.Write([]byte("event: error\ndata: {\"error\":{\"message\":\"Internal streaming error\",\"type\":\"internal_error\"}}\n\n"))
				if flusher, ok := c.Writer.(http.Flusher); ok {
					flusher.Flush()
				}
			}
		}
		if stream != nil {
			if err := stream.Close(); err != nil {
				logrus.WithContext(c.Request.Context()).Errorf("Error closing OpenAI stream: %v", err)
			}
		}
		logrus.WithContext(c.Request.Context()).Info("Finished OpenAI to Anthropic beta streaming response handler")
	}()

	conv := newOpenAIToAnthropicConverter(stream, responseModel, req, hooks, mapOpenAIFinishReasonToAnthropicBeta)
	_, err := RunConverter(hc, conv, anthropicSSEWriter(c))

	if hookErr := conv.HookErr(); hookErr != nil {
		return conv.Usage(), hookErr
	}
	if err != nil {
		if errors.Is(err, context.Canceled) {
			logrus.WithContext(c.Request.Context()).Debug("OpenAI to Anthropic beta stream canceled by client")
			return conv.Usage(), nil
		}
		logrus.WithContext(c.Request.Context()).Errorf("OpenAI stream error: %v", err)
		hc.DispatchStreamError(err)
		if !conv.MessageStarted() {
			SendStreamingError(c, err)
			return conv.Usage(), err
		}
		sendAnthropicStreamEvent(c, "error", map[string]interface{}{
			"type": "error",
			"error": map[string]interface{}{
				"message": err.Error(),
				"type":    "stream_error",
				"code":    "stream_failed",
			},
		}, nil)
		return conv.Usage(), err
	}
	if streamErr := stream.Err(); streamErr != nil {
		if errors.Is(streamErr, context.Canceled) {
			logrus.WithContext(c.Request.Context()).Debug("OpenAI to Anthropic beta stream canceled by client")
			return conv.Usage(), nil
		}
		logrus.WithContext(c.Request.Context()).Errorf("OpenAI stream error: %v", streamErr)
		hc.DispatchStreamError(streamErr)
		if !conv.MessageStarted() {
			SendStreamingError(c, streamErr)
			return conv.Usage(), streamErr
		}
		sendAnthropicStreamEvent(c, "error", map[string]interface{}{
			"type": "error",
			"error": map[string]interface{}{
				"message": streamErr.Error(),
				"type":    "stream_error",
				"code":    "stream_failed",
			},
		}, nil)
		return conv.Usage(), streamErr
	}
	return conv.Usage(), nil
}

// HandleResponsesToAnthropicBetaStream processes OpenAI Responses API streaming events and converts them to Anthropic beta format.
// Returns TokenUsage containing token usage information for tracking.
func HandleResponsesToAnthropicBetaStream(hc *protocol.HandleContext, stream ResponsesStreamIter, responseModel string) (*protocol.TokenUsage, error) {
	return handleResponsesToAnthropicStream(hc, stream, responseModel)
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
