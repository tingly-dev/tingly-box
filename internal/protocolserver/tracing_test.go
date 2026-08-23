package protocolserver

import (
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/gin-gonic/gin"
	pkgotel "github.com/tingly-dev/tingly-box/internal/otel"
	sdktrace "go.opentelemetry.io/otel/sdk/trace"
	"go.opentelemetry.io/otel/sdk/trace/tracetest"

	"github.com/tingly-dev/tingly-box/internal/constant"
	"github.com/tingly-dev/tingly-box/internal/loadbalance"
	"github.com/tingly-dev/tingly-box/internal/typ"
)

// newTestTracerHandler builds a ProtocolHandler whose Tracer records spans
// into the returned in-memory exporter (synchronous, no batching).
func newTestTracerHandler() (*ProtocolHandler, *tracetest.InMemoryExporter) {
	exporter := tracetest.NewInMemoryExporter()
	tp := sdktrace.NewTracerProvider(sdktrace.WithSyncer(exporter))
	ph := NewHandler(ProtocolHandlerDeps{Tracer: pkgotel.NewTracer(tp)})
	return ph, exporter
}

func attrMap(span tracetest.SpanStub) map[string]interface{} {
	m := make(map[string]interface{}, len(span.Attributes))
	for _, kv := range span.Attributes {
		m[string(kv.Key)] = kv.Value.AsInterface()
	}
	return m
}

func TestTracingMiddleware_RootSpanLifecycle(t *testing.T) {
	gin.SetMode(gin.TestMode)
	ph, exporter := newTestTracerHandler()

	var traceIDInCtx string
	engine := gin.New()
	engine.POST("/tingly/:scenario/v1/messages", ph.tracingMiddleware, func(c *gin.Context) {
		rule := &typ.Rule{UUID: "rule-1"}
		provider := &typ.Provider{UUID: "prov-1", Name: "acme"}
		SetTrackingContext(c, rule, provider, "claude-sonnet-4-6", "tingly/cc", true)
		traceIDInCtx = c.GetString(constant.CtxKeyTraceID)
		c.Status(http.StatusOK)
	})

	req := httptest.NewRequest(http.MethodPost, "/tingly/claude_code/v1/messages", nil)
	engine.ServeHTTP(httptest.NewRecorder(), req)

	spans := exporter.GetSpans()
	if len(spans) != 1 {
		t.Fatalf("expected 1 span, got %d", len(spans))
	}
	span := spans[0]

	// Named per GenAI convention after the handler populated the request model.
	if span.Name != "chat tingly/cc" {
		t.Errorf("span name = %q, want %q", span.Name, "chat tingly/cc")
	}
	attrs := attrMap(span)
	checks := map[string]interface{}{
		"gen_ai.operation.name":     "chat",
		"gen_ai.request.model":      "tingly/cc",
		"gen_ai.response.model":     "claude-sonnet-4-6",
		"gen_ai.provider.name":      "acme",
		"tingly.provider.uuid":      "prov-1",
		"tingly.rule.uuid":          "rule-1",
		"tingly.scenario":           "claude_code",
		"tingly.streaming":          true,
		"http.response.status_code": int64(200),
	}
	for k, want := range checks {
		if got := attrs[k]; got != want {
			t.Errorf("attr %s = %v, want %v", k, got, want)
		}
	}
	if span.Status.Code.String() == "Error" {
		t.Errorf("expected non-error status for 200 response, got %v", span.Status)
	}

	// The sampled trace id must be exposed for the correlation consumers
	// (usage record, access log) and match the exported span.
	if traceIDInCtx == "" {
		t.Fatal("expected CtxKeyTraceID to be set for a sampled span")
	}
	if want := span.SpanContext.TraceID().String(); traceIDInCtx != want {
		t.Errorf("ctx trace id = %s, want %s", traceIDInCtx, want)
	}
}

func TestTracingMiddleware_ErrorStatus(t *testing.T) {
	gin.SetMode(gin.TestMode)
	ph, exporter := newTestTracerHandler()

	engine := gin.New()
	engine.POST("/tingly/:scenario/v1/messages", ph.tracingMiddleware, func(c *gin.Context) {
		c.Status(http.StatusTooManyRequests)
	})
	req := httptest.NewRequest(http.MethodPost, "/tingly/openai/v1/messages", nil)
	engine.ServeHTTP(httptest.NewRecorder(), req)

	spans := exporter.GetSpans()
	if len(spans) != 1 {
		t.Fatalf("expected 1 span, got %d", len(spans))
	}
	span := spans[0]
	if span.Status.Code.String() != "Error" {
		t.Fatalf("expected error status for 429, got %v", span.Status.Code)
	}
	attrs := attrMap(span)
	if attrs["error.type"] != "429" {
		t.Errorf("error.type = %v, want %q", attrs["error.type"], "429")
	}
	if attrs["http.response.status_code"] != int64(429) {
		t.Errorf("http.response.status_code = %v, want 429", attrs["http.response.status_code"])
	}
}

