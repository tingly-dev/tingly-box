package adaptor

import (
	"encoding/json"
	"errors"
	"fmt"
	"net/http"
	"time"

	"github.com/gin-gonic/gin"
	"github.com/openai/openai-go/v3"
	openaistream "github.com/openai/openai-go/v3/packages/ssestream"
	"github.com/sirupsen/logrus"
)

const (
	// OpenAI finish reasons not defined in openai package
	openaiFinishReasonToolCalls = "tool_calls"

	// Anthropic stop reasons
	anthropicStopReasonEndTurn       = "end_turn"
	anthropicStopReasonMaxTokens     = "max_tokens"
	anthropicStopReasonToolUse       = "tool_use"
	anthropicStopReasonContentFilter = "content_filter"
)

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
		// Text block state
		textBlockIndex = -1 // -1 means not initialized yet
		hasTextContent = false
		outputTokens   int64
		inputTokens    int64
		// Next available block index (auto-incremented as content blocks appear)
		nextBlockIndex = 0
		// Track tool call state
		pendingToolCalls = make(map[int]*pendingToolCall)
		// Map OpenAI tool index to Anthropic block index
		toolIndexToBlockIndex = make(map[int]int)
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
	chunkCount := 0
	for stream.Next() {
		chunkCount++
		chunk := stream.Current()

		// Skip empty chunks (no choices)
		if len(chunk.Choices) == 0 {
			// Check for usage info in the last chunk
			if chunk.Usage.PromptTokens > 0 || chunk.Usage.CompletionTokens > 0 {
				inputTokens = chunk.Usage.PromptTokens
				outputTokens = chunk.Usage.CompletionTokens
			}
			continue
		}

		choice := chunk.Choices[0]

		logrus.Debugf("Processing chunk #%d: len(choices)=%d, content=%q, finish_reason=%q",
			chunkCount, len(chunk.Choices),
			choice.Delta.Content, choice.FinishReason)

		// Log first few chunks in detail for debugging
		if chunkCount <= 5 || choice.FinishReason != "" {
			logrus.Debugf("Full chunk #%d: %+v", chunkCount, chunk)
		}

		delta := choice.Delta

		// Handle refusal (when model refuses to respond due to safety policies)
		if delta.Refusal != "" {
			// Parse delta raw JSON to get extra fields
			deltaExtras := parseRawJSON(delta.RawJSON())

			// Refusal should be sent as content
			if textBlockIndex == -1 {
				textBlockIndex = nextBlockIndex
				nextBlockIndex++

				contentBlockStartEvent := map[string]interface{}{
					"type":  "content_block_start",
					"index": textBlockIndex,
					"content_block": map[string]interface{}{
						"type": "text",
						"text": "",
					},
				}
				sendAnthropicStreamEvent(c, "content_block_start", contentBlockStartEvent, flusher)
			}
			hasTextContent = true

			deltaMap := map[string]interface{}{
				"type": "text_delta",
				"text": delta.Refusal,
			}
			deltaMap = mergeMaps(deltaMap, deltaExtras)

			deltaEvent := map[string]interface{}{
				"type":  "content_block_delta",
				"index": textBlockIndex,
				"delta": deltaMap,
			}
			sendAnthropicStreamEvent(c, "content_block_delta", deltaEvent, flusher)
		}

		// Initialize text block on first chunk with choices (even if content is empty)
		// This ensures client knows the stream is active
		if textBlockIndex == -1 {
			textBlockIndex = nextBlockIndex
			nextBlockIndex++

			// Send content_block_start for text content
			contentBlockStartEvent := map[string]interface{}{
				"type":  "content_block_start",
				"index": textBlockIndex,
				"content_block": map[string]interface{}{
					"type": "text",
					"text": "",
				},
			}
			sendAnthropicStreamEvent(c, "content_block_start", contentBlockStartEvent, flusher)
		}

		// Handle content delta
		if delta.Content != "" {
			hasTextContent = true

			// Parse delta raw JSON to get extra fields
			deltaExtras := parseRawJSON(delta.RawJSON())

			// Send content_block_delta with actual content
			deltaMap := map[string]interface{}{
				"type": "text_delta",
				"text": delta.Content,
			}
			deltaMap = mergeMaps(deltaMap, deltaExtras)

			deltaEvent := map[string]interface{}{
				"type":  "content_block_delta",
				"index": textBlockIndex,
				"delta": deltaMap,
			}
			sendAnthropicStreamEvent(c, "content_block_delta", deltaEvent, flusher)
		} else if choice.FinishReason == "" {
			// Parse delta raw JSON to get extra fields
			deltaExtras := parseRawJSON(delta.RawJSON())

			// Send empty delta for empty chunks to keep client informed
			deltaMap := map[string]interface{}{
				"type": "text_delta",
				"text": "",
			}
			deltaMap = mergeMaps(deltaMap, deltaExtras)

			deltaEvent := map[string]interface{}{
				"type":  "content_block_delta",
				"index": textBlockIndex,
				"delta": deltaMap,
			}
			sendAnthropicStreamEvent(c, "content_block_delta", deltaEvent, flusher)
		}

		// Handle tool_calls delta
		if len(delta.ToolCalls) > 0 {
			for _, toolCall := range delta.ToolCalls {
				openaiIndex := int(toolCall.Index)

				// Map OpenAI tool index to Anthropic block index
				anthropicIndex, exists := toolIndexToBlockIndex[openaiIndex]
				if !exists {
					anthropicIndex = nextBlockIndex
					toolIndexToBlockIndex[openaiIndex] = anthropicIndex
					nextBlockIndex++

					// Initialize pending tool call
					pendingToolCalls[anthropicIndex] = &pendingToolCall{
						id:   toolCall.ID,
						name: toolCall.Function.Name,
					}

					// Send content_block_start for tool_use
					contentBlockStartEvent := map[string]interface{}{
						"type":  "content_block_start",
						"index": anthropicIndex,
						"content_block": map[string]interface{}{
							"type": "tool_use",
							"id":   toolCall.ID,
							"name": toolCall.Function.Name,
						},
					}
					sendAnthropicStreamEvent(c, "content_block_start", contentBlockStartEvent, flusher)
				}

				// Accumulate arguments and send delta
				if toolCall.Function.Arguments != "" {
					pendingToolCalls[anthropicIndex].input += toolCall.Function.Arguments

					// Send content_block_delta with input_json_delta
					deltaEvent := map[string]interface{}{
						"type":  "content_block_delta",
						"index": anthropicIndex,
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
			inputTokens = chunk.Usage.PromptTokens
			outputTokens = chunk.Usage.CompletionTokens
		}

		// Handle finish_reason (last chunk for this choice)
		if choice.FinishReason != "" {
			// Send content_block_stop for text content only if we had text content
			if hasTextContent {
				contentBlockStopEvent := map[string]interface{}{
					"type":  "content_block_stop",
					"index": textBlockIndex,
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
			stopReason := mapOpenAIFinishReasonToAnthropic(choice.FinishReason)

			// Send message_delta with stop_reason and usage
			messageDeltaEvent := map[string]interface{}{
				"type": "message_delta",
				"delta": map[string]interface{}{
					"stop_reason":   stopReason,
					"stop_sequence": nil,
				},
				"usage": map[string]interface{}{
					"output_tokens": outputTokens,
					"input_tokens":  inputTokens,
				},
			}
			sendAnthropicStreamEvent(c, "message_delta", messageDeltaEvent, flusher)

			// Send message_stop with detailed data
			messageStopEvent := map[string]interface{}{
				"type": "message_stop",
				"message": map[string]interface{}{
					"id":            messageID,
					"type":          "message",
					"role":          "assistant",
					"content":       []interface{}{},
					"model":         responseModel,
					"stop_reason":   stopReason,
					"stop_sequence": nil,
					"usage": map[string]interface{}{
						"input_tokens":  inputTokens,
						"output_tokens": outputTokens,
					},
				},
				"delta": map[string]interface{}{
					"stop_reason":   stopReason,
					"stop_sequence": nil,
					"text":          "",
				},
				"usage": map[string]interface{}{
					"input_tokens":  inputTokens,
					"output_tokens": outputTokens,
				},
			}
			sendAnthropicStreamEvent(c, "message_stop", messageStopEvent, flusher)

			// Send final simple data line (without event prefix)
			c.Writer.Write([]byte("data: {\"type\":\"message_stop\"}\n\n"))
			flusher.Flush()

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

// parseRawJSON parses raw JSON string into map[string]interface{}
func parseRawJSON(rawJSON string) map[string]interface{} {
	if rawJSON == "" {
		return nil
	}
	var result map[string]interface{}
	if err := json.Unmarshal([]byte(rawJSON), &result); err != nil {
		return nil
	}
	return result
}

// mergeMaps merges extra fields into the base map
func mergeMaps(base map[string]interface{}, extra map[string]interface{}) map[string]interface{} {
	if extra == nil || len(extra) == 0 {
		return base
	}
	if base == nil {
		base = make(map[string]interface{})
	}
	for k, v := range extra {
		base[k] = v
	}
	return base
}

// pendingToolCall tracks a tool call being assembled from stream chunks
type pendingToolCall struct {
	id    string
	name  string
	input string
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
		return anthropicStopReasonContentFilter
	default:
		return anthropicStopReasonEndTurn
	}
}
