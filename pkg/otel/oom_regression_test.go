package otel

import (
	"context"
	"strings"
	"testing"
	"unsafe"

	sdktrace "go.opentelemetry.io/otel/sdk/trace"
	"go.opentelemetry.io/otel/sdk/trace/tracetest"
)

// TestStartRequestSpan_DetachesModelString guards the trace-side variant of
// the #1255 OOM: the model string handed to StartRequestSpan typically
// aliases the gjson-parsed request body (a substring sharing the multi-MB
// backing array). A span sits in the batch export queue after End, so an
// aliased attribute would pin that whole buffer. The retained attribute
// value must live in fresh storage — verified at the pointer level.
func TestStartRequestSpan_DetachesModelString(t *testing.T) {
	// Simulate a model name carved out of a large request body buffer.
	body := strings.Repeat("x", 1<<20) + "claude-sonnet-4-6"
	model := body[len(body)-17:] // substring aliasing the 1MB buffer

	exporter := tracetest.NewInMemoryExporter()
	tp := sdktrace.NewTracerProvider(sdktrace.WithSyncer(exporter))
	defer tp.Shutdown(context.Background())

	tracer := NewTracer(tp)
	_, span := tracer.StartRequestSpan(context.Background(), "anthropic", model, "claude_code")
	span.End()

	spans := exporter.GetSpans()
	if len(spans) != 1 {
		t.Fatalf("expected 1 span, got %d", len(spans))
	}

	found := false
	for _, kv := range spans[0].Attributes {
		if string(kv.Key) != "gen_ai.request.model" {
			continue
		}
		found = true
		got := kv.Value.AsString()
		if got != "claude-sonnet-4-6" {
			t.Fatalf("gen_ai.request.model = %q, want claude-sonnet-4-6", got)
		}
		if unsafe.StringData(got) == unsafe.StringData(model) {
			t.Error("span attribute aliases the request-body buffer — it would pin the whole buffer while queued for export (#1255 trace variant)")
		}
	}
	if !found {
		t.Error("span missing gen_ai.request.model attribute")
	}
}
