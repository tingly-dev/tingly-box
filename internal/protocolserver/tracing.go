package protocolserver

import (
	"context"
	"net/http"
	"strconv"
	"strings"

	"github.com/gin-gonic/gin"
	pkgotel "github.com/tingly-dev/tingly-box/internal/otel"
	"go.opentelemetry.io/otel"
	"go.opentelemetry.io/otel/attribute"
	"go.opentelemetry.io/otel/codes"
	"go.opentelemetry.io/otel/propagation"
	"go.opentelemetry.io/otel/semconv/v1.37.0/genaiconv"
	"go.opentelemetry.io/otel/trace"

	"github.com/tingly-dev/tingly-box/internal/constant"
	"github.com/tingly-dev/tingly-box/internal/loadbalance"
	"github.com/tingly-dev/tingly-box/internal/typ"
)

// This file is the single home for the gateway's span code: the request
// middleware below, the routing and failover attempt spans, and the traced
// selection wrappers handlers call. It lives in protocolserver rather than
// internal/middleware because that package holds what server and
// protocolserver share (auth, access log, CORS, gzip, timeouts), while the
// gateway's own middlewares — legacyScenarioAlias, profileAlias, context and
// this one — belong to the routes they are registered on. Moving it would
// also need internal/middleware to reach back for GetTrackingContext, which
// protocolserver already imports the other way around.

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

	scenario := ExtractScenarioFromPath(c.Request.URL.Path)

	// The span is named at the end, once the operation (declared by the
	// route, which has not run yet) and the request model (parsed from the
	// body) are both known — hence the placeholder name here.
	ctx := otel.GetTextMapPropagator().Extract(c.Request.Context(), propagation.HeaderCarrier(c.Request.Header))
	ctx, span := tr.StartSpan(ctx, "gen_ai.request",
		trace.WithSpanKind(trace.SpanKindClient),
		trace.WithAttributes(pkgotel.AttrTinglyScenario.String(scenario)),
	)
	c.Request = c.Request.WithContext(ctx)

	// Expose the trace id only when it is worth storing: an unsampled or
	// invalid (no-op tracer) span can never be looked up on the backend.
	if sc := span.SpanContext(); sc.IsValid() && sc.IsSampled() {
		c.Set(constant.CtxKeyTraceID, sc.TraceID().String())
	}

	// Deferred, not called after c.Next(): a panicking handler unwinds
	// straight through c.Next() to gin.Recovery (registered on the engine,
	// outside this group), and a span that never ends is never exported —
	// losing the trace of exactly the request worth inspecting. The recorded
	// status may then predate Recovery's 500, since our defer runs first;
	// the value of the span in that case is the stage breakdown, not the code.
	defer finishRequestSpan(c, span)

	c.Next()
}

// DeclareOperation records the gen_ai.operation.name of a route. Registered
// on the routes whose operation is not the default "chat", it gives the
// metrics and trace pipelines one shared source for that axis — deriving it
// separately is how they came to disagree (spans said "embeddings" while
// every metric reported the "chat" default).
func DeclareOperation(operation string) gin.HandlerFunc {
	return func(c *gin.Context) { c.Set(constant.CtxKeyOperation, operation) }
}

// OperationFromContext returns the route's declared gen_ai operation, or the
// convention's default when the route did not declare one.
func OperationFromContext(c *gin.Context) string {
	if op := c.GetString(constant.CtxKeyOperation); op != "" {
		return op
	}
	return string(genaiconv.OperationNameChat)
}

