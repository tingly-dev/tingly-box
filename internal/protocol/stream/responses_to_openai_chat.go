package stream

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"time"

	"github.com/gin-gonic/gin"
	responsesstream "github.com/openai/openai-go/v3/packages/ssestream"
	"github.com/openai/openai-go/v3/responses"
	"github.com/sirupsen/logrus"

	"github.com/tingly-dev/tingly-box/internal/protocol"
)

// responsesToChatState tracks the streaming conversion state from Responses API to Chat Completions
type responsesToChatState struct {
	chatID         string
	createdAt      int64
	accumulated    string
	inputTokens    int64
	outputTokens   int64
	cacheTokens    int64
	hasSentCreated bool
}

// HandleResponsesToOpenAIChatStream converts Responses API streaming to Chat Completions format.
// Returns TokenUsage containing token usage information for tracking.
func HandleResponsesToOpenAIChatStream(
	hc *protocol.HandleContext,
	stream *responsesstream.Stream[responses.ResponseStreamEventUnion],
	responseModel string,
) (*protocol.TokenUsage, error) {
	c := hc.GinContext
	logrus.Debug("Starting Responses to Chat streaming conversion handler")
	defer func() {
		if r := recover(); r != nil {
			logrus.Errorf("Panic in Responses to Chat streaming handler: %v", r)
			if c.Writer != nil {
				c.Writer.WriteHeader(http.StatusInternalServerError)
				c.Writer.Write([]byte("data: {\"error\":{\"message\":\"Internal streaming error\",\"type\":\"internal_error\"}}\n\n"))
				if flusher, ok := c.Writer.(http.Flusher); ok {
					flusher.Flush()
				}
			}
		}
		if stream != nil {
			if err := stream.Close(); err != nil {
				logrus.Errorf("Error closing Responses stream: %v", err)
			}
		}
		logrus.Info("Finished Responses to Chat streaming conversion handler")
	}()

	// Set SSE headers for Chat Completions
	c.Header("Content-Type", "text/event-stream")
	c.Header("Cache-Control", "no-cache")
	c.Header("Connection", "keep-alive")
	c.Header("Access-Control-Allow-Origin", "*")
	c.Header("Access-Control-Allow-Headers", "Cache-Control")

	flusher, ok := c.Writer.(http.Flusher)
	if !ok {
		return protocol.ZeroTokenUsage(), errors.New("streaming not supported by this connection")
	}

	state := &responsesToChatState{
		createdAt: time.Now().Unix(),
	}

	// Trigger stream event hook
	for _, hook := range hc.OnStreamEventHooks {
		if err := hook(nil); err != nil {
			logrus.Errorf("Stream event hook error: %v", err)
		}
	}

	// Process the stream
	c.Stream(func(w io.Writer) bool {
		select {
		case <-c.Request.Context().Done():
			logrus.Debug("Client disconnected, stopping Responses to Chat stream")
			return false
		default:
		}

		if !stream.Next() {
			return false
		}

		evt := stream.Current()

		switch evt.Type {
		case "response.created":
			// Extract response ID and timestamp
			state.chatID = evt.Response.ID
			if !state.hasSentCreated {
				// Send initial chat.chunk with created event
				chunk := map[string]interface{}{
					"id":      state.chatID,
					"object":  "chat.completion.chunk",
					"created": state.createdAt,
					"model":   responseModel,
					"choices": []map[string]interface{}{
						{
							"index": 0,
							"delta": map[string]interface{}{
								"role": "assistant",
							},
							"finish_reason": nil,
						},
					},
				}
				writeSSEChunk(c, flusher, chunk)
				state.hasSentCreated = true
			}

		case "response.output_text.delta":
			// Text delta - send as chat chunk
			state.accumulated += evt.Text
			chunk := map[string]interface{}{
				"id":      state.chatID,
				"object":  "chat.completion.chunk",
				"created": state.createdAt,
				"model":   responseModel,
				"choices": []map[string]interface{}{
					{
						"index": 0,
						"delta": map[string]interface{}{
							"content": evt.Text,
						},
						"finish_reason": nil,
					},
				},
			}
			writeSSEChunk(c, flusher, chunk)

		case "response.output_text.done":
			// Text output is complete - handled in response.completed

		case "response.completed":
			// Response is complete, send final chunk with usage
			state.inputTokens = evt.Response.Usage.InputTokens
			state.outputTokens = evt.Response.Usage.OutputTokens
			state.cacheTokens = evt.Response.Usage.InputTokensDetails.CachedTokens

			// Send final chunk with finish_reason and usage
			finalChunk := map[string]interface{}{
				"id":      state.chatID,
				"object":  "chat.completion.chunk",
				"created": state.createdAt,
				"model":   responseModel,
				"choices": []map[string]interface{}{
					{
						"index":         0,
						"delta":         map[string]interface{}{},
						"finish_reason": "stop",
					},
				},
			}
			// Only include usage if not disabled
			if !hc.DisableStreamUsage {
				finalChunk["usage"] = map[string]interface{}{
					"prompt_tokens":     state.inputTokens,
					"completion_tokens": state.outputTokens,
					"total_tokens":      state.inputTokens + state.outputTokens,
				}
			}
			writeSSEChunk(c, flusher, finalChunk)

		case "error":
			// Handle error event
			errorChunk := map[string]interface{}{
				"error": map[string]interface{}{
					"message": evt.Message,
					"type":    "error",
					"code":    evt.Param,
				},
			}
			errorJSON, _ := json.Marshal(errorChunk)
			c.Writer.Write([]byte(fmt.Sprintf("data: %s\n\n", string(errorJSON))))
			flusher.Flush()
			return false
		}

		return true
	})

	// Check for stream errors
	if err := stream.Err(); err != nil {
		if errors.Is(err, context.Canceled) {
			logrus.Debug("Responses to Chat stream canceled by client")
			return protocol.NewTokenUsageWithCache(int(state.inputTokens), int(state.outputTokens), int(state.cacheTokens)), err
		}
		logrus.Errorf("Responses to Chat stream error: %v", err)
		return protocol.NewTokenUsageWithCache(int(state.inputTokens), int(state.outputTokens), int(state.cacheTokens)), err
	}

	// Send final [DONE] message
	c.Writer.Write([]byte("data: [DONE]\n\n"))
	flusher.Flush()

	return protocol.NewTokenUsageWithCache(int(state.inputTokens), int(state.outputTokens), int(state.cacheTokens)), nil
}

// writeSSEChunk writes a single SSE chunk
func writeSSEChunk(c *gin.Context, flusher http.Flusher, chunk map[string]interface{}) {
	jsonBytes, err := json.Marshal(chunk)
	if err != nil {
		logrus.Errorf("Failed to marshal chunk: %v", err)
		return
	}
	c.Writer.Write([]byte(fmt.Sprintf("data: %s\n\n", string(jsonBytes))))
	flusher.Flush()
}
