package server

import (
	"net/http"
	"strconv"

	"github.com/gin-gonic/gin"

	pkgotel "github.com/tingly-dev/tingly-box/pkg/otel"
)

// Trace API — read side of the in-memory SpanStore (.design/otel.md §7.4).
// This is the default trace egress for users without an OTLP backend; the
// WebUI logs page links log entries to it via their trace_id field.

// TraceListResponse is the response of GET /api/v1/traces.
type TraceListResponse struct {
	Total  int                    `json:"total"`
	Traces []pkgotel.TraceSummary `json:"traces"`
}

// TraceDetailResponse is the response of GET /api/v1/traces/:trace_id.
type TraceDetailResponse struct {
	TraceID string               `json:"trace_id"`
	Spans   []pkgotel.StoredSpan `json:"spans"`
	// DroppedSpans counts spans discarded because the trace exceeded the
	// per-trace cap; nonzero means the view is a truncated prefix.
	DroppedSpans int `json:"dropped_spans,omitempty"`
}

// GetRecentTraces lists recent traces from the in-memory store, newest
// first. Query parameter: limit (default 50, max 200 — the store holds at
// most 200 traces anyway).
func (s *Server) GetRecentTraces(c *gin.Context) {
	store := s.otelSetup.SpanStore()
	if store == nil {
		c.JSON(http.StatusServiceUnavailable, gin.H{"error": "tracing is not initialized"})
		return
	}

	limit := 50
	if raw := c.Query("limit"); raw != "" {
		if v, err := strconv.Atoi(raw); err == nil && v > 0 {
			limit = min(v, 200)
		}
	}

	traces := store.RecentTraces(limit)
	c.JSON(http.StatusOK, TraceListResponse{Total: len(traces), Traces: traces})
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
