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

// MemoryLogMiddleware provides Gin middleware with memory log storage
type MemoryLogMiddleware struct {
	hook   *obs.MemoryLogHook
	logger *logrus.Logger
}

// NewMemoryLogMiddleware creates a new memory log middleware
func NewMemoryLogMiddleware(maxEntries int) *MemoryLogMiddleware {
	hook := obs.NewMemoryLogHook(maxEntries)

	logger := logrus.New()
	logger.SetOutput(io.Discard) // Discard default output, only use hook
	logger.AddHook(hook)

	return &MemoryLogMiddleware{
		hook:   hook,
		logger: logger,
	}
}

// Middleware returns a Gin middleware compatible with gin.Logger()
// It logs all HTTP requests to the memory log hook
func (m *MemoryLogMiddleware) Middleware() gin.HandlerFunc {
	return func(c *gin.Context) {
		// Skip health checks and static assets
		if c.Request.URL.Path == "/health" ||
			c.Request.URL.Path == "/favicon.ico" ||
			c.Request.URL.Path == "/robots.txt" {
			c.Next()
			return
		}

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

		fields := logrus.Fields{
			"status":     statusCode,
			"latency":    latency,
			"client_ip":  clientIP,
			"method":     method,
			"path":       path,
			"body_size":  bodySize,
			"user_agent": c.Request.UserAgent(),
		}

		// Extract error message from response body for 4xx/5xx responses
		if statusCode >= http.StatusBadRequest && w.body.Len() > 0 {
			if errMsg := extractErrorMessage(w.body.Bytes()); errMsg != "" {
				fields["error"] = errMsg
			}
		}

		entry := m.logger.WithFields(fields)

		msg := fmt.Sprintf("%s %s %d %v %s %d",
			method,
			path,
			statusCode,
			latency,
			clientIP,
			bodySize,
		)

		if statusCode >= http.StatusInternalServerError {
			entry.Error(msg)
		} else if statusCode >= http.StatusBadRequest {
			entry.Warn(msg)
		} else {
			entry.Info(msg)
		}
	}
}

// extractErrorMessage parses JSON response body to extract error message
func extractErrorMessage(body []byte) string {
	if !json.Valid(body) {
		// Not JSON, return as string if short enough
		if len(body) <= 200 {
			return string(body)
		}
		return string(body[:200]) + "..."
	}

	// Try to extract "error" field from JSON
	var parsed map[string]interface{}
	if err := json.Unmarshal(body, &parsed); err != nil {
		return ""
	}

	// Handle gin.H{"error": "..."} format
	if errMsg, ok := parsed["error"]; ok {
		switch v := errMsg.(type) {
		case string:
			return v
		case map[string]interface{}:
			// Handle {"error": {"message": "...", "type": "..."}} format
			if msg, ok := v["message"].(string); ok {
				return msg
			}
		}
	}

	return ""
}

// GetEntries returns all log entries in chronological order
func (m *MemoryLogMiddleware) GetEntries() []*logrus.Entry {
	return m.hook.GetEntries()
}

// GetLatest returns the newest N log entries
func (m *MemoryLogMiddleware) GetLatest(n int) []*logrus.Entry {
	return m.hook.GetLatest(n)
}

// GetEntriesSince returns log entries after the specified time
func (m *MemoryLogMiddleware) GetEntriesSince(since time.Time) []*logrus.Entry {
	return m.hook.GetEntriesSince(since)
}

// GetEntriesByLevel returns log entries matching the specified level
func (m *MemoryLogMiddleware) GetEntriesByLevel(level logrus.Level) []*logrus.Entry {
	return m.hook.GetEntriesByLevel(level)
}

// Clear removes all log entries
func (m *MemoryLogMiddleware) Clear() {
	m.hook.Clear()
}

// Size returns the current number of stored log entries
func (m *MemoryLogMiddleware) Size() int {
	return m.hook.Size()
}
