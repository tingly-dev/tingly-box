package stream

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"time"

	"github.com/anthropics/anthropic-sdk-go"
	"github.com/anthropics/anthropic-sdk-go/shared/constant"
	"github.com/gin-gonic/gin"
	"github.com/openai/openai-go/v3"
	openaistream "github.com/openai/openai-go/v3/packages/ssestream"
	"github.com/openai/openai-go/v3/responses"
	"github.com/sirupsen/logrus"

	"github.com/tingly-dev/tingly-box/internal/protocol"
	"github.com/tingly-dev/tingly-box/internal/protocol/token"
)

// HandleOpenAIToAnthropicBetaStream processes OpenAI streaming events and converts them to Anthropic beta format.
// Returns UsageStat containing token usage information for tracking.
func HandleOpenAIToAnthropicBetaStream(c *gin.Context, req *openai.ChatCompletionNewParams, stream *openaistream.Stream[openai.ChatCompletionChunk], responseModel string) (*protocol.TokenUsage, error) {
	logrus.Info("Starting OpenAI to Anthropic beta streaming response handler")
	defer func() {
		if r := recover(); r != nil {
			logrus.Errorf("Panic in OpenAI to Anthropic beta streaming handler: %v", r)
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
		return protocol.ZeroTokenUsage(), errors.New("streaming not supported by this connection")
	}

	// Generate message ID for Anthropic beta format
	messageID := fmt.Sprintf("msg_%d", time.Now().Unix())

	// Initialize streaming state
	state := newStreamState()

	// Initialize token counter for accurate usage tracking
	tokenCounter, err := token.NewStreamTokenCounter()
	if err != nil {
		logrus.Errorf("Failed to create token counter: %v", err)
		// Continue without token counter - will fall back to estimation
		tokenCounter = nil
	}

	// Estimate input tokens from request if counter available
	var estimatedInputTokens int
	if tokenCounter != nil && req != nil {
		if inputTokens, err := token.EstimateInputTokens(req); err == nil {
			tokenCounter.SetInputTokens(inputTokens)
			estimatedInputTokens = inputTokens
		}
	}

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
				"input_tokens":  estimatedInputTokens,
				"output_tokens": 0,
			},
		},
	}
	sendAnthropicBetaStreamEvent(c, eventTypeMessageStart, messageStartEvent, flusher)

	// Process the stream with context cancellation checking
	chunkCount := 0
	c.Stream(func(w io.Writer) bool {
		// Check context cancellation first
		select {
		case <-c.Request.Context().Done():
			logrus.Debug("Client disconnected, stopping OpenAI to Anthropic beta stream")
			return false
		default:
		}

		// Try to get next chunk
		if !stream.Next() {
			return false
		}

		chunkCount++
		chunk := stream.Current()

		// Skip empty chunks (no choices)
		if len(chunk.Choices) == 0 {
			// Token counter will handle usage tracking if present in chunk
			if tokenCounter != nil {
				_, _, _ = tokenCounter.ConsumeOpenAIChunk(&chunk)
				inputTokens, outputTokens := tokenCounter.GetCounts()
				if inputTokens > 0 {
					state.inputTokens = int64(inputTokens)
				}
				if outputTokens > 0 {
					state.outputTokens = int64(outputTokens)
				}
			}
			return true
		}

		choice := chunk.Choices[0]

		logrus.Debugf("Processing chunk #%d: len(choices)=%d, content=%q, finish_reason=%q",
			chunkCount, len(chunk.Choices),
			choice.Delta.Content, choice.FinishReason)

		delta := choice.Delta

		// Check for server_tool_use at chunk level (not delta level)
		if chunk.JSON.ExtraFields != nil {
			if serverToolUse, exists := chunk.JSON.ExtraFields["server_tool_use"]; exists && serverToolUse.Valid() {
				state.deltaExtras["server_tool_use"] = serverToolUse.Raw()
			}
		}

		// Collect extra fields from this delta (for final message_delta)
		// Handle special fields that need dedicated content blocks
		if extras := parseRawJSON(delta.RawJSON()); extras != nil {
			// Filter out OpenAI protocol fields that should NOT appear in Anthropic message_delta
			extras = FilterOpenAIProtocolFields(extras)

			for k, v := range extras {
				// Handle reasoning_content -> thinking block
				if k == OpenaiFieldReasoningContent {
					// Initialize thinking block on first occurrence
					if state.thinkingBlockIndex == -1 {
						state.thinkingBlockIndex = state.nextBlockIndex
						state.nextBlockIndex++
						logrus.Debugf("[Thinking] Initializing thinking block at index %d", state.thinkingBlockIndex)
						sendBetaContentBlockStart(c, state.thinkingBlockIndex, blockTypeThinking, map[string]interface{}{
							"thinking": "",
						}, flusher)
					}

					// Extract thinking content (handle different types)
					thinkingText := extractString(v)
					if thinkingText != "" {
						preview := thinkingText
						logrus.Debugf("[Thinking] Sending thinking_delta: len=%d, preview=%q", len(thinkingText), preview)
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
			currentExtras = FilterSpecialFields(currentExtras)

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
			currentExtras = FilterSpecialFields(currentExtras)

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

					// Truncate tool call ID to meet OpenAI's 40 character limit
					truncatedID := truncateToolCallID(toolCall.ID)

					// Initialize pending tool call
					state.pendingToolCalls[anthropicIndex] = &pendingToolCall{
						id:   truncatedID,
						name: toolCall.Function.Name,
					}

					// Send content_block_start for tool_use
					sendBetaContentBlockStart(c, anthropicIndex, blockTypeToolUse, map[string]interface{}{
						"id":   truncatedID,
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

		// Track usage from chunk using token counter
		if tokenCounter != nil {
			_, _, _ = tokenCounter.ConsumeOpenAIChunk(&chunk)
			inputTokens, outputTokens := tokenCounter.GetCounts()
			if inputTokens > 0 {
				state.inputTokens = int64(inputTokens)
			}
			if outputTokens > 0 {
				state.outputTokens = int64(outputTokens)
			}
		}

		// Handle finish_reason (last chunk for this choice)
		if choice.FinishReason != "" {
			// Get final token counts from counter
			if tokenCounter != nil {
				inputTokens, outputTokens := tokenCounter.GetCounts()
				state.inputTokens = int64(inputTokens)
				state.outputTokens = int64(outputTokens)
			}

			sendBetaStopEvents(c, state, flusher)
			sendBetaMessageDelta(c, state, mapOpenAIFinishReasonToAnthropicBeta(choice.FinishReason), flusher)
			sendBetaMessageStop(c, messageID, responseModel, state, mapOpenAIFinishReasonToAnthropicBeta(choice.FinishReason), flusher)
			return false
		}

		return true
	})

	// Check for stream errors
	if err := stream.Err(); err != nil {
		// Check if it was a client cancellation
		if errors.Is(err, context.Canceled) {
			logrus.Debug("OpenAI to Anthropic beta stream canceled by client")
			return protocol.NewTokenUsageWithCache(int(state.inputTokens), int(state.outputTokens), int(state.cacheTokens)), nil
		}
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
		return protocol.NewTokenUsageWithCache(int(state.inputTokens), int(state.outputTokens), int(state.cacheTokens)), err
	}
	return protocol.NewTokenUsageWithCache(int(state.inputTokens), int(state.outputTokens), int(state.cacheTokens)), nil
}

// HandleResponsesToAnthropicBetaStream processes OpenAI Responses API streaming events and converts them to Anthropic beta format.
// This is a thin wrapper that uses the shared core logic with beta event senders.
// Returns UsageStat containing token usage information for tracking.
func HandleResponsesToAnthropicBetaStream(c *gin.Context, stream *openaistream.Stream[responses.ResponseStreamEventUnion], responseModel string) (*protocol.TokenUsage, error) {
	return handlerResponsesToAnthropicStream(c, stream, responseModel, responsesAPIEventSenders{
		SendMessageStart: func(event map[string]interface{}, flusher http.Flusher) {
			sendAnthropicBetaStreamEvent(c, eventTypeMessageStart, event, flusher)
		},
		SendContentBlockStart: func(index int, blockType string, content map[string]interface{}, flusher http.Flusher) {
			sendBetaContentBlockStart(c, index, blockType, content, flusher)
		},
		SendContentBlockDelta: func(index int, content map[string]interface{}, flusher http.Flusher) {
			sendBetaContentBlockDelta(c, index, content, flusher)
		},
		SendContentBlockStop: func(state *streamState, index int, flusher http.Flusher) {
			sendBetaContentBlockStop(c, state, index, flusher)
		},
		SendStopEvents: func(state *streamState, flusher http.Flusher) {
			sendBetaStopEvents(c, state, flusher)
		},
		SendMessageDelta: func(state *streamState, stopReason string, flusher http.Flusher) {
			sendBetaMessageDelta(c, state, stopReason, flusher)
		},
		SendMessageStop: func(messageID, model string, state *streamState, stopReason string, flusher http.Flusher) {
			sendBetaMessageStop(c, messageID, model, state, stopReason, flusher)
		},
		SendErrorEvent: func(event map[string]interface{}, flusher http.Flusher) {
			sendAnthropicBetaStreamEvent(c, "error", event, flusher)
		},
	})
}

func HandleResponsesToAnthropicBetaAssembly(c *gin.Context, stream *openaistream.Stream[responses.ResponseStreamEventUnion], responseModel string) (*protocol.TokenUsage, error) {
	blocks := make(map[int]*anthropic.BetaContentBlockUnion)

	msg := anthropic.BetaMessage{
		Type: constant.Message("message"),
		Role: constant.Assistant("assistant"),
	}

	return handlerResponsesToAnthropicStream(c, stream, responseModel, responsesAPIEventSenders{
		SendMessageStart: func(event map[string]interface{}, flusher http.Flusher) {
			if msgData, ok := event["message"].(map[string]interface{}); ok {
				if id, ok := msgData["id"].(string); ok {
					msg.ID = id
				}
				if model, ok := msgData["model"].(string); ok {
					msg.Model = anthropic.Model(model)
				}
			}
		},
		SendContentBlockStart: func(index int, blockType string, content map[string]interface{}, flusher http.Flusher) {
			block := anthropic.BetaContentBlockUnion{Type: blockType}
			if id, ok := content["id"].(string); ok {
				block.ID = id
			}
			if name, ok := content["name"].(string); ok {
				block.Name = name
			}
			blocks[index] = &block
		},
		SendContentBlockDelta: func(index int, content map[string]interface{}, flusher http.Flusher) {
			block, ok := blocks[index]
			if !ok {
				return
			}
			if deltaType, ok := content["type"].(string); ok {
				switch deltaType {
				case "text_delta":
					if text, ok := content["text"].(string); ok {
						block.Text += text
					}
				case "thinking_delta":
					if thinking, ok := content["thinking"].(string); ok {
						block.Thinking += thinking
					}
				case "input_json_delta":
					if partialJSON, ok := content["partial_json"].(string); ok {
						if block.Input == nil {
							block.Input = json.RawMessage(partialJSON)
						} else {
							block.Input = append(block.Input, partialJSON...)
						}
					}
				}
			}
			blocks[index] = block
		},
		SendContentBlockStop: func(state *streamState, index int, flusher http.Flusher) {
			if block, ok := blocks[index]; ok {
				msg.Content = append(msg.Content, *block)
				delete(blocks, index)
			}
		},
		SendStopEvents: func(state *streamState, flusher http.Flusher) {
			msg.StopReason = anthropic.BetaStopReasonEndTurn
		},
		SendMessageDelta: func(state *streamState, stopReason string, flusher http.Flusher) {
			msg.StopReason = anthropic.BetaStopReason(stopReason)
		},
		SendMessageStop: func(messageID, model string, state *streamState, stopReason string, flusher http.Flusher) {
			msg.ID = messageID
			//TODO: the id is special
			//msg.ID = fmt.Sprintf("msg_%s", uuid.New().String())
			msg.Model = anthropic.Model(model)
			msg.StopReason = anthropic.BetaStopReason(mapOpenAIFinishReasonToAnthropicBeta(stopReason))

			// Set usage
			msg.Usage.InputTokens = state.inputTokens
			msg.Usage.OutputTokens = state.outputTokens
			if state.cacheTokens > 0 {
				msg.Usage.CacheReadInputTokens = state.cacheTokens
			}

			bs, _ := json.Marshal(msg)
			logrus.Debugf("Assemble response: %s", string(bs))

			// Send result
			c.JSON(200, msg)
			flusher.Flush()
			return
		},
		SendErrorEvent: func(event map[string]interface{}, flusher http.Flusher) {
			// For error, still try to send what we have
			c.JSON(200, msg)
		},
	})
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
