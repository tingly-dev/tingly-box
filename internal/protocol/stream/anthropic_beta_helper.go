package stream

import (
	"encoding/json"
	"net/http"
	"sort"

	"github.com/gin-gonic/gin"
	"github.com/sirupsen/logrus"
)

// sendBetaStopEvents sends content_block_stop events for all active blocks in index order (beta)
func sendBetaStopEvents(c *gin.Context, state *streamState, flusher http.Flusher) {
	// Collect block indices to stop
	var blockIndices []int
	if state.thinkingBlockIndex != -1 && !state.stoppedBlocks[state.thinkingBlockIndex] {
		blockIndices = append(blockIndices, state.thinkingBlockIndex)
	}
	if state.hasTextContent && !state.stoppedBlocks[state.textBlockIndex] {
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
		sendBetaContentBlockStop(c, idx, flusher)
		state.stoppedBlocks[idx] = true
	}
}

// sendBetaMessageDelta sends message_delta event for beta
func sendBetaMessageDelta(c *gin.Context, state *streamState, stopReason string, flusher http.Flusher) {
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
	sendAnthropicBetaStreamEvent(c, eventTypeMessageDelta, event, flusher)
}

// sendBetaMessageStop sends message_stop event for beta
func sendBetaMessageStop(c *gin.Context, messageID, model string, state *streamState, stopReason string, flusher http.Flusher) {
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
	sendAnthropicBetaStreamEvent(c, eventTypeMessageStop, event, flusher)

	// Send final simple data with type (without event, aka empty)
	c.SSEvent("", map[string]interface{}{"type": eventTypeMessageStop})
	flusher.Flush()
}

// sendAnthropicBetaStreamEvent helper function to send an event in Anthropic beta SSE format
func sendAnthropicBetaStreamEvent(c *gin.Context, eventType string, eventData map[string]interface{}, flusher http.Flusher) {
	eventJSON, err := json.Marshal(eventData)
	if err != nil {
		logrus.Errorf("Failed to marshal Anthropic beta stream event: %v", err)
		return
	}

	// Anthropic beta SSE format: event: <type>\ndata: <json>\n\n
	c.SSEvent(eventType, string(eventJSON))
	flusher.Flush()
}

// sendBetaContentBlockStart sends a content_block_start event for beta
func sendBetaContentBlockStart(c *gin.Context, index int, blockType string, initialContent map[string]interface{}, flusher http.Flusher) {
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
	sendAnthropicBetaStreamEvent(c, eventTypeContentBlockStart, event, flusher)
}

// sendBetaContentBlockDelta sends a content_block_delta event for beta
func sendBetaContentBlockDelta(c *gin.Context, index int, content map[string]interface{}, flusher http.Flusher) {
	event := map[string]interface{}{
		"type":  eventTypeContentBlockDelta,
		"index": index,
		"delta": content,
	}
	sendAnthropicBetaStreamEvent(c, eventTypeContentBlockDelta, event, flusher)
}

// sendBetaContentBlockStop sends a content_block_stop event for beta
func sendBetaContentBlockStop(c *gin.Context, index int, flusher http.Flusher) {
	event := map[string]interface{}{
		"type":  eventTypeContentBlockStop,
		"index": index,
	}
	sendAnthropicBetaStreamEvent(c, eventTypeContentBlockStop, event, flusher)
}
