package stream

import (
	"encoding/json"
	"errors"
	"fmt"
	"net/http"
	"time"

	"github.com/anthropics/anthropic-sdk-go"
	"github.com/gin-gonic/gin"
	"github.com/openai/openai-go/v3"
	openaistream "github.com/openai/openai-go/v3/packages/ssestream"
	"github.com/openai/openai-go/v3/responses"
	"github.com/sirupsen/logrus"
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

// HandleOpenAIToAnthropicStreamResponse processes OpenAI streaming events and converts them to Anthropic format
func HandleOpenAIToAnthropicStreamResponse(c *gin.Context, req *openai.ChatCompletionNewParams, stream *openaistream.Stream[openai.ChatCompletionChunk], responseModel string) error {
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

					// Initialize pending tool call
					state.pendingToolCalls[anthropicIndex] = &pendingToolCall{
						id:   toolCall.ID,
						name: toolCall.Function.Name,
					}

					// Send content_block_start for tool_use
					sendContentBlockStart(c, anthropicIndex, blockTypeToolUse, map[string]interface{}{
						"id":   toolCall.ID,
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
		return err
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

	// Check if original request was v1 format for logging purposes
	originalFormat := "v1"
	if fmt, exists := c.Get("original_request_format"); exists {
		if formatStr, ok := fmt.(string); ok {
			originalFormat = formatStr
		}
	}

	// Log the event being sent for debugging (important events and content events)
	if eventType == "message_start" || eventType == "message_stop" || eventType == "content_block_start" || eventType == "content_block_delta" || eventType == "content_block_stop" || eventType == "error" {
		logrus.Infof("[V1Stream] Sending SSE event: type=%s, original_format=%s, data=%s", eventType, originalFormat, string(eventJSON))
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
	textBlockIndex        int
	thinkingBlockIndex    int
	hasTextContent        bool
	nextBlockIndex        int
	pendingToolCalls      map[int]*pendingToolCall
	toolIndexToBlockIndex map[int]int
	deltaExtras           map[string]interface{}
	outputTokens          int64
	inputTokens           int64
	stoppedBlocks         map[int]bool // Tracks blocks that have already sent content_block_stop
}

// newStreamState creates a new streamState
func newStreamState() *streamState {
	return &streamState{
		textBlockIndex:        -1,
		thinkingBlockIndex:    -1,
		nextBlockIndex:        0,
		pendingToolCalls:      make(map[int]*pendingToolCall),
		toolIndexToBlockIndex: make(map[int]int),
		deltaExtras:           make(map[string]interface{}),
		stoppedBlocks:         make(map[int]bool),
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

// responsesAPIEventSenders defines callbacks for sending Anthropic events in a specific format (v1 or beta)
type responsesAPIEventSenders struct {
	SendMessageStart      func(event map[string]interface{}, flusher http.Flusher)
	SendContentBlockStart func(index int, blockType string, content map[string]interface{}, flusher http.Flusher)
	SendContentBlockDelta func(index int, content map[string]interface{}, flusher http.Flusher)
	SendContentBlockStop  func(index int, flusher http.Flusher)
	SendStopEvents        func(state *streamState, flusher http.Flusher)
	SendMessageDelta      func(state *streamState, stopReason string, flusher http.Flusher)
	SendMessageStop       func(messageID, model string, state *streamState, stopReason string, flusher http.Flusher)
	SendErrorEvent        func(event map[string]interface{}, flusher http.Flusher)
}

// HandleResponsesToAnthropicStreamResponse is the shared core logic for processing OpenAI Responses API streams
// and converting them to Anthropic format (v1 or beta depending on the senders provided)
func HandleResponsesToAnthropicStreamResponse(c *gin.Context, stream *openaistream.Stream[responses.ResponseStreamEventUnion], responseModel string, senders responsesAPIEventSenders) error {
	logrus.Infof("[ResponsesAPI] Starting Responses API to Anthropic streaming response handler, model=%s", responseModel)
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
		logrus.Info("[ResponsesAPI] Finished Responses API to Anthropic streaming response handler")
	}()

	// Set SSE headers
	c.Header("Content-Type", "text/event-stream")
	c.Header("Cache-Control", "no-cache")
	c.Header("Connection", "keep-alive")
	c.Header("Access-Control-Allow-Origin", "*")
	c.Header("Access-Control-Allow-Headers", "Cache-Control")

	flusher, ok := c.Writer.(http.Flusher)
	if !ok {
		return fmt.Errorf("streaming not supported by this connection")
	}

	// Generate message ID
	messageID := fmt.Sprintf("msg_%d", time.Now().Unix())

	// Initialize streaming state
	state := newStreamState()
	textBlockIndex := -1

	// Track tool calls by item ID for Responses API
	type pendingToolCall struct {
		blockIndex int
		itemID     string
		name       string
		arguments  string
	}
	pendingToolCalls := make(map[string]*pendingToolCall) // key: itemID

	// Track if any tool calls were processed during the stream
	hasToolCalls := false

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
				if textBlockIndex == -1 {
					textBlockIndex = state.nextBlockIndex
					state.nextBlockIndex++
					senders.SendContentBlockStart(textBlockIndex, blockTypeText, map[string]interface{}{"text": ""}, flusher)
				}
				if partAdded.Part.Text != "" {
					senders.SendContentBlockDelta(textBlockIndex, map[string]interface{}{
						"type": deltaTypeTextDelta,
						"text": partAdded.Part.Text,
					}, flusher)
				}
			}

		case "response.output_text.delta":
			if textBlockIndex == -1 {
				textBlockIndex = state.nextBlockIndex
				state.nextBlockIndex++
				senders.SendContentBlockStart(textBlockIndex, blockTypeText, map[string]interface{}{"text": ""}, flusher)
			}
			textDelta := currentEvent.AsResponseOutputTextDelta()
			senders.SendContentBlockDelta(textBlockIndex, map[string]interface{}{
				"type": deltaTypeTextDelta,
				"text": textDelta.Delta,
			}, flusher)

		case "response.output_text.done", "response.content_part.done":
			if textBlockIndex != -1 {
				senders.SendContentBlockStop(textBlockIndex, flusher)
				state.stoppedBlocks[textBlockIndex] = true
				textBlockIndex = -1
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
				senders.SendContentBlockStop(state.thinkingBlockIndex, flusher)
				state.stoppedBlocks[state.thinkingBlockIndex] = true
				state.thinkingBlockIndex = -1
			}

		case "response.reasoning_summary_text.delta":
			summaryDelta := currentEvent.AsResponseReasoningSummaryTextDelta()
			if textBlockIndex == -1 {
				textBlockIndex = state.nextBlockIndex
				state.nextBlockIndex++
				senders.SendContentBlockStart(textBlockIndex, blockTypeText, map[string]interface{}{"text": ""}, flusher)
			}
			senders.SendContentBlockDelta(textBlockIndex, map[string]interface{}{
				"type": deltaTypeTextDelta,
				"text": summaryDelta.Delta,
			}, flusher)

		case "response.reasoning_summary_text.done":
			if textBlockIndex != -1 {
				senders.SendContentBlockStop(textBlockIndex, flusher)
				state.stoppedBlocks[textBlockIndex] = true
				textBlockIndex = -1
			}

		case "response.refusal.delta":
			refusalDelta := currentEvent.AsResponseRefusalDelta()
			if textBlockIndex == -1 {
				textBlockIndex = state.nextBlockIndex
				state.nextBlockIndex++
				senders.SendContentBlockStart(textBlockIndex, blockTypeText, map[string]interface{}{"text": ""}, flusher)
			}
			senders.SendContentBlockDelta(textBlockIndex, map[string]interface{}{
				"type": deltaTypeTextDelta,
				"text": refusalDelta.Delta,
			}, flusher)

		case "response.refusal.done":
			if textBlockIndex != -1 {
				senders.SendContentBlockStop(textBlockIndex, flusher)
				state.stoppedBlocks[textBlockIndex] = true
				textBlockIndex = -1
			}

		case "response.output_item.added":
			itemAdded := currentEvent.AsResponseOutputItemAdded()
			if itemAdded.Item.Type == "function_call" || itemAdded.Item.Type == "custom_tool_call" || itemAdded.Item.Type == "mcp_call" {
				itemID := itemAdded.Item.ID
				blockIndex := state.nextBlockIndex
				state.nextBlockIndex++

				toolName := ""
				if itemAdded.Item.Name != "" {
					toolName = itemAdded.Item.Name
				}

				pendingToolCalls[itemID] = &pendingToolCall{
					blockIndex: blockIndex,
					itemID:     itemID,
					name:       toolName,
					arguments:  "",
				}
				hasToolCalls = true

				senders.SendContentBlockStart(blockIndex, blockTypeToolUse, map[string]interface{}{
					"id":   itemID,
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
				senders.SendContentBlockStop(toolCall.blockIndex, flusher)
				state.stoppedBlocks[toolCall.blockIndex] = true
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
				senders.SendContentBlockStop(toolCall.blockIndex, flusher)
				state.stoppedBlocks[toolCall.blockIndex] = true
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
				senders.SendContentBlockStop(toolCall.blockIndex, flusher)
				state.stoppedBlocks[toolCall.blockIndex] = true
				delete(pendingToolCalls, mcpDone.ItemID)
			}

		case "response.output_item.done":
			// Handled by respective done events above

		case "response.completed":
			completed := currentEvent.AsResponseCompleted()
			state.inputTokens = int64(completed.Response.Usage.InputTokens)
			state.outputTokens = int64(completed.Response.Usage.OutputTokens)

			logrus.Infof("[ResponsesAPI] Response completed: input_tokens=%d, output_tokens=%d", state.inputTokens, state.outputTokens)

			senders.SendStopEvents(state, flusher)

			stopReason := anthropicStopReasonEndTurn
			if hasToolCalls || state.thinkingBlockIndex != -1 {
				stopReason = anthropicStopReasonToolUse
			}

			senders.SendMessageDelta(state, stopReason, flusher)
			senders.SendMessageStop(messageID, responseModel, state, stopReason, flusher)

			logrus.Infof("[ResponsesAPI] Sent message_stop event with stop_reason=%s, finishing stream", stopReason)
			return nil

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
			return fmt.Errorf("Responses API error: %v", currentEvent)

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
		return err
	}

	return nil
}

// HandleResponsesToAnthropicV1StreamResponse processes OpenAI Responses API streaming events and converts them to Anthropic v1 format
// This is a thin wrapper that uses the shared core logic with v1 event senders
func HandleResponsesToAnthropicV1StreamResponse(c *gin.Context, stream *openaistream.Stream[responses.ResponseStreamEventUnion], responseModel string) error {
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
		SendContentBlockStop: func(index int, flusher http.Flusher) {
			sendContentBlockStop(c, index, flusher)
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