// finishRequestSpan renames and enriches the root span from the tracking
// context populated during the request, sets the final status, and ends it.
func finishRequestSpan(c *gin.Context, span trace.Span) {
	rule, provider, model, requestModel, _, streamed, _ := GetTrackingContext(c)
	operation := OperationFromContext(c)
	attrs := make([]attribute.KeyValue, 0, 10)
	attrs = append(attrs, pkgotel.AttrGenAIOperationName.String(operation))

	// Per the GenAI convention the span is named "{operation} {request
	// model}". The request model is parsed from the body, so the name can
	// only be completed after the handler ran. strings.Clone detaches the
	// value from the request buffer (cardinality rule 3, .design/otel.md §4).
	name := operation
	if requestModel != "" {
		requestModel = strings.Clone(requestModel)
		name += " " + requestModel
		attrs = append(attrs, pkgotel.AttrGenAIRequestModel.String(requestModel))
	}
	span.SetName(name)
	if model != "" {
		attrs = append(attrs, pkgotel.AttrGenAIResponseModel.String(strings.Clone(model)))
	}
	if provider != nil {
		attrs = append(attrs,
			pkgotel.AttrGenAIProviderName.String(provider.Name),
			pkgotel.AttrTinglyProviderUUID.String(provider.UUID),
		)
	}
	if rule != nil {
		attrs = append(attrs, pkgotel.AttrTinglyRuleUUID.String(rule.UUID))
	}
	attrs = append(attrs, pkgotel.AttrTinglyStreaming.Bool(streamed))
	attrs = append(attrs, lbAttributes(c)...)
	if requestID := c.GetString(constant.CtxKeyRequestID); requestID != "" {
		attrs = append(attrs, pkgotel.AttrTinglyRequestID.String(requestID))
	}

	status := c.Writer.Status()
	attrs = append(attrs, pkgotel.AttrHTTPResponseStatus.Int(status))

	// canceled ≠ error (same rule as the metrics path, .design/otel.md §3):
	// a client hanging up mid-stream is routine LLM UI behavior, not a
	// gateway failure — leave the span status unset in that case.
	if status >= http.StatusBadRequest && c.Request.Context().Err() == nil {
		span.SetStatus(codes.Error, http.StatusText(status))
		attrs = append(attrs, pkgotel.AttrErrorType.String(httpStatusErrorType(status)))
	}

	// One call rather than eight: each SetAttributes takes the span lock and
	// runs the SDK's limit/dedup pass.
	span.SetAttributes(attrs...)
	span.End()
}

// lbAttributes returns the load-balance decision recorded on the gin context,
// shared by the request span and the failover attempt spans.
func lbAttributes(c *gin.Context) []attribute.KeyValue {
	var attrs []attribute.KeyValue
	if svcID := c.GetString(ContextKeyLBServiceID); svcID != "" {
		attrs = append(attrs, pkgotel.AttrTinglyLBServiceID.String(svcID))
	}
	if tactic := c.GetString(ContextKeyLBTactic); tactic != "" {
		attrs = append(attrs, pkgotel.AttrTinglyLBTactic.String(tactic))
	}
	return attrs
}

// httpStatusErrorType renders an HTTP status as a low-cardinality error.type
// value ("429", "502", ...), matching the semconv guidance of using the
// status code when no more specific error class is known.
func httpStatusErrorType(status int) string {
	if status < 100 || status > 999 {
		return string(genaiconv.ErrorTypeOther)
	}
	return strconv.Itoa(status)
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

// selectService, selectServiceForEmbeddings and selectServiceForImageGeneration
// are the traced entry points to the routing pipeline. Handlers call these
// instead of ph.deps.RoutingSelector directly, so the span lives here once
// rather than as a bracket pair copied into every handler — instrumentation
// nobody has to remember to add when an endpoint is written.
func (ph *ProtocolHandler) selectService(c *gin.Context, scenario typ.RuleScenario, rule *typ.Rule, req interface{}) (*typ.Provider, *loadbalance.Service, error) {
	end := ph.startRoutingSpan(c)
	provider, service, err := ph.deps.RoutingSelector.SelectService(c, scenario, rule, req)
	end(err)
	return provider, service, err
}

func (ph *ProtocolHandler) selectServiceForEmbeddings(c *gin.Context, scenario typ.RuleScenario, rule *typ.Rule) (*typ.Provider, *loadbalance.Service, error) {
	end := ph.startRoutingSpan(c)
	provider, service, err := ph.deps.RoutingSelector.SelectServiceForEmbeddings(c, scenario, rule)
	end(err)
	return provider, service, err
}

func (ph *ProtocolHandler) selectServiceForImageGeneration(c *gin.Context, scenario typ.RuleScenario, rule *typ.Rule) (*typ.Provider, *loadbalance.Service, error) {
	end := ph.startRoutingSpan(c)
	provider, service, err := ph.deps.RoutingSelector.SelectServiceForImageGeneration(c, scenario, rule)
	end(err)
	return provider, service, err
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
		span.SetAttributes(lbAttributes(c)...)
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
		errType := httpStatusErrorType(status)
		span.SetStatus(codes.Error, "upstream status "+errType)
		span.SetAttributes(
			pkgotel.AttrHTTPResponseStatus.Int(status),
			pkgotel.AttrErrorType.String(errType),
		)
	}
	span.End()
}
