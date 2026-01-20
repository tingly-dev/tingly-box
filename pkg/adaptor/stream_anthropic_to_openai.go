package adaptor

import (
	"errors"
	"fmt"
	"io"
	"net/http"
	"strings"
	"time"

	"github.com/anthropics/anthropic-sdk-go"
	anthropicstream "github.com/anthropics/anthropic-sdk-go/packages/ssestream"
	"github.com/gin-gonic/gin"
	"github.com/sirupsen/logrus"
)

// HandleAnthropicToOpenAIStreamResponse processes Anthropic streaming events and converts them to OpenAI format
func HandleAnthropicToOpenAIStreamResponse(c *gin.Context, req *anthropic.MessageNewParams, stream *anthropicstream.Stream[anthropic.MessageStreamEventUnion], responseModel string) error {
	logrus.Info("Starting Anthropic to OpenAI streaming response handler")
	defer func() {
		if r := recover(); r != nil {
			logrus.Errorf("Panic in Anthropic to OpenAI streaming handler: %v", r)
			// Try to send an error event if possible
			if c.Writer != nil {
				c.Writer.WriteHeader(http.StatusInternalServerError)
				c.SSEvent("", map[string]interface{}{
					"error": map[string]interface{}{
						"message": "Internal streaming error",
						"type":    "internal_error",
					},
				})
			}
		}
		// Ensure stream is always closed
		if stream != nil {
			if err := stream.Close(); err != nil {
				logrus.Errorf("Error closing Anthropic stream: %v", err)
			}
		}
		logrus.Info("Finished Anthropic to OpenAI streaming response handler")
	}()

	// Set SSE headers
	c.Header("Content-Type", "text/event-stream")
	c.Header("Cache-Control", "no-cache")
	c.Header("Connection", "keep-alive")
	c.Header("Access-Control-Allow-Origin", "*")
	c.Header("Access-Control-Allow-Headers", "Cache-Control")

	// Track streaming state
	var (
		chatID      = fmt.Sprintf("chatcmpl-%d", time.Now().Unix())
		created     = time.Now().Unix()
		contentText = strings.Builder{}
		usage       *anthropic.MessageDeltaUsage
	)

	// Process the stream
	for stream.Next() {
		event := stream.Current()

		// Handle different event types
		switch event.Type {
		case "message_start":
			// Send initial chat completion chunk
			chunk := map[string]interface{}{
				"id":      chatID,
				"object":  "chat.completion.chunk",
				"created": created,
				"model":   responseModel,
				"choices": []map[string]interface{}{
					{
						"index":         0,
						"delta":         map[string]interface{}{"role": "assistant"},
						"finish_reason": nil,
					},
				},
			}
			sendOpenAIStreamChunk(c, chunk)

		case "content_block_start":
			// Content block starting (usually text)
			if event.ContentBlock.Type == "text" {
				// Reset content builder for new block
				contentText.Reset()
			}

		case "content_block_delta":
			// Text delta - send as OpenAI chunk
			if event.Delta.Type == "text_delta" && event.Delta.Text != "" {
				text := event.Delta.Text
				contentText.WriteString(text)

				chunk := map[string]interface{}{
					"id":      chatID,
					"object":  "chat.completion.chunk",
					"created": created,
					"model":   responseModel,
					"choices": []map[string]interface{}{
						{
							"index":         0,
							"delta":         map[string]interface{}{"content": text},
							"finish_reason": nil,
						},
					},
				}
				sendOpenAIStreamChunk(c, chunk)
			}

		case "content_block_stop":
			// Content block finished - no specific action needed

		case "message_delta":
			// Message delta (includes usage info)
			if event.Usage.InputTokens != 0 || event.Usage.OutputTokens != 0 {
				usage = &event.Usage
			}

		case "message_stop":
			// Send final chunk with finish_reason and usage
			chunk := map[string]interface{}{
				"id":      chatID,
				"object":  "chat.completion.chunk",
				"created": created,
				"model":   responseModel,
				"choices": []map[string]interface{}{
					{
						"index":         0,
						"delta":         map[string]interface{}{},
						"finish_reason": "stop",
					},
				},
			}

			// Add usage if available
			if usage != nil {
				chunk["usage"] = map[string]interface{}{
					"prompt_tokens":     usage.InputTokens,
					"completion_tokens": usage.OutputTokens,
					"total_tokens":      usage.InputTokens + usage.OutputTokens,
				}
			}

			sendOpenAIStreamChunk(c, chunk)
			// Send final [DONE] message
			c.SSEvent("", "[DONE]")
			return nil
		}
	}

	// Check for stream errors
	if err := stream.Err(); err != nil {
		// EOF is expected when stream ends normally
		if errors.Is(err, io.EOF) {
			logrus.Info("Anthropic stream ended normally (EOF)")
			return nil
		}
		logrus.Errorf("Anthropic stream error: %v", err)
		// Send error event
		c.SSEvent("", map[string]interface{}{
			"error": map[string]interface{}{
				"message": err.Error(),
				"type":    "stream_error",
				"code":    "stream_failed",
			},
		})
		return nil
	}

	return nil
}

// sendOpenAIStreamChunk helper function to send a chunk in OpenAI format
func sendOpenAIStreamChunk(c *gin.Context, chunk map[string]interface{}) {
	c.SSEvent("", chunk)
}
