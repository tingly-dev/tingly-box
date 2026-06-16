package middleware

import (
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"time"

	"github.com/gin-gonic/gin"
	"github.com/sirupsen/logrus"
	"gopkg.in/natefinch/lumberjack.v2"
)

// Rotation settings for the bad-request sink (smart defaults, not user-tunable).
const (
	badRequestMaxFileMB = 10
	badRequestMaxFiles  = 5
)

// logEntry is the in-memory record the middleware hands to the sink. It is not
// serialized directly — maybeLog renders the on-disk JSON (bodies as raw JSON
// when valid, else quoted; duration in ms) — so it carries no json tags.
type logEntry struct {
	Timestamp             time.Time
	Method                string
	Path                  string
	Query                 string
	StatusCode            int
	Duration              time.Duration
	RequestBody           []byte
	RequestBodyTruncated  bool
	ResponseBody          []byte
	ResponseBodyTruncated bool
	Headers               map[string]string
	UserAgent             string
	ClientIP              string
}

// BadRequestSink persistently records error request/response pairs (4xx/5xx on
// the /api or /tbe proxy paths) to a dedicated rotating file, independent of the
// general structured log. It owns no middleware: the unified logging middleware
// feeds it the already-captured request/response bytes, so bodies are captured
// exactly once. Rotation is delegated to lumberjack (as in pkg/obs).
type BadRequestSink struct {
	writer  *lumberjack.Logger
	mu      sync.Mutex
	enabled bool
}

// NewBadRequestSink creates a sink writing to logPath, rotated by lumberjack
// with smart defaults (badRequestMaxFileMB per file, badRequestMaxFiles kept).
func NewBadRequestSink(logPath string) *BadRequestSink {
	s := &BadRequestSink{enabled: true}
	if logPath != "" {
		if err := os.MkdirAll(filepath.Dir(logPath), 0755); err != nil {
			logrus.Errorf("Failed to create bad-request log directory: %v", err)
			return s
		}
		s.writer = &lumberjack.Logger{
			Filename:   logPath,
			MaxSize:    badRequestMaxFileMB,
			MaxBackups: badRequestMaxFiles,
			Compress:   false, // keep readable for diagnostics
		}
	}
	return s
}

// shouldLog records error responses (>=400) on the AI/proxy paths. This is a
// fixed predicate by design — a configurable filter was carried over from the
// old middleware but never actually used, so it's not worth the machinery.
func shouldLog(entry *logEntry) bool {
	return entry.StatusCode >= 400 &&
		(strings.HasPrefix(entry.Path, "/api/") || strings.HasPrefix(entry.Path, "/tbe/"))
}

// maybeLog writes entry to disk when it is enabled and matches the predicate.
// Rotation is handled transparently by the lumberjack writer.
func (s *BadRequestSink) maybeLog(entry *logEntry) {
	if s == nil || !shouldLog(entry) {
		return
	}
	s.mu.Lock()
	defer s.mu.Unlock()
	if !s.enabled || s.writer == nil {
		return
	}

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
	if len(entry.RequestBody) > 0 {
		if json.Valid(entry.RequestBody) {
			logData["request_body"] = json.RawMessage(entry.RequestBody)
		} else {
			logData["request_body"] = string(entry.RequestBody)
		}
	}
	if entry.RequestBodyTruncated {
		logData["request_body_truncated"] = true
	}
	if len(entry.ResponseBody) > 0 {
		logData["response_body"] = string(entry.ResponseBody)
	}
	if entry.ResponseBodyTruncated {
		logData["response_body_truncated"] = true
	}

	jsonData, err := json.Marshal(logData)
	if err != nil {
		logrus.Errorf("Failed to marshal bad-request log entry: %v", err)
		return
	}
	if _, err := s.writer.Write(append(jsonData, '\n')); err != nil {
		logrus.Errorf("Failed to write bad-request log entry: %v", err)
	}
}

// Stop closes the log file and disables further writes.
func (s *BadRequestSink) Stop() {
	if s == nil {
		return
	}
	s.mu.Lock()
	defer s.mu.Unlock()
	if s.writer != nil {
		if err := s.writer.Close(); err != nil {
			logrus.Errorf("Failed to close bad-request log file: %v", err)
		}
		s.writer = nil
	}
	s.enabled = false
}

// getHeaders extracts a relevant, masked subset of request headers.
func getHeaders(c *gin.Context) map[string]string {
	headers := make(map[string]string)
	relevant := []string{
		"Authorization", "Content-Type", "Accept", "User-Agent",
		"X-Forwarded-For", "X-Real-IP", "X-Request-ID",
	}
	for _, header := range relevant {
		if value := c.GetHeader(header); value != "" {
			if header == "Authorization" && len(value) > 10 {
				headers[header] = value[:7] + "..."
			} else {
				headers[header] = value
			}
		}
	}
	return headers
}
