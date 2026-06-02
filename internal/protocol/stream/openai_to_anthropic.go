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
	"github.com/sirupsen/logrus"

	"github.com/tingly-dev/tingly-box/internal/protocol"
	"github.com/tingly-dev/tingly-box/internal/protocol/token"
)

// OpenAIToAnthropicToolCall captures a complete tool call assembled from OpenAI stream chunks.
type OpenAIToAnthropicToolCall struct {
	ID        string
	Name      string
	Arguments string
}

// OpenAIToAnthropicMCPHooks provides optional hooks for MCP-aware stream handling.
type OpenAIToAnthropicMCPHooks struct {
	ShouldSuppressTool func(name string) bool
	OnToolCallsFinal   func(calls []OpenAIToAnthropicToolCall) error
}

var ErrMCPStreamContinue = errors.New("mcp stream should continue")

// HandleOpenAIToAnthropicStreamResponse processes OpenAI streaming events and converts them to Anthropic format.
// Returns UsageStat containing token usage information for tracking.
func HandleOpenAIToAnthropicStreamResponse(c *gin.Context, req *openai.ChatCompletionNewParams, stream *openaistream.Stream[openai.ChatCompletionChunk], responseModel string) (*protocol.TokenUsage, error) {
	return handleOpenAIToAnthropicStreamResponse(c, req, stream, responseModel, nil)
}

// HandleOpenAIToAnthropicStreamResponseWithMCPHooks enables MCP-aware tool suppression/finalization during conversion.
func HandleOpenAIToAnthropicStreamResponseWithMCPHooks(
	c *gin.Context,
	req *openai.ChatCompletionNewParams,
	stream *openaistream.Stream[openai.ChatCompletionChunk],
	responseModel string,
	hooks *OpenAIToAnthropicMCPHooks,
) (*protocol.TokenUsage, error) {
	return handleOpenAIToAnthropicStreamResponse(c, req, stream, responseModel, hooks)
}

