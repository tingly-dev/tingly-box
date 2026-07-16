package otel

import (
	"context"

	"go.opentelemetry.io/otel"
	"go.opentelemetry.io/otel/attribute"
	"go.opentelemetry.io/otel/codes"
	"go.opentelemetry.io/otel/trace"
)

// Tracer provides distributed tracing capabilities for LLM requests.
type Tracer struct {
	tracer trace.Tracer
}

// NewTracer creates a new Tracer with the provided tracer provider.
func NewTracer(tp trace.TracerProvider) *Tracer {
	if tp == nil {
		tp = otel.GetTracerProvider()
	}
	return &Tracer{
		tracer: tp.Tracer("tingly-box"),
	}
}

// StartSpan begins a new span with the given name and options.
func (t *Tracer) StartSpan(ctx context.Context, name string, opts ...trace.SpanStartOption) (context.Context, trace.Span) {
	return t.tracer.Start(ctx, name, opts...)
}

// StartRequestSpan begins a span for an LLM request with standard attributes.
func (t *Tracer) StartRequestSpan(ctx context.Context, provider, model, scenario string) (context.Context, trace.Span) {
	attrs := []attribute.KeyValue{
		AttrLLMProvider.String(provider),
		AttrLLMModel.String(model),
		AttrLLMScenario.String(scenario),
	}

	return t.tracer.Start(ctx, "llm.request",
		trace.WithAttributes(attrs...),
		trace.WithSpanKind(trace.SpanKindClient),
	)
}

// RecordTokenUsageEvent records token usage as a span event.
func (t *Tracer) RecordTokenUsageEvent(ctx context.Context, inputTokens, outputTokens int) {
	span := trace.SpanFromContext(ctx)
	if !span.IsRecording() {
		return
	}

	span.AddEvent("token_usage",
		trace.WithAttributes(
			AttrLLMTokenType.String("input"),
			attribute.Int("input_tokens", inputTokens),
			attribute.Int("output_tokens", outputTokens),
			attribute.Int("total_tokens", inputTokens+outputTokens),
		),
	)
}

// RecordError records an error to the current span.
func (t *Tracer) RecordError(ctx context.Context, err error, attrs ...attribute.KeyValue) {
	span := trace.SpanFromContext(ctx)
	if !span.IsRecording() {
		return
	}
	span.RecordError(err, trace.WithAttributes(attrs...))
}

// EndSpan ends a span with optional error handling.
func (t *Tracer) EndSpan(span trace.Span, err error) {
	if err != nil {
		span.RecordError(err)
		span.SetStatus(codes.Error, err.Error())
	}
	span.End()
}

// SetSpanAttributes sets attributes on the current span.
func (t *Tracer) SetSpanAttributes(ctx context.Context, attrs ...attribute.KeyValue) {
	span := trace.SpanFromContext(ctx)
	if span.IsRecording() {
		span.SetAttributes(attrs...)
	}
}
