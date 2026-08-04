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
	lastEnd time.Time
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

// TraceSummary describes one trace for the recent-traces listing.
type TraceSummary struct {
	TraceID    string    `json:"trace_id"`
	RootName   string    `json:"root_name"`
	SpanCount  int       `json:"span_count"`
	Dropped    int       `json:"dropped_spans,omitempty"`
	HasError   bool      `json:"has_error"`
	StartTime  time.Time `json:"start_time"`
	DurationMs int64     `json:"duration_ms"`
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
	span := convertSpan(ro)
	cost := spanCost(span)

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
	if span.EndTime.After(tr.lastEnd) {
		tr.lastEnd = span.EndTime
	}

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
	defer s.mu.Unlock()

	tr, found := s.traces[traceID]
	if !found {
		return nil, 0, false
	}
	spans = make([]StoredSpan, len(tr.spans))
	copy(spans, tr.spans)
	sort.Slice(spans, func(i, j int) bool { return spans[i].StartTime.Before(spans[j].StartTime) })
	return spans, tr.dropped, true
}

// RecentTraces lists up to limit trace summaries, newest first.
func (s *SpanStore) RecentTraces(limit int) []TraceSummary {
	s.mu.Lock()
	defer s.mu.Unlock()

	summaries := make([]TraceSummary, 0, len(s.order))
	for _, id := range s.order {
		tr := s.traces[id]
		if tr == nil || len(tr.spans) == 0 {
			continue
		}
		summaries = append(summaries, summarize(id, tr))
	}
	sort.Slice(summaries, func(i, j int) bool { return summaries[i].StartTime.After(summaries[j].StartTime) })
	if limit > 0 && len(summaries) > limit {
		summaries = summaries[:limit]
	}
	return summaries
}

func summarize(id string, tr *storedTrace) TraceSummary {
	sum := TraceSummary{TraceID: id, SpanCount: len(tr.spans), Dropped: tr.dropped}
	var root *StoredSpan
	start, end := tr.spans[0].StartTime, tr.spans[0].EndTime
	for i := range tr.spans {
		sp := &tr.spans[i]
		if sp.ParentSpanID == "" && root == nil {
			root = sp
		}
		if sp.StatusCode == "Error" {
			sum.HasError = true
		}
		if sp.StartTime.Before(start) {
			start = sp.StartTime
		}
		if sp.EndTime.After(end) {
			end = sp.EndTime
		}
	}
	if root != nil {
		sum.RootName = root.Name
	} else {
		sum.RootName = tr.spans[0].Name
	}
	sum.StartTime = start
	sum.DurationMs = end.Sub(start).Milliseconds()
	return sum
}

func convertSpan(ro sdktrace.ReadOnlySpan) StoredSpan {
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
	attrs := ro.Attributes()
	if len(attrs) > 0 {
		span.Attributes = make(map[string]string, len(attrs))
		for _, kv := range attrs {
			span.Attributes[string(kv.Key)] = kv.Value.Emit()
		}
	}
	return span
}

func spanCost(sp StoredSpan) int {
	cost := storedSpanOverhead + len(sp.Name) + len(sp.StatusMsg)
	for k, v := range sp.Attributes {
		cost += len(k) + len(v) + 32
	}
	return cost
}
