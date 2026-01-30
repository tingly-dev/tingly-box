package stream

import (
	"encoding/json"
	"net/http"
	"sort"

	"github.com/gin-gonic/gin"
)

// sendStopEvents sends content_block_stop events for all active blocks in index order
func sendStopEvents(c *gin.Context, state *streamState, flusher http.Flusher) {
	// Collect block indices to stop
	var blockIndices []int
	if state.thinkingBlockIndex != -1 {
		blockIndices = append(blockIndices, state.thinkingBlockIndex)
	}
	if state.hasTextContent {
		blockIndices = append(blockIndices, state.textBlockIndex)
	}
	for i := range state.pendingToolCalls {
		blockIndices = append(blockIndices, i)
	}

	// Sort by index to stop in order
	sort.Ints(blockIndices)

	// Send stop events in sorted order
	for _, idx := range blockIndices {
		sendContentBlockStop(c, idx, flusher)
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

	event := map[string]interface{}{
		"type":  eventTypeMessageDelta,
		"delta": deltaMap,
		"usage": map[string]interface{}{
			"output_tokens": state.outputTokens,
			"input_tokens":  state.inputTokens,
		},
	}
	sendAnthropicStreamEvent(c, eventTypeMessageDelta, event, flusher)
}

// sendMessageStop sends message_stop event
func sendMessageStop(c *gin.Context, messageID, model string, state *streamState, stopReason string, flusher http.Flusher) {
	// Send message_stop with detailed data
	messageData := map[string]interface{}{
		"id":            messageID,
		"type":          "message",
		"role":          "assistant",
		"content":       []interface{}{},
		"model":         model,
		"stop_reason":   stopReason,
		"stop_sequence": nil,
		"usage": map[string]interface{}{
			"input_tokens":  state.inputTokens,
			"output_tokens": state.outputTokens,
		},
	}
	event := map[string]interface{}{
		"type":    eventTypeMessageStop,
		"message": messageData,
	}
	sendAnthropicStreamEvent(c, eventTypeMessageStop, event, flusher)

	// Send final simple data with type (without event, aka empty)
	// This matches SendFinishEvent behavior from v1 handler
	finishEvent := map[string]interface{}{"type": eventTypeMessageStop}
	finishJSON, _ := json.Marshal(finishEvent)
	c.SSEvent("", string(finishJSON))
	flusher.Flush()
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

// sendContentBlockStop sends a content_block_stop event
func sendContentBlockStop(c *gin.Context, index int, flusher http.Flusher) {
	event := map[string]interface{}{
		"type":  eventTypeContentBlockStop,
		"index": index,
	}
	sendAnthropicStreamEvent(c, eventTypeContentBlockStop, event, flusher)
}
