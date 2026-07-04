package stream

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"net/http"
	"time"

	"github.com/anthropics/anthropic-sdk-go"
	"github.com/anthropics/anthropic-sdk-go/shared/constant"
	"github.com/gin-gonic/gin"
	"github.com/openai/openai-go/v3"
	openaistream "github.com/openai/openai-go/v3/packages/ssestream"
	"github.com/sirupsen/logrus"

	"github.com/tingly-dev/tingly-box/internal/protocol"
	protocolusage "github.com/tingly-dev/tingly-box/internal/protocol/usage"
)

// handleResponsesToAnthropicStream is the shared implementation for both v1 and beta
// Responses API → Anthropic stream conversions using the iterator pattern.
func handleResponsesToAnthropicStream(hc *protocol.HandleContext, stream ResponsesStreamIter, responseModel string) (*protocol.TokenUsage, error) {
	c := hc.GinContext
	logrus.WithContext(c.Request.Context()).Debugf("[ResponsesAPI] Starting Responses to Anthropic stream, model=%s", responseModel)
	defer func() {
		if r := recover(); r != nil {
			logrus.WithContext(c.Request.Context()).Errorf("Panic in Responses to Anthropic streaming handler: %v", r)
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
		logrus.WithContext(c.Request.Context()).Debug("[ResponsesAPI] Finished Responses to Anthropic stream")
	}()

	conv := newResponsesToAnthropicConverter(c.Request.Context(), stream, responseModel)
	_, err := RunConverter(hc, conv, anthropicSSEWriterWithFirstChunk(c))

	// Protocol-level error (response.failed, etc.): SSE error event already sent by converter.
	if hookErr := conv.HookErr(); hookErr != nil {
		logrus.WithContext(c.Request.Context()).Errorf("[ResponsesAPI] Protocol error: %v", hookErr)
		return conv.Usage(), hookErr
	}

	if err != nil {
		if errors.Is(err, context.Canceled) {
			logrus.WithContext(c.Request.Context()).Debug("[ResponsesAPI] Stream canceled by client")
			return conv.Usage(), nil
		}
		logrus.WithContext(c.Request.Context()).Errorf("[ResponsesAPI] Stream error: %v", err)
		hc.DispatchStreamError(err)
		sendAnthropicStreamEvent(c, "error", map[string]interface{}{
			"type": "error",
			"error": map[string]interface{}{
				"message": err.Error(),
				"type":    "stream_error",
				"code":    "stream_failed",
			},
		}, nil)
		return conv.Usage(), err
	}

	if streamErr := stream.Err(); streamErr != nil {
		if errors.Is(streamErr, context.Canceled) {
			logrus.WithContext(c.Request.Context()).Debug("[ResponsesAPI] Stream canceled by client")
			return conv.Usage(), nil
		}
		logrus.WithContext(c.Request.Context()).Errorf("[ResponsesAPI] Stream error: %v", streamErr)
		hc.DispatchStreamError(streamErr)
		sendAnthropicStreamEvent(c, "error", map[string]interface{}{
			"type": "error",
			"error": map[string]interface{}{
				"message": streamErr.Error(),
				"type":    "stream_error",
				"code":    "stream_failed",
			},
		}, nil)
		return conv.Usage(), streamErr
	}

	return conv.Usage(), nil
}

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

func applyTokenUsageToStreamState(state *streamState, usage *protocol.TokenUsage) {
	if state == nil || usage == nil {
		return
	}
	state.inputTokens = int64(usage.InputTokens)
	state.outputTokens = int64(usage.OutputTokens)
	state.cacheTokens = int64(usage.CacheInputTokens)
	state.reasoningTokens = int64(usage.ReasoningTokens)
}

// HandleOpenAIToAnthropicStreamResponse processes OpenAI streaming events and converts them to Anthropic format.
// Returns UsageStat containing token usage information for tracking.
func HandleOpenAIToAnthropicStreamResponse(hc *protocol.HandleContext, req *openai.ChatCompletionNewParams, stream *openaistream.Stream[openai.ChatCompletionChunk], responseModel string) (*protocol.TokenUsage, error) {
	return handleOpenAIToAnthropicStreamResponse(hc, req, stream, responseModel, nil)
}

// HandleOpenAIToAnthropicStreamResponseWithMCPHooks enables MCP-aware tool suppression/finalization during conversion.
func HandleOpenAIToAnthropicStreamResponseWithMCPHooks(
	hc *protocol.HandleContext,
	req *openai.ChatCompletionNewParams,
	stream *openaistream.Stream[openai.ChatCompletionChunk],
	responseModel string,
	hooks *OpenAIToAnthropicMCPHooks,
) (*protocol.TokenUsage, error) {
	return handleOpenAIToAnthropicStreamResponse(hc, req, stream, responseModel, hooks)
}

func handleOpenAIToAnthropicStreamResponse(
	hc *protocol.HandleContext,
	req *openai.ChatCompletionNewParams,
	stream *openaistream.Stream[openai.ChatCompletionChunk],
	responseModel string,
	hooks *OpenAIToAnthropicMCPHooks,
) (*protocol.TokenUsage, error) {
	c := hc.GinContext
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

	conv := newOpenAIToAnthropicConverter(stream, responseModel, req, hooks, mapOpenAIFinishReasonToAnthropic)
	_, err := RunConverter(hc, conv, anthropicSSEWriter(c))

	if hookErr := conv.HookErr(); hookErr != nil {
		return conv.Usage(), hookErr
	}
	if err != nil {
		if errors.Is(err, context.Canceled) {
			logrus.WithContext(c.Request.Context()).Debug("OpenAI to Anthropic stream canceled by client")
			return conv.Usage(), nil
		}
		logrus.WithContext(c.Request.Context()).Errorf("OpenAI stream error: %v", err)
		hc.DispatchStreamError(err)
		if !conv.MessageStarted() {
			SendStreamingError(c, err)
			return conv.Usage(), err
		}
		sendAnthropicStreamEvent(c, "error", map[string]interface{}{
			"type": "error",
			"error": map[string]interface{}{
				"message": err.Error(),
				"type":    "stream_error",
				"code":    "stream_failed",
			},
		}, nil)
		return conv.Usage(), err
	}
	if streamErr := stream.Err(); streamErr != nil {
		if errors.Is(streamErr, context.Canceled) {
			logrus.WithContext(c.Request.Context()).Debug("OpenAI to Anthropic stream canceled by client")
			return conv.Usage(), nil
		}
		logrus.WithContext(c.Request.Context()).Errorf("OpenAI stream error: %v", streamErr)
		hc.DispatchStreamError(streamErr)
		if !conv.MessageStarted() {
			SendStreamingError(c, streamErr)
			return conv.Usage(), streamErr
		}
		sendAnthropicStreamEvent(c, "error", map[string]interface{}{
			"type": "error",
			"error": map[string]interface{}{
				"message": streamErr.Error(),
				"type":    "stream_error",
				"code":    "stream_failed",
			},
		}, nil)
		return conv.Usage(), streamErr
	}
	if errors.Is(c.Request.Context().Err(), context.Canceled) {
		return conv.Usage(), context.Canceled
	}
	return conv.Usage(), nil
}

// HandleResponsesToAnthropicV1Stream processes OpenAI Responses API streaming events and converts them to Anthropic v1 format.
// Returns TokenUsage containing token usage information for tracking.
func HandleResponsesToAnthropicV1Stream(hc *protocol.HandleContext, stream ResponsesStreamIter, responseModel string) (*protocol.TokenUsage, error) {
	return handleResponsesToAnthropicStream(hc, stream, responseModel)
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
	usage := protocol.ZeroTokenUsage()

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
			usage = protocolusage.FromOpenAIResponses(completed.Response.Usage)
			applyTokenUsageToStreamState(state, usage)

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
			return usage, nil

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

		case "response.incomplete":
			// response.incomplete is a terminal Responses status (max_output_tokens,
			// content_filter), not a transport error. Preserve the partial output
			// and map to Anthropic's max_tokens stop reason.
			incomplete := currentEvent.AsResponseIncomplete()
			usage = protocolusage.FromOpenAIResponses(incomplete.Response.Usage)
			applyTokenUsageToStreamState(state, usage)

			senders.SendStopEvents(state, flusher)

			stopReason := "max_tokens"
			if incomplete.Response.IncompleteDetails.Reason == "content_filter" {
				stopReason = "end_turn"
			}

			senders.SendMessageDelta(state, stopReason, flusher)
			senders.SendMessageStop(messageID, responseModel, state, stopReason, flusher)

			logrus.WithContext(c.Request.Context()).Debugf("[ResponsesAPI] Incomplete response: reason=%s, stop_reason=%s", incomplete.Response.IncompleteDetails.Reason, stopReason)
			return usage, nil

		case "error", "response.failed":
			logrus.WithContext(c.Request.Context()).Errorf("Responses API error event: %v", currentEvent)
			errorEvent := map[string]interface{}{
				"type": "error",
				"error": map[string]interface{}{
					"message": fmt.Sprintf("Responses API error: %v", currentEvent),
					"type":    "api_error",
				},
			}
			senders.SendErrorEvent(errorEvent, flusher)
			return usage, fmt.Errorf("Responses API error: %v", currentEvent)

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
		return usage, err
	}

	return usage, nil
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
