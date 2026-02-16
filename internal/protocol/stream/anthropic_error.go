package stream

import (
	"encoding/json"
	"errors"
	"net/http"

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
		logrus.Debugf("Failed to marshal error event: %v", marshalErr)
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

// SendGuardrailsTextStream sends a minimal Anthropic stream with a guardrails message.
func SendGuardrailsTextStream(c *gin.Context, beta bool, model, message string) error {
	flusher, ok := c.Writer.(http.Flusher)
	if !ok {
		return errors.New("streaming not supported")
	}

	messageStartEvent := map[string]interface{}{
		"type": eventTypeMessageStart,
		"message": map[string]interface{}{
			"id":            "guardrails_blocked",
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

	if beta {
		sendAnthropicBetaStreamEvent(c, eventTypeMessageStart, messageStartEvent, flusher)
	} else {
		sendAnthropicStreamEvent(c, eventTypeMessageStart, messageStartEvent, flusher)
	}

	if err := emitGuardrailsTextBlock(c, beta, 0, message, flusher); err != nil {
		return err
	}

	SendFinishEvent(c)
	return nil
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
	c.JSON(http.StatusInternalServerError, protocol.ErrorResponse{
		Error: protocol.ErrorDetail{
			Message: "Failed to create streaming request: " + err.Error(),
			Type:    "api_error",
		},
	})
}

// SendForwardingError sends an error response for request forwarding failures
func SendForwardingError(c *gin.Context, err error) {
	c.JSON(http.StatusInternalServerError, protocol.ErrorResponse{
		Error: protocol.ErrorDetail{
			Message: "Failed to forward request: " + err.Error(),
			Type:    "api_error",
		},
	})
}

// SendInternalError sends an error response for internal errors
func SendInternalError(c *gin.Context, errMsg string) {
	c.JSON(http.StatusInternalServerError, protocol.ErrorResponse{
		Error: protocol.ErrorDetail{
			Message: errMsg,
			Type:    "api_error",
			Code:    "streaming_unsupported",
		},
	})
}

func injectGuardrailsBlock(c *gin.Context, beta bool) error {
	val, exists := c.Get("guardrails_block_message")
	if !exists {
		return nil
	}
	message, ok := val.(string)
	if !ok || message == "" {
		return nil
	}

	index := 0
	if raw, ok := c.Get("guardrails_block_index"); ok {
		switch v := raw.(type) {
		case int:
			index = v
		case int64:
			index = int(v)
		case float64:
			index = int(v)
		}
	}
	flusher, ok := c.Writer.(http.Flusher)
	if !ok {
		return errors.New("streaming not supported")
	}

	start := map[string]interface{}{
		"type":  eventTypeContentBlockStart,
		"index": index,
		"content_block": map[string]interface{}{
			"type": "text",
			"text": "",
		},
	}
	delta := map[string]interface{}{
		"type":  eventTypeContentBlockDelta,
		"index": index,
		"delta": map[string]interface{}{
			"type": "text_delta",
			"text": message,
		},
	}
	stop := map[string]interface{}{
		"type":  eventTypeContentBlockStop,
		"index": index,
	}

	if beta {
		sendAnthropicBetaStreamEvent(c, eventTypeContentBlockStart, start, flusher)
		sendAnthropicBetaStreamEvent(c, eventTypeContentBlockDelta, delta, flusher)
		sendAnthropicBetaStreamEvent(c, eventTypeContentBlockStop, stop, flusher)
		return nil
	}

	sendAnthropicStreamEvent(c, eventTypeContentBlockStart, start, flusher)
	sendAnthropicStreamEvent(c, eventTypeContentBlockDelta, delta, flusher)
	sendAnthropicStreamEvent(c, eventTypeContentBlockStop, stop, flusher)
	return nil
}
