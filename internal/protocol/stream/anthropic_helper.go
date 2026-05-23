package stream

import (
	"encoding/json"
	"fmt"
	"net/http"
	"sort"
	"time"

	"github.com/gin-gonic/gin"
	"github.com/sirupsen/logrus"
	"github.com/tingly-dev/tingly-box/internal/protocol"
)

// SendSSErrorEvent sends an error event through SSE
func SendSSErrorEvent(c *gin.Context, message, errorType string) {
	c.SSEvent("error", "{\"error\":{\"message\":\""+message+"\",\"type\":\""+errorType+"\"}}")
}

// SendSSErrorEventJSON sends a JSON error event through SSE
func SendSSErrorEventJSON(c *gin.Context, errorJSON []byte) {
	c.SSEvent("error", string(errorJSON))
}

// BuildErrorEvent builds a standard error event map
func BuildErrorEvent(message, errorType, code string) map[string]interface{} {
	return map[string]interface{}{
		"type": "error",
		"error": map[string]interface{}{
			"message": message,
			"type":    errorType,
			"code":    code,
		},
	}
}

// MarshalAndSendErrorEvent marshals and sends an error event
func MarshalAndSendErrorEvent(c *gin.Context, message, errorType, code string) {
	errorEvent := BuildErrorEvent(message, errorType, code)
	errorJSON, marshalErr := json.Marshal(errorEvent)
	if marshalErr != nil {
		logrus.WithContext(c.Request.Context()).Debugf("Failed to marshal error event: %v", marshalErr)
		SendSSErrorEvent(c, "Failed to marshal error", "internal_error")
	} else {
		SendSSErrorEventJSON(c, errorJSON)
	}
}

// SendFinishEvent sends a message_stop event to indicate completion
func SendFinishEvent(c *gin.Context) {
	finishEvent := map[string]interface{}{
		"type": "message_stop",
	}
	finishJSON, _ := json.Marshal(finishEvent)
	c.SSEvent("", string(finishJSON))
}

// =============================================
// HTTP Error Response Helpers
// =============================================

// SendInvalidRequestBodyError sends an error response for invalid request body
func SendInvalidRequestBodyError(c *gin.Context, err error) {
	c.JSON(http.StatusBadRequest, protocol.ErrorResponse{
		Error: protocol.ErrorDetail{
			Message: "Invalid request body: " + err.Error(),
			Type:    "invalid_request_error",
		},
	})
}

// SendStreamingError sends an error response for streaming request failures
func SendStreamingError(c *gin.Context, err error) {
	c.Error(err).SetType(gin.ErrorTypePublic) //nolint:errcheck
	c.JSON(http.StatusInternalServerError, protocol.ErrorResponse{
		Error: protocol.ErrorDetail{
			Message: "Failed to create streaming request: " + err.Error(),
			Type:    "api_error",
		},
	})
}

// SendForwardingError sends an error response for request forwarding failures
func SendForwardingError(c *gin.Context, err error) {
	c.Error(err).SetType(gin.ErrorTypePublic) //nolint:errcheck
	c.JSON(http.StatusInternalServerError, protocol.ErrorResponse{
		Error: protocol.ErrorDetail{
			Message: "Failed to forward request: " + err.Error(),
			Type:    "api_error",
		},
	})
}

// SendInternalError sends an error response for internal errors
func SendInternalError(c *gin.Context, errMsg string) {
	c.Error(fmt.Errorf("%s", errMsg)).SetType(gin.ErrorTypePublic) //nolint:errcheck
	c.JSON(http.StatusInternalServerError, protocol.ErrorResponse{
		Error: protocol.ErrorDetail{
			Message: errMsg,
			Type:    "api_error",
			Code:    "streaming_unsupported",
		},
	})
}

// buildStreamState creates a streamState from Anthropic usage stats.
func buildStreamState(inputTokens, outputTokens int64) *streamState {
	s := newStreamState()
	s.inputTokens = inputTokens
	s.outputTokens = outputTokens
	return s
}

// setAnthropicSSEHeaders sets standard SSE headers for Anthropic streaming responses.
func setAnthropicSSEHeaders(c *gin.Context) {
	c.Header("Content-Type", "text/event-stream")
	c.Header("Cache-Control", "no-cache")
	c.Header("Connection", "keep-alive")
	c.Header("Access-Control-Allow-Origin", "*")
	c.Header("Access-Control-Allow-Headers", "Cache-Control")
}

// sendMessageStart emits the message_start SSE event with the given id/model.
func sendMessageStart(
	c *gin.Context,
	flusher http.Flusher,
	model string,
	eventType string,
	sendEvent func(*gin.Context, string, map[string]interface{}, http.Flusher),
	inputTokens int64,
) {
	event := map[string]interface{}{
		"type": eventType,
		"message": map[string]interface{}{
			"id":            fmt.Sprintf("msg_%d", time.Now().Unix()),
			"type":          "message",
			"role":          "assistant",
			"content":       []interface{}{},
			"model":         model,
			"stop_reason":   nil,
			"stop_sequence": nil,
			"usage": map[string]interface{}{
				"input_tokens":  inputTokens,
				"output_tokens": 0,
			},
		},
	}
	sendEvent(c, eventType, event, flusher)
}

