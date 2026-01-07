package server

import (
	"encoding/json"
	"fmt"
	"net/http"

	"github.com/gin-gonic/gin"
	"github.com/sirupsen/logrus"
)

// SSEEventWriter is an interface for writing SSE events
type SSEEventWriter interface {
	SSEvent(event string, data string)
	Header(key, value string)
}

// StreamRecoveryHandler provides panic recovery for streaming handlers
func StreamRecoveryHandler(c *gin.Context, stream StreamClosable) {
	if r := recover(); r != nil {
		logrus.Debugf("Panic in Anthropic streaming handler: %v", r)
		SendSSErrorEvent(c, "Internal streaming error", "internal_error")
		if flusher, ok := c.Writer.(http.Flusher); ok {
			flusher.Flush()
		}
	}
	// Ensure stream is always closed
	if stream != nil {
		if err := stream.Close(); err != nil {
			logrus.Debugf("Error closing Anthropic stream: %v", err)
		}
	}
}

// StreamClosable is an interface for types that can be closed
type StreamClosable interface {
	Close() error
}

// SetupSSEHeaders sets up the required headers for Server-Sent Events
func SetupSSEHeaders(c *gin.Context) {
	c.Header("Content-Type", "text/event-stream")
	c.Header("Cache-Control", "no-cache")
	c.Header("Connection", "keep-alive")
	c.Header("Access-Control-Allow-Origin", "*")
	c.Header("Access-Control-Allow-Headers", "Cache-Control")
}

// CheckSSESupport verifies if the connection supports SSE
func CheckSSESupport(c *gin.Context) bool {
	_, ok := c.Writer.(http.Flusher)
	if !ok {
		c.JSON(http.StatusInternalServerError, ErrorResponse{
			Error: ErrorDetail{
				Message: "Streaming not supported by this connection",
				Type:    "api_error",
				Code:    "streaming_unsupported",
			},
		})
		return false
	}
	return true
}

// SendSSErrorEvent sends an error event through SSE
func SendSSErrorEvent(c *gin.Context, message, errorType string) {
	c.SSEvent("error", "{\"error\":{\"message\":\""+message+"\",\"type\":\""+errorType+"\"}}")
}

// SendSSErrorEventJSON sends a JSON error event through SSE
func SendSSErrorEventJSON(c *gin.Context, errorJSON []byte) {
	c.SSEvent("error", string(errorJSON))
}

// SendSSEEvent sends a generic SSE event with JSON data
func SendSSEEvent(c *gin.Context, eventType string, data interface{}) error {
	eventJSON, err := json.Marshal(data)
	if err != nil {
		logrus.Debugf("Failed to marshal SSE event: %v", err)
		return err
	}
	c.SSEvent(eventType, string(eventJSON))
	return nil
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

// ParseAndSendStreamError handles stream errors and sends appropriate error events
func ParseAndSendStreamError(c *gin.Context, err error) {
	logrus.Debugf("Anthropic stream error: %v", err)
	MarshalAndSendErrorEvent(c, err.Error(), "stream_error", "stream_failed")
}

// =============================================
// HTTP Error Response Helpers
// =============================================

// SendInvalidRequestBodyError sends an error response for invalid request body
func SendInvalidRequestBodyError(c *gin.Context, err error) {
	c.JSON(http.StatusBadRequest, ErrorResponse{
		Error: ErrorDetail{
			Message: "Invalid request body: " + err.Error(),
			Type:    "invalid_request_error",
		},
	})
}

// SendStreamingError sends an error response for streaming request failures
func SendStreamingError(c *gin.Context, err error) {
	c.JSON(http.StatusInternalServerError, ErrorResponse{
		Error: ErrorDetail{
			Message: "Failed to create streaming request: " + err.Error(),
			Type:    "api_error",
		},
	})
}

// SendForwardingError sends an error response for request forwarding failures
func SendForwardingError(c *gin.Context, err error) {
	c.JSON(http.StatusInternalServerError, ErrorResponse{
		Error: ErrorDetail{
			Message: "Failed to forward request: " + err.Error(),
			Type:    "api_error",
		},
	})
}

// SendAdapterDisabledError sends an error response when adapter is disabled
func SendAdapterDisabledError(c *gin.Context, providerName string) {
	c.JSON(http.StatusUnprocessableEntity, ErrorResponse{
		Error: ErrorDetail{
			Message: fmt.Sprintf("Request format adaptation is disabled. Cannot send Anthropic request to OpenAI-style provider '%s'. Use --adapter flag to enable format conversion.", providerName),
			Type:    "adapter_disabled",
		},
	})
}

// SendInternalError sends an error response for internal errors
func SendInternalError(c *gin.Context, errMsg string) {
	c.JSON(http.StatusInternalServerError, ErrorResponse{
		Error: ErrorDetail{
			Message: errMsg,
			Type:    "api_error",
			Code:    "streaming_unsupported",
		},
	})
}
