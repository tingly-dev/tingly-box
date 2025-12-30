package middleware

import (
	"fmt"
	"io"
	"net/http"
	"time"
	"tingly-box/pkg/obs"

	"github.com/gin-gonic/gin"
	"github.com/sirupsen/logrus"
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

		entry := m.logger.WithFields(logrus.Fields{
			"status":     statusCode,
			"latency":    latency,
			"client_ip":  clientIP,
			"method":     method,
			"path":       path,
			"body_size":  bodySize,
			"user_agent": c.Request.UserAgent(),
		})

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