func TestTracingMiddleware_NilTracerPassthrough(t *testing.T) {
	gin.SetMode(gin.TestMode)
	ph := NewHandler(ProtocolHandlerDeps{})

	engine := gin.New()
	called := false
	engine.POST("/tingly/:scenario/v1/messages", ph.tracingMiddleware, func(c *gin.Context) {
		called = true
		if c.GetString(constant.CtxKeyTraceID) != "" {
			t.Error("trace id must not be set without a tracer")
		}
		c.Status(http.StatusOK)
	})
	req := httptest.NewRequest(http.MethodPost, "/tingly/openai/v1/messages", nil)
	engine.ServeHTTP(httptest.NewRecorder(), req)
	if !called {
		t.Fatal("handler not reached with nil tracer")
	}
}

func TestFailoverAttemptSpans(t *testing.T) {
	gin.SetMode(gin.TestMode)
	ph, exporter := newTestTracerHandler()
	c, _ := gin.CreateTestContext(httptest.NewRecorder())
	c.Request = httptest.NewRequest(http.MethodPost, "/tingly/openai/v1/chat/completions", nil)

	serviceID := loadbalance.FormatServiceID("prov-1", "gpt-4")

	failed := ph.startFailoverAttemptSpan(c.Request.Context(), 1, serviceID, "prov-1")
	endFailoverAttemptSpan(failed, false, http.StatusBadGateway)

	succeeded := ph.startFailoverAttemptSpan(c.Request.Context(), 2, serviceID, "prov-1")
	endFailoverAttemptSpan(succeeded, true, 0)

	spans := exporter.GetSpans()
	if len(spans) != 2 {
		t.Fatalf("expected 2 attempt spans, got %d", len(spans))
	}

	first, second := spans[0], spans[1]
	if first.Status.Code.String() != "Error" {
		t.Errorf("failed attempt should carry error status, got %v", first.Status.Code)
	}
	attrs := attrMap(first)
	if attrs["tingly.failover.attempt"] != int64(1) {
		t.Errorf("attempt attr = %v, want 1", attrs["tingly.failover.attempt"])
	}
	if attrs["http.response.status_code"] != int64(http.StatusBadGateway) {
		t.Errorf("status attr = %v, want 502", attrs["http.response.status_code"])
	}
	if second.Status.Code.String() != "Ok" {
		t.Errorf("committed attempt should carry OK status, got %v", second.Status.Code)
	}
}

func TestTracingMiddleware_SpanSurvivesPanic(t *testing.T) {
	gin.SetMode(gin.TestMode)
	ph, exporter := newTestTracerHandler()

	engine := gin.New()
	engine.Use(gin.Recovery())
	engine.POST("/tingly/:scenario/v1/messages", ph.tracingMiddleware, func(c *gin.Context) {
		rule := &typ.Rule{UUID: "rule-1"}
		provider := &typ.Provider{UUID: "prov-1", Name: "acme"}
		SetTrackingContext(c, rule, provider, "claude-sonnet-4-6", "tingly/cc", false)
		panic("handler exploded")
	})

	req := httptest.NewRequest(http.MethodPost, "/tingly/claude_code/v1/messages", nil)
	engine.ServeHTTP(httptest.NewRecorder(), req)

	// A panicking request is the one most worth inspecting: its span must
	// still be ended (and therefore exported), with the stages it reached.
	spans := exporter.GetSpans()
	if len(spans) != 1 {
		t.Fatalf("expected the request span to be exported despite the panic, got %d", len(spans))
	}
	if got := attrMap(spans[0])["gen_ai.request.model"]; got != "tingly/cc" {
		t.Errorf("span should carry what the handler set before panicking, got %v", got)
	}
}

func TestDeclaredOperationDrivesSpanAndMetrics(t *testing.T) {
	gin.SetMode(gin.TestMode)
	ph, exporter := newTestTracerHandler()

	var metricsOperation string
	engine := gin.New()
	engine.POST("/tingly/:scenario/v1/embeddings", ph.tracingMiddleware,
		DeclareOperation("embeddings"),
		func(c *gin.Context) {
			rule := &typ.Rule{UUID: "rule-1"}
			provider := &typ.Provider{UUID: "prov-1", Name: "acme"}
			SetTrackingContext(c, rule, provider, "text-embed-3", "text-embed-3", false)
			// Both pipelines read the operation from the same declaration;
			// deriving it separately is how they came to disagree.
			metricsOperation = OperationFromContext(c)
			c.Status(http.StatusOK)
		})

	req := httptest.NewRequest(http.MethodPost, "/tingly/openai/v1/embeddings", nil)
	engine.ServeHTTP(httptest.NewRecorder(), req)

	spans := exporter.GetSpans()
	if len(spans) != 1 {
		t.Fatalf("expected 1 span, got %d", len(spans))
	}
	if spans[0].Name != "embeddings text-embed-3" {
		t.Errorf("span name = %q, want %q", spans[0].Name, "embeddings text-embed-3")
	}
	if got := attrMap(spans[0])["gen_ai.operation.name"]; got != "embeddings" {
		t.Errorf("span operation = %v, want embeddings", got)
	}
	if metricsOperation != "embeddings" {
		t.Errorf("metrics operation = %q, want embeddings", metricsOperation)
	}
}

func TestOperationDefaultsToChat(t *testing.T) {
	gin.SetMode(gin.TestMode)
	c, _ := gin.CreateTestContext(httptest.NewRecorder())
	if got := OperationFromContext(c); got != "chat" {
		t.Errorf("undeclared operation = %q, want chat", got)
	}
}
