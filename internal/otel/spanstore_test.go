package otel

import (
	"context"
	"strconv"
	"testing"

	"go.opentelemetry.io/otel/codes"
	sdktrace "go.opentelemetry.io/otel/sdk/trace"
	"go.opentelemetry.io/otel/trace"
)

// newStoreWithProvider returns a store fed by a real SDK provider so spans
// go through the exact production conversion path.
func newStoreWithProvider(t *testing.T) (*SpanStore, trace.Tracer) {
	t.Helper()
	store := NewSpanStore()
	tp := sdktrace.NewTracerProvider(sdktrace.WithSpanProcessor(store))
	t.Cleanup(func() { _ = tp.Shutdown(context.Background()) })
	return store, tp.Tracer("test")
}

func TestSpanStore_ParentChildOrdering(t *testing.T) {
	store, tr := newStoreWithProvider(t)
	ctx := context.Background()

	rootCtx, root := tr.Start(ctx, "chat gpt-4")
	_, child := tr.Start(rootCtx, "failover.attempt")
	child.SetStatus(codes.Error, "upstream status 502")
	child.End()
	root.End()

	traceID := root.SpanContext().TraceID().String()
	spans, dropped, ok := store.GetTrace(traceID)
	if !ok || dropped != 0 {
		t.Fatalf("GetTrace: ok=%v dropped=%d", ok, dropped)
	}
	if len(spans) != 2 {
		t.Fatalf("expected 2 spans, got %d", len(spans))
	}
	// Sorted by start time: root first.
	if spans[0].Name != "chat gpt-4" || spans[0].ParentSpanID != "" {
		t.Errorf("root span wrong: %+v", spans[0])
	}
	if spans[1].ParentSpanID != spans[0].SpanID {
		t.Errorf("child parent id = %q, want %q", spans[1].ParentSpanID, spans[0].SpanID)
	}

}

func TestSpanStore_EvictsOldestTraceOverCount(t *testing.T) {
	store, tr := newStoreWithProvider(t)
	store.maxTraces = 3
	ctx := context.Background()

	var ids []string
	for i := 0; i < 5; i++ {
		_, span := tr.Start(ctx, "chat "+strconv.Itoa(i))
		ids = append(ids, span.SpanContext().TraceID().String())
		span.End()
	}

	for _, id := range ids[:2] {
		if _, _, ok := store.GetTrace(id); ok {
			t.Errorf("trace %s should have been evicted", id)
		}
	}
	for _, id := range ids[2:] {
		if _, _, ok := store.GetTrace(id); !ok {
			t.Errorf("trace %s should be retained", id)
		}
	}
	if got := len(store.traces); got != 3 {
		t.Errorf("retained traces = %d, want 3", got)
	}
}

func TestSpanStore_EvictsOverBytes(t *testing.T) {
	store, tr := newStoreWithProvider(t)
	store.maxBytes = 1024 // a handful of spans
	ctx := context.Background()

	var last string
	for i := 0; i < 50; i++ {
		_, span := tr.Start(ctx, "chat some-model-name-with-length")
		last = span.SpanContext().TraceID().String()
		span.End()
	}

	if store.bytes > store.maxBytes+1024 {
		t.Errorf("store bytes %d far exceeds cap %d", store.bytes, store.maxBytes)
	}
	// The newest trace always survives eviction.
	if _, _, ok := store.GetTrace(last); !ok {
		t.Error("newest trace must never be evicted")
	}
}

func TestSpanStore_CapsSpansPerTrace(t *testing.T) {
	store, tr := newStoreWithProvider(t)
	store.maxSpansPerTrace = 5
	rootCtx, root := tr.Start(context.Background(), "chat loop")

	for i := 0; i < 10; i++ {
		_, child := tr.Start(rootCtx, "failover.attempt")
		child.End()
	}
	root.End()

	spans, dropped, ok := store.GetTrace(root.SpanContext().TraceID().String())
	if !ok {
		t.Fatal("trace missing")
	}
	if len(spans) != 5 {
		t.Errorf("stored spans = %d, want cap 5", len(spans))
	}
	// 10 children + 1 root = 11 ended spans, 6 past the cap.
	if dropped != 6 {
		t.Errorf("dropped = %d, want 6", dropped)
	}
}
