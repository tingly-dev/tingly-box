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

// HandleOpenAIToAnthropicStreamResponse processes OpenAI streaming events and converts them to Anthropic format.
// Returns UsageStat containing token usage information for tracking.
func HandleOpenAIToAnthropicStreamResponse(c *gin.Context, req *openai.ChatCompletionNewParams, stream *openaistream.Stream[openai.ChatCompletionChunk], responseModel string) (*protocol.TokenUsage, error) {
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
		return protocol.ZeroTokenUsage(), errors.New("streaming not supported by this connection")
	}

	// Generate message ID for Anthropic format
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
	sendAnthropicStreamEvent(c, eventTypeMessageStart, messageStartEvent, flusher)

	// Process the stream with context cancellation checking
	chunkCount := 0
	c.Stream(func(w io.Writer) bool {
		// Check context cancellation first
		select {
		case <-c.Request.Context().Done():
			logrus.Debug("Client disconnected, stopping OpenAI to Anthropic stream")
			return false
		default:
		}

		// Try to get next chunk
		if !stream.Next() {
			return false
		}

		chunkCount++
		chunk := stream.Current()

		logrus.Infof("[Stream] Got chunk #%d: len(choices)=%d", chunkCount, len(chunk.Choices))

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

		// Log first few chunks in detail for debugging
		if chunkCount <= 5 || choice.FinishReason != "" {
			logrus.Debugf("Full chunk #%d: %v", chunkCount, chunk)
		}

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
						sendContentBlockStart(c, state.thinkingBlockIndex, blockTypeThinking, map[string]interface{}{
							"thinking": "",
						}, flusher)
					}

					// Extract thinking content (handle different types)
					thinkingText := extractString(v)
					if thinkingText != "" {
						// Send content_block_delta with thinking_delta
						sendContentBlockDelta(c, state.thinkingBlockIndex, map[string]interface{}{
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
				sendContentBlockStart(c, state.textBlockIndex, blockTypeText, map[string]interface{}{
					"text": "",
				}, flusher)
			}
			state.hasTextContent = true

			sendContentBlockDelta(c, state.textBlockIndex, map[string]interface{}{
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
				sendContentBlockStart(c, state.textBlockIndex, blockTypeText, map[string]interface{}{
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
			sendContentBlockDelta(c, state.textBlockIndex, deltaMap, flusher)
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
			sendContentBlockDelta(c, state.textBlockIndex, deltaMap, flusher)
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
					sendContentBlockStart(c, anthropicIndex, blockTypeToolUse, map[string]interface{}{
						"id":   truncatedID,
						"name": toolCall.Function.Name,
					}, flusher)
				}

				// Accumulate arguments and send delta
				if toolCall.Function.Arguments != "" {
					state.pendingToolCalls[anthropicIndex].input += toolCall.Function.Arguments

					// Send content_block_delta with input_json_delta
					sendContentBlockDelta(c, anthropicIndex, map[string]interface{}{
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

			sendStopEvents(c, state, flusher)
			sendMessageDelta(c, state, mapOpenAIFinishReasonToAnthropic(choice.FinishReason), flusher)
			sendMessageStop(c, messageID, responseModel, state, mapOpenAIFinishReasonToAnthropic(choice.FinishReason), flusher)
			return false
		}

		return true
	})

	// Check for stream errors
	if err := stream.Err(); err != nil {
		// Check if it was a client cancellation
		if errors.Is(err, context.Canceled) {
			logrus.Debug("OpenAI to Anthropic stream canceled by client")
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
		sendAnthropicStreamEvent(c, "error", errorEvent, flusher)
		return protocol.NewTokenUsageWithCache(int(state.inputTokens), int(state.outputTokens), int(state.cacheTokens)), err
	}
	return protocol.NewTokenUsageWithCache(int(state.inputTokens), int(state.outputTokens), int(state.cacheTokens)), nil
}

// HandleResponsesToAnthropicV1Stream processes OpenAI Responses API streaming events and converts them to Anthropic v1 format.
// This is a thin wrapper that uses the shared core logic with v1 event senders.
// Returns UsageStat containing token usage information for tracking.
func HandleResponsesToAnthropicV1Stream(c *gin.Context, stream *openaistream.Stream[responses.ResponseStreamEventUnion], responseModel string) (*protocol.TokenUsage, error) {
	return handlerResponsesToAnthropicStream(c, stream, responseModel, responsesAPIEventSenders{
		SendMessageStart: func(event map[string]interface{}, flusher http.Flusher) {
			sendAnthropicStreamEvent(c, eventTypeMessageStart, event, flusher)
		},
		SendContentBlockStart: func(index int, blockType string, content map[string]interface{}, flusher http.Flusher) {
			sendContentBlockStart(c, index, blockType, content, flusher)
		},
		SendContentBlockDelta: func(index int, content map[string]interface{}, flusher http.Flusher) {
			sendContentBlockDelta(c, index, content, flusher)
		},
		SendContentBlockStop: func(state *streamState, index int, flusher http.Flusher) {
			sendContentBlockStop(c, state, index, flusher)
		},
		SendStopEvents: func(state *streamState, flusher http.Flusher) {
			sendStopEvents(c, state, flusher)
		},
		SendMessageDelta: func(state *streamState, stopReason string, flusher http.Flusher) {
			sendMessageDelta(c, state, stopReason, flusher)
		},
		SendMessageStop: func(messageID, model string, state *streamState, stopReason string, flusher http.Flusher) {
			sendMessageStop(c, messageID, model, state, stopReason, flusher)
		},
		SendErrorEvent: func(event map[string]interface{}, flusher http.Flusher) {
			sendAnthropicStreamEvent(c, "error", event, flusher)
		},
	})
}

// handlerResponsesToAnthropicStream is the shared core logic for processing OpenAI Responses API streams
// and converting them to Anthropic format (v1 or beta depending on the senders provided).
// Returns UsageStat containing token usage information for tracking.
func handlerResponsesToAnthropicStream(c *gin.Context, stream *openaistream.Stream[responses.ResponseStreamEventUnion], responseModel string, senders responsesAPIEventSenders) (*protocol.TokenUsage, error) {
	logrus.Debugf("[ResponsesAPI] Starting Responses API to Anthropic streaming response handler, model=%s", responseModel)
	defer func() {
		if r := recover(); r != nil {
			logrus.Errorf("Panic in Responses API to Anthropic streaming handler: %v", r)
			if c.Writer != nil {
				c.SSEvent("error", "{\"error\":{\"message\":\"Internal streaming error\",\"type\":\"internal_error\"}}")
				if flusher, ok := c.Writer.(http.Flusher); ok {
					flusher.Flush()
				}
			}
		}
		if stream != nil {
			if err := stream.Close(); err != nil {
				logrus.Errorf("Error closing Responses API stream: %v", err)
			}
		}
		logrus.Debug("[ResponsesAPI] Finished Responses API to Anthropic streaming response handler")
	}()

	// Set SSE headers
	c.Header("Content-Type", "text/event-stream")
	c.Header("Cache-Control", "no-cache")
	c.Header("Connection", "keep-alive")
	c.Header("Access-Control-Allow-Origin", "*")
	c.Header("Access-Control-Allow-Headers", "Cache-Control")

	flusher, ok := c.Writer.(http.Flusher)
	if !ok {
		return protocol.ZeroTokenUsage(), fmt.Errorf("streaming not supported by this connection")
	}

	// Generate message ID
	messageID := fmt.Sprintf("msg_%d", time.Now().Unix())

	// Initialize streaming state
	state := newStreamState()

	// Track tool calls by item ID for Responses API
	type pendingToolCall struct {
		blockIndex  int
		itemID      string // original item ID (used as map key)
		truncatedID string // truncated ID for OpenAI compatibility (sent to client)
		name        string
		arguments   string
	}
	pendingToolCalls := make(map[string]*pendingToolCall) // key: itemID

	// Track the last output item type to determine correct stop reason
	lastOutputItemType := "" // "text", "function_call", etc.

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
	senders.SendMessageStart(messageStartEvent, flusher)

	// Process the stream
	eventCount := 0
	for stream.Next() {
		eventCount++
		currentEvent := stream.Current()

		switch currentEvent.Type {
		case "response.created", "response.in_progress", "response.queued":
			continue

		case "response.content_part.added":
			partAdded := currentEvent.AsResponseContentPartAdded()
			if partAdded.Part.Type == "output_text" {
				if state.textBlockIndex == -1 {
					state.textBlockIndex = state.nextBlockIndex
					state.hasTextContent = true
					state.nextBlockIndex++
					senders.SendContentBlockStart(state.textBlockIndex, blockTypeText, map[string]interface{}{"text": ""}, flusher)
				}
				if partAdded.Part.Text != "" {
					senders.SendContentBlockDelta(state.textBlockIndex, map[string]interface{}{
						"type": deltaTypeTextDelta,
						"text": partAdded.Part.Text,
					}, flusher)
				}
				lastOutputItemType = "text"
			}

		case "response.output_text.delta":
			if state.textBlockIndex == -1 {
				state.textBlockIndex = state.nextBlockIndex
				state.hasTextContent = true
				state.nextBlockIndex++
				senders.SendContentBlockStart(state.textBlockIndex, blockTypeText, map[string]interface{}{"text": ""}, flusher)
			}
			textDelta := currentEvent.AsResponseOutputTextDelta()
			senders.SendContentBlockDelta(state.textBlockIndex, map[string]interface{}{
				"type": deltaTypeTextDelta,
				"text": textDelta.Delta,
			}, flusher)
			lastOutputItemType = "text"
			logrus.Debugf("Processing Responses API event #%d: type=%s", eventCount, textDelta.Delta)
		case "response.output_text.done", "response.content_part.done":
			if state.textBlockIndex != -1 {
				senders.SendContentBlockStop(state, state.textBlockIndex, flusher)
				state.textBlockIndex = -1
			}

		case "response.reasoning_text.delta":
			reasoningDelta := currentEvent.AsResponseReasoningTextDelta()
			if state.thinkingBlockIndex == -1 {
				state.thinkingBlockIndex = state.nextBlockIndex
				state.nextBlockIndex++
				logrus.Debugf("[Thinking][ResponsesAPI] Initializing thinking block at index %d", state.thinkingBlockIndex)
				senders.SendContentBlockStart(state.thinkingBlockIndex, blockTypeThinking, map[string]interface{}{"thinking": ""}, flusher)
			}
			preview := reasoningDelta.Delta
			logrus.Debugf("[Thinking][ResponsesAPI] Sending thinking_delta: len=%d, preview=%q", len(reasoningDelta.Delta), preview)
			senders.SendContentBlockDelta(state.thinkingBlockIndex, map[string]interface{}{
				"type":     deltaTypeThinkingDelta,
				"thinking": reasoningDelta.Delta,
			}, flusher)

		case "response.reasoning_text.done":
			logrus.Debugf("[Thinking][ResponsesAPI] Thinking block done at index %d", state.thinkingBlockIndex)
			if state.thinkingBlockIndex != -1 {
				senders.SendContentBlockStop(state, state.thinkingBlockIndex, flusher)
				state.thinkingBlockIndex = -1
			}

		case "response.reasoning_summary_text.delta":
			summaryDelta := currentEvent.AsResponseReasoningSummaryTextDelta()
			// Reasoning summary is converted to thinking block (per Claude Code spec)
			if state.reasoningSummaryBlockIndex == -1 {
				state.reasoningSummaryBlockIndex = state.nextBlockIndex
				state.hasTextContent = true
				state.nextBlockIndex++
				senders.SendContentBlockStart(state.reasoningSummaryBlockIndex, blockTypeThinking, map[string]interface{}{"thinking": ""}, flusher)
			}
			senders.SendContentBlockDelta(state.reasoningSummaryBlockIndex, map[string]interface{}{
				"type":     deltaTypeThinkingDelta,
				"thinking": summaryDelta.Delta,
			}, flusher)

		case "response.reasoning_summary_text.done":
			if state.reasoningSummaryBlockIndex != -1 {
				senders.SendContentBlockStop(state, state.reasoningSummaryBlockIndex, flusher)
				state.reasoningSummaryBlockIndex = -1
			}

		case "response.refusal.delta":
			refusalDelta := currentEvent.AsResponseRefusalDelta()
			// Refusal should be sent as a separate text block
			if state.refusalBlockIndex == -1 {
				state.refusalBlockIndex = state.nextBlockIndex
				state.nextBlockIndex++
				senders.SendContentBlockStart(state.refusalBlockIndex, blockTypeText, map[string]interface{}{"text": ""}, flusher)
			}
			senders.SendContentBlockDelta(state.refusalBlockIndex, map[string]interface{}{
				"type": deltaTypeTextDelta,
				"text": refusalDelta.Delta,
			}, flusher)

		case "response.refusal.done":
			if state.refusalBlockIndex != -1 {
				senders.SendContentBlockStop(state, state.refusalBlockIndex, flusher)
				state.refusalBlockIndex = -1
			}

		case "response.output_item.added":
			itemAdded := currentEvent.AsResponseOutputItemAdded()
			logrus.Debugf("item type: %s", itemAdded.Item.Type)
			switch itemAdded.Item.Type {
			case "reasoning":
				reasoningDelta := currentEvent.AsResponseReasoningTextDelta()
				if state.thinkingBlockIndex == -1 {
					state.thinkingBlockIndex = state.nextBlockIndex
					state.nextBlockIndex++
					logrus.Debugf("[Thinking][ResponsesAPI] Initializing thinking block at index %d", state.thinkingBlockIndex)
					senders.SendContentBlockStart(state.thinkingBlockIndex, blockTypeThinking, map[string]interface{}{"thinking": ""}, flusher)
				}
				preview := reasoningDelta.Delta
				logrus.Debugf("[Thinking][ResponsesAPI] Sending thinking_delta: len=%d, preview=%q", len(reasoningDelta.Delta), preview)
				senders.SendContentBlockDelta(state.thinkingBlockIndex, map[string]interface{}{
					"type":     deltaTypeThinkingDelta,
					"thinking": reasoningDelta.Delta,
				}, flusher)
			case "message":
				if state.textBlockIndex == -1 {
					state.textBlockIndex = state.nextBlockIndex
					state.hasTextContent = true
					state.nextBlockIndex++
					senders.SendContentBlockStart(state.textBlockIndex, blockTypeText, map[string]interface{}{"text": ""}, flusher)
				}
				textDelta := currentEvent.AsResponseOutputTextDelta()
				senders.SendContentBlockDelta(state.textBlockIndex, map[string]interface{}{
					"type": deltaTypeTextDelta,
					"text": textDelta.Delta,
				}, flusher)
			case "function_call", "custom_tool_call", "mcp_call":
				itemID := itemAdded.Item.ID
				// Truncate tool call ID to meet OpenAI's 40 character limit
				truncatedID := truncateToolCallID(itemID)
				blockIndex := state.nextBlockIndex
				state.nextBlockIndex++

				toolName := ""
				if itemAdded.Item.Name != "" {
					toolName = itemAdded.Item.Name
				}

				pendingToolCalls[itemID] = &pendingToolCall{
					blockIndex:  blockIndex,
					itemID:      itemID,
					truncatedID: truncatedID,
					name:        toolName,
					arguments:   "",
				}
				lastOutputItemType = "function_call"

				senders.SendContentBlockStart(blockIndex, blockTypeToolUse, map[string]interface{}{
					"id":   truncatedID,
					"name": toolName,
				}, flusher)
			default:
				logrus.Warnf("missing process for stream chunk: %s, %s", itemAdded.Type, itemAdded.Item.Type)
			}

		case "response.function_call_arguments.delta":
			argsDelta := currentEvent.AsResponseFunctionCallArgumentsDelta()
			if toolCall, exists := pendingToolCalls[argsDelta.ItemID]; exists {
				toolCall.arguments += argsDelta.Delta
				senders.SendContentBlockDelta(toolCall.blockIndex, map[string]interface{}{
					"type":         deltaTypeInputJSONDelta,
					"partial_json": argsDelta.Delta,
				}, flusher)
			}

		case "response.function_call_arguments.done":
			argsDone := currentEvent.AsResponseFunctionCallArgumentsDone()
			if toolCall, exists := pendingToolCalls[argsDone.ItemID]; exists {
				if toolCall.name == "" && argsDone.Name != "" {
					toolCall.name = argsDone.Name
				}
				senders.SendContentBlockStop(state, toolCall.blockIndex, flusher)
				delete(pendingToolCalls, argsDone.ItemID)
			}

		case "response.custom_tool_call_input.delta":
			customDelta := currentEvent.AsResponseCustomToolCallInputDelta()
			if toolCall, exists := pendingToolCalls[customDelta.ItemID]; exists {
				toolCall.arguments += customDelta.Delta
				senders.SendContentBlockDelta(toolCall.blockIndex, map[string]interface{}{
					"type":         deltaTypeInputJSONDelta,
					"partial_json": customDelta.Delta,
				}, flusher)
			}

		case "response.custom_tool_call_input.done":
			customDone := currentEvent.AsResponseCustomToolCallInputDone()
			if toolCall, exists := pendingToolCalls[customDone.ItemID]; exists {
				senders.SendContentBlockStop(state, toolCall.blockIndex, flusher)
				delete(pendingToolCalls, customDone.ItemID)
			}

		case "response.mcp_call_arguments.delta":
			mcpDelta := currentEvent.AsResponseMcpCallArgumentsDelta()
			if toolCall, exists := pendingToolCalls[mcpDelta.ItemID]; exists {
				toolCall.arguments += mcpDelta.Delta
				senders.SendContentBlockDelta(toolCall.blockIndex, map[string]interface{}{
					"type":         deltaTypeInputJSONDelta,
					"partial_json": mcpDelta.Delta,
				}, flusher)
			}

		case "response.mcp_call_arguments.done":
			mcpDone := currentEvent.AsResponseMcpCallArgumentsDone()
			if toolCall, exists := pendingToolCalls[mcpDone.ItemID]; exists {
				senders.SendContentBlockStop(state, toolCall.blockIndex, flusher)
				delete(pendingToolCalls, mcpDone.ItemID)
			}

		case "response.output_item.done":
			// Handled by respective done events above

		case "response.completed":
			completed := currentEvent.AsResponseCompleted()
			state.inputTokens = completed.Response.Usage.InputTokens
			state.outputTokens = completed.Response.Usage.OutputTokens

			logrus.Debugf("[ResponsesAPI] Response completed: input_tokens=%d, output_tokens=%d", state.inputTokens, state.outputTokens)

			// Process any tool calls from the output array that weren't already handled via streaming events
			// This handles cases where tool calls come in the final response without intermediate events
			for _, outputItem := range completed.Response.Output {
				if outputItem.Type == "function_call" || outputItem.Type == "custom_tool_call" || outputItem.Type == "mcp_call" {
					itemID := outputItem.ID

					// Check if we already processed this tool call via streaming events
					if _, wasProcessed := pendingToolCalls[itemID]; wasProcessed {
						continue
					}

					// This is a new tool call that wasn't streamed - process it now
					truncatedID := truncateToolCallID(itemID)
					blockIndex := state.nextBlockIndex
					state.nextBlockIndex++

					var toolName string
					var arguments string

					switch outputItem.Type {
					case "function_call":
						fnCall := outputItem.AsFunctionCall()
						toolName = fnCall.Name
						arguments = fnCall.Arguments
					case "custom_tool_call":
						customCall := outputItem.AsCustomToolCall()
						toolName = customCall.Name
						arguments = customCall.Input
					case "mcp_call":
						mcpCall := outputItem.AsMcpCall()
						toolName = mcpCall.Name
						arguments = mcpCall.Arguments
					}

					lastOutputItemType = "function_call"

					// Send content_block_start for this tool
					senders.SendContentBlockStart(blockIndex, blockTypeToolUse, map[string]interface{}{
						"id":   truncatedID,
						"name": toolName,
					}, flusher)

					// Send the arguments as content_block_delta
					if arguments != "" {
						senders.SendContentBlockDelta(blockIndex, map[string]interface{}{
							"type":         deltaTypeInputJSONDelta,
							"partial_json": arguments,
						}, flusher)
					}

					// Send content_block_stop
					senders.SendContentBlockStop(state, blockIndex, flusher)
				}
			}

			senders.SendStopEvents(state, flusher)

			// Determine stop reason based on the last output item type
			// tool_use: response ended with a function call (expecting tool response)
			// end_turn: response ended with text content
			stopReason := anthropicStopReasonEndTurn
			if lastOutputItemType == "function_call" {
				stopReason = anthropicStopReasonToolUse
			}

			senders.SendMessageDelta(state, stopReason, flusher)
			senders.SendMessageStop(messageID, responseModel, state, stopReason, flusher)

			logrus.Debugf("[ResponsesAPI] Sent message_stop event with stop_reason=%s, finishing stream", stopReason)
			return protocol.NewTokenUsageWithCache(int(state.inputTokens), int(state.outputTokens), int(state.cacheTokens)), nil

		case "response.output_text.annotation.added":
			annotationAdded := currentEvent.AsResponseOutputTextAnnotationAdded()
			logrus.Debugf("[ResponsesAPI] Text annotation added: index=%d, citation_type=%s",
				annotationAdded.OutputIndex,
				annotationAdded.Annotation)

		case "response.text.done":
			// Finalize text content - already handled by content_part.done for output_text type

		case "response.reasoning_summary_part.added":
			summaryPartAdded := currentEvent.AsResponseReasoningSummaryPartAdded()
			if summaryPartAdded.Part.Type == "text" {
				if state.reasoningSummaryBlockIndex == -1 {
					state.reasoningSummaryBlockIndex = state.nextBlockIndex
					state.hasTextContent = true
					state.nextBlockIndex++
					// reasoning_summary should be converted to thinking block (per Claude Code spec)
					senders.SendContentBlockStart(state.reasoningSummaryBlockIndex, blockTypeThinking, map[string]interface{}{"thinking": ""}, flusher)
				}
				if summaryPartAdded.Part.Text != "" {
					senders.SendContentBlockDelta(state.reasoningSummaryBlockIndex, map[string]interface{}{
						"type":     deltaTypeThinkingDelta,
						"thinking": summaryPartAdded.Part.Text,
					}, flusher)
				}
			}

		case "response.reasoning_summary_part.done":
			if state.reasoningSummaryBlockIndex != -1 {
				senders.SendContentBlockStop(state, state.reasoningSummaryBlockIndex, flusher)
				state.reasoningSummaryBlockIndex = -1
			}

		case "response.audio.delta":
			audioDelta := currentEvent.AsResponseAudioDelta()
			logrus.Debugf("[ResponsesAPI] Audio delta: sequence=%d, len=%d", audioDelta.SequenceNumber, len(audioDelta.Delta))

		case "response.audio.done":
			logrus.Debugf("[ResponsesAPI] Audio done")

		case "response.audio.transcript.delta":
			transcriptDelta := currentEvent.AsResponseAudioTranscriptDelta()
			logrus.Debugf("[ResponsesAPI] Audio transcript delta: sequence=%d, len=%d", transcriptDelta.SequenceNumber, len(transcriptDelta.Delta))

		case "response.audio.transcript.done":
			logrus.Debugf("[ResponsesAPI] Audio transcript done")

		case "response.code_interpreter_call_code.delta":
			codeDelta := currentEvent.AsResponseCodeInterpreterCallCodeDelta()
			logrus.Debugf("[ResponsesAPI] Code interpreter code delta: len=%d", len(codeDelta.Delta))

		case "response.code_interpreter_call_code.done":
			logrus.Debugf("[ResponsesAPI] Code interpreter code done")

		case "response.code_interpreter_call.in_progress":
			logrus.Debugf("[ResponsesAPI] Code interpreter in progress")

		case "response.code_interpreter_call.interpreting":
			logrus.Debugf("[ResponsesAPI] Code interpreter interpreting")

		case "response.code_interpreter_call.completed":
			logrus.Debugf("[ResponsesAPI] Code interpreter completed")

		case "response.file_search_call.in_progress":
			logrus.Debugf("[ResponsesAPI] File search in progress")

		case "response.file_search_call.searching":
			logrus.Debugf("[ResponsesAPI] File search searching: query=%s", currentEvent.RawJSON())

		case "response.file_search_call.completed":
			logrus.Debugf("[ResponsesAPI] File search completed")

		case "response.web_search_call.in_progress":
			logrus.Debugf("[ResponsesAPI] Web search in progress")

		case "response.web_search_call.searching":
			searching := currentEvent.AsResponseWebSearchCallSearching()
			logrus.Debugf("[ResponsesAPI] Web search searching: %v", searching)

		case "response.web_search_call.completed":
			logrus.Debugf("[ResponsesAPI] Web search completed")

		case "response.image_generation_call.in_progress":
			logrus.Debugf("[ResponsesAPI] Image generation in progress")

		case "response.image_generation_call.generating":
			generating := currentEvent.AsResponseImageGenerationCallGenerating()
			logrus.Debugf("[ResponsesAPI] Image generation generating: %v", generating)

		case "response.image_generation_call.partial_image":
			partial := currentEvent.AsResponseImageGenerationCallPartialImage()
			logrus.Debugf("[ResponsesAPI] Image generation partial: index=%d", partial.PartialImageIndex)

		case "response.image_generation_call.completed":
			logrus.Debugf("[ResponsesAPI] Image generation completed")

		case "response.mcp_call.in_progress":
			mcpInProgress := currentEvent.AsResponseMcpCallInProgress()
			logrus.Debugf("[ResponsesAPI] MCP call in progress: %v", mcpInProgress)

		case "response.mcp_call.completed":
			mcpCompleted := currentEvent.AsResponseMcpCallCompleted()
			logrus.Debugf("[ResponsesAPI] MCP call completed: %v", mcpCompleted)

		case "response.mcp_call.failed":
			mcpFailed := currentEvent.AsResponseMcpCallFailed()
			logrus.Debugf("[ResponsesAPI] MCP call failed: %v", mcpFailed)

		case "response.mcp_list_tools.in_progress":
			logrus.Debugf("[ResponsesAPI] MCP list tools in progress")

		case "response.mcp_list_tools.completed":
			logrus.Debugf("[ResponsesAPI] MCP list tools completed")

		case "response.mcp_list_tools.failed":
			logrus.Debugf("[ResponsesAPI] MCP list tools failed")

		case "error", "response.failed", "response.incomplete":
			logrus.Errorf("Responses API error event: %v", currentEvent)
			errorEvent := map[string]interface{}{
				"type": "error",
				"error": map[string]interface{}{
					"message": fmt.Sprintf("Responses API error: %v", currentEvent),
					"type":    "api_error",
				},
			}
			senders.SendErrorEvent(errorEvent, flusher)
			return protocol.NewTokenUsageWithCache(int(state.inputTokens), int(state.outputTokens), int(state.cacheTokens)), fmt.Errorf("Responses API error: %v", currentEvent)

		default:
			logrus.Debugf("Unhandled Responses API event type: %s", currentEvent.Type)
		}
	}

	if err := stream.Err(); err != nil {
		logrus.Errorf("Responses API stream error: %v", err)
		errorEvent := map[string]interface{}{
			"type": "error",
			"error": map[string]interface{}{
				"message": err.Error(),
				"type":    "stream_error",
				"code":    "stream_failed",
			},
		}
		senders.SendErrorEvent(errorEvent, flusher)
		return protocol.NewTokenUsageWithCache(int(state.inputTokens), int(state.outputTokens), int(state.cacheTokens)), err
	}

	return protocol.NewTokenUsageWithCache(int(state.inputTokens), int(state.outputTokens), int(state.cacheTokens)), nil
}

func HandleResponsesToAnthropicV1Assembly(c *gin.Context, stream *openaistream.Stream[responses.ResponseStreamEventUnion], responseModel string) (*protocol.TokenUsage, error) {
	blocks := make(map[int]*anthropic.ContentBlockUnion)

	msg := anthropic.Message{
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
			block := anthropic.ContentBlockUnion{Type: blockType}
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
			msg.StopReason = anthropic.StopReasonEndTurn
		},
		SendMessageDelta: func(state *streamState, stopReason string, flusher http.Flusher) {
			msg.StopReason = anthropic.StopReason(stopReason)
		},
		SendMessageStop: func(messageID, model string, state *streamState, stopReason string, flusher http.Flusher) {
			msg.ID = messageID
			//TODO: the id is special
			//msg.ID = fmt.Sprintf("msg_%s", uuid.New().String())
			msg.Model = anthropic.Model(model)
			msg.StopReason = anthropic.StopReason(mapOpenAIFinishReasonToAnthropic(stopReason))

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
