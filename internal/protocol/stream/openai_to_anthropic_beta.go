package stream

import (
	"context"
	"errors"
	"fmt"
	"io"
	"net/http"
	"strings"
	"time"

	"github.com/anthropics/anthropic-sdk-go"
	"github.com/gin-gonic/gin"
	"github.com/openai/openai-go/v3"
	openaistream "github.com/openai/openai-go/v3/packages/ssestream"
	"github.com/openai/openai-go/v3/responses"
	"github.com/sirupsen/logrus"

	"github.com/tingly-dev/tingly-box/internal/protocol"
	"github.com/tingly-dev/tingly-box/internal/protocol/token"
)

// StreamEventRecorder is an interface for recording stream events during protocol conversion
type StreamEventRecorder interface {
	RecordRawMapEvent(eventType string, event map[string]interface{})
}

// HandleOpenAIToAnthropicV1BetaStreamResponse processes OpenAI streaming events and converts them to Anthropic beta format.
// Returns UsageStat containing token usage information for tracking.
func HandleOpenAIToAnthropicV1BetaStreamResponse(c *gin.Context, req *openai.ChatCompletionNewParams, stream *openaistream.Stream[openai.ChatCompletionChunk], responseModel string) (protocol.UsageStat, error) {
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
		return protocol.ZeroUsageStat(), errors.New("streaming not supported by this connection")
	}

	// Generate message ID for Anthropic beta format
	messageID := fmt.Sprintf("msg_%d", time.Now().Unix())

	// Initialize streaming state
	state := newStreamState()

	// Track accumulated content for usage estimation
	var contentBuilder strings.Builder
	var hasUsageFromUpstream bool = false

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
			// Check for usage info in the last chunk
			if chunk.Usage.PromptTokens > 0 || chunk.Usage.CompletionTokens > 0 {
				state.inputTokens = chunk.Usage.PromptTokens
				state.outputTokens = chunk.Usage.CompletionTokens
				hasUsageFromUpstream = true
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
			for k, v := range extras {
				// Handle reasoning_content -> thinking block
				if k == OpenaiFieldReasoningContent {
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

			// Accumulate refusal content for usage estimation
			contentBuilder.WriteString(delta.Refusal)

			sendBetaContentBlockDelta(c, state.textBlockIndex, map[string]interface{}{
				"type": deltaTypeTextDelta,
				"text": delta.Refusal,
			}, flusher)
		}

		// Handle content delta
		if delta.Content != "" {
			state.hasTextContent = true

			// Accumulate content for usage estimation
			contentBuilder.WriteString(delta.Content)

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

		// Track usage from chunk
		if chunk.Usage.CompletionTokens > 0 {
			state.inputTokens = chunk.Usage.PromptTokens
			state.outputTokens = chunk.Usage.CompletionTokens
			hasUsageFromUpstream = true
		}

		// Handle finish_reason (last chunk for this choice)
		if choice.FinishReason != "" {
			// Estimate usage if upstream didn't provide it
			if !hasUsageFromUpstream {
				inputTokens, _ := token.EstimateInputTokens(req)
				state.outputTokens = int64(token.EstimateOutputTokens(contentBuilder.String()))
				state.inputTokens = int64(inputTokens)
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
			return protocol.NewUsageStat(int(state.inputTokens), int(state.outputTokens)), nil
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
		return protocol.NewUsageStat(int(state.inputTokens), int(state.outputTokens)), err
	}
	return protocol.NewUsageStat(int(state.inputTokens), int(state.outputTokens)), nil
}

// HandleResponsesToAnthropicV1BetaStreamResponse processes OpenAI Responses API streaming events
// and routes to the appropriate handler (v1 or beta) based on the original request format.
// Returns UsageStat containing token usage information for tracking.
func HandleResponsesToAnthropicV1BetaStreamResponse(c *gin.Context, stream *openaistream.Stream[responses.ResponseStreamEventUnion], responseModel string) (protocol.UsageStat, error) {
	// Check if the original request was v1 format
	originalFormat := "beta"
	if fmt, exists := c.Get("original_request_format"); exists {
		if formatStr, ok := fmt.(string); ok {
			originalFormat = formatStr
		}
	}

	if originalFormat == "v1" {
		return HandleResponsesToAnthropicV1StreamResponse(c, stream, responseModel)
	}
	return HandleResponsesToAnthropicBetaStreamResponse(c, stream, responseModel)
}

// HandleResponsesToAnthropicBetaStreamResponse processes OpenAI Responses API streaming events and converts them to Anthropic beta format.
// This is a thin wrapper that uses the shared core logic with beta event senders.
// Returns UsageStat containing token usage information for tracking.
func HandleResponsesToAnthropicBetaStreamResponse(c *gin.Context, stream *openaistream.Stream[responses.ResponseStreamEventUnion], responseModel string) (protocol.UsageStat, error) {
	return HandleResponsesToAnthropicStreamResponse(c, stream, responseModel, responsesAPIEventSenders{
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

// HandleResponsesToAnthropicStreamResponse is the shared core logic for processing OpenAI Responses API streams
// and converting them to Anthropic format (v1 or beta depending on the senders provided).
// Returns UsageStat containing token usage information for tracking.
func HandleResponsesToAnthropicStreamResponse(c *gin.Context, stream *openaistream.Stream[responses.ResponseStreamEventUnion], responseModel string, senders responsesAPIEventSenders) (protocol.UsageStat, error) {
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
		return protocol.ZeroUsageStat(), fmt.Errorf("streaming not supported by this connection")
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

		logrus.Debugf("Processing Responses API event #%d: type=%s", eventCount, currentEvent.Type)

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
				senders.SendContentBlockStart(state.thinkingBlockIndex, blockTypeThinking, map[string]interface{}{"thinking": ""}, flusher)
			}
			senders.SendContentBlockDelta(state.thinkingBlockIndex, map[string]interface{}{
				"type":     deltaTypeThinkingDelta,
				"thinking": reasoningDelta.Delta,
			}, flusher)

		case "response.reasoning_text.done":
			if state.thinkingBlockIndex != -1 {
				senders.SendContentBlockStop(state, state.thinkingBlockIndex, flusher)
				state.thinkingBlockIndex = -1
			}

		case "response.reasoning_summary_text.delta":
			summaryDelta := currentEvent.AsResponseReasoningSummaryTextDelta()
			// Reasoning summary is condensed reasoning shown to user as visible text
			// This is separate from both hidden thinking (reasoning_text) and main output text
			if state.reasoningSummaryBlockIndex == -1 {
				state.reasoningSummaryBlockIndex = state.nextBlockIndex
				state.hasTextContent = true
				state.nextBlockIndex++
				senders.SendContentBlockStart(state.reasoningSummaryBlockIndex, blockTypeText, map[string]interface{}{"text": ""}, flusher)
			}
			senders.SendContentBlockDelta(state.reasoningSummaryBlockIndex, map[string]interface{}{
				"type": deltaTypeTextDelta,
				"text": summaryDelta.Delta,
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
			if itemAdded.Item.Type == "function_call" || itemAdded.Item.Type == "custom_tool_call" || itemAdded.Item.Type == "mcp_call" {
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
			state.inputTokens = int64(completed.Response.Usage.InputTokens)
			state.outputTokens = int64(completed.Response.Usage.OutputTokens)

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
			return protocol.NewUsageStat(int(state.inputTokens), int(state.outputTokens)), nil

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
			return protocol.NewUsageStat(int(state.inputTokens), int(state.outputTokens)), fmt.Errorf("Responses API error: %v", currentEvent)

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
		return protocol.NewUsageStat(int(state.inputTokens), int(state.outputTokens)), err
	}

	return protocol.NewUsageStat(int(state.inputTokens), int(state.outputTokens)), nil
}