func handleOpenAIToAnthropicStreamResponse(
	c *gin.Context,
	req *openai.ChatCompletionNewParams,
	stream *openaistream.Stream[openai.ChatCompletionChunk],
	responseModel string,
	hooks *OpenAIToAnthropicMCPHooks,
) (*protocol.TokenUsage, error) {
	logrus.WithContext(c.Request.Context()).Debug("Starting OpenAI to Anthropic streaming response handler")
	defer func() {
		if r := recover(); r != nil {
			logrus.WithContext(c.Request.Context()).Errorf("Panic in OpenAI to Anthropic streaming handler: %v", r)
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
				logrus.WithContext(c.Request.Context()).Errorf("Error closing OpenAI stream: %v", err)
			}
		}
		logrus.WithContext(c.Request.Context()).Info("Finished OpenAI to Anthropic streaming response handler")
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
	var hookErr error

	// Initialize token counter for accurate usage tracking
	tokenCounter, err := token.NewStreamTokenCounter()
	if err != nil {
		logrus.WithContext(c.Request.Context()).Errorf("Failed to create token counter: %v", err)
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

	messageStarted := false
	ensureMessageStart := func() {
		if messageStarted {
			return
		}
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
		messageStarted = true
	}

	// Process the stream with context cancellation checking.
	// Note: when stream_options.include_usage=true, OpenAI sends a final
	// usage-only chunk (choices:[], usage:{...}) AFTER the finish_reason chunk.
	// We must keep draining the stream after seeing finish_reason so the
	// upstream usage isn't silently dropped.
	pendingFinishReason := ""
	finishSeen := false
	StreamLoop(c, func(w io.Writer) bool {
		// Check context cancellation first
		select {
		case <-c.Request.Context().Done():
			logrus.WithContext(c.Request.Context()).Debug("Client disconnected, stopping OpenAI to Anthropic stream")
			return false
		default:
		}

		// Try to get next chunk
		if !stream.Next() {
			return false
		}

		chunk := stream.Current()

		// Skip empty chunks (no choices).
		// The trailing usage-only chunk (choices:[], usage:{...}) lands here
		// when stream_options.include_usage=true.
		if len(chunk.Choices) == 0 {
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
						state.thinkingBlocks[state.thinkingBlockIndex] = true
						ensureMessageStart()
						sendContentBlockStart(c, state.thinkingBlockIndex, blockTypeThinking, map[string]interface{}{
							"thinking": "",
						}, flusher)
					}

					// Extract thinking content (handle different types)
					thinkingText := extractString(v)
					if thinkingText != "" {
						// Send content_block_delta with thinking_delta
						ensureMessageStart()
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
				// Close any open block (e.g. thinking) before opening text block
				closeOpenBlock(c, state, flusher)
				state.textBlockIndex = state.nextBlockIndex
				state.nextBlockIndex++
				ensureMessageStart()
				sendContentBlockStart(c, state.textBlockIndex, blockTypeText, map[string]interface{}{
					"text": "",
				}, flusher)
			}
			state.hasTextContent = true

			ensureMessageStart()
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
				// Close any open block (e.g. thinking) before opening text block
				closeOpenBlock(c, state, flusher)
				state.textBlockIndex = state.nextBlockIndex
				state.nextBlockIndex++
				ensureMessageStart()
				sendContentBlockStart(c, state.textBlockIndex, blockTypeText, map[string]interface{}{
					"text": "",
				}, flusher)
			}

			// Send content_block_delta with only text - no OpenAI fields merged in
			ensureMessageStart()
			sendContentBlockDelta(c, state.textBlockIndex, map[string]interface{}{
				"type": deltaTypeTextDelta,
				"text": delta.Content,
			}, flusher)
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

					// Rewrite OpenAI call_ prefix to Anthropic toolu_ prefix
					truncatedID := rewriteToolCallIDForAnthropic(toolCall.ID)

					// Initialize pending tool call
					state.pendingToolCalls[anthropicIndex] = &pendingToolCall{
						id:   truncatedID,
						name: toolCall.Function.Name,
						emit: !(hooks != nil && hooks.ShouldSuppressTool != nil && hooks.ShouldSuppressTool(toolCall.Function.Name)),
					}

					// Close any open block (text/thinking) before opening tool_use block
					closeOpenBlock(c, state, flusher)

					// Send content_block_start for tool_use unless suppressed by MCP hook.
					if state.pendingToolCalls[anthropicIndex].emit {
						ensureMessageStart()
						sendContentBlockStart(c, anthropicIndex, blockTypeToolUse, map[string]interface{}{
							"id":    truncatedID,
							"name":  toolCall.Function.Name,
							"input": map[string]interface{}{},
						}, flusher)
					}
				}

				// Accumulate arguments and send delta
				if toolCall.ID != "" {
					state.pendingToolCalls[anthropicIndex].id = truncateToolCallID(toolCall.ID)
				}
				if toolCall.Function.Name != "" {
					state.pendingToolCalls[anthropicIndex].name = toolCall.Function.Name
				}
				if toolCall.Function.Arguments != "" {
					state.pendingToolCalls[anthropicIndex].input += toolCall.Function.Arguments

					// Send content_block_delta unless suppressed by MCP hook.
					if state.pendingToolCalls[anthropicIndex].emit {
						ensureMessageStart()
						sendContentBlockDelta(c, anthropicIndex, map[string]interface{}{
							"type":         deltaTypeInputJSONDelta,
							"partial_json": toolCall.Function.Arguments,
						}, flusher)
					}
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

		// Handle finish_reason (last chunk for this choice).
		// Keep the loop alive so the trailing usage-only chunk (sent when
		// stream_options.include_usage=true) is still consumed; the final
		// Anthropic events are emitted after the loop.
		if choice.FinishReason != "" {
			pendingFinishReason = choice.FinishReason
			finishSeen = true
			if hooks != nil && hooks.OnToolCallsFinal != nil && len(state.pendingToolCalls) > 0 {
				toolCalls := make([]OpenAIToAnthropicToolCall, 0, len(state.pendingToolCalls))
				for _, tc := range state.pendingToolCalls {
					toolCalls = append(toolCalls, OpenAIToAnthropicToolCall{
						ID:        tc.id,
						Name:      tc.name,
						Arguments: tc.input,
					})
				}
				if err := hooks.OnToolCallsFinal(toolCalls); err != nil {
					hookErr = err
					return false
				}
			}
			if !messageStarted && errors.Is(hookErr, ErrMCPStreamContinue) {
				return false
			}
		}

		return true
	})

	// Emit the terminal Anthropic events using the final tallied usage
	// (which may have been updated by a post-finish_reason usage chunk).
	if finishSeen && hookErr == nil {
		if tokenCounter != nil {
			inputTokens, outputTokens := tokenCounter.GetCounts()
			state.inputTokens = int64(inputTokens)
			state.outputTokens = int64(outputTokens)
			cacheTokens, reasoningTokens := tokenCounter.GetUpstreamDetails()
			if cacheTokens > 0 {
				state.cacheTokens = int64(cacheTokens)
			}
			if reasoningTokens > 0 {
				state.reasoningTokens = int64(reasoningTokens)
			}
		}
		ensureMessageStart()
		sendStopEvents(c, state, flusher)
		stopReason := mapOpenAIFinishReasonToAnthropic(pendingFinishReason)
		sendMessageDelta(c, state, stopReason, flusher)
		sendMessageStop(c, messageID, responseModel, state, stopReason, flusher)
		logrus.WithContext(c.Request.Context()).WithFields(logrus.Fields{
			"model":            responseModel,
			"input_tokens":     state.inputTokens,
			"output_tokens":    state.outputTokens,
			"cache_tokens":     state.cacheTokens,
			"reasoning_tokens": state.reasoningTokens,
			"stop_reason":      stopReason,
		}).Info("OpenAI->Anthropic stream usage")
	}
	if hookErr != nil {
		return protocol.NewTokenUsageFull(int(state.inputTokens), int(state.outputTokens), int(state.cacheTokens), int(state.reasoningTokens)), hookErr
	}

	// Check for stream errors
	if err := stream.Err(); err != nil {
		// Check if it was a client cancellation
		if errors.Is(err, context.Canceled) {
			logrus.WithContext(c.Request.Context()).Debug("OpenAI to Anthropic stream canceled by client")
			return protocol.NewTokenUsageFull(int(state.inputTokens), int(state.outputTokens), int(state.cacheTokens), int(state.reasoningTokens)), nil
		}
		logrus.WithContext(c.Request.Context()).Errorf("OpenAI stream error: %v", err)
		// Nothing has reached the client yet (no message_start emitted), so the
		// stream failed before any content. Surface it as a retryable HTTP error
		// instead of a 200 SSE error event, so mid-request failover can fall
		// through to the next priority tier. Once content is flowing the SSE
		// error event is the only honest option.
		if !messageStarted {
			SendStreamingError(c, err)
			return protocol.NewTokenUsageFull(int(state.inputTokens), int(state.outputTokens), int(state.cacheTokens), int(state.reasoningTokens)), err
		}
		errorEvent := map[string]interface{}{
			"type": "error",
			"error": map[string]interface{}{
				"message": err.Error(),
				"type":    "stream_error",
				"code":    "stream_failed",
			},
		}
		sendAnthropicStreamEvent(c, "error", errorEvent, flusher)
		return protocol.NewTokenUsageFull(int(state.inputTokens), int(state.outputTokens), int(state.cacheTokens), int(state.reasoningTokens)), err
	}
	if errors.Is(c.Request.Context().Err(), context.Canceled) {
		return protocol.NewTokenUsageFull(int(state.inputTokens), int(state.outputTokens), int(state.cacheTokens), int(state.reasoningTokens)), context.Canceled
	}
	return protocol.NewTokenUsageFull(int(state.inputTokens), int(state.outputTokens), int(state.cacheTokens), int(state.reasoningTokens)), nil
}

