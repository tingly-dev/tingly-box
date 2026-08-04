package protocolserver

import (
	"context"
	"net/http"
	"strconv"
	"strings"

	"github.com/gin-gonic/gin"
	"go.opentelemetry.io/otel"
	"go.opentelemetry.io/otel/codes"
	"go.opentelemetry.io/otel/propagation"
	"go.opentelemetry.io/otel/trace"

	"github.com/tingly-dev/tingly-box/internal/constant"
	pkgotel "github.com/tingly-dev/tingly-box/pkg/otel"
)

// tracingMiddleware owns the lifecycle of the per-request root span. It is
// the only place that starts and ends it: trackUsage* fires more than once
// per request (failover setup failures, MCP loop iterations), so span
// start/end cannot live there — see .design/otel.md §7.1.
//
// Flow: extract the inbound W3C traceparent (no-op propagator when tracing
// is disabled), start the span with what is known from the path (operation,
// scenario), expose the trace id for the correlation consumers (usage
// record, access log), run the chain, then enrich the span from the tracking
// context the handlers populated and close it.
func (ph *ProtocolHandler) tracingMiddleware(c *gin.Context) {
	tr := ph.deps.Tracer
	if tr == nil {
		c.Next()
		return
	}

	operation := operationFromPath(c.Request.URL.Path)
	scenario := ExtractScenarioFromPath(c.Request.URL.Path)

	ctx := otel.GetTextMapPropagator().Extract(c.Request.Context(), propagation.HeaderCarrier(c.Request.Header))
	ctx, span := tr.StartSpan(ctx, operation,
		trace.WithSpanKind(trace.SpanKindClient),
		trace.WithAttributes(
			pkgotel.AttrGenAIOperationName.String(operation),
			pkgotel.AttrTinglyScenario.String(scenario),
		),
	)
	c.Request = c.Request.WithContext(ctx)

	// Expose the trace id only when it is worth storing: an unsampled or
	// invalid (no-op tracer) span can never be looked up on the backend.
	if sc := span.SpanContext(); sc.IsValid() && sc.IsSampled() {
		c.Set(constant.CtxKeyTraceID, sc.TraceID().String())
	}

	c.Next()

	finishRequestSpan(c, span, operation)
}

// finishRequestSpan renames and enriches the root span from the tracking
// context populated during the request, sets the final status, and ends it.
func finishRequestSpan(c *gin.Context, span trace.Span, operation string) {
	rule, provider, model, requestModel, _, streamed, _ := GetTrackingContext(c)

	// Per the GenAI convention the span is named "{operation} {request
	// model}". The request model is parsed from the body, so the name can
	// only be completed after the handler ran. strings.Clone detaches the
	// value from the request buffer (cardinality rule 3, .design/otel.md §4).
	if requestModel != "" {
		requestModel = strings.Clone(requestModel)
		span.SetName(operation + " " + requestModel)
		span.SetAttributes(pkgotel.AttrGenAIRequestModel.String(requestModel))
	}
	if model != "" {
		span.SetAttributes(pkgotel.AttrGenAIResponseModel.String(strings.Clone(model)))
	}
	if provider != nil {
		span.SetAttributes(
			pkgotel.AttrGenAIProviderName.String(provider.Name),
			pkgotel.AttrTinglyProviderUUID.String(provider.UUID),
		)
	}
	if rule != nil {
		span.SetAttributes(pkgotel.AttrTinglyRuleUUID.String(rule.UUID))
	}
	span.SetAttributes(pkgotel.AttrTinglyStreaming.Bool(streamed))
	if svcID := c.GetString(ContextKeyLBServiceID); svcID != "" {
		span.SetAttributes(pkgotel.AttrTinglyLBServiceID.String(svcID))
	}
	if tactic := c.GetString(ContextKeyLBTactic); tactic != "" {
		span.SetAttributes(pkgotel.AttrTinglyLBTactic.String(tactic))
	}
	if requestID := c.GetString(constant.CtxKeyRequestID); requestID != "" {
		span.SetAttributes(pkgotel.AttrTinglyRequestID.String(requestID))
	}

	status := c.Writer.Status()
	span.SetAttributes(pkgotel.AttrHTTPResponseStatus.Int(status))

	// canceled ≠ error (same rule as the metrics path, .design/otel.md §3):
	// a client hanging up mid-stream is routine LLM UI behavior, not a
	// gateway failure — leave the span status unset in that case.
	if status >= http.StatusBadRequest && c.Request.Context().Err() == nil {
		span.SetStatus(codes.Error, http.StatusText(status))
		span.SetAttributes(pkgotel.AttrErrorType.String(httpStatusErrorType(status)))
	}

	span.End()
}

