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
	"github.com/gin-gonic/gin"
	"github.com/openai/openai-go/v3"
	openaistream "github.com/openai/openai-go/v3/packages/ssestream"
	"github.com/openai/openai-go/v3/responses"
	"github.com/sirupsen/logrus"

	"github.com/tingly-dev/tingly-box/internal/protocol"
)

const (
	// OpenAI finish reasons not defined in openai package
	openaiFinishReasonToolCalls = "tool_calls"

	// Anthropic stop reasons
	anthropicStopReasonEndTurn       = string(anthropic.BetaStopReasonEndTurn)
	anthropicStopReasonMaxTokens     = string(anthropic.BetaStopReasonMaxTokens)
	anthropicStopReasonToolUse       = string(anthropic.BetaStopReasonToolUse)
	anthropicStopReasonContentFilter = string(anthropic.BetaStopReasonRefusal) // "content_filter"

	// OpenAI extra field names that map to Anthropic content blocks
	OpenaiFieldReasoningContent = "reasoning_content"

	// OpenAI tool call ID max length (40 characters per OpenAI API spec)
	maxToolCallIDLength = 40

	// Anthropic event types
	eventTypeMessageStart      = "message_start"
	eventTypeContentBlockStart = "content_block_start"
	eventTypeContentBlockDelta = "content_block_delta"
	eventTypeContentBlockStop  = "content_block_stop"
	eventTypeMessageDelta      = "message_delta"
	eventTypeMessageStop       = "message_stop"
	eventTypeError             = "error"

	// Anthropic block types
	blockTypeText     = "text"
	blockTypeThinking = "thinking"
	blockTypeToolUse  = "tool_use"

	// Anthropic delta types
	deltaTypeTextDelta      = "text_delta"
	deltaTypeThinkingDelta  = "thinking_delta"
	deltaTypeInputJSONDelta = "input_json_delta"
)

// HandleOpenAIToAnthropicStreamResponse processes OpenAI streaming events and converts them to Anthropic format.
// Returns UsageStat containing token usage information for tracking.
func HandleOpenAIToAnthropicStreamResponse(c *gin.Context, req *openai.ChatCompletionNewParams, stream *openaistream.Stream[openai.ChatCompletionChunk], responseModel string) (protocol.UsageStat, error) {
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
		return protocol.ZeroUsageStat(), errors.New("streaming not supported by this connection")
	}

	// Generate message ID for Anthropic format
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

		// Skip empty chunks (no choices)
		if len(chunk.Choices) == 0 {
			// Check for usage info in the last chunk
			if chunk.Usage.PromptTokens > 0 || chunk.Usage.CompletionTokens > 0 {
				state.inputTokens = chunk.Usage.PromptTokens
				state.outputTokens = chunk.Usage.CompletionTokens
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

		// Track usage from chunk
		if chunk.Usage.CompletionTokens > 0 {
			state.inputTokens = chunk.Usage.PromptTokens
			state.outputTokens = chunk.Usage.CompletionTokens
		}

		// Handle finish_reason (last chunk for this choice)
		if choice.FinishReason != "" {
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
		sendAnthropicStreamEvent(c, "error", errorEvent, flusher)
		return protocol.NewUsageStat(int(state.inputTokens), int(state.outputTokens)), err
	}
	return protocol.NewUsageStat(int(state.inputTokens), int(state.outputTokens)), nil
}

// sendAnthropicStreamEvent helper function to send an event in Anthropic SSE format
func sendAnthropicStreamEvent(c *gin.Context, eventType string, eventData map[string]interface{}, flusher http.Flusher) {
	eventJSON, err := json.Marshal(eventData)
	if err != nil {
		logrus.Errorf("Failed to marshal Anthropic stream event: %v", err)
		return
	}

	// Anthropic SSE format: event: <type>\ndata: <json>\n\n
	c.SSEvent(eventType, string(eventJSON))
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

// streamState tracks the streaming conversion state
type streamState struct {
	textBlockIndex             int // Main output text content block
	thinkingBlockIndex         int // Hidden reasoning/thinking block
	refusalBlockIndex          int // Refusal content block (when model refuses)
	reasoningSummaryBlockIndex int // Reasoning summary content block (condensed reasoning shown to user)
	hasTextContent             bool
	nextBlockIndex             int
	pendingToolCalls           map[int]*pendingToolCall
	toolIndexToBlockIndex      map[int]int
	deltaExtras                map[string]interface{}
	outputTokens               int64
	inputTokens                int64
	stoppedBlocks              map[int]bool // Tracks blocks that have already sent content_block_stop
}

// newStreamState creates a new streamState
func newStreamState() *streamState {
	return &streamState{
		textBlockIndex:             -1,
		thinkingBlockIndex:         -1,
		refusalBlockIndex:          -1,
		reasoningSummaryBlockIndex: -1,
		nextBlockIndex:             0,
		pendingToolCalls:           make(map[int]*pendingToolCall),
		toolIndexToBlockIndex:      make(map[int]int),
		deltaExtras:                make(map[string]interface{}),
		stoppedBlocks:              make(map[int]bool),
	}
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

// extractString extracts string value from interface{}, handling different types
func extractString(v interface{}) string {
	switch tv := v.(type) {
	case string:
		return tv
	case []byte:
		return string(tv)
	default:
		return fmt.Sprintf("%v", tv)
	}
}

// truncateToolCallID ensures tool call ID doesn't exceed OpenAI's 40 character limit
// OpenAI API requires tool_call.id to be <= 40 characters
func truncateToolCallID(id string) string {
	if len(id) <= maxToolCallIDLength {
		return id
	}
	// Truncate to max length and add a suffix to indicate truncation
	return id[:maxToolCallIDLength-3] + "..."
}

// responsesAPIEventSenders defines callbacks for sending Anthropic events in a specific format (v1 or beta)
type responsesAPIEventSenders struct {
	SendMessageStart      func(event map[string]interface{}, flusher http.Flusher)
	SendContentBlockStart func(index int, blockType string, content map[string]interface{}, flusher http.Flusher)
	SendContentBlockDelta func(index int, content map[string]interface{}, flusher http.Flusher)
	SendContentBlockStop  func(state *streamState, index int, flusher http.Flusher)
	SendStopEvents        func(state *streamState, flusher http.Flusher)
	SendMessageDelta      func(state *streamState, stopReason string, flusher http.Flusher)
	SendMessageStop       func(messageID, model string, state *streamState, stopReason string, flusher http.Flusher)
	SendErrorEvent        func(event map[string]interface{}, flusher http.Flusher)
}

// HandleResponsesToAnthropicV1StreamResponse processes OpenAI Responses API streaming events and converts them to Anthropic v1 format.
// This is a thin wrapper that uses the shared core logic with v1 event senders.
// Returns UsageStat containing token usage information for tracking.
func HandleResponsesToAnthropicV1StreamResponse(c *gin.Context, stream *openaistream.Stream[responses.ResponseStreamEventUnion], responseModel string) (protocol.UsageStat, error) {
	return HandleResponsesToAnthropicStreamResponse(c, stream, responseModel, responsesAPIEventSenders{
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
