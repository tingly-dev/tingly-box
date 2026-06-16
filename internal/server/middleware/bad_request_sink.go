package middleware

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"time"

	"github.com/expr-lang/expr"
	"github.com/expr-lang/expr/vm"
	"github.com/gin-gonic/gin"
	"github.com/sirupsen/logrus"
	"gopkg.in/natefinch/lumberjack.v2"
)

// FilterContext provides the context for filter expression evaluation.
type FilterContext struct {
	StatusCode int    `expr:"StatusCode"`
	Method     string `expr:"Method"`
	Path       string `expr:"Path"`
	Query      string `expr:"Query"`
}

// defaultBadRequestFilter matches API/TBE error responses. Kept identical to the
// previous ErrorLogMiddleware default so existing config stays compatible.
const defaultBadRequestFilter = "StatusCode >= 400 && (Path matches '^/api/' || Path matches '^/tbe/')"

// Rotation settings for the bad-request sink (smart defaults, not user-tunable).
const (
	badRequestMaxFileMB = 10
	badRequestMaxFiles  = 5
)

// logEntry represents a single bad-request log entry written to disk.
type logEntry struct {
	Timestamp             time.Time         `json:"timestamp"`
	Method                string            `json:"method"`
	Path                  string            `json:"path"`
	Query                 string            `json:"query,omitempty"`
	StatusCode            int               `json:"status_code"`
	Duration              time.Duration     `json:"duration_ms"`
	RequestBody           []byte            `json:"request_body,omitempty"`
	RequestBodyTruncated  bool              `json:"request_body_truncated,omitempty"`
	ResponseBody          []byte            `json:"response_body,omitempty"`
	ResponseBodyTruncated bool              `json:"response_body_truncated,omitempty"`
	Headers               map[string]string `json:"headers,omitempty"`
	UserAgent             string            `json:"user_agent,omitempty"`
	ClientIP              string            `json:"client_ip,omitempty"`
}

// BadRequestSink persistently records request/response pairs that match a
// configurable expr filter (default: 4xx/5xx on /api or /tbe) to a dedicated
// rotating file, independent of the general structured log. It owns no
// middleware: the unified logging middleware feeds it the already-captured
// request/response bytes, so bodies are captured exactly once.
//
// Rotation is delegated to the standard lumberjack rotator (the same library
// pkg/obs uses for the json/text logs) — no bespoke size/rename/glob logic.
type BadRequestSink struct {
	writer  *lumberjack.Logger
	mu      sync.RWMutex
	enabled bool

	// Compiled expression program for filtering.
	filterProgram  *vm.Program
	filterCompiled bool
	// matchesSub400 is true when the compiled filter can match a <400 status,
	// meaning the unified middleware must capture response bodies for some
	// non-error responses too. Recomputed whenever the filter changes.
	matchesSub400 bool
}

// NewBadRequestSink creates a sink writing to logPath, rotated by lumberjack
// with smart defaults (badRequestMaxFileMB per file, badRequestMaxFiles kept).
func NewBadRequestSink(logPath string) *BadRequestSink {
	s := &BadRequestSink{
		enabled: true,
	}

	if program, err := expr.Compile(defaultBadRequestFilter, expr.Env(FilterContext{})); err != nil {
		logrus.Errorf("Failed to compile default bad-request filter: %v", err)
	} else {
		s.filterProgram = program
		s.filterCompiled = true
		s.matchesSub400 = computeMatchesSub400(program)
	}

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

// SetFilterExpression recompiles and sets a new filter expression. An empty
// expression resets to the default.
func (s *BadRequestSink) SetFilterExpression(expression string) error {
	s.mu.Lock()
	defer s.mu.Unlock()

	if expression == "" {
		expression = defaultBadRequestFilter
	}

	program, err := expr.Compile(expression, expr.Env(FilterContext{}))
	if err != nil {
		return fmt.Errorf("failed to compile filter expression: %w", err)
	}

	s.filterProgram = program
	s.filterCompiled = true
	s.matchesSub400 = computeMatchesSub400(program)
	return nil
}

// CapturesBelow400 reports whether the current filter can match a <400 status,
// so the unified middleware knows it must capture bodies for some non-error
// responses. False for the default (>=400) filter — the common case — which
// keeps the happy path allocation-free.
func (s *BadRequestSink) CapturesBelow400() bool {
	if s == nil {
		return false
	}
	s.mu.RLock()
	defer s.mu.RUnlock()
	return s.enabled && s.matchesSub400
}

// computeMatchesSub400 probes the compiled program with representative sub-400
// contexts; if any matches, the filter is considered to want non-error bodies.
func computeMatchesSub400(program *vm.Program) bool {
	if program == nil {
		return false
	}
	probes := []FilterContext{
		{StatusCode: 200, Method: "POST", Path: "/api/_probe"},
		{StatusCode: 200, Method: "POST", Path: "/tbe/_probe"},
		{StatusCode: 301, Method: "GET", Path: "/api/_probe"},
	}
	for _, ctx := range probes {
		if out, err := expr.Run(program, ctx); err == nil {
			if matched, ok := out.(bool); ok && matched {
				return true
			}
		}
	}
	return false
}

// shouldLog evaluates the filter against the entry's metadata.
func (s *BadRequestSink) shouldLog(entry *logEntry) bool {
	if s.filterCompiled && s.filterProgram != nil {
		result, err := expr.Run(s.filterProgram, FilterContext{
			StatusCode: entry.StatusCode,
			Method:     entry.Method,
			Path:       entry.Path,
			Query:      entry.Query,
		})
		if err != nil {
			logrus.Errorf("Failed to evaluate bad-request filter: %v", err)
			// Fallback: API errors only.
			return entry.StatusCode >= 400 && strings.HasPrefix(entry.Path, "/api/")
		}
		if matched, ok := result.(bool); ok {
			return matched
		}
		return true
	}
	// Fallback when no filter compiled.
	return entry.StatusCode >= 400 && strings.HasPrefix(entry.Path, "/api/")
}

// maybeLog writes entry to disk when it is enabled and the filter matches.
// Rotation is handled transparently by the lumberjack writer.
func (s *BadRequestSink) maybeLog(entry *logEntry) {
	if s == nil {
		return
	}
	s.mu.RLock()
	defer s.mu.RUnlock()

	if !s.enabled || s.writer == nil {
		return
	}
	if !s.shouldLog(entry) {
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