// HandleResponsesToAnthropicV1Stream processes OpenAI Responses API streaming events and converts them to Anthropic v1 format.
// This is a thin wrapper that uses the shared core logic with v1 event senders.
// Returns UsageStat containing token usage information for tracking.
func HandleResponsesToAnthropicV1Stream(c *gin.Context, stream ResponsesStreamIter, responseModel string) (*protocol.TokenUsage, error) {
	return handlerResponsesToAnthropicStream(c, stream, responseModel, responsesAPIEventSenders{
		SendMessageStart: func(event map[string]interface{}, flusher http.Flusher) {
			// message_start is the first SSE byte for this stream; priming
			// already confirmed the upstream works, so open the failover gate
			// before writing so the client sees output immediately.
			CommitFirstChunk(c)
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
func handlerResponsesToAnthropicStream(c *gin.Context, stream ResponsesStreamIter, responseModel string, senders responsesAPIEventSenders) (*protocol.TokenUsage, error) {
	logrus.WithContext(c.Request.Context()).Debugf("[ResponsesAPI] Starting Responses API to Anthropic streaming response handler, model=%s", responseModel)
	defer func() {
		if r := recover(); r != nil {
			logrus.WithContext(c.Request.Context()).Errorf("Panic in Responses API to Anthropic streaming handler: %v", r)
			if c.Writer != nil {
				c.SSEvent("error", "{\"error\":{\"message\":\"Internal streaming error\",\"type\":\"internal_error\"}}")
				if flusher, ok := c.Writer.(http.Flusher); ok {
					flusher.Flush()
				}
			}
		}
		if stream != nil {
			if err := stream.Close(); err != nil {
				logrus.WithContext(c.Request.Context()).Errorf("Error closing Responses API stream: %v", err)
			}
		}
		logrus.WithContext(c.Request.Context()).Debug("[ResponsesAPI] Finished Responses API to Anthropic streaming response handler")
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
		completed   bool // true when content_block_stop has been sent
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
	for stream.Next() {
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
				state.thinkingBlocks[state.thinkingBlockIndex] = true
				logrus.WithContext(c.Request.Context()).Debugf("[Thinking][ResponsesAPI] Initializing thinking block at index %d", state.thinkingBlockIndex)
				senders.SendContentBlockStart(state.thinkingBlockIndex, blockTypeThinking, map[string]interface{}{"thinking": ""}, flusher)
			}
			senders.SendContentBlockDelta(state.thinkingBlockIndex, map[string]interface{}{
				"type":     deltaTypeThinkingDelta,
				"thinking": reasoningDelta.Delta,
			}, flusher)

		case "response.reasoning_text.done":
			logrus.WithContext(c.Request.Context()).Debugf("[Thinking][ResponsesAPI] Thinking block done at index %d", state.thinkingBlockIndex)
			if state.thinkingBlockIndex != -1 && !state.stoppedBlocks[state.thinkingBlockIndex] {
				// Send signature_delta before stopping thinking block (Anthropic extended thinking requirement)
				senders.SendContentBlockDelta(state.thinkingBlockIndex, map[string]interface{}{
					"type":      "signature_delta",
					"signature": GenerateObfuscationString(),
				}, flusher)
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
				state.thinkingBlocks[state.reasoningSummaryBlockIndex] = true
				senders.SendContentBlockStart(state.reasoningSummaryBlockIndex, blockTypeThinking, map[string]interface{}{"thinking": ""}, flusher)
			}
			senders.SendContentBlockDelta(state.reasoningSummaryBlockIndex, map[string]interface{}{
				"type":     deltaTypeThinkingDelta,
				"thinking": summaryDelta.Delta,
			}, flusher)

		case "response.reasoning_summary_text.done":
			if state.reasoningSummaryBlockIndex != -1 && !state.stoppedBlocks[state.reasoningSummaryBlockIndex] {
				// Send signature_delta before stopping thinking block (Anthropic extended thinking requirement)
				senders.SendContentBlockDelta(state.reasoningSummaryBlockIndex, map[string]interface{}{
					"type":      "signature_delta",
					"signature": GenerateObfuscationString(),
				}, flusher)
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
			switch itemAdded.Item.Type {
			case "reasoning":
				reasoningDelta := currentEvent.AsResponseReasoningTextDelta()
				if state.thinkingBlockIndex == -1 {
					state.thinkingBlockIndex = state.nextBlockIndex
					state.nextBlockIndex++
					state.thinkingBlocks[state.thinkingBlockIndex] = true
					logrus.WithContext(c.Request.Context()).Debugf("[Thinking][ResponsesAPI] Initializing thinking block at index %d", state.thinkingBlockIndex)
					senders.SendContentBlockStart(state.thinkingBlockIndex, blockTypeThinking, map[string]interface{}{"thinking": ""}, flusher)
				}
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
					"id":    truncatedID,
					"name":  toolName,
					"input": map[string]interface{}{},
				}, flusher)
			default:
				logrus.WithContext(c.Request.Context()).Warnf("missing process for stream chunk: %s, %s", itemAdded.Type, itemAdded.Item.Type)
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
				// Mark as completed but don't delete yet - we need this for response.completed check
				toolCall.completed = true
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
				// Mark as completed but don't delete yet - we need this for response.completed check
				toolCall.completed = true
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
				// Mark as completed but don't delete yet - we need this for response.completed check
				toolCall.completed = true
			}

		case "response.output_item.done":
			// Handled by respective done events above

		case "response.completed":
			completed := currentEvent.AsResponseCompleted()
			state.cacheTokens = completed.Response.Usage.InputTokensDetails.CachedTokens
			state.inputTokens = completed.Response.Usage.InputTokens - state.cacheTokens
			state.outputTokens = completed.Response.Usage.OutputTokens
			state.reasoningTokens = completed.Response.Usage.OutputTokensDetails.ReasoningTokens

			logrus.WithContext(c.Request.Context()).Debugf("[ResponsesAPI] Response completed: input_tokens=%d, output_tokens=%d", state.inputTokens, state.outputTokens)

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
						"id":    truncatedID,
						"name":  toolName,
						"input": map[string]interface{}{},
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

			logrus.WithContext(c.Request.Context()).Debugf("[ResponsesAPI] Sent message_stop event with stop_reason=%s, finishing stream", stopReason)
			return protocol.NewTokenUsageFull(int(state.inputTokens), int(state.outputTokens), int(state.cacheTokens), int(state.reasoningTokens)), nil

		case "response.output_text.annotation.added":
			// Per-annotation event; pass through silently.

		case "response.text.done":
			// Finalize text content - already handled by content_part.done for output_text type

		case "response.reasoning_summary_part.added":
			summaryPartAdded := currentEvent.AsResponseReasoningSummaryPartAdded()
			if summaryPartAdded.Part.Type == "text" {
				if state.reasoningSummaryBlockIndex == -1 {
					state.reasoningSummaryBlockIndex = state.nextBlockIndex
					state.hasTextContent = true
					state.nextBlockIndex++
					state.thinkingBlocks[state.reasoningSummaryBlockIndex] = true
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
			if state.reasoningSummaryBlockIndex != -1 && !state.stoppedBlocks[state.reasoningSummaryBlockIndex] {
				// Send signature_delta before stopping thinking block (Anthropic extended thinking requirement)
				senders.SendContentBlockDelta(state.reasoningSummaryBlockIndex, map[string]interface{}{
					"type":      "signature_delta",
					"signature": GenerateObfuscationString(),
				}, flusher)
				senders.SendContentBlockStop(state, state.reasoningSummaryBlockIndex, flusher)
				state.reasoningSummaryBlockIndex = -1
			}

		case "response.audio.delta", "response.audio.done",
			"response.audio.transcript.delta", "response.audio.transcript.done",
			"response.code_interpreter_call_code.delta", "response.code_interpreter_call_code.done":
			// Pass-through events not converted to Anthropic blocks; ignore silently.

		case "response.code_interpreter_call.in_progress":
			logrus.WithContext(c.Request.Context()).Debugf("[ResponsesAPI] Code interpreter in progress")

		case "response.code_interpreter_call.interpreting":
			logrus.WithContext(c.Request.Context()).Debugf("[ResponsesAPI] Code interpreter interpreting")

		case "response.code_interpreter_call.completed":
			logrus.WithContext(c.Request.Context()).Debugf("[ResponsesAPI] Code interpreter completed")

		case "response.file_search_call.in_progress":
			logrus.WithContext(c.Request.Context()).Debugf("[ResponsesAPI] File search in progress")

		case "response.file_search_call.searching":
			// Status event; query payload elided to keep logs small.

		case "response.file_search_call.completed":
			logrus.WithContext(c.Request.Context()).Debugf("[ResponsesAPI] File search completed")

		case "response.web_search_call.in_progress":
			logrus.WithContext(c.Request.Context()).Debugf("[ResponsesAPI] Web search in progress")

		case "response.web_search_call.searching":
			searching := currentEvent.AsResponseWebSearchCallSearching()
			logrus.WithContext(c.Request.Context()).Debugf("[ResponsesAPI] Web search searching: %v", searching)

		case "response.web_search_call.completed":
			logrus.WithContext(c.Request.Context()).Debugf("[ResponsesAPI] Web search completed")

		case "response.image_generation_call.in_progress":
			logrus.WithContext(c.Request.Context()).Debugf("[ResponsesAPI] Image generation in progress")

		case "response.image_generation_call.generating":
			generating := currentEvent.AsResponseImageGenerationCallGenerating()
			logrus.WithContext(c.Request.Context()).Debugf("[ResponsesAPI] Image generation generating: %v", generating)

		case "response.image_generation_call.partial_image":
			partial := currentEvent.AsResponseImageGenerationCallPartialImage()
			logrus.WithContext(c.Request.Context()).Debugf("[ResponsesAPI] Image generation partial: index=%d", partial.PartialImageIndex)

		case "response.image_generation_call.completed":
			logrus.WithContext(c.Request.Context()).Debugf("[ResponsesAPI] Image generation completed")

		case "response.mcp_call.in_progress":
			mcpInProgress := currentEvent.AsResponseMcpCallInProgress()
			logrus.WithContext(c.Request.Context()).Debugf("[ResponsesAPI] MCP call in progress: %v", mcpInProgress)

		case "response.mcp_call.completed":
			mcpCompleted := currentEvent.AsResponseMcpCallCompleted()
			logrus.WithContext(c.Request.Context()).Debugf("[ResponsesAPI] MCP call completed: %v", mcpCompleted)

		case "response.mcp_call.failed":
			mcpFailed := currentEvent.AsResponseMcpCallFailed()
			logrus.WithContext(c.Request.Context()).Debugf("[ResponsesAPI] MCP call failed: %v", mcpFailed)

		case "response.mcp_list_tools.in_progress":
			logrus.WithContext(c.Request.Context()).Debugf("[ResponsesAPI] MCP list tools in progress")

		case "response.mcp_list_tools.completed":
			logrus.WithContext(c.Request.Context()).Debugf("[ResponsesAPI] MCP list tools completed")

		case "response.mcp_list_tools.failed":
			logrus.WithContext(c.Request.Context()).Debugf("[ResponsesAPI] MCP list tools failed")

		case "error", "response.failed", "response.incomplete":
			logrus.WithContext(c.Request.Context()).Errorf("Responses API error event: %v", currentEvent)
			errorEvent := map[string]interface{}{
				"type": "error",
				"error": map[string]interface{}{
					"message": fmt.Sprintf("Responses API error: %v", currentEvent),
					"type":    "api_error",
				},
			}
			senders.SendErrorEvent(errorEvent, flusher)
			return protocol.NewTokenUsageFull(int(state.inputTokens), int(state.outputTokens), int(state.cacheTokens), int(state.reasoningTokens)), fmt.Errorf("Responses API error: %v", currentEvent)

		default:
			logrus.WithContext(c.Request.Context()).Debugf("Unhandled Responses API event type: %s", currentEvent.Type)
		}
	}

	if err := stream.Err(); err != nil {
		logrus.WithContext(c.Request.Context()).Errorf("Responses API stream error: %v", err)
		errorEvent := map[string]interface{}{
			"type": "error",
			"error": map[string]interface{}{
				"message": err.Error(),
				"type":    "stream_error",
				"code":    "stream_failed",
			},
		}
		senders.SendErrorEvent(errorEvent, flusher)
		return protocol.NewTokenUsageFull(int(state.inputTokens), int(state.outputTokens), int(state.cacheTokens), int(state.reasoningTokens)), err
	}

	return protocol.NewTokenUsageFull(int(state.inputTokens), int(state.outputTokens), int(state.cacheTokens), int(state.reasoningTokens)), nil
}

func HandleResponsesToAnthropicV1Assembly(c *gin.Context, stream ResponsesStreamIter, responseModel string) (*protocol.TokenUsage, error) {
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
		// MENTION: we may use `refusal` but it works badly - then we use end turn as normal
		return string(anthropic.StopReasonEndTurn)
	default:
		return anthropicStopReasonEndTurn
	}
}
