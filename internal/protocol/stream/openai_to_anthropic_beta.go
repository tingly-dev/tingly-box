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
	return handleOpenAIToAnthropicBetaStream(c, req, stream, responseModel, nil)
}

// HandleOpenAIToAnthropicBetaStreamWithMCPHooks enables MCP-aware tool suppression/finalization during conversion.
func HandleOpenAIToAnthropicBetaStreamWithMCPHooks(
	c *gin.Context,
	req *openai.ChatCompletionNewParams,
	stream *openaistream.Stream[openai.ChatCompletionChunk],
	responseModel string,
	hooks *OpenAIToAnthropicMCPHooks,
) (*protocol.TokenUsage, error) {
	return handleOpenAIToAnthropicBetaStream(c, req, stream, responseModel, hooks)
}

func handleOpenAIToAnthropicBetaStream(
	c *gin.Context,
	req *openai.ChatCompletionNewParams,
	stream *openaistream.Stream[openai.ChatCompletionChunk],
	responseModel string,
	hooks *OpenAIToAnthropicMCPHooks,
) (*protocol.TokenUsage, error) {
	logrus.Debug("Starting OpenAI to Anthropic beta streaming response handler")
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
	var hookErr error

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
						state.thinkingBlocks[state.thinkingBlockIndex] = true
						logrus.Debugf("[Thinking] Initializing thinking block at index %d", state.thinkingBlockIndex)
						ensureMessageStart()
						sendContentBlockStart(c, state.thinkingBlockIndex, blockTypeThinking, map[string]interface{}{
							"thinking": "",
						}, flusher)
					}

					// Extract thinking content (handle different types)
					thinkingText := extractString(v)
					if thinkingText != "" {
						preview := thinkingText
						logrus.Debugf("[Thinking] Sending thinking_delta: len=%d, preview=%q", len(thinkingText), preview)
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

		// Handle finish_reason (last chunk for this choice)
		if choice.FinishReason != "" {
			// Get final token counts from counter
			if tokenCounter != nil {
				inputTokens, outputTokens := tokenCounter.GetCounts()
				state.inputTokens = int64(inputTokens)
				state.outputTokens = int64(outputTokens)
			}
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
			ensureMessageStart()
			sendStopEvents(c, state, flusher)
			sendMessageDelta(c, state, mapOpenAIFinishReasonToAnthropicBeta(choice.FinishReason), flusher)
			sendMessageStop(c, messageID, responseModel, state, mapOpenAIFinishReasonToAnthropicBeta(choice.FinishReason), flusher)
			return false
		}

		return true
	})
	if hookErr != nil {
		return protocol.NewTokenUsageWithCache(int(state.inputTokens), int(state.outputTokens), int(state.cacheTokens)), hookErr
	}

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
		sendAnthropicStreamEvent(c, "error", errorEvent, flusher)
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
