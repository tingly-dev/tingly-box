package adaptor

import (
	"encoding/json"
	"errors"
	"fmt"
	"net/http"
	"sort"
	"time"

	"github.com/anthropics/anthropic-sdk-go"
	"github.com/gin-gonic/gin"
	"github.com/openai/openai-go/v3"
	openaistream "github.com/openai/openai-go/v3/packages/ssestream"
	"github.com/sirupsen/logrus"
)

// HandleOpenAIToAnthropicV1BetaStreamResponse processes OpenAI streaming events and converts them to Anthropic beta format
func HandleOpenAIToAnthropicV1BetaStreamResponse(c *gin.Context, req *openai.ChatCompletionNewParams, stream *openaistream.Stream[openai.ChatCompletionChunk], responseModel string) error {
	logrus.Info("Starting OpenAI to Anthropic beta streaming response handler")
	defer func() {
		if r := recover(); r != nil {
			logrus.Errorf("Panic in OpenAI to Anthropic beta streaming handler: %v", r)
			if c.Writer != nil {
				c.SSEvent("error", "{\"error\":{\"message\":\"Internal streaming error\",\"type\":\"internal_error\"}}")
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
		logrus.Info("Finished OpenAI to Anthropic beta streaming response handler")
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

	// Generate message ID for Anthropic beta format
	messageID := fmt.Sprintf("msg_%d", time.Now().Unix())

	// Initialize streaming state
	state := newStreamState()

	// Send message_start event first
	messageStartEvent := map[string]interface{}{
		"type": eventTypeMessageStart,
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
	sendAnthropicBetaStreamEvent(c, eventTypeMessageStart, messageStartEvent, flusher)

	// Process the stream
	chunkCount := 0
	for stream.Next() {
		chunkCount++
		chunk := stream.Current()

		// Skip empty chunks (no choices)
		if len(chunk.Choices) == 0 {
			// Check for usage info in the last chunk
			if chunk.Usage.PromptTokens > 0 || chunk.Usage.CompletionTokens > 0 {
				state.inputTokens = chunk.Usage.PromptTokens
				state.outputTokens = chunk.Usage.CompletionTokens
			}
			continue
		}

		choice := chunk.Choices[0]

		logrus.Debugf("Processing chunk #%d: len(choices)=%d, content=%q, finish_reason=%q",
			chunkCount, len(chunk.Choices),
			choice.Delta.Content, choice.FinishReason)

		delta := choice.Delta

		// Collect extra fields from this delta (for final message_delta)
		// Handle special fields that need dedicated content blocks
		if extras := parseRawJSON(delta.RawJSON()); extras != nil {
			for k, v := range extras {
				// Handle reasoning_content -> thinking block
				if k == openaiFieldReasoningContent {
					// Initialize thinking block on first occurrence
					if state.thinkingBlockIndex == -1 {
						state.thinkingBlockIndex = state.nextBlockIndex
						state.nextBlockIndex++
						sendBetaContentBlockStart(c, state.thinkingBlockIndex, blockTypeThinking, map[string]interface{}{
							"thinking": "",
						}, flusher)
					}

					// Extract thinking content (handle different types)
					thinkingText := extractString(v)
					if thinkingText != "" {
						// Send content_block_delta with thinking_delta
						sendBetaContentBlockDelta(c, state.thinkingBlockIndex, map[string]interface{}{
							"type":     deltaTypeThinkingDelta,
							"thinking": thinkingText,
						}, flusher)
					}

					// Don't add to deltaExtras (already handled as thinking block)
					continue
				}

				// Other extra fields: collect for final message_delta
				state.deltaExtras[k] = v
			}
		}

		// Handle refusal (when model refuses to respond due to safety policies)
		if delta.Refusal != "" {
			// Refusal should be sent as content
			if state.textBlockIndex == -1 {
				state.textBlockIndex = state.nextBlockIndex
				state.nextBlockIndex++
				sendBetaContentBlockStart(c, state.textBlockIndex, blockTypeText, map[string]interface{}{
					"text": "",
				}, flusher)
			}
			state.hasTextContent = true

			sendBetaContentBlockDelta(c, state.textBlockIndex, map[string]interface{}{
				"type": deltaTypeTextDelta,
				"text": delta.Refusal,
			}, flusher)
		}

		// Handle content delta
		if delta.Content != "" {
			state.hasTextContent = true

			// Initialize text block on first content
			if state.textBlockIndex == -1 {
				state.textBlockIndex = state.nextBlockIndex
				state.nextBlockIndex++
				sendBetaContentBlockStart(c, state.textBlockIndex, blockTypeText, map[string]interface{}{
					"text": "",
				}, flusher)
			}

			// Parse delta raw JSON to get extra fields
			currentExtras := parseRawJSON(delta.RawJSON())
			currentExtras = filterSpecialFields(currentExtras)

			// Send content_block_delta with actual content
			deltaMap := map[string]interface{}{
				"type": deltaTypeTextDelta,
				"text": delta.Content,
			}
			deltaMap = mergeMaps(deltaMap, currentExtras)
			sendBetaContentBlockDelta(c, state.textBlockIndex, deltaMap, flusher)
		} else if choice.FinishReason == "" && state.textBlockIndex != -1 {
			// Send empty delta for empty chunks to keep client informed
			// Only if text block has been initialized
			currentExtras := parseRawJSON(delta.RawJSON())
			currentExtras = filterSpecialFields(currentExtras)

			deltaMap := map[string]interface{}{
				"type": deltaTypeTextDelta,
				"text": "",
			}
			deltaMap = mergeMaps(deltaMap, currentExtras)
			sendBetaContentBlockDelta(c, state.textBlockIndex, deltaMap, flusher)
		}

		// Handle tool_calls delta
		if len(delta.ToolCalls) > 0 {
			for _, toolCall := range delta.ToolCalls {
				openaiIndex := int(toolCall.Index)

				// Map OpenAI tool index to Anthropic block index
				anthropicIndex, exists := state.toolIndexToBlockIndex[openaiIndex]
				if !exists {
					anthropicIndex = state.nextBlockIndex
					state.toolIndexToBlockIndex[openaiIndex] = anthropicIndex
					state.nextBlockIndex++

					// Initialize pending tool call
					state.pendingToolCalls[anthropicIndex] = &pendingToolCall{
						id:   toolCall.ID,
						name: toolCall.Function.Name,
					}

					// Send content_block_start for tool_use
					sendBetaContentBlockStart(c, anthropicIndex, blockTypeToolUse, map[string]interface{}{
						"id":   toolCall.ID,
						"name": toolCall.Function.Name,
					}, flusher)
				}

				// Accumulate arguments and send delta
				if toolCall.Function.Arguments != "" {
					state.pendingToolCalls[anthropicIndex].input += toolCall.Function.Arguments

					// Send content_block_delta with input_json_delta
					sendBetaContentBlockDelta(c, anthropicIndex, map[string]interface{}{
						"type":         deltaTypeInputJSONDelta,
						"partial_json": toolCall.Function.Arguments,
					}, flusher)
				}
			}
		}

		// Track usage from chunk
		if chunk.Usage.CompletionTokens > 0 {
			state.inputTokens = chunk.Usage.PromptTokens
			state.outputTokens = chunk.Usage.CompletionTokens
		}

		// Handle finish_reason (last chunk for this choice)
		if choice.FinishReason != "" {
			sendBetaStopEvents(c, state, flusher)
			sendBetaMessageDelta(c, state, mapOpenAIFinishReasonToAnthropicBeta(choice.FinishReason), flusher)
			sendBetaMessageStop(c, messageID, responseModel, state, mapOpenAIFinishReasonToAnthropicBeta(choice.FinishReason), flusher)
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
		sendAnthropicBetaStreamEvent(c, "error", errorEvent, flusher)
		return err
	}
	return nil
}

// sendAnthropicBetaStreamEvent helper function to send an event in Anthropic beta SSE format
func sendAnthropicBetaStreamEvent(c *gin.Context, eventType string, eventData map[string]interface{}, flusher http.Flusher) {
	eventJSON, err := json.Marshal(eventData)
	if err != nil {
		logrus.Errorf("Failed to marshal Anthropic beta stream event: %v", err)
		return
	}

	// Anthropic beta SSE format: event: <type>\ndata: <json>\n\n
	c.SSEvent(eventType, string(eventJSON))
	flusher.Flush()
}

// sendBetaContentBlockStart sends a content_block_start event for beta
func sendBetaContentBlockStart(c *gin.Context, index int, blockType string, initialContent map[string]interface{}, flusher http.Flusher) {
	contentBlock := map[string]interface{}{
		"type": blockType,
	}
	for k, v := range initialContent {
		contentBlock[k] = v
	}

	event := map[string]interface{}{
		"type":          eventTypeContentBlockStart,
		"index":         index,
		"content_block": contentBlock,
	}
	sendAnthropicBetaStreamEvent(c, eventTypeContentBlockStart, event, flusher)
}

// sendBetaContentBlockDelta sends a content_block_delta event for beta
func sendBetaContentBlockDelta(c *gin.Context, index int, content map[string]interface{}, flusher http.Flusher) {
	event := map[string]interface{}{
		"type":  eventTypeContentBlockDelta,
		"index": index,
		"delta": content,
	}
	sendAnthropicBetaStreamEvent(c, eventTypeContentBlockDelta, event, flusher)
}

// sendBetaContentBlockStop sends a content_block_stop event for beta
func sendBetaContentBlockStop(c *gin.Context, index int, flusher http.Flusher) {
	event := map[string]interface{}{
		"type":  eventTypeContentBlockStop,
		"index": index,
	}
	sendAnthropicBetaStreamEvent(c, eventTypeContentBlockStop, event, flusher)
}

// mapOpenAIFinishReasonToAnthropicBeta converts OpenAI finish_reason to Anthropic beta stop_reason
func mapOpenAIFinishReasonToAnthropicBeta(finishReason string) string {
	switch finishReason {
	case string(openai.CompletionChoiceFinishReasonStop):
		return string(anthropic.BetaStopReasonEndTurn)
	case string(openai.CompletionChoiceFinishReasonLength):
		return string(anthropic.BetaStopReasonMaxTokens)
	case openaiFinishReasonToolCalls:
		return string(anthropic.BetaStopReasonToolUse)
	case string(openai.CompletionChoiceFinishReasonContentFilter):
		return string(anthropic.BetaStopReasonRefusal)
	default:
		return string(anthropic.BetaStopReasonEndTurn)
	}
}

// sendBetaStopEvents sends content_block_stop events for all active blocks in index order (beta)
func sendBetaStopEvents(c *gin.Context, state *streamState, flusher http.Flusher) {
	// Collect block indices to stop
	var blockIndices []int
	if state.thinkingBlockIndex != -1 {
		blockIndices = append(blockIndices, state.thinkingBlockIndex)
	}
	if state.hasTextContent {
		blockIndices = append(blockIndices, state.textBlockIndex)
	}
	for i := range state.pendingToolCalls {
		blockIndices = append(blockIndices, i)
	}

	// Sort by index to stop in order
	sort.Ints(blockIndices)

	// Send stop events in sorted order
	for _, idx := range blockIndices {
		sendBetaContentBlockStop(c, idx, flusher)
	}
}

// sendBetaMessageDelta sends message_delta event for beta
func sendBetaMessageDelta(c *gin.Context, state *streamState, stopReason string, flusher http.Flusher) {
	// Build delta with accumulated extras
	deltaMap := map[string]interface{}{
		"stop_reason":   stopReason,
		"stop_sequence": nil,
	}
	// Merge all collected extra fields
	for k, v := range state.deltaExtras {
		deltaMap[k] = v
	}

	event := map[string]interface{}{
		"type":  eventTypeMessageDelta,
		"delta": deltaMap,
		"usage": map[string]interface{}{
			"output_tokens": state.outputTokens,
			"input_tokens":  state.inputTokens,
		},
	}
	sendAnthropicBetaStreamEvent(c, eventTypeMessageDelta, event, flusher)
}

// sendBetaMessageStop sends message_stop event for beta
func sendBetaMessageStop(c *gin.Context, messageID, model string, state *streamState, stopReason string, flusher http.Flusher) {
	// Send message_stop with detailed data
	messageData := map[string]interface{}{
		"id":            messageID,
		"type":          "message",
		"role":          "assistant",
		"content":       []interface{}{},
		"model":         model,
		"stop_reason":   stopReason,
		"stop_sequence": nil,
		"usage": map[string]interface{}{
			"input_tokens":  state.inputTokens,
			"output_tokens": state.outputTokens,
		},
	}
	event := map[string]interface{}{
		"type":    eventTypeMessageStop,
		"message": messageData,
	}
	sendAnthropicBetaStreamEvent(c, eventTypeMessageStop, event, flusher)

	// Send final simple data with type (without event, aka empty)
	c.SSEvent("", map[string]interface{}{"type": eventTypeMessageStop})
	flusher.Flush()
}