// sendAnthropicStreamEvent sends one Anthropic SSE event and optionally records it
// via StreamEventRecorder if one is stored in the Gin context.
func sendAnthropicStreamEvent(c *gin.Context, eventType string, eventData map[string]interface{}, flusher http.Flusher) {
	eventJSON, err := json.Marshal(eventData)
	if err != nil {
		logrus.WithContext(c.Request.Context()).Errorf("Failed to marshal Anthropic stream event: %v", err)
		return
	}

	// Anthropic SSE format: event: <type>\ndata: <json>\n\n
	c.SSEvent(eventType, string(eventJSON))
	flusher.Flush()

	if recorder, exists := c.Get("stream_event_recorder"); exists {
		if r, ok := recorder.(StreamEventRecorder); ok {
			r.RecordRawMapEvent(eventType, eventData)
		}
	}
}

// sendThinkingSignature sends a signature_delta for a thinking block before it is stopped.
// Anthropic extended thinking requires a signature before content_block_stop.
func sendThinkingSignature(c *gin.Context, index int, flusher http.Flusher) {
	// Generate a minimal placeholder signature (base64-encoded random bytes)
	sig := GenerateObfuscationString()
	event := map[string]interface{}{
		"type":  eventTypeContentBlockDelta,
		"index": index,
		"delta": map[string]interface{}{
			"type":      "signature_delta",
			"signature": sig,
		},
	}
	sendAnthropicStreamEvent(c, eventTypeContentBlockDelta, event, flusher)
}

// closeOpenBlock closes any currently open content block, emitting signature_delta first for
// thinking blocks. After this call the block is stopped and its state index reset to -1.
// If no block is open this is a no-op.
func closeOpenBlock(c *gin.Context, state *streamState, flusher http.Flusher) {
	// Thinking block takes priority (it must be stopped before anything else)
	if state.thinkingBlockIndex != -1 && !state.stoppedBlocks[state.thinkingBlockIndex] {
		sendThinkingSignature(c, state.thinkingBlockIndex, flusher)
		sendContentBlockStop(c, state, state.thinkingBlockIndex, flusher)
		state.thinkingBlockIndex = -1
		return
	}
	if state.reasoningSummaryBlockIndex != -1 && !state.stoppedBlocks[state.reasoningSummaryBlockIndex] {
		sendThinkingSignature(c, state.reasoningSummaryBlockIndex, flusher)
		sendContentBlockStop(c, state, state.reasoningSummaryBlockIndex, flusher)
		state.reasoningSummaryBlockIndex = -1
		return
	}
	if state.refusalBlockIndex != -1 && !state.stoppedBlocks[state.refusalBlockIndex] {
		sendContentBlockStop(c, state, state.refusalBlockIndex, flusher)
		state.refusalBlockIndex = -1
		return
	}
	if state.textBlockIndex != -1 && !state.stoppedBlocks[state.textBlockIndex] {
		sendContentBlockStop(c, state, state.textBlockIndex, flusher)
		state.textBlockIndex = -1
		return
	}
}

// sendStopEvents sends content_block_stop events for all active blocks in index order
func sendStopEvents(c *gin.Context, state *streamState, flusher http.Flusher) {
	// Collect block indices to stop
	var blockIndices []int
	if state.thinkingBlockIndex != -1 && !state.stoppedBlocks[state.thinkingBlockIndex] {
		blockIndices = append(blockIndices, state.thinkingBlockIndex)
	}
	if state.refusalBlockIndex != -1 && !state.stoppedBlocks[state.refusalBlockIndex] {
		blockIndices = append(blockIndices, state.refusalBlockIndex)
	}
	if state.reasoningSummaryBlockIndex != -1 && !state.stoppedBlocks[state.reasoningSummaryBlockIndex] {
		blockIndices = append(blockIndices, state.reasoningSummaryBlockIndex)
	}
	if state.textBlockIndex != -1 && !state.stoppedBlocks[state.textBlockIndex] {
		blockIndices = append(blockIndices, state.textBlockIndex)
	}
	for i := range state.pendingToolCalls {
		if !state.stoppedBlocks[i] {
			blockIndices = append(blockIndices, i)
		}
	}

	// Sort by index to stop in order
	sort.Ints(blockIndices)

	// Send stop events in sorted order and mark as stopped
	for _, idx := range blockIndices {
		// Thinking blocks need a signature_delta before content_block_stop
		if state.thinkingBlocks[idx] {
			sendThinkingSignature(c, idx, flusher)
		}
		sendContentBlockStop(c, state, idx, flusher)
	}
}