// httpStatusErrorType renders an HTTP status as a low-cardinality error.type
// value ("429", "502", ...), matching the semconv guidance of using the
// status code when no more specific error class is known.
func httpStatusErrorType(status int) string {
	if status < 100 || status > 999 {
		return "other"
	}
	return strconv.Itoa(status)
}

// operationFromPath derives the gen_ai.operation.name for the endpoint. The
// default mirrors tracker.UsageOptions.Operation ("chat") so metrics and
// spans always agree on the operation axis.
func operationFromPath(path string) string {
	switch {
	case strings.HasSuffix(path, "/embeddings"):
		return "embeddings"
	case strings.HasSuffix(path, "/images/generations"):
		return "image_generation"
	default:
		return "chat"
	}
}

// setTokenUsageOnSpan mirrors the recorded token usage onto the ambient
// request span. The tracking context in c is not touched — the span is a
// projection of it, never a data source (.design/otel.md §7.1).
func (ph *ProtocolHandler) setTokenUsageOnSpan(c *gin.Context, inputTokens, outputTokens int) {
	if ph.deps.Tracer == nil {
		return
	}
	ph.deps.Tracer.SetTokenUsage(c.Request.Context(), inputTokens, outputTokens)
}

// startRoutingSpan opens a child span covering service selection (the
// health → smart → affinity → strategy pipeline) and returns the function
// that closes it. The selection outcome is read back from the tracking
// context at close time, so the span answers "which upstream was picked and
// by which tactic" without the call sites passing anything.
//
// Like the failover attempt span it is not swapped into c.Request's context:
// nothing nests under routing, and the ambient span must stay the root one
// so token usage lands there.
func (ph *ProtocolHandler) startRoutingSpan(c *gin.Context) func(error) {
	if ph.deps.Tracer == nil {
		return func(error) {}
	}
	_, span := ph.deps.Tracer.StartSpan(c.Request.Context(), "routing")
	return func(err error) {
		if svcID := c.GetString(ContextKeyLBServiceID); svcID != "" {
			span.SetAttributes(pkgotel.AttrTinglyLBServiceID.String(svcID))
		}
		if tactic := c.GetString(ContextKeyLBTactic); tactic != "" {
			span.SetAttributes(pkgotel.AttrTinglyLBTactic.String(tactic))
		}
		if err != nil {
			span.RecordError(err)
			span.SetStatus(codes.Error, err.Error())
		}
		span.End()
	}
}

// startFailoverAttemptSpan opens a child span recording one failover attempt.
// It deliberately does NOT replace c.Request's context: token usage written
// via the ambient context must always land on the root span, and the
// attempt span is a pure outcome record (.design/otel.md §7.2).
func (ph *ProtocolHandler) startFailoverAttemptSpan(ctx context.Context, attempt int, serviceID, providerUUID string) trace.Span {
	if ph.deps.Tracer == nil {
		return nil
	}
	_, span := ph.deps.Tracer.StartSpan(ctx, "failover.attempt",
		trace.WithAttributes(
			pkgotel.AttrTinglyFailoverAttempt.Int(attempt),
			pkgotel.AttrTinglyLBServiceID.String(serviceID),
			pkgotel.AttrTinglyProviderUUID.String(providerUUID),
		),
	)
	return span
}

// endFailoverAttemptSpan closes an attempt span with its outcome: committed
// means the response reached the wire (success); any other outcome carries
// the buffered upstream status.
func endFailoverAttemptSpan(span trace.Span, committed bool, status int) {
	if span == nil {
		return
	}
	if committed {
		span.SetStatus(codes.Ok, "")
	} else {
		span.SetAttributes(pkgotel.AttrHTTPResponseStatus.Int(status))
		span.SetStatus(codes.Error, "upstream status "+httpStatusErrorType(status))
		span.SetAttributes(pkgotel.AttrErrorType.String(httpStatusErrorType(status)))
	}
	span.End()
}
