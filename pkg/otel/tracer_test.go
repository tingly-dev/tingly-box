package otel

import (
	"context"
	"testing"

	"go.opentelemetry.io/otel/sdk/trace"
)

func TestNewTracer(t *testing.T) {
	tp := trace.NewTracerProvider()
	defer tp.Shutdown(context.Background())

	tracer := NewTracer(tp)
	if tracer == nil {
		t.Fatal("Tracer should not be nil")
	}

	if tracer.tracer == nil {
		t.Error("tracer.tracer should be initialized")
	}
}

func TestNewTracer_NilProvider(t *testing.T) {
	// Should use global tracer provider when nil is passed
	tracer := NewTracer(nil)
	if tracer == nil {
		t.Fatal("Tracer should not be nil")
	}
}

func TestTracer_StartSpan(t *testing.T) {
	tp := trace.NewTracerProvider()
	defer tp.Shutdown(context.Background())

	tracer := NewTracer(tp)
	ctx := context.Background()

	ctx, span := tracer.StartSpan(ctx, "test-span")
	defer span.End()

	if span == nil {
		t.Error("Span should not be nil")
	}

	if !span.IsRecording() {
		t.Error("Span should be recording")
	}
}

func TestTracer_StartRequestSpan(t *testing.T) {
	tp := trace.NewTracerProvider()
	defer tp.Shutdown(context.Background())

	tracer := NewTracer(tp)
	ctx := context.Background()

	ctx, span := tracer.StartRequestSpan(ctx, "openai", "gpt-4", "openai")
	defer span.End()

	if span == nil {
		t.Error("Span should not be nil")
	}

	if !span.IsRecording() {
		t.Error("Span should be recording")
	}
}

func TestTracer_RecordTokenUsageEvent(t *testing.T) {
	tp := trace.NewTracerProvider()
	defer tp.Shutdown(context.Background())

	tracer := NewTracer(tp)
	ctx := context.Background()

	ctx, span := tracer.StartSpan(ctx, "test-span")

	tracer.RecordTokenUsageEvent(ctx, 100, 50)

	span.End()
}

func TestTracer_RecordTokenUsageEvent_NoSpan(t *testing.T) {
	tp := trace.NewTracerProvider()
	defer tp.Shutdown(context.Background())

	tracer := NewTracer(tp)
	ctx := context.Background()

	// Should not panic when no span in context
	tracer.RecordTokenUsageEvent(ctx, 100, 50)
}

func TestTracer_RecordError(t *testing.T) {
	tp := trace.NewTracerProvider()
	defer tp.Shutdown(context.Background())

	tracer := NewTracer(tp)
	ctx := context.Background()

	ctx, span := tracer.StartSpan(ctx, "test-span")

	testErr := &testError{"test error"}
	tracer.RecordError(ctx, testErr)

	span.End()
}

func TestTracer_RecordError_WithAttributes(t *testing.T) {
	tp := trace.NewTracerProvider()
	defer tp.Shutdown(context.Background())

	tracer := NewTracer(tp)
	ctx := context.Background()

	ctx, span := tracer.StartSpan(ctx, "test-span")

	testErr := &testError{"test error"}
	tracer.RecordError(ctx, testErr, AttrLLMProvider.String("openai"))

	span.End()
}

func TestTracer_EndSpan(t *testing.T) {
	tp := trace.NewTracerProvider()
	defer tp.Shutdown(context.Background())

	tracer := NewTracer(tp)
	ctx := context.Background()

	_, span := tracer.StartSpan(ctx, "test-span")

	// End without error
	tracer.EndSpan(span, nil)

	if !span.IsRecording() {
		// Span should be ended
	}
}

func TestTracer_EndSpan_WithError(t *testing.T) {
	tp := trace.NewTracerProvider()
	defer tp.Shutdown(context.Background())

	tracer := NewTracer(tp)
	ctx := context.Background()

	_, span := tracer.StartSpan(ctx, "test-span")

	testErr := &testError{"test error"}
	tracer.EndSpan(span, testErr)
}

func TestTracer_SetSpanAttributes(t *testing.T) {
	tp := trace.NewTracerProvider()
	defer tp.Shutdown(context.Background())

	tracer := NewTracer(tp)
	ctx := context.Background()

	ctx, span := tracer.StartSpan(ctx, "test-span")

	tracer.SetSpanAttributes(ctx, AttrLLMProvider.String("openai"))

	span.End()
}

func TestTracer_SetSpanAttributes_NoSpan(t *testing.T) {
	tp := trace.NewTracerProvider()
	defer tp.Shutdown(context.Background())

	tracer := NewTracer(tp)
	ctx := context.Background()

	// Should not panic when no span in context
	tracer.SetSpanAttributes(ctx, AttrLLMProvider.String("openai"))
}

// testError is a simple error type for testing
type testError struct {
	msg string
}

func (e *testError) Error() string {
	return e.msg
}
