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

// handleResponsesToAnthropicStream is the shared implementation for both v1 and beta
// Responses API → Anthropic stream conversions using the iterator pattern.
func handleResponsesToAnthropicStream(hc *protocol.HandleContext, stream ResponsesStreamIter, responseModel string) (*protocol.TokenUsage, error) {
	c := hc.GinContext
	logrus.WithContext(c.Request.Context()).Debugf("[ResponsesAPI] Starting Responses to Anthropic stream, model=%s", responseModel)
	defer func() {
		if r := recover(); r != nil {
			logrus.WithContext(c.Request.Context()).Errorf("Panic in Responses to Anthropic streaming handler: %v", r)
			if c.Writer != nil {
				c.SSEvent("error", "{\"error\":{\"message\":\"Internal streaming error\",\"type\":\"internal_error\"}}")
				if flusher, ok := c.Writer.(http.Flusher); ok {
					flusher.Flush()
				}
			}
		}
		if stream != nil {
			if err := stream.Close(); err != nil {
				logrus.WithContext(c.Request.Context()).Errorf("Error closing Responses API stream: %v", err)
			}
		}
		logrus.WithContext(c.Request.Context()).Debug("[ResponsesAPI] Finished Responses to Anthropic stream")
	}()

	conv := newResponsesToAnthropicConverter(c.Request.Context(), stream, responseModel)
	_, err := RunConverter(hc, conv, anthropicSSEWriterWithFirstChunk(c))

	// Protocol-level error (response.failed, etc.): SSE error event already sent by converter.
	if hookErr := conv.HookErr(); hookErr != nil {
		logrus.WithContext(c.Request.Context()).Errorf("[ResponsesAPI] Protocol error: %v", hookErr)
		return conv.Usage(), hookErr
	}

	if err != nil {
		if errors.Is(err, context.Canceled) {
			logrus.WithContext(c.Request.Context()).Debug("[ResponsesAPI] Stream canceled by client")
			return conv.Usage(), nil
		}
		logrus.WithContext(c.Request.Context()).Errorf("[ResponsesAPI] Stream error: %v", err)
		hc.DispatchStreamError(err)
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
			logrus.WithContext(c.Request.Context()).Debug("[ResponsesAPI] Stream canceled by client")
			return conv.Usage(), nil
		}
		logrus.WithContext(c.Request.Context()).Errorf("[ResponsesAPI] Stream error: %v", streamErr)
		hc.DispatchStreamError(streamErr)
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

// HandleOpenAIToAnthropicStreamResponse processes OpenAI streaming events and converts them to Anthropic format.
// Returns UsageStat containing token usage information for tracking.
func HandleOpenAIToAnthropicStreamResponse(hc *protocol.HandleContext, req *openai.ChatCompletionNewParams, stream *openaistream.Stream[openai.ChatCompletionChunk], responseModel string) (*protocol.TokenUsage, error) {
	return handleOpenAIToAnthropicStreamResponse(hc, req, stream, responseModel, nil)
}

// HandleOpenAIToAnthropicStreamResponseWithMCPHooks enables MCP-aware tool suppression/finalization during conversion.
func HandleOpenAIToAnthropicStreamResponseWithMCPHooks(
	hc *protocol.HandleContext,
	req *openai.ChatCompletionNewParams,
	stream *openaistream.Stream[openai.ChatCompletionChunk],
	responseModel string,
	hooks *OpenAIToAnthropicMCPHooks,
) (*protocol.TokenUsage, error) {
	return handleOpenAIToAnthropicStreamResponse(hc, req, stream, responseModel, hooks)
}

func handleOpenAIToAnthropicStreamResponse(
	hc *protocol.HandleContext,
	req *openai.ChatCompletionNewParams,
	stream *openaistream.Stream[openai.ChatCompletionChunk],
	responseModel string,
	hooks *OpenAIToAnthropicMCPHooks,
) (*protocol.TokenUsage, error) {
	c := hc.GinContext
	logrus.WithContext(c.Request.Context()).Debug("Starting OpenAI to Anthropic streaming response handler")
	defer func() {
		if r := recover(); r != nil {
			logrus.WithContext(c.Request.Context()).Errorf("Panic in OpenAI to Anthropic streaming handler: %v", r)
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
		logrus.WithContext(c.Request.Context()).Info("Finished OpenAI to Anthropic streaming response handler")
	}()

	conv := newOpenAIToAnthropicConverter(stream, responseModel, req, hooks, mapOpenAIFinishReasonToAnthropic)
	_, err := RunConverter(hc, conv, anthropicSSEWriter(c))

	if hookErr := conv.HookErr(); hookErr != nil {
		return conv.Usage(), hookErr
	}
	if err != nil {
		if errors.Is(err, context.Canceled) {
			logrus.WithContext(c.Request.Context()).Debug("OpenAI to Anthropic stream canceled by client")
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
			logrus.WithContext(c.Request.Context()).Debug("OpenAI to Anthropic stream canceled by client")
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
	if errors.Is(c.Request.Context().Err(), context.Canceled) {
		return conv.Usage(), context.Canceled
	}
	return conv.Usage(), nil
}

// HandleResponsesToAnthropicV1Stream processes OpenAI Responses API streaming events and converts them to Anthropic v1 format.
// Returns TokenUsage containing token usage information for tracking.
func HandleResponsesToAnthropicV1Stream(hc *protocol.HandleContext, stream ResponsesStreamIter, responseModel string) (*protocol.TokenUsage, error) {
	return handleResponsesToAnthropicStream(hc, stream, responseModel)
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
