package middleware

import (
	"bytes"
	"encoding/json"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"sync"
	"time"

	"github.com/gin-gonic/gin"
	"github.com/sirupsen/logrus"
)

// ErrorLogMiddleware logs requests and responses to a file when debug mode is enabled
type ErrorLogMiddleware struct {
	logFile   *os.File
	logPath   string
	mu        sync.RWMutex
	enabled   bool
	maxSize   int64 // Maximum log file size in bytes
	rotateLog bool
}

// NewErrorLogMiddleware creates a new debug middleware
func NewErrorLogMiddleware(logPath string, maxSizeMB int) *ErrorLogMiddleware {
	dm := &ErrorLogMiddleware{
		logPath:   logPath,
		maxSize:   int64(maxSizeMB) * 1024 * 1024, // Convert MB to bytes
		rotateLog: true,
		enabled:   true, // Debug middleware is enabled when created
	}

	// Create log directory if it doesn't exist
	if logPath != "" {
		if err := os.MkdirAll(filepath.Dir(logPath), 0755); err != nil {
			logrus.Errorf("Failed to create debug log directory: %v", err)
			return dm
		}

		// Open the log file
		dm.openLogFile()
	}

	return dm
}

// Enable enables debug logging
func (dm *ErrorLogMiddleware) Enable() error {
	dm.mu.Lock()
	defer dm.mu.Unlock()

	if dm.enabled {
		return nil
	}

	dm.enabled = true
	return dm.openLogFile()
}

// Disable disables debug logging
func (dm *ErrorLogMiddleware) Disable() {
	dm.mu.Lock()
	defer dm.mu.Unlock()

	if !dm.enabled {
		return
	}

	dm.enabled = false
	if dm.logFile != nil {
		dm.logFile.Close()
		dm.logFile = nil
	}
}

// IsEnabled returns whether debug logging is enabled
func (dm *ErrorLogMiddleware) IsEnabled() bool {
	dm.mu.RLock()
	defer dm.mu.RUnlock()
	return dm.enabled
}

// openLogFile opens or creates the log file
func (dm *ErrorLogMiddleware) openLogFile() error {
	if dm.logFile != nil {
		dm.logFile.Close()
	}

	// Check if we need to rotate the log file
	if dm.rotateLog && dm.fileExists(dm.logPath) {
		if stat, err := os.Stat(dm.logPath); err == nil {
			if stat.Size() >= dm.maxSize {
				if err := dm.rotateLogFile(); err != nil {
					logrus.Errorf("Failed to rotate log file: %v", err)
				}
			}
		}
	}

	file, err := os.OpenFile(dm.logPath, os.O_CREATE|os.O_WRONLY|os.O_APPEND, 0644)
	if err != nil {
		return fmt.Errorf("failed to open debug log file: %w", err)
	}

	dm.logFile = file
	return nil
}

// fileExists checks if a file exists
func (dm *ErrorLogMiddleware) fileExists(path string) bool {
	_, err := os.Stat(path)
	return err == nil
}

// rotateLogFile rotates the current log file
func (dm *ErrorLogMiddleware) rotateLogFile() error {
	// Rename current log file with timestamp
	timestamp := time.Now().Format("20060102-150405")
	oldPath := fmt.Sprintf("%s.%s", dm.logPath, timestamp)

	if dm.logFile != nil {
		dm.logFile.Close()
	}

	if err := os.Rename(dm.logPath, oldPath); err != nil {
		return err
	}

	// Keep only last 5 log files
	pattern := fmt.Sprintf("%s.*", dm.logPath)
	matches, err := filepath.Glob(pattern)
	if err == nil && len(matches) > 5 {
		// Sort by modification time and delete oldest
		for i := 0; i < len(matches)-5; i++ {
			os.Remove(matches[i])
		}
	}

	return nil
}

