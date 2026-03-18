package stream

import (
	"context"
	"encoding/json"
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
// Returns inputTokens, outputTokens, and error for usage tracking
func HandleAnthropicToOpenAIStreamResponse(c *gin.Context, req *anthropic.MessageNewParams, stream *anthropicstream.Stream[anthropic.MessageStreamEventUnion], responseModel string, disableStreamUsage bool) (int, int, error) {
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
		chatID       = fmt.Sprintf("chatcmpl-%d", time.Now().Unix())
		created      = time.Now().Unix()
		contentText  = strings.Builder{}
		usage        *anthropic.MessageDeltaUsage
		inputTokens  int
		outputTokens int
		finished     bool
		// Track tool call state for proper streaming
		toolCallID   string
		toolCallName string
		toolCallArgs strings.Builder
		hasToolCalls bool
		// Track thinking state for extended thinking support
		thinkingText strings.Builder
	)

	// Process the stream with context cancellation checking
	c.Stream(func(w io.Writer) bool {
		// Check context cancellation first
		select {
		case <-c.Request.Context().Done():
			logrus.Debug("Client disconnected, stopping Anthropic to OpenAI stream")
			return false
		default:
		}

		// Try to get next event
		if !stream.Next() {
			return false
		}

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
			// Content block starting
			if event.ContentBlock.Type == "text" {
				// Reset content builder for new text block
				contentText.Reset()
			} else if event.ContentBlock.Type == "tool_use" {
				// Tool use block starting - send first tool_call chunk
				toolCallID = event.ContentBlock.ID
				toolCallName = event.ContentBlock.Name
				toolCallArgs.Reset()
				hasToolCalls = true

				// Send initial tool_call chunk with id, type, and name
				chunk := map[string]interface{}{
					"id":      chatID,
					"object":  "chat.completion.chunk",
					"created": created,
					"model":   responseModel,
					"choices": []map[string]interface{}{
						{
							"index": 0,
							"delta": map[string]interface{}{
								"tool_calls": []map[string]interface{}{
									{
										"index": 0,
										"id":    toolCallID,
										"type":  "function",
										"function": map[string]interface{}{
											"name":      toolCallName,
											"arguments": "",
										},
									},
								},
							},
							"finish_reason": nil,
						},
					},
				}
				sendOpenAIStreamChunk(c, chunk)
			} else if event.ContentBlock.Type == "thinking" {
				// Thinking block starting - reset thinking builder
				thinkingText.Reset()
				// Note: Initial thinking text from ContentBlock.Thinking is handled in first delta
			} else if event.ContentBlock.Type == "redacted_thinking" {
				// Redacted thinking - should be included as reasoning_content with placeholder
				thinkingText.Reset()
				thinkingText.WriteString("[REDACTED THINKING]")
			}

		case "content_block_delta":
			// Text, tool arguments, or thinking delta - send as OpenAI chunk
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
			} else if event.Delta.Type == "input_json_delta" && event.Delta.PartialJSON != "" {
				// Tool call arguments delta
				args := event.Delta.PartialJSON
				toolCallArgs.WriteString(args)

				// Send subsequent tool_call chunks with only arguments (no id, no name, no type)
				chunk := map[string]interface{}{
					"id":      chatID,
					"object":  "chat.completion.chunk",
					"created": created,
					"model":   responseModel,
					"choices": []map[string]interface{}{
						{
							"index": 0,
							"delta": map[string]interface{}{
								"tool_calls": []map[string]interface{}{
									{
										"index": 0,
										"function": map[string]interface{}{
											"arguments": args,
										},
									},
								},
							},
							"finish_reason": nil,
						},
					},
				}
				sendOpenAIStreamChunk(c, chunk)
			} else if event.Delta.Type == "thinking_delta" && event.Delta.Thinking != "" {
				// Thinking content delta - convert to OpenAI's reasoning_content format
				thinking := event.Delta.Thinking
				thinkingText.WriteString(thinking)

				// Send as reasoning_content in the delta (OpenAI extended thinking format)
				chunk := map[string]interface{}{
					"id":      chatID,
					"object":  "chat.completion.chunk",
					"created": created,
					"model":   responseModel,
					"choices": []map[string]interface{}{
						{
							"index": 0,
							"delta": map[string]interface{}{
								"reasoning_content": thinking,
							},
							"finish_reason": nil,
						},
					},
				}
				sendOpenAIStreamChunk(c, chunk)
			}
			// Note: signature_delta is intentionally ignored as OpenAI doesn't have an equivalent

		case "content_block_stop":
			// Content block finished - no specific action needed

		case "message_delta":
			// Message delta (includes usage info)
			if event.Usage.InputTokens != 0 || event.Usage.OutputTokens != 0 {
				usage = &event.Usage
				inputTokens = int(event.Usage.InputTokens)
				outputTokens = int(event.Usage.OutputTokens)
			}

		case "message_stop":
			// Determine the correct finish_reason
			// "tool_calls" if we had tool use, "stop" otherwise
			finishReason := "stop"
			if hasToolCalls {
				finishReason = "tool_calls"
			}

			// Send final chunk with finish_reason and usage
			delta := map[string]interface{}{}
			if hasToolCalls {
				// For tool_calls, content should be empty string (matching DeepSeek format)
				delta["content"] = ""
			}

			chunk := map[string]interface{}{
				"id":      chatID,
				"object":  "chat.completion.chunk",
				"created": created,
				"model":   responseModel,
				"choices": []map[string]interface{}{
					{
						"index":         0,
						"delta":         delta,
						"finish_reason": finishReason,
					},
				},
			}

			// Add usage if available and not disabled
			if !disableStreamUsage && usage != nil {
				chunk["usage"] = map[string]interface{}{
					"prompt_tokens":     usage.InputTokens,
					"completion_tokens": usage.OutputTokens,
					"total_tokens":      usage.InputTokens + usage.OutputTokens,
				}
			}

			sendOpenAIStreamChunk(c, chunk)
			// Send final [DONE] message
			// MENTION: must keep extra space (matching openai_chat.go:462)
			c.SSEvent("", " [DONE]")
			finished = true
			return false
		}

		return true
	})

	if finished {
		return inputTokens, outputTokens, nil
	}

	// Check for stream errors
	if err := stream.Err(); err != nil {
		// Check if it was a client cancellation
		if errors.Is(err, context.Canceled) {
			logrus.Debug("Anthropic to OpenAI stream canceled by client")
			return inputTokens, outputTokens, nil
		}
		// EOF is expected when stream ends normally
		if errors.Is(err, io.EOF) {
			logrus.Info("Anthropic stream ended normally (EOF)")
			return inputTokens, outputTokens, nil
		}
		logrus.Errorf("Anthropic stream error: %v", err)
		// Send error event in OpenAI format
		errorChunk := map[string]interface{}{
			"error": map[string]interface{}{
				"message": err.Error(),
				"type":    "stream_error",
				"code":    "stream_failed",
			},
		}
		errorJSON, marshalErr := json.Marshal(errorChunk)
		if marshalErr == nil {
			c.Writer.WriteString(fmt.Sprintf("data: %s\n\n", errorJSON))
		}
		if flusher, ok := c.Writer.(http.Flusher); ok {
			flusher.Flush()
		}
		return inputTokens, outputTokens, nil
	}

	return inputTokens, outputTokens, nil
}

// sendOpenAIStreamChunk helper function to send a chunk in OpenAI format
func sendOpenAIStreamChunk(c *gin.Context, chunk map[string]interface{}) {
	chunkJSON, err := json.Marshal(chunk)
	if err != nil {
		logrus.Errorf("Failed to marshal chunk: %v", err)
		return
	}
	// MENTION: Must keep extra space (matching openai_chat.go:365)
	c.Writer.WriteString(fmt.Sprintf("data: %s\n\n", chunkJSON))
	if flusher, ok := c.Writer.(http.Flusher); ok {
		flusher.Flush()
	}
}
