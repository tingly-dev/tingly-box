package otel

import (
	"context"
	"strings"

	"go.opentelemetry.io/otel"
	"go.opentelemetry.io/otel/attribute"
	"go.opentelemetry.io/otel/codes"
	"go.opentelemetry.io/otel/semconv/v1.37.0/genaiconv"
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

// StartRequestSpan begins a CLIENT span for an LLM inference request with
// the standard GenAI attributes. Per the convention the span is named
// "{operation} {request model}". operation is the gen_ai.operation.name
// ("chat", "embeddings", ...) and defaults to "chat" when empty — mirroring
// UsageOptions.Operation so metrics and spans always agree on the operation
// axis.
//
// The model string is cloned: it typically originates from the gjson-parsed
// request body and would otherwise pin the entire multi-MB buffer for as
// long as the span sits in the batch export queue (up to 2048 spans when
// the collector is slow) — the trace-side variant of the #1255 OOM.
// Callers passing request-derived strings via SetSpanAttributes must apply
// the same discipline.
func (t *Tracer) StartRequestSpan(ctx context.Context, operation, provider, model, scenario string) (context.Context, trace.Span) {
	if operation == "" {
		operation = string(genaiconv.OperationNameChat)
	}
	model = strings.Clone(model)
	attrs := []attribute.KeyValue{
		AttrGenAIOperationName.String(operation),
		AttrGenAIProviderName.String(provider),
		AttrGenAIRequestModel.String(model),
		AttrTinglyScenario.String(scenario),
	}

	return t.tracer.Start(ctx, operation+" "+model,
		trace.WithAttributes(attrs...),
		trace.WithSpanKind(trace.SpanKindClient),
	)
}

// SetTokenUsage records token usage on the current span using the standard
// gen_ai.usage.* span attributes.
func (t *Tracer) SetTokenUsage(ctx context.Context, inputTokens, outputTokens int) {
	span := trace.SpanFromContext(ctx)
	if !span.IsRecording() {
		return
	}

	span.SetAttributes(
		AttrGenAIUsageInputTokens.Int(inputTokens),
		AttrGenAIUsageOutputTokens.Int(outputTokens),
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

// EndSpan ends a span with optional error handling. When err is non-nil it
// records the exception event and sets error status — callers should not
// also invoke RecordError for the same error, or the event is duplicated.
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
