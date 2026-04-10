package middleware

import (
	"bytes"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"time"

	"github.com/gin-gonic/gin"
	"github.com/sirupsen/logrus"
	"github.com/tingly-dev/tingly-box/pkg/obs"
)

// MultiModeMemoryLogMiddleware provides Gin middleware with both persistent and memory log storage
// Logs are written to:
// 1. Multi-mode logger (text + JSON files for persistence)
// 2. Memory (circular buffer for quick API access)
type MultiModeMemoryLogMiddleware struct {
	logger      *logrus.Logger
	multiLogger *obs.MultiLogger
}

// NewMultiModeMemoryLogMiddleware creates a new middleware with both persistent and memory logging
func NewMultiModeMemoryLogMiddleware(multiLogger *obs.MultiLogger) *MultiModeMemoryLogMiddleware {
	if multiLogger == nil {
		// Fallback for test environments where no multi-logger is configured.
		l := logrus.New()
		if gin.Mode() == gin.TestMode {
			l.SetOutput(io.Discard)
		}
		return &MultiModeMemoryLogMiddleware{
			logger:      l,
			multiLogger: nil,
		}
	}
	// Get a logger scoped to HTTP source
	httpLogger := multiLogger.GetLogrusLogger(obs.LogSourceHTTP)

	return &MultiModeMemoryLogMiddleware{
		logger:      httpLogger,
		multiLogger: multiLogger,
	}
}

// Middleware returns a Gin middleware compatible with gin.Logger()
// It logs all HTTP requests to both the multi-mode logger and memory
func (m *MultiModeMemoryLogMiddleware) Middleware() gin.HandlerFunc {
	return func(c *gin.Context) {
		start := time.Now()
		path := c.Request.URL.Path
		raw := c.Request.URL.RawQuery

		// Wrap response writer to capture body for error responses
		w := &responseBodyWriter{
			ResponseWriter: c.Writer,
			body:           &bytes.Buffer{},
		}
		c.Writer = w

		// Process request
		c.Next()

		// Build log entry manually for more control
		latency := time.Since(start)
		clientIP := c.ClientIP()
		method := c.Request.Method
		statusCode := c.Writer.Status()
		bodySize := c.Writer.Size()

		if raw != "" {
			path = path + "?" + raw
		}

		// Extract error details if any (including panics caught by gin.Recovery)
		var errorMsg string
		var errorType string
		if len(c.Errors) > 0 {
			// Get the last error (most recent)
			lastErr := c.Errors.Last()
			errorMsg = lastErr.Error()
			errorType = string(lastErr.Type)

			// For panic errors, include additional context
			if lastErr.Type == gin.ErrorTypeBind {
				errorType = "bind_error"
			} else if lastErr.Type == gin.ErrorTypePublic {
				errorType = "public_error"
			} else if lastErr.Type == gin.ErrorTypePrivate {
				errorType = "private_error"
			}
		}

		// Build fields with error message if available
		fields := logrus.Fields{
			"type":       "http_request",
			"status":     statusCode,
			"latency":    latency,
			"client_ip":  clientIP,
			"method":     method,
			"path":       path,
			"body_size":  bodySize,
			"user_agent": c.Request.UserAgent(),
		}

		// Add error message field if error occurred
		if errorMsg != "" {
			fields["error"] = errorMsg
			if errorType != "" {
				fields["error_type"] = errorType
			}
		}

		// Add response body for error responses (4xx/5xx)
		if statusCode >= 400 && w.body.Len() > 0 {
			respBytes := w.body.Bytes()
			if json.Valid(respBytes) {
				fields["response_body"] = json.RawMessage(respBytes)
			} else {
				fields["response_body"] = string(respBytes)
			}
		}

		// Log with structured fields including error details
		m.logger.WithFields(fields).Log(getLogLevel(statusCode), fmt.Sprintf("%s %s %d %v %s %d",
			method,
			path,
			statusCode,
			latency,
			clientIP,
			bodySize,
		))
	}
}

// getLogLevel returns the appropriate log level based on status code
func getLogLevel(statusCode int) logrus.Level {
	if statusCode >= http.StatusInternalServerError {
		return logrus.ErrorLevel
	} else if statusCode >= http.StatusBadRequest {
		return logrus.WarnLevel
	}
	return logrus.InfoLevel
}

// GetEntries returns all log entries from memory in chronological order
func (m *MultiModeMemoryLogMiddleware) GetEntries() []*logrus.Entry {
	if m.multiLogger == nil {
		return []*logrus.Entry{}
	}
	httpLogger := m.multiLogger.WithSource(obs.LogSourceHTTP)
	return httpLogger.GetMemoryEntries()
}

// GetLatestEntries returns the newest N log entries from memory
func (m *MultiModeMemoryLogMiddleware) GetLatestEntries(n int) []*logrus.Entry {
	if m.multiLogger == nil {
		return []*logrus.Entry{}
	}
	httpLogger := m.multiLogger.WithSource(obs.LogSourceHTTP)
	return httpLogger.GetMemoryLatest(n)
}

// GetEntriesSince returns log entries from memory after the specified time
func (m *MultiModeMemoryLogMiddleware) GetEntriesSince(since time.Time) []*logrus.Entry {
	// Get the HTTP scoped memory sink from MultiLogger
	memorySink := m.multiLogger.GetMemorySink(obs.LogSourceHTTP)
	if memorySink == nil {
		return []*logrus.Entry{}
	}
	return memorySink.GetEntriesSince(since)
}

// GetEntriesByLevel returns log entries from memory matching the specified level
func (m *MultiModeMemoryLogMiddleware) GetEntriesByLevel(level logrus.Level) []*logrus.Entry {
	// Get the HTTP scoped memory sink from MultiLogger
	memorySink := m.multiLogger.GetMemorySink(obs.LogSourceHTTP)
	if memorySink == nil {
		return []*logrus.Entry{}
	}
	return memorySink.GetEntriesByLevel(level)
}

// Clear removes all log entries from memory
func (m *MultiModeMemoryLogMiddleware) Clear() {
	if m.multiLogger == nil {
		return
	}
	httpLogger := m.multiLogger.WithSource(obs.LogSourceHTTP)
	httpLogger.ClearMemory()
}

// Size returns the current number of stored log entries in memory
func (m *MultiModeMemoryLogMiddleware) Size() int {
	// Get the HTTP scoped memory sink from MultiLogger and return its size
	memorySink := m.multiLogger.GetMemorySink(obs.LogSourceHTTP)
	if memorySink == nil {
		return 0
	}
	return memorySink.Size()
}
