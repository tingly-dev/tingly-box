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

		b, _ := json.Marshal(chunk)
		fmt.Printf("chunk: %s\n", b)

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
			logrus.Debugf("Full chunk #%d: %+v", chunkCount, chunk)
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
