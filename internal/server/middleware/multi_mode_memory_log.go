package middleware

import (
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

// GinRequestIDKey is the gin context key under which the per-request
// correlation id is stored. Handlers and later stages read it via
// c.GetString(GinRequestIDKey) to tie their logs to the request.
const GinRequestIDKey = "request_id"

// MultiModeMemoryLogMiddleware is the HTTP access log for the whole request
// chain. It records one structured entry per request — method, path, status,
// latency, error, and (for AI routes) routing metadata — correlated across
// stages by a request_id. Entries go to the multi-mode logger (text + JSON
// files) and an in-memory ring buffer for the logs API.
//
// It deliberately does NOT capture request/response bodies. Opportunistically
// mirroring bodies here (wrapping c.Request.Body / c.Writer) is unstable —
// it interferes with streaming, Flush/Hijack, and large/Expect-100-continue
// uploads — for little gain: the bodies that matter for diagnosis are recorded
// where they are understood (the handler and the model_request client stage).
type MultiModeMemoryLogMiddleware struct {
	logger      *logrus.Logger
	multiLogger *obs.MultiLogger
}

// NewMultiModeMemoryLogMiddleware creates the HTTP access log middleware.
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

		c.Next()

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
		m.logger.WithFields(fields).Log(getLogLevel(statusCode), fmt.Sprintf("%s %s %d %s %d",
			method,
			path,
			statusCode,
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
