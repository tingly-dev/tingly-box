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

// Request body storage defaults (used when config leaves a field at zero).
const (
	// MaxRequestBodySize is the default request-body capture cap (1MB). This bounds
	// how much of a request body is mirrored during the request, which is both the
	// transient in-memory cost (≈ concurrency × cap) and the maximum the disk
	// bad-request sink can record. Raise it (via config) when full-fidelity capture
	// of large bad requests matters; bodies beyond it are recorded truncated.
	MaxRequestBodySize = 1024 * 1024
	// MaxRequestBodies is the default maximum number of request bodies to keep in memory
	MaxRequestBodies = 50
	// MaxRequestBodyStoreBytes is the default total-byte budget for the in-memory
	// request body store (16MiB), bounding it independent of the per-body cap.
	MaxRequestBodyStoreBytes = 16 * 1024 * 1024
)

// captureNever is a sentinel response-capture gate that no real HTTP status can
// reach, used to disable response-body capture entirely.
const captureNever = 1 << 30

// CaptureConfig tunes the middleware's in-memory body capture. Zero size fields
// fall back to the package defaults; Disabled turns body capture off entirely
// (request metadata + status code are still logged).
type CaptureConfig struct {
	Disabled                 bool
	MaxCapturedBodySize      int
	MaxRequestBodySize       int
	MaxRequestBodies         int
	MaxRequestBodyStoreBytes int
}

// Option customizes a MultiModeMemoryLogMiddleware at construction.
type Option func(*MultiModeMemoryLogMiddleware)

// WithCaptureConfig applies body-capture tuning (size caps / disable toggle).
func WithCaptureConfig(cfg CaptureConfig) Option {
	return func(m *MultiModeMemoryLogMiddleware) {
		m.captureDisabled = cfg.Disabled
		if cfg.MaxCapturedBodySize > 0 {
			m.maxCapturedBodySize = cfg.MaxCapturedBodySize
		}
		if cfg.MaxRequestBodySize > 0 {
			m.maxRequestBodySize = cfg.MaxRequestBodySize
		}
		if cfg.MaxRequestBodies > 0 {
			m.maxRequestBodies = cfg.MaxRequestBodies
		}
		if cfg.MaxRequestBodyStoreBytes > 0 {
			m.maxRequestBodyStoreBytes = cfg.MaxRequestBodyStoreBytes
		}
	}
}

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
	badReqSink       *BadRequestSink

	// Capture tuning (resolved from defaults + options).
	captureDisabled          bool
	maxCapturedBodySize      int
	maxRequestBodySize       int
	maxRequestBodies         int
	maxRequestBodyStoreBytes int
}

// NewMultiModeMemoryLogMiddleware creates a new middleware with both persistent and memory logging
func NewMultiModeMemoryLogMiddleware(multiLogger *obs.MultiLogger, opts ...Option) *MultiModeMemoryLogMiddleware {
	m := &MultiModeMemoryLogMiddleware{
		maxCapturedBodySize:      MaxCapturedBodySize,
		maxRequestBodySize:       MaxRequestBodySize,
		maxRequestBodies:         MaxRequestBodies,
		maxRequestBodyStoreBytes: MaxRequestBodyStoreBytes,
	}
	for _, opt := range opts {
		opt(m)
	}

	if multiLogger == nil {
		// Fallback for test environments where no multi-logger is configured.
		l := logrus.New()
		if gin.Mode() == gin.TestMode {
			l.SetOutput(io.Discard)
		}
		m.logger = l
		return m
	}

	m.logger = multiLogger.GetLogrusLogger(obs.LogSourceHTTP)
	m.multiLogger = multiLogger
	if !m.captureDisabled {
		m.requestBodyStore = obs.NewRequestBodyStore(m.maxRequestBodies, m.maxRequestBodyStoreBytes)
	}
	return m
}

