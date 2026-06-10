package stream

import (
	"context"
	"errors"
	"fmt"
	"io"
	"net/http"

	"github.com/anthropics/anthropic-sdk-go"
	anthropicstream "github.com/anthropics/anthropic-sdk-go/packages/ssestream"
	"github.com/gin-gonic/gin"
	"github.com/sirupsen/logrus"

	"github.com/tingly-dev/tingly-box/internal/protocol"
)

type AnthropicToOpenAIToolCall struct {
	ID        string
	Name      string
	Arguments string
}

type AnthropicToOpenAIMCPHooks struct {
	ShouldSuppressTool func(name string) bool
	OnToolCallsFinal   func(calls []AnthropicToOpenAIToolCall) error
}

// AnthropicToOpenAIStream processes Anthropic streaming events and converts them to OpenAI format.
// Returns the normalized TokenUsage (input/output plus cache-read and reasoning
// tokens) and an error for usage tracking. Returning the full usage — rather
// than just input/output — keeps cache tokens out of the dropped column when
// the recorded usage is persisted on the conversion path.
func AnthropicToOpenAIStream(hc *protocol.HandleContext, req *anthropic.BetaMessageNewParams, stream *anthropicstream.Stream[anthropic.BetaRawMessageStreamEventUnion], responseModel string, disableStreamUsage bool) (*protocol.TokenUsage, error) {
	return AnthropicToOpenAIStreamWithMCPHooks(hc, req, stream, responseModel, disableStreamUsage, nil)
}

func AnthropicToOpenAIStreamWithMCPHooks(hc *protocol.HandleContext, req *anthropic.BetaMessageNewParams, stream *anthropicstream.Stream[anthropic.BetaRawMessageStreamEventUnion], responseModel string, disableStreamUsage bool, hooks *AnthropicToOpenAIMCPHooks) (*protocol.TokenUsage, error) {
	c := hc.GinContext
	logrus.WithContext(c.Request.Context()).Info("Starting Anthropic to OpenAI streaming response handler")
	defer func() {
		if r := recover(); r != nil {
			logrus.WithContext(c.Request.Context()).Errorf("Panic in Anthropic to OpenAI streaming handler: %v", r)
			if c.Writer != nil {
				c.Writer.WriteHeader(http.StatusInternalServerError)
				sendOpenAIStreamError(c, "Internal streaming error", "internal_error")
			}
		}
		if stream != nil {
			if err := stream.Close(); err != nil {
				logrus.WithContext(c.Request.Context()).Errorf("Error closing Anthropic stream: %v", err)
			}
		}
		logrus.WithContext(c.Request.Context()).Info("Finished Anthropic to OpenAI streaming response handler")
	}()

	conv := newAnthropicToOpenAIConverter(stream, responseModel, disableStreamUsage, hooks)
	usage, err := RunConverter(hc, conv, openaiChatSSEWriter(c))

	// MCP continuation: hook requested the stream to be retried
	if hookErr := conv.HookErr(); errors.Is(hookErr, ErrMCPStreamContinue) {
		return usage, hookErr
	}

	if err != nil {
		if errors.Is(err, context.Canceled) {
			logrus.WithContext(c.Request.Context()).Debug("Anthropic to OpenAI stream canceled by client")
			return usage, nil
		}
		if errors.Is(err, io.EOF) {
			logrus.WithContext(c.Request.Context()).Info("Anthropic stream ended normally (EOF)")
			return usage, nil
		}
		logrus.WithContext(c.Request.Context()).Errorf("Anthropic stream error: %v", err)
		streamErr := fmt.Errorf("anthropic stream error: %w", err)
		hc.DispatchStreamError(streamErr)
		if !c.Writer.Written() {
			SendStreamingError(c, err)
			return usage, streamErr
		}
		sendOpenAIStreamError(c, err.Error(), "stream_error")
		return usage, streamErr
	}

	if err := stream.Err(); err != nil {
		if errors.Is(err, context.Canceled) {
			logrus.WithContext(c.Request.Context()).Debug("Anthropic to OpenAI stream canceled by client")
			return usage, nil
		}
		if errors.Is(err, io.EOF) {
			return usage, nil
		}
		logrus.WithContext(c.Request.Context()).Errorf("Anthropic stream error: %v", err)
		streamErr := fmt.Errorf("anthropic stream error: %w", err)
		hc.DispatchStreamError(streamErr)
		if !c.Writer.Written() {
			SendStreamingError(c, err)
			return usage, streamErr
		}
		sendOpenAIStreamError(c, err.Error(), "stream_error")
		return usage, streamErr
	}

	OpenAISSEDone(c)
	return usage, nil
}

// sendOpenAIStreamChunkForce helper function to send a chunk in OpenAI format
func sendOpenAIStreamChunkForce(c *gin.Context, chunk map[string]interface{}) {
	OpenAISSE(c, chunk)
}

// sendOpenAIStreamError sends an error chunk in OpenAI format
func sendOpenAIStreamError(c *gin.Context, message, errorType string) {
	errorMap := map[string]interface{}{
		"error": map[string]interface{}{
			"message": message,
			"type":    errorType,
		},
	}
	OpenAISSE(c, errorMap)
}
