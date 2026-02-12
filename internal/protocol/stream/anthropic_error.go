package stream

import (
	"encoding/json"
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
