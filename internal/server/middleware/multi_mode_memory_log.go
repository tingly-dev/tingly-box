package middleware

import (
	"bytes"
	"fmt"
	"io"
	"net/http"
	"time"

	"github.com/gin-gonic/gin"
	"github.com/google/uuid"
	"github.com/sirupsen/logrus"
	"github.com/tingly-dev/tingly-box/internal/constant"
	"github.com/tingly-dev/tingly-box/internal/typ"
	"github.com/tingly-dev/tingly-box/pkg/obs"
)

// Request body storage configuration
const (
	// MaxRequestBodySize is the maximum size of request body to store (1MB)
	MaxRequestBodySize = 1024 * 1024
	// MaxRequestBodies is the maximum number of request bodies to keep in memory
	MaxRequestBodies = 50
)

// GinRequestIDKey is the gin context key under which the per-request
// correlation id is stored. Handlers and later stages read it via
// c.GetString(GinRequestIDKey) to tie their logs to the request.
const GinRequestIDKey = "request_id"

// MultiModeMemoryLogMiddleware provides Gin middleware with both persistent and memory log storage
// Logs are written to:
// 1. Multi-mode logger (text + JSON files for persistence)
// 2. Memory (circular buffer for quick API access)
// 3. Request body store (pure memory, referenced by body_ref ID)
type MultiModeMemoryLogMiddleware struct {
	logger           *logrus.Logger
	multiLogger      *obs.MultiLogger
	requestBodyStore *obs.RequestBodyStore
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
			logger:           l,
			multiLogger:      nil,
			requestBodyStore: nil,
		}
	}
	// Get a logger scoped to HTTP source
	httpLogger := multiLogger.GetLogrusLogger(obs.LogSourceHTTP)

	return &MultiModeMemoryLogMiddleware{
		logger:           httpLogger,
		multiLogger:      multiLogger,
		requestBodyStore: obs.NewRequestBodyStore(MaxRequestBodies),
	}
}

// Middleware returns a Gin middleware compatible with gin.Logger()
// It logs all HTTP requests to both the multi-mode logger and memory
func (m *MultiModeMemoryLogMiddleware) Middleware() gin.HandlerFunc {
	return func(c *gin.Context) {
		start := time.Now()
		path := c.Request.URL.Path
		raw := c.Request.URL.RawQuery

		// Establish a correlation id for this request as early as possible so
		// every stage (routing, protocol conversion, upstream client call)
		// can tie its logs back to one request. Reuse an inbound X-Request-Id
		// when the caller supplies one, otherwise mint a fresh id.
		requestID := c.GetHeader("X-Request-Id")
		if requestID == "" {
			requestID = uuid.NewString()
		}
		c.Set(GinRequestIDKey, requestID)

		// Carry the id on the request context so request-scoped code (protocol
		// conversion, upstream client calls) can log via logrus.WithContext(ctx);
		// the MultiLogger hook routes those entries to the model_request source.
		c.Request = c.Request.WithContext(obs.ContextWithRequestID(c.Request.Context(), requestID))

		// Capture request body using TeeReader for logging.
		// We ALWAYS read (to support error debugging), but only STORE on errors (4xx/5xx).
		// This minimizes storage overhead while keeping logging capability.
		var bodyBuffer *bytes.Buffer
		if m.requestBodyStore != nil && c.Request.Body != nil && c.Request.Method != "GET" && c.Request.Method != "HEAD" {
			// Mirror the request body for diagnostics, but cap the in-memory mirror
			// so a large (e.g. base64 vision) upload can't buffer unbounded. The
			// handler still reads the full body. The cap is MaxRequestBodySize+1 so
			// the store's existing truncation check (len > MaxRequestBodySize) still
			// fires for oversized bodies while memory stays bounded at ~1MB.
			bodyBuffer = &bytes.Buffer{}
			teeReader := io.TeeReader(c.Request.Body, &limitedBufferWriter{buf: bodyBuffer, limit: MaxRequestBodySize + 1})
			c.Request.Body = io.NopCloser(teeReader)
		}

		// Wrap response writer to capture body for error responses (lazily;
		// only buffered for status >= 400 — see responseBodyWriter).
		w := &responseBodyWriter{
			ResponseWriter: c.Writer,
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

			// For panic errors, include additional context
			if lastErr.Type == gin.ErrorTypeBind {
				errorType = "bind_error"
			} else if lastErr.Type == gin.ErrorTypePublic {
				errorType = "public_error"
			} else if lastErr.Type == gin.ErrorTypePrivate {
				errorType = "private_error"
			} else {
				// Convert ErrorType to string safely
				errorType = fmt.Sprintf("error_type_%d", lastErr.Type)
			}
		}

		// Build fields with error message if available
		fields := logrus.Fields{
			"type":       "http_request",
			"request_id": requestID,
			"status":     statusCode,
			"latency":    latency,
			"client_ip":  clientIP,
			"method":     method,
			"path":       path,
			"body_size":  bodySize,
			"user_agent": c.Request.UserAgent(),
		}

		// Add body reference - ONLY for error responses (4xx/5xx) to minimize storage overhead
		// Happy path (2xx) requests don't store body, saving memory and CPU
		if bodyBuffer != nil && statusCode >= 400 && bodyBuffer.Len() > 0 {
			bodyRef := m.requestBodyStore.Store(method, path, bodyBuffer.String(), MaxRequestBodySize)
			fields["body_ref"] = bodyRef
		}

		// Add error message field if error occurred
		if errorMsg != "" {
			fields["error"] = errorMsg
			if errorType != "" {
				fields["error_type"] = errorType
			}
		}

		// Add response body for error responses (4xx/5xx); w.body is lazily
		// allocated and only populated when the status warranted capture.
		if statusCode >= 400 && w.body != nil && w.body.Len() > 0 {
			respBytes := w.body.Bytes()
			fields["response_body"] = string(respBytes)
		}

		// Append routing metadata when set by AI handlers (absent for non-AI routes)
		if rm := c.GetString(constant.CtxKeyRequestModel); rm != "" {
			fields["request_model"] = rm
		}
		if am := c.GetString(constant.CtxKeyModel); am != "" {
			fields["routed_model"] = am
		}
		if sc := c.GetString(constant.CtxKeyScenario); sc != "" {
			fields["scenario"] = sc
		}
		if pv, exists := c.Get(constant.CtxKeyProvider); exists {
			if p, ok := pv.(*typ.Provider); ok && p != nil {
				fields["routed_provider"] = p.Name
				fields["api_style"] = string(p.APIStyle)
				fields["base_url"] = p.APIBase
			}
		}
		if svcID := c.GetString(constant.CtxKeyLBServiceID); svcID != "" {
			fields["lb_service_id"] = svcID
		}
		if tactic := c.GetString(constant.CtxKeyLBTactic); tactic != "" {
			fields["lb_tactic"] = tactic
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

// GetRequestBodyStore returns the request body store for retrieving stored request bodies
func (m *MultiModeMemoryLogMiddleware) GetRequestBodyStore() *obs.RequestBodyStore {
	return m.requestBodyStore
}