// sendMessageDelta sends message_delta event
func sendMessageDelta(c *gin.Context, state *streamState, stopReason string, flusher http.Flusher) {
	// Build delta with accumulated extras
	deltaMap := map[string]interface{}{
		"stop_reason":   stopReason,
		"stop_sequence": nil,
	}
	// Merge all collected extra fields
	for k, v := range state.deltaExtras {
		deltaMap[k] = v
	}

	usageMap := map[string]interface{}{
		"output_tokens": state.outputTokens,
	}
	if state.cacheTokens > 0 {
		usageMap["cache_read_input_tokens"] = state.cacheTokens
	}

	event := map[string]interface{}{
		"type":  eventTypeMessageDelta,
		"delta": deltaMap,
		"usage": usageMap,
	}
	sendAnthropicStreamEvent(c, eventTypeMessageDelta, event, flusher)
}

// sendMessageStop sends message_stop event
func sendMessageStop(c *gin.Context, messageID, model string, state *streamState, stopReason string, flusher http.Flusher) {
	event := map[string]interface{}{
		"type": eventTypeMessageStop,
	}
	sendAnthropicStreamEvent(c, eventTypeMessageStop, event, flusher)
}

// sendContentBlockStart sends a content_block_start event
func sendContentBlockStart(c *gin.Context, index int, blockType string, initialContent map[string]interface{}, flusher http.Flusher) {
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
	sendAnthropicStreamEvent(c, eventTypeContentBlockStart, event, flusher)
}

// sendContentBlockDelta sends a content_block_delta event
func sendContentBlockDelta(c *gin.Context, index int, content map[string]interface{}, flusher http.Flusher) {
	event := map[string]interface{}{
		"type":  eventTypeContentBlockDelta,
		"index": index,
		"delta": content,
	}
	sendAnthropicStreamEvent(c, eventTypeContentBlockDelta, event, flusher)
}

// sendContentBlockStop sends a content_block_stop event and marks the block as stopped
func sendContentBlockStop(c *gin.Context, state *streamState, index int, flusher http.Flusher) {
	event := map[string]interface{}{
		"type":  eventTypeContentBlockStop,
		"index": index,
	}
	sendAnthropicStreamEvent(c, eventTypeContentBlockStop, event, flusher)
	state.stoppedBlocks[index] = true
}

// sendAnthropicV1MessageStart sends a message_start event for a simple single-text-block response.
func sendAnthropicV1MessageStart(c *gin.Context, messageID, model string, flusher http.Flusher) {
	event := map[string]interface{}{
		"type": eventTypeMessageStart,
		"message": map[string]interface{}{
			"id":            messageID,
			"type":          "message",
			"role":          "assistant",
			"content":       []interface{}{},
			"model":         model,
			"stop_reason":   nil,
			"stop_sequence": nil,
			"usage": map[string]interface{}{
				"input_tokens":  0,
				"output_tokens": 0,
			},
		},
	}
	sendAnthropicStreamEvent(c, eventTypeMessageStart, event, flusher)
}

// sendAnthropicV1ContentBlockStart sends a content_block_start event at index 0 with an empty text block.
func sendAnthropicV1ContentBlockStart(c *gin.Context, flusher http.Flusher) {
	event := map[string]interface{}{
		"type":  eventTypeContentBlockStart,
		"index": 0,
		"content_block": map[string]interface{}{
			"type": blockTypeText,
			"text": "",
		},
	}
	sendAnthropicStreamEvent(c, eventTypeContentBlockStart, event, flusher)
}

// sendAnthropicV1ContentBlockDelta sends a text_delta content_block_delta event at index 0.
func sendAnthropicV1ContentBlockDelta(c *gin.Context, text string, flusher http.Flusher) {
	event := map[string]interface{}{
		"type":  eventTypeContentBlockDelta,
		"index": 0,
		"delta": map[string]interface{}{
			"type": deltaTypeTextDelta,
			"text": text,
		},
	}
	sendAnthropicStreamEvent(c, eventTypeContentBlockDelta, event, flusher)
}

// sendAnthropicV1ContentBlockStop sends a content_block_stop event at index 0.
func sendAnthropicV1ContentBlockStop(c *gin.Context, flusher http.Flusher) {
	event := map[string]interface{}{
		"type":  eventTypeContentBlockStop,
		"index": 0,
	}
	sendAnthropicStreamEvent(c, eventTypeContentBlockStop, event, flusher)
}

// sendAnthropicV1MessageStop sends a message_stop event.
func sendAnthropicV1MessageStop(c *gin.Context, inputTokens, outputTokens int, flusher http.Flusher) {
	event := map[string]interface{}{
		"type": eventTypeMessageStop,
	}
	sendAnthropicStreamEvent(c, eventTypeMessageStop, event, flusher)
}
