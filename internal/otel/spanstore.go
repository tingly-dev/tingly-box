package otel

import (
	"context"
	"sort"
	"sync"
	"time"

	sdktrace "go.opentelemetry.io/otel/sdk/trace"
)

// SpanStore is a bounded in-memory ring of finished spans grouped by trace.
// It is the default trace egress: users who never configure an OTLP backend
// inspect traces through it via the logs page (.design/otel.md §7.4).
//
// Bounds are hard and enforced on every insert (the #1255 discipline):
// exceeding the trace-count or byte cap evicts whole oldest-first traces;
// a single runaway trace stops accepting spans past maxSpansPerTrace.
// Values copied out of ReadOnlySpan (name, attribute strings) are freshly
// allocated by the SDK accessors, so nothing here aliases request buffers.
type SpanStore struct {
	mu sync.Mutex

	traces map[string]*storedTrace
	order  []string // trace ids, oldest first (first-seen order)
	bytes  int

	maxTraces        int
	maxBytes         int
	maxSpansPerTrace int
}

// storedTrace groups the finished spans of one trace id.
type storedTrace struct {
	spans   []StoredSpan
	bytes   int
	dropped int
}

// StoredSpan is the retained projection of a finished span — just enough
// for a waterfall view. Request/response content is deliberately absent
// (content capture belongs to the recording pipeline, not the trace view).
type StoredSpan struct {
	TraceID      string            `json:"trace_id"`
	SpanID       string            `json:"span_id"`
	ParentSpanID string            `json:"parent_span_id,omitempty"`
	Name         string            `json:"name"`
	Kind         string            `json:"kind"`
	StartTime    time.Time         `json:"start_time"`
	EndTime      time.Time         `json:"end_time"`
	StatusCode   string            `json:"status_code"` // Unset | Ok | Error
	StatusMsg    string            `json:"status_message,omitempty"`
	Attributes   map[string]string `json:"attributes,omitempty"`
}

// Default bounds. Deliberately not configurable yet (smart defaults over
// toggles); revisit only with a concrete need.
const (
	defaultMaxTraces        = 200
	defaultMaxBytes         = 8 << 20 // 8 MiB estimated retained size
	defaultMaxSpansPerTrace = 200
	storedSpanOverhead      = 128 // rough fixed cost per span (ids, times, struct)
)

// NewSpanStore creates a store with the default bounds.
func NewSpanStore() *SpanStore {
	return &SpanStore{
		traces:           make(map[string]*storedTrace),
		maxTraces:        defaultMaxTraces,
		maxBytes:         defaultMaxBytes,
		maxSpansPerTrace: defaultMaxSpansPerTrace,
	}
}

var _ sdktrace.SpanProcessor = (*SpanStore)(nil)

// OnStart implements sdktrace.SpanProcessor (no-op: spans are stored when
// they finish, so partially-built spans never sit in the ring).
func (s *SpanStore) OnStart(_ context.Context, _ sdktrace.ReadWriteSpan) {}

// OnEnd stores the finished span, evicting oldest traces past the bounds.
// Only sampled spans reach a SpanProcessor, so the store never sees no-ops.
func (s *SpanStore) OnEnd(ro sdktrace.ReadOnlySpan) {
	// Converted before taking the lock: OnEnd runs on every finished span of
	// every request, so allocation must not sit inside the critical section.
	span, cost := convertSpan(ro)

	s.mu.Lock()
	defer s.mu.Unlock()

	tr, ok := s.traces[span.TraceID]
	if !ok {
		tr = &storedTrace{}
		s.traces[span.TraceID] = tr
		s.order = append(s.order, span.TraceID)
	}

	if len(tr.spans) >= s.maxSpansPerTrace {
		tr.dropped++
		return
	}

	tr.spans = append(tr.spans, span)
	tr.bytes += cost
	s.bytes += cost

	// Evict oldest whole traces until back under both caps; never evict the
	// trace we just wrote to (it is by definition the newest active one).
	for (len(s.order) > s.maxTraces || s.bytes > s.maxBytes) && len(s.order) > 1 {
		oldest := s.order[0]
		if oldest == span.TraceID {
			break
		}
		s.order = s.order[1:]
		if old, ok := s.traces[oldest]; ok {
			s.bytes -= old.bytes
			delete(s.traces, oldest)
		}
	}
}

// Shutdown implements sdktrace.SpanProcessor.
func (s *SpanStore) Shutdown(_ context.Context) error { return nil }

// ForceFlush implements sdktrace.SpanProcessor.
func (s *SpanStore) ForceFlush(_ context.Context) error { return nil }

// GetTrace returns the stored spans of one trace, sorted by start time.
// ok is false when the trace was never stored or has been evicted.
func (s *SpanStore) GetTrace(traceID string) (spans []StoredSpan, dropped int, ok bool) {
	s.mu.Lock()
	tr, found := s.traces[traceID]
	if !found {
		s.mu.Unlock()
		return nil, 0, false
	}
	spans = make([]StoredSpan, len(tr.spans))
	copy(spans, tr.spans)
	dropped = tr.dropped
	s.mu.Unlock()

	// Sorted outside the lock — every OnEnd in flight would otherwise wait
	// on a read that only touches this caller's copy.
	sort.Slice(spans, func(i, j int) bool { return spans[i].StartTime.Before(spans[j].StartTime) })
	return spans, dropped, true
}

// convertSpan projects a finished span and returns its estimated retained
// size alongside, so the attribute map is walked once rather than twice.
func convertSpan(ro sdktrace.ReadOnlySpan) (StoredSpan, int) {
	sc := ro.SpanContext()
	span := StoredSpan{
		TraceID:    sc.TraceID().String(),
		SpanID:     sc.SpanID().String(),
		Name:       ro.Name(),
		Kind:       ro.SpanKind().String(),
		StartTime:  ro.StartTime(),
		EndTime:    ro.EndTime(),
		StatusCode: ro.Status().Code.String(),
		StatusMsg:  ro.Status().Description,
	}
	if parent := ro.Parent(); parent.HasSpanID() && parent.TraceID() == sc.TraceID() {
		span.ParentSpanID = parent.SpanID().String()
	}
	cost := storedSpanOverhead + len(span.Name) + len(span.StatusMsg)
	attrs := ro.Attributes()
	if len(attrs) > 0 {
		span.Attributes = make(map[string]string, len(attrs))
		for _, kv := range attrs {
			key, value := string(kv.Key), kv.Value.Emit()
			span.Attributes[key] = value
			cost += len(key) + len(value) + 32
		}
	}
	return span, cost
}
