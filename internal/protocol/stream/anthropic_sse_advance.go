package stream

import (
	"net/http"

	"github.com/gin-gonic/gin"
)

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

// sendAnthropicV1MessageStop sends a message_stop event with token usage.
func sendAnthropicV1MessageStop(c *gin.Context, inputTokens, outputTokens int, flusher http.Flusher) {
	event := map[string]interface{}{
		"type": eventTypeMessageStop,
		"message": map[string]interface{}{
			"id":            "",
			"type":          "message",
			"role":          "assistant",
			"content":       []interface{}{},
			"model":         "",
			"stop_reason":   anthropicStopReasonEndTurn,
			"stop_sequence": nil,
			"usage": map[string]interface{}{
				"input_tokens":  inputTokens,
				"output_tokens": outputTokens,
			},
		},
	}
	sendAnthropicStreamEvent(c, eventTypeMessageStop, event, flusher)
}
