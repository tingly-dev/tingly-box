package middleware

import (
	"fmt"
	"net/http"
	"time"

	"github.com/gin-gonic/gin"
	"github.com/sirupsen/logrus"

	"github.com/tingly-dev/tingly-box/internal/obs"
)

// MultiModeMemoryLogMiddleware provides Gin middleware with both persistent and memory log storage
// Logs are written to:
// 1. Multi-mode logger (text + JSON files for persistence)
// 2. Memory (circular buffer for quick API access)
type MultiModeMemoryLogMiddleware struct {
	logger *logrus.Logger
}

// NewMultiModeMemoryLogMiddleware creates a new middleware with both persistent and memory logging
func NewMultiModeMemoryLogMiddleware(multiLogger *obs.MultiLogger) *MultiModeMemoryLogMiddleware {
	// Get a logger scoped to HTTP source
	httpLogger := multiLogger.GetLogrusLogger(obs.LogSourceHTTP)

	return &MultiModeMemoryLogMiddleware{
		logger: httpLogger,
	}
}

// Middleware returns a Gin middleware compatible with gin.Logger()
// It logs all HTTP requests to both the multi-mode logger and memory
func (m *MultiModeMemoryLogMiddleware) Middleware() gin.HandlerFunc {
	return func(c *gin.Context) {
		start := time.Now()
		path := c.Request.URL.Path
		raw := c.Request.URL.RawQuery

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

		// Log with structured fields
		m.logger.WithFields(logrus.Fields{
			"type":       "http_request",
			"status":     statusCode,
			"latency":    latency,
			"client_ip":  clientIP,
			"method":     method,
			"path":       path,
			"body_size":  bodySize,
			"user_agent": c.Request.UserAgent(),
		}).Log(getLogLevel(statusCode), fmt.Sprintf("%s %s %d %v %s %d",
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

// GetMemoryEntries returns all log entries from memory in chronological order
func (m *MultiModeMemoryLogMiddleware) GetMemoryEntries() []*logrus.Entry {
	// This method is kept for API compatibility but is no longer used
	// The memory sink is managed internally by MultiLogger
	return []*logrus.Entry{}
}

// GetEntries returns all log entries from memory in chronological order (alias for compatibility)
func (m *MultiModeMemoryLogMiddleware) GetEntries() []*logrus.Entry {
	return m.GetMemoryEntries()
}

// GetMemoryLatest returns the newest N log entries from memory
func (m *MultiModeMemoryLogMiddleware) GetMemoryLatest(n int) []*logrus.Entry {
	// This method is kept for API compatibility but is no longer used
	return []*logrus.Entry{}
}

// GetLatest returns the newest N log entries from memory (alias for compatibility)
func (m *MultiModeMemoryLogMiddleware) GetLatest(n int) []*logrus.Entry {
	return m.GetMemoryLatest(n)
}

// GetMemoryEntriesSince returns log entries from memory after the specified time
func (m *MultiModeMemoryLogMiddleware) GetMemoryEntriesSince(since time.Time) []*logrus.Entry {
	return []*logrus.Entry{}
}

// GetEntriesSince returns log entries from memory after the specified time (alias for compatibility)
func (m *MultiModeMemoryLogMiddleware) GetEntriesSince(since time.Time) []*logrus.Entry {
	return m.GetMemoryEntriesSince(since)
}

// GetMemoryEntriesByLevel returns log entries from memory matching the specified level
func (m *MultiModeMemoryLogMiddleware) GetMemoryEntriesByLevel(level logrus.Level) []*logrus.Entry {
	return []*logrus.Entry{}
}

// GetEntriesByLevel returns log entries from memory matching the specified level (alias for compatibility)
func (m *MultiModeMemoryLogMiddleware) GetEntriesByLevel(level logrus.Level) []*logrus.Entry {
	return m.GetMemoryEntriesByLevel(level)
}

// ClearMemory removes all log entries from memory
func (m *MultiModeMemoryLogMiddleware) ClearMemory() {
	// No-op - memory is managed by MultiLogger
}

// Clear removes all log entries from memory (alias for compatibility)
func (m *MultiModeMemoryLogMiddleware) Clear() {
	m.ClearMemory()
}

// MemorySize returns the current number of stored log entries in memory
func (m *MultiModeMemoryLogMiddleware) MemorySize() int {
	// Return 0 - memory is managed by MultiLogger
	return 0
}

// Size returns the current number of stored log entries in memory (alias for compatibility)
func (m *MultiModeMemoryLogMiddleware) Size() int {
	return m.MemorySize()
}
