package server

import (
	"net/http"

	"github.com/gin-gonic/gin"
	pkgotel "github.com/tingly-dev/tingly-box/internal/otel"
)

// Trace API — read side of the in-memory SpanStore (.design/otel.md §7.4).
// This is the default trace egress for users without an OTLP backend; the
// WebUI logs page links log entries to it via their trace_id field.

// TraceDetailResponse is the response of GET /api/v1/traces/:trace_id.
type TraceDetailResponse struct {
	TraceID string               `json:"trace_id"`
	Spans   []pkgotel.StoredSpan `json:"spans"`
	// DroppedSpans counts spans discarded because the trace exceeded the
	// per-trace cap; nonzero means the view is a truncated prefix.
	DroppedSpans int `json:"dropped_spans,omitempty"`
}

// GetTrace returns the spans of one trace. A 404 means the trace was never
// sampled into the store or has been evicted from the ring buffer — the
// frontend renders that as "no longer buffered", not as an error.
func (s *Server) GetTrace(c *gin.Context) {
	store := s.otelSetup.SpanStore()
	if store == nil {
		c.JSON(http.StatusServiceUnavailable, gin.H{"error": "tracing is not initialized"})
		return
	}

	traceID := c.Param("trace_id")
	spans, dropped, ok := store.GetTrace(traceID)
	if !ok {
		c.JSON(http.StatusNotFound, gin.H{"error": "trace not found (never sampled, or evicted from the in-memory buffer)"})
		return
	}
	c.JSON(http.StatusOK, TraceDetailResponse{TraceID: traceID, Spans: spans, DroppedSpans: dropped})
}
