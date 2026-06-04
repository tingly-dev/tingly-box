package stream

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"

	"github.com/anthropics/anthropic-sdk-go"
	anthropicstream "github.com/anthropics/anthropic-sdk-go/packages/ssestream"
	"github.com/gin-gonic/gin"
	"github.com/openai/openai-go/v3"
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

// AnthropicToOpenAIStream processes Anthropic streaming events and converts them to OpenAI format
// Returns inputTokens, outputTokens, and error for usage tracking
func AnthropicToOpenAIStream(hc *protocol.HandleContext, req *anthropic.BetaMessageNewParams, stream *anthropicstream.Stream[anthropic.BetaRawMessageStreamEventUnion], responseModel string, disableStreamUsage bool) (int, int, error) {
	return AnthropicToOpenAIStreamWithMCPHooks(hc, req, stream, responseModel, disableStreamUsage, nil)
}

func AnthropicToOpenAIStreamWithMCPHooks(hc *protocol.HandleContext, req *anthropic.BetaMessageNewParams, stream *anthropicstream.Stream[anthropic.BetaRawMessageStreamEventUnion], responseModel string, disableStreamUsage bool, hooks *AnthropicToOpenAIMCPHooks) (int, int, error) {
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
	in, out := usage.InputTokens, usage.OutputTokens

	// MCP continuation: hook requested the stream to be retried
	if hookErr := conv.HookErr(); errors.Is(hookErr, ErrMCPStreamContinue) {
		return in, out, hookErr
	}

	if err != nil {
		if errors.Is(err, context.Canceled) {
			logrus.WithContext(c.Request.Context()).Debug("Anthropic to OpenAI stream canceled by client")
			return in, out, nil
		}
		if errors.Is(err, io.EOF) {
			logrus.WithContext(c.Request.Context()).Info("Anthropic stream ended normally (EOF)")
			return in, out, nil
		}
		logrus.WithContext(c.Request.Context()).Errorf("Anthropic stream error: %v", err)
		streamErr := fmt.Errorf("anthropic stream error: %w", err)
		hc.DispatchStreamError(streamErr)
		if !c.Writer.Written() {
			SendStreamingError(c, err)
			return in, out, streamErr
		}
		sendOpenAIStreamError(c, err.Error(), "stream_error")
		return in, out, streamErr
	}

	if err := stream.Err(); err != nil {
		if errors.Is(err, context.Canceled) {
			logrus.WithContext(c.Request.Context()).Debug("Anthropic to OpenAI stream canceled by client")
			return in, out, nil
		}
		if errors.Is(err, io.EOF) {
			return in, out, nil
		}
		logrus.WithContext(c.Request.Context()).Errorf("Anthropic stream error: %v", err)
		streamErr := fmt.Errorf("anthropic stream error: %w", err)
		hc.DispatchStreamError(streamErr)
		if !c.Writer.Written() {
			SendStreamingError(c, err)
			return in, out, streamErr
		}
		sendOpenAIStreamError(c, err.Error(), "stream_error")
		return in, out, streamErr
	}

	OpenAISSEDone(c)
	hc.CallOnStreamComplete()
	return in, out, nil
}

// sendOpenAIStreamChunk sends a ChatCompletionChunk as SSE
func sendOpenAIStreamChunk(c *gin.Context, chunk openai.ChatCompletionChunk, disableStreamUsage bool) {
	chunkMap, err := chunkToMap(chunk)
	if err != nil {
		logrus.WithContext(c.Request.Context()).Errorf("Failed to convert chunk to map: %v", err)
		return
	}

	// Cursor compatibility path must not expose usage in stream chunks.
	if disableStreamUsage {
		delete(chunkMap, "usage")
	}

	OpenAISSE(c, chunkMap)
}

func chunkToMap(chunk openai.ChatCompletionChunk) (map[string]interface{}, error) {
	bytes, err := json.Marshal(chunk)
	if err != nil {
		return nil, err
	}
	var chunkMap map[string]interface{}
	if err := json.Unmarshal(bytes, &chunkMap); err != nil {
		return nil, err
	}
	return chunkMap, nil
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

// createReasoningContentChunk creates a chunk with reasoning_content field
// This is a workaround for OpenAI's extended thinking format which is not natively supported in the SDK
func createReasoningContentChunk(chatID string, created int64, model, reasoning string) openai.ChatCompletionChunk {
	chunk := openai.ChatCompletionChunk{
		ID:      chatID,
		Created: created,
		Model:   model,
		Choices: []openai.ChatCompletionChunkChoice{
			{
				Index: 0,
				Delta: openai.ChatCompletionChunkChoiceDelta{
					Content: "",
				},
			},
		},
	}

	if reasoning == "" {
		return chunk
	}

	chunkJSON, _ := json.Marshal(chunk)
	var chunkMap map[string]interface{}
	json.Unmarshal(chunkJSON, &chunkMap)

	if choices, ok := chunkMap["choices"].([]interface{}); ok && len(choices) > 0 {
		if choice, ok := choices[0].(map[string]interface{}); ok {
			if delta, ok := choice["delta"].(map[string]interface{}); ok {
				delta["reasoning_content"] = reasoning
			}
		}
	}

	updatedJSON, _ := json.Marshal(chunkMap)
	json.Unmarshal(updatedJSON, &chunk)

	return chunk
}