// Middleware returns the Gin middleware function
func (dm *ErrorLogMiddleware) Middleware() gin.HandlerFunc {
	return func(c *gin.Context) {
		if !dm.IsEnabled() {
			c.Next()
			return
		}

		// Skip logging for health checks and static assets
		if c.Request.URL.Path == "/health" ||
			c.Request.URL.Path == "/favicon.ico" ||
			c.Request.URL.Path == "/robots.txt" {
			c.Next()
			return
		}

		// Read request body
		var requestBody []byte
		if c.Request.Body != nil {
			requestBody, _ = io.ReadAll(c.Request.Body)
			c.Request.Body = io.NopCloser(bytes.NewBuffer(requestBody))
		}

		// Create response writer wrapper to capture response
		w := &responseBodyWriter{
			ResponseWriter: c.Writer,
			body:           &bytes.Buffer{},
		}
		c.Writer = w

		// Process request
		start := time.Now()
		c.Next()
		duration := time.Since(start)

		// Log the request and response
		dm.logEntry(&logEntry{
			Timestamp:    start,
			Method:       c.Request.Method,
			Path:         c.Request.URL.Path,
			Query:        c.Request.URL.RawQuery,
			StatusCode:   c.Writer.Status(),
			Duration:     duration,
			RequestBody:  requestBody,
			ResponseBody: w.body.Bytes(),
			Headers:      getHeaders(c),
			UserAgent:    c.GetHeader("User-Agent"),
			ClientIP:     c.ClientIP(),
		})
	}
}

// logEntry represents a single log entry
type logEntry struct {
	Timestamp    time.Time         `json:"timestamp"`
	Method       string            `json:"method"`
	Path         string            `json:"path"`
	Query        string            `json:"query,omitempty"`
	StatusCode   int               `json:"status_code"`
	Duration     time.Duration     `json:"duration_ms"`
	RequestBody  []byte            `json:"request_body,omitempty"`
	ResponseBody []byte            `json:"response_body,omitempty"`
	Headers      map[string]string `json:"headers,omitempty"`
	UserAgent    string            `json:"user_agent,omitempty"`
	ClientIP     string            `json:"client_ip,omitempty"`
}

// logEntry writes a log entry to the file
func (dm *ErrorLogMiddleware) logEntry(entry *logEntry) {
	dm.mu.RLock()
	defer dm.mu.RUnlock()

	if !dm.enabled || dm.logFile == nil {
		return
	}

	// Only log error responses (4xx, 5xx)
	if entry.StatusCode < 400 {
		return
	}

	// Format for JSON output
	logData := map[string]interface{}{
		"timestamp":   entry.Timestamp.Format(time.RFC3339Nano),
		"method":      entry.Method,
		"path":        entry.Path,
		"query":       entry.Query,
		"status_code": entry.StatusCode,
		"duration_ms": entry.Duration.Milliseconds(),
		"headers":     entry.Headers,
		"user_agent":  entry.UserAgent,
		"client_ip":   entry.ClientIP,
	}

	// Add request body if it's JSON
	if len(entry.RequestBody) > 0 {
		if json.Valid(entry.RequestBody) {
			logData["request_body"] = json.RawMessage(entry.RequestBody)
		} else {
			logData["request_body"] = string(entry.RequestBody)
		}
	}

	// Add response body if it's JSON and status code indicates error
	if entry.StatusCode >= 400 && len(entry.ResponseBody) > 0 {
		if json.Valid(entry.ResponseBody) {
			logData["response_body"] = json.RawMessage(entry.ResponseBody)
		} else {
			logData["response_body"] = string(entry.ResponseBody)
		}
	}

	// Convert to JSON
	jsonData, err := json.Marshal(logData)
	if err != nil {
		logrus.Errorf("Failed to marshal debug log entry: %v", err)
		return
	}

	// Write to file with rotation check
	if dm.rotateLog {
		if stat, err := dm.logFile.Stat(); err == nil {
			if stat.Size() >= dm.maxSize {
				if err := dm.rotateLogFile(); err != nil {
					logrus.Errorf("Failed to rotate log file: %v", err)
				}
				dm.openLogFile()
			}
		}
	}

	// Write the log entry
	if dm.logFile != nil {
		dm.logFile.WriteString(string(jsonData) + "\n")
		dm.logFile.Sync() // Ensure immediate write to disk
	}
}

// getHeaders extracts relevant headers from the request
func getHeaders(c *gin.Context) map[string]string {
	headers := make(map[string]string)

	// Include relevant headers
	relevantHeaders := []string{
		"Authorization",
		"Content-Type",
		"Accept",
		"User-Agent",
		"X-Forwarded-For",
		"X-Real-IP",
		"X-Request-ID",
	}

	for _, header := range relevantHeaders {
		if value := c.GetHeader(header); value != "" {
			// Mask sensitive headers
			if header == "Authorization" && len(value) > 10 {
				headers[header] = value[:7] + "..."
			} else {
				headers[header] = value
			}
		}
	}

	return headers
}

// Stop closes the log file
func (dm *ErrorLogMiddleware) Stop() {
	dm.mu.Lock()
	defer dm.mu.Unlock()

	if dm.logFile != nil {
		dm.logFile.Close()
		dm.logFile = nil
	}
	dm.enabled = false
}
