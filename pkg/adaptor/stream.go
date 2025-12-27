package adaptor

import (
	"encoding/json"
	"errors"
	"fmt"
	"net/http"
	"strings"
	"time"

	"github.com/anthropics/anthropic-sdk-go"
	anthropicstream "github.com/anthropics/anthropic-sdk-go/packages/ssestream"
	"github.com/gin-gonic/gin"
	"github.com/openai/openai-go/v3"
	openaistream "github.com/openai/openai-go/v3/packages/ssestream"
	"github.com/sirupsen/logrus"
)

// HandleAnthropicToOpenAIStreamResponse processes Anthropic streaming events and converts them to OpenAI format
func HandleAnthropicToOpenAIStreamResponse(c *gin.Context, stream *anthropicstream.Stream[anthropic.MessageStreamEventUnion], responseModel string) error {
	logrus.Info("Starting Anthropic to OpenAI streaming response handler")
	defer func() {
		if r := recover(); r != nil {
			logrus.Errorf("Panic in Anthropic to OpenAI streaming handler: %v", r)
			// Try to send an error event if possible
			if c.Writer != nil {
				c.Writer.WriteHeader(http.StatusInternalServerError)
				c.Writer.Write([]byte("data: {\"error\":{\"message\":\"Internal streaming error\",\"type\":\"internal_error\"}}\n\n"))
				if flusher, ok := c.Writer.(http.Flusher); ok {
					flusher.Flush()
				}
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

	// Create a flusher to ensure immediate sending of data
	flusher, ok := c.Writer.(http.Flusher)
	if !ok {

		return errors.New("Streaming not supported by this connection")
	}

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
			sendOpenAIStreamChunk(c, chunk, flusher)

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
				sendOpenAIStreamChunk(c, chunk, flusher)
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

			sendOpenAIStreamChunk(c, chunk, flusher)
			// Send final [DONE] message
			c.Writer.Write([]byte("data: [DONE]\n\n"))
			flusher.Flush()
			return nil
		}
	}

	// Check for stream errors
	if err := stream.Err(); err != nil {
		logrus.Errorf("Anthropic stream error: %v", err)
		// Send error event
		errorChunk := map[string]interface{}{
			"error": map[string]interface{}{
				"message": err.Error(),
				"type":    "stream_error",
				"code":    "stream_failed",
			},
		}
		errorJSON, marshalErr := json.Marshal(errorChunk)
		if marshalErr != nil {
			logrus.Errorf("Failed to marshal error chunk: %v", marshalErr)
			c.Writer.Write([]byte("data: {\"error\":{\"message\":\"Failed to marshal error\",\"type\":\"internal_error\"}}\n\n"))
		} else {
			c.Writer.Write([]byte(fmt.Sprintf("data: %s\n\n", string(errorJSON))))
		}
		flusher.Flush()
		return nil
	}

	return nil
}

// sendOpenAIStreamChunk helper function to send a chunk in OpenAI format
func sendOpenAIStreamChunk(c *gin.Context, chunk map[string]interface{}, flusher http.Flusher) {
	chunkJSON, err := json.Marshal(chunk)
	if err != nil {
		logrus.Errorf("Failed to marshal OpenAI stream chunk: %v", err)
		return
	}
	c.Writer.Write([]byte(fmt.Sprintf("data: %s\n\n", string(chunkJSON))))
	flusher.Flush()
}

// HandleOpenAIToAnthropicStreamResponse processes OpenAI streaming events and converts them to Anthropic format
func HandleOpenAIToAnthropicStreamResponse(c *gin.Context, stream *openaistream.Stream[openai.ChatCompletionChunk], responseModel string) error {
	logrus.Info("Starting OpenAI to Anthropic streaming response handler")
	defer func() {
		if r := recover(); r != nil {
			logrus.Errorf("Panic in OpenAI to Anthropic streaming handler: %v", r)
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
				logrus.Errorf("Error closing OpenAI stream: %v", err)
			}
		}
		logrus.Info("Finished OpenAI to Anthropic streaming response handler")
	}()

	// Set SSE headers
	c.Header("Content-Type", "text/event-stream")
	c.Header("Cache-Control", "no-cache")
	c.Header("Connection", "keep-alive")
	c.Header("Access-Control-Allow-Origin", "*")
	c.Header("Access-Control-Allow-Headers", "Cache-Control")

	flusher, ok := c.Writer.(http.Flusher)
	if !ok {

		return errors.New("Streaming not supported by this connection")
	}

	// Generate message ID for Anthropic format
	messageID := fmt.Sprintf("msg_%d", time.Now().Unix())

	// Track streaming state
	var (
		sentContentBlockStart bool
		contentIndex          = 0
		outputTokens          int64
		// Track tool call state
		pendingToolCalls = make(map[int]*pendingToolCall)
	)

	// Send message_start event first
	messageStartEvent := map[string]interface{}{
		"type": "message_start",
		"message": map[string]interface{}{
			"id":            messageID,
			"type":          "message",
			"role":          "assistant",
			"content":       []interface{}{},
			"model":         responseModel,
			"stop_reason":   nil,
			"stop_sequence": nil,
			"usage": map[string]interface{}{
				"input_tokens":  0,
				"output_tokens": 0,
			},
		},
	}
	sendAnthropicStreamEvent(c, "message_start", messageStartEvent, flusher)

	// Process the stream
	for stream.Next() {
		chunk := stream.Current()

		// Check if we have choices
		if len(chunk.Choices) == 0 {
			// Check for usage info in the last chunk
			if chunk.Usage.PromptTokens > 0 || chunk.Usage.CompletionTokens > 0 {
				outputTokens = chunk.Usage.CompletionTokens
			}
			continue
		}

		choice := chunk.Choices[0]
		delta := choice.Delta

		// Handle role delta (first chunk)
		if delta.Role != "" && !sentContentBlockStart {
			// Send content_block_start for text content
			contentBlockStartEvent := map[string]interface{}{
				"type":  "content_block_start",
				"index": contentIndex,
				"content_block": map[string]interface{}{
					"type": "text",
					"text": "",
				},
			}
			sendAnthropicStreamEvent(c, "content_block_start", contentBlockStartEvent, flusher)
			sentContentBlockStart = true
		}

		// Handle content delta
		if delta.Content != "" {
			if !sentContentBlockStart {
				// Send content_block_start first if not sent
				contentBlockStartEvent := map[string]interface{}{
					"type":  "content_block_start",
					"index": contentIndex,
					"content_block": map[string]interface{}{
						"type": "text",
						"text": "",
					},
				}
				sendAnthropicStreamEvent(c, "content_block_start", contentBlockStartEvent, flusher)
				sentContentBlockStart = true
			}

			// Send content_block_delta
			deltaEvent := map[string]interface{}{
				"type":  "content_block_delta",
				"index": contentIndex,
				"delta": map[string]interface{}{
					"type": "text_delta",
					"text": delta.Content,
				},
			}
			sendAnthropicStreamEvent(c, "content_block_delta", deltaEvent, flusher)
		}

		// Handle tool_calls delta
		if len(delta.ToolCalls) > 0 {
			for _, toolCall := range delta.ToolCalls {
				index := int(toolCall.Index)

				// Initialize pending tool call if not exists
				if pendingToolCalls[index] == nil {
					pendingToolCalls[index] = &pendingToolCall{
						id:   toolCall.ID,
						name: toolCall.Function.Name,
					}

					// Send content_block_start for tool_use
					// Note: input is omitted here, it will be sent via input_json_delta
					contentBlockStartEvent := map[string]interface{}{
						"type":  "content_block_start",
						"index": index,
						"content_block": map[string]interface{}{
							"type": "tool_use",
							"id":   toolCall.ID,
							"name": toolCall.Function.Name,
						},
					}
					sendAnthropicStreamEvent(c, "content_block_start", contentBlockStartEvent, flusher)
					if index >= contentIndex {
						contentIndex = index + 1
					}
				}

				// Accumulate arguments and send delta
				if toolCall.Function.Arguments != "" {
					pendingToolCalls[index].input += toolCall.Function.Arguments

					// Send content_block_delta with input_json_delta
					deltaEvent := map[string]interface{}{
						"type":  "content_block_delta",
						"index": index,
						"delta": map[string]interface{}{
							"type":         "input_json_delta",
							"partial_json": toolCall.Function.Arguments,
						},
					}
					sendAnthropicStreamEvent(c, "content_block_delta", deltaEvent, flusher)
				}
			}
		}

		// Track usage from chunk
		if chunk.Usage.CompletionTokens > 0 {
			outputTokens = chunk.Usage.CompletionTokens
		}

		// Handle finish_reason (last chunk for this choice)
		if choice.FinishReason != "" {
			// Send content_block_stop for text content if started
			if sentContentBlockStart {
				contentBlockStopEvent := map[string]interface{}{
					"type":  "content_block_stop",
					"index": 0,
				}
				sendAnthropicStreamEvent(c, "content_block_stop", contentBlockStopEvent, flusher)
			}

			// Send content_block_stop for each tool call
			for i := range pendingToolCalls {
				contentBlockStopEvent := map[string]interface{}{
					"type":  "content_block_stop",
					"index": i,
				}
				sendAnthropicStreamEvent(c, "content_block_stop", contentBlockStopEvent, flusher)
			}

			// Map OpenAI finish_reason to Anthropic stop_reason
			stopReason := "end_turn"
			switch choice.FinishReason {
			case "stop":
				stopReason = "end_turn"
			case "length":
				stopReason = "max_tokens"
			case "tool_calls":
				stopReason = "tool_use"
			case "content_filter":
				stopReason = "content_filter"
			}

			// Send message_delta with stop_reason and usage
			messageDeltaEvent := map[string]interface{}{
				"type": "message_delta",
				"delta": map[string]interface{}{
					"stop_reason":   stopReason,
					"stop_sequence": nil,
				},
				"usage": map[string]interface{}{
					"output_tokens": outputTokens,
				},
			}
			sendAnthropicStreamEvent(c, "message_delta", messageDeltaEvent, flusher)

			// Send message_stop
			messageStopEvent := map[string]interface{}{
				"type": "message_stop",
			}
			sendAnthropicStreamEvent(c, "message_stop", messageStopEvent, flusher)
			return nil
		}
	}

	// Check for stream errors
	if err := stream.Err(); err != nil {
		logrus.Errorf("OpenAI stream error: %v", err)
		errorEvent := map[string]interface{}{
			"type": "error",
			"error": map[string]interface{}{
				"message": err.Error(),
				"type":    "stream_error",
				"code":    "stream_failed",
			},
		}
		sendAnthropicStreamEvent(c, "error", errorEvent, flusher)
		return nil
	}
	return nil
}

// sendAnthropicStreamEvent helper function to send an event in Anthropic SSE format
func sendAnthropicStreamEvent(c *gin.Context, eventType string, eventData map[string]interface{}, flusher http.Flusher) {
	eventJSON, err := json.Marshal(eventData)
	if err != nil {
		logrus.Errorf("Failed to marshal Anthropic stream event: %v", err)
		return
	}

	// Anthropic SSE format: event: <type>\ndata: <json>\n\n
	c.Writer.Write([]byte(fmt.Sprintf("event: %s\ndata: %s\n\n", eventType, string(eventJSON))))
	flusher.Flush()
}

// pendingToolCall tracks a tool call being assembled from stream chunks
type pendingToolCall struct {
	id    string
	name  string
	input string
}