// SetBadRequestSink attaches a dedicated disk sink for bad/error requests. The
// unified middleware feeds it the same captured request/response bytes it
// already holds, so bodies are captured once. Once attached the middleware owns
// the sink's lifecycle (Close).
func (m *MultiModeMemoryLogMiddleware) SetBadRequestSink(sink *BadRequestSink) {
	m.badReqSink = sink
}

// Close releases resources the middleware owns (currently the bad-request sink).
func (m *MultiModeMemoryLogMiddleware) Close() {
	if m.badReqSink != nil {
		m.badReqSink.Stop()
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
		// The mirror is capped (limitedBufferWriter) so large bodies — e.g. base64
		// vision payloads — are not buffered in full just to be discarded on success.
		var bodyBuffer *bytes.Buffer
		var reqMirror *limitedBufferWriter
		if !m.captureDisabled && (m.requestBodyStore != nil || m.badReqSink != nil) &&
			c.Request.Body != nil && c.Request.Method != "GET" && c.Request.Method != "HEAD" {
			// Mirror at most maxRequestBodySize bytes (the capture cap). This bounds
			// the transient per-request buffer and is the max the disk sink can
			// record; reqMirror.truncated flags when a larger body was clipped. The
			// buffer is pooled to cut GC churn.
			bodyBuffer = getReqBodyBuf()
			defer putReqBodyBuf(bodyBuffer)
			reqMirror = &limitedBufferWriter{buf: bodyBuffer, limit: m.maxRequestBodySize}
			teeReader := io.TeeReader(c.Request.Body, reqMirror)
			c.Request.Body = io.NopCloser(teeReader)
		}

		// Wrap response writer to capture body for error responses only. Capture
		// is status-gated (>=400) and the buffer is lazily allocated, so successful
		// responses — including every 200 streaming SSE response — buffer nothing.
		// When capture is disabled, the gate is set so nothing is ever buffered.
		gate := 400
		if m.captureDisabled {
			gate = captureNever
		}
		w := &responseBodyWriter{
			ResponseWriter:   c.Writer,
			limit:            m.maxCapturedBodySize,
			minCaptureStatus: gate,
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
		// Happy path (2xx) requests don't store body, saving memory and CPU. The store
		// applies its own count/byte bounds (truncating only as a last resort).
		if m.requestBodyStore != nil && bodyBuffer != nil && statusCode >= 400 && bodyBuffer.Len() > 0 {
			bodyRef := m.requestBodyStore.Store(method, path, bodyBuffer.String())
			fields["body_ref"] = bodyRef
			if reqMirror != nil && reqMirror.truncated {
				fields["request_body_truncated"] = true
			}
		}

		// Add error message field if error occurred
		if errorMsg != "" {
			fields["error"] = errorMsg
			if errorType != "" {
				fields["error_type"] = errorType
			}
		}

		// Add response body for error responses (4xx/5xx). w.body is lazily
		// allocated and only populated when the status warranted capture.
		if statusCode >= 400 && w.body != nil && w.body.Len() > 0 {
			respBytes := w.body.Bytes()
			fields["response_body"] = string(respBytes)
			if w.truncated {
				fields["response_body_truncated"] = true
			}
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

		// Feed the dedicated bad-request disk sink (expr-filtered). It reuses the
		// request/response bytes already captured above, so bodies are read once.
		if m.badReqSink != nil {
			entry := &logEntry{
				Timestamp:  start,
				Method:     method,
				Path:       c.Request.URL.Path,
				Query:      raw,
				StatusCode: statusCode,
				Duration:   latency,
				Headers:    getHeaders(c),
				UserAgent:  c.Request.UserAgent(),
				ClientIP:   clientIP,
			}
			if bodyBuffer != nil {
				entry.RequestBody = bodyBuffer.Bytes()
				entry.RequestBodyTruncated = reqMirror != nil && reqMirror.truncated
			}
			if w.body != nil {
				entry.ResponseBody = w.body.Bytes()
				entry.ResponseBodyTruncated = w.truncated
			}
			m.badReqSink.maybeLog(entry)
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
