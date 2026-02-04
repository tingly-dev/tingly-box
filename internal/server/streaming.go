package server

import (
	"context"
	"encoding/json"
	"errors"
	"io"
	"net/http"

	"github.com/gin-gonic/gin"
	"github.com/sirupsen/logrus"
)

// StreamHandlerFunc is the callback function for streaming operations.
// It receives the request context and gin.Context, and returns an error if streaming should stop.
// Return nil to continue streaming, any error to stop.
type StreamHandlerFunc func(ctx context.Context, c *gin.Context) error

// StreamWithContext wraps streaming with proper context handling and SSE headers.
// It checks for context cancellation and handles panic recovery.
func StreamWithContext(c *gin.Context, handler StreamHandlerFunc) {
	// Set SSE headers
	c.Header("Content-Type", "text/event-stream")
	c.Header("Cache-Control", "no-cache")
	c.Header("Connection", "keep-alive")
	c.Header("Access-Control-Allow-Origin", "*")
	c.Header("Access-Control-Allow-Headers", "Cache-Control")

	// Check if streaming is supported
	_, ok := c.Writer.(http.Flusher)
	if !ok {
		c.JSON(http.StatusInternalServerError, ErrorResponse{
			Error: ErrorDetail{
				Message: "Streaming not supported by this connection",
				Type:    "api_error",
				Code:    "streaming_unsupported",
			},
		})
		return
	}

	// Use gin.Stream for proper streaming handling
	c.Stream(func(w io.Writer) bool {
		// Check context cancellation first
		select {
		case <-c.Request.Context().Done():
			logrus.Debug("Client disconnected, stopping stream")
			return false
		default:
		}

		// Call the handler
		err := handler(c.Request.Context(), c)

		// If handler returns an error, check if we should continue
		if err != nil {
			// Context cancellation is expected, don't log as error
			if errors.Is(err, context.Canceled) {
				logrus.Debug("Stream canceled due to context cancellation")
				return false
			}
			// Other errors - log and stop
			logrus.Errorf("Stream handler error: %v", err)
			return false
		}

		// Continue streaming
		return true
	})
}

// StreamSSEvent sends an SSE event with the given data.
// Returns an error if JSON marshaling fails.
func StreamSSEvent(c *gin.Context, eventType string, data interface{}) error {
	jsonData, err := json.Marshal(data)
	if err != nil {
		return err
	}
	c.SSEvent(eventType, string(jsonData))
	return nil
}

// StreamSSEventRaw sends an SSE event with raw string data.
func StreamSSEventRaw(c *gin.Context, eventType string, data string) {
	c.SSEvent(eventType, data)
}

// StreamSendDone sends the final [DONE] message for OpenAI-style streams.
func StreamSendDone(c *gin.Context) {
	c.SSEvent("", "[DONE]")
}

// StreamError sends an error event through SSE.
func StreamError(c *gin.Context, message, errorType, code string) {
	errorData := map[string]interface{}{
		"error": map[string]interface{}{
			"message": message,
			"type":    errorType,
			"code":    code,
		},
	}
	StreamSSEvent(c, "error", errorData)
}

// IsContextCanceled checks if the error is due to context cancellation.
func IsContextCanceled(err error) bool {
	return errors.Is(err, context.Canceled)
}

// CheckContext checks if context is canceled and returns true if so.
func CheckContext(ctx context.Context) bool {
	select {
	case <-ctx.Done():
		return true
	default:
		return false
	}
}
