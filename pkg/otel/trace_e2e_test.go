package otel

import (
	"compress/gzip"
	"context"
	"errors"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync"
	"testing"
	"time"

	"go.opentelemetry.io/otel"
	"go.opentelemetry.io/otel/attribute"
	"go.opentelemetry.io/otel/propagation"
	colmetricpb "go.opentelemetry.io/proto/otlp/collector/metrics/v1"
	coltracepb "go.opentelemetry.io/proto/otlp/collector/trace/v1"
	"google.golang.org/protobuf/encoding/protojson"
	"google.golang.org/protobuf/proto"
)

// otlpTraceCollector is a minimal in-process OTLP/HTTP trace collector: it
// decodes standard ExportTraceServiceRequest protobufs exactly like Jaeger,
// Tempo or an otel-collector would.
type otlpTraceCollector struct {
	mu       sync.Mutex
	requests []*coltracepb.ExportTraceServiceRequest
}

func (c *otlpTraceCollector) handler(t *testing.T) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		// Metrics flush on Shutdown hits the same endpoint; accept and discard.
		if r.URL.Path == "/v1/metrics" {
			resp, _ := proto.Marshal(&colmetricpb.ExportMetricsServiceResponse{})
			w.Header().Set("Content-Type", "application/x-protobuf")
			_, _ = w.Write(resp)
			return
		}
		if r.URL.Path != "/v1/traces" {
			http.NotFound(w, r)
			return
		}
		body := r.Body
		if r.Header.Get("Content-Encoding") == "gzip" {
			gz, err := gzip.NewReader(r.Body)
			if err != nil {
				t.Errorf("bad gzip body: %v", err)
				w.WriteHeader(http.StatusBadRequest)
				return
			}
			body = gz
		}
		raw, err := io.ReadAll(body)
		if err != nil {
			t.Errorf("read body: %v", err)
			w.WriteHeader(http.StatusBadRequest)
			return
		}

		req := &coltracepb.ExportTraceServiceRequest{}
		if err := proto.Unmarshal(raw, req); err != nil {
			t.Errorf("payload is not a valid OTLP ExportTraceServiceRequest: %v", err)
			w.WriteHeader(http.StatusBadRequest)
			return
		}

		c.mu.Lock()
		c.requests = append(c.requests, req)
		c.mu.Unlock()

		resp, _ := proto.Marshal(&coltracepb.ExportTraceServiceResponse{})
		w.Header().Set("Content-Type", "application/x-protobuf")
		_, _ = w.Write(resp)
	}
}

// TestTraceE2E_OTLPWireFormat drives the real pipeline end to end:
// Setup -> Tracer spans -> batch processor -> OTLP/HTTP exporter -> collector,
// then asserts on the standard OTLP payload the collector received.
func TestTraceE2E_OTLPWireFormat(t *testing.T) {
	collector := &otlpTraceCollector{}
	srv := httptest.NewServer(collector.handler(t))
	defer srv.Close()
	endpoint := strings.TrimPrefix(srv.URL, "http://")

	ctx := context.Background()
	setup, err := NewSetup(ctx, &Config{
		Enabled:        true,
		ExportInterval: time.Hour, // metrics reader never fires during the test
		ExportTimeout:  5 * time.Second,
		OTLP: OTLPConfig{
			Enabled:  true,
			Endpoint: endpoint,
			Protocol: "http/protobuf",
			Insecure: true,
		},
	})
	if err != nil {
		t.Fatalf("NewSetup failed: %v", err)
	}

	tracer := setup.Tracer()

	// Simulate one gateway request: root request span with a nested
	// upstream-call span, token usage event, and an error on the child.
	reqCtx, reqSpan := tracer.StartRequestSpan(ctx, "", "anthropic", "claude-sonnet-4-6", "claude_code")
	tracer.SetSpanAttributes(reqCtx,
		AttrTinglyRuleUUID.String("rule-42"),
		AttrTinglyStreaming.Bool(true),
	)

	// The W3C traceparent header that would be injected on the upstream hop.
	carrier := propagation.MapCarrier{}
	otel.GetTextMapPropagator().Inject(reqCtx, carrier)
	if carrier.Get("traceparent") == "" {
		t.Error("traceparent header should be injected (W3C propagation)")
	}
	t.Logf("outgoing traceparent: %s", carrier.Get("traceparent"))

	upCtx, upSpan := tracer.StartSpan(reqCtx, "llm.upstream.call")
	tracer.SetSpanAttributes(upCtx, attribute.String("http.url", "https://api.anthropic.com/v1/messages"))
	// EndSpan(span, err) records the exception event AND sets error status;
	// do not also call RecordError for the same error or it duplicates.
	tracer.EndSpan(upSpan, errors.New("upstream 529: overloaded"))

	tracer.SetTokenUsage(reqCtx, 1200, 350)
	tracer.EndSpan(reqSpan, nil)

	// Shutdown flushes the batch processor to the collector.
	if err := setup.Shutdown(ctx); err != nil {
		t.Fatalf("Shutdown failed: %v", err)
	}

	collector.mu.Lock()
	defer collector.mu.Unlock()
	if len(collector.requests) == 0 {
		t.Fatal("collector received no OTLP export requests")
	}

	// Dump the exact wire payload (protojson of the OTLP protobuf).
	marshaler := protojson.MarshalOptions{Multiline: true, Indent: "  "}
	for _, req := range collector.requests {
		t.Logf("OTLP ExportTraceServiceRequest:\n%s", marshaler.Format(req))
	}

	// Assert the standard shape: resource + scope + both spans in one trace.
	req := collector.requests[0]
	if len(req.ResourceSpans) == 0 {
		t.Fatal("missing resourceSpans")
	}
	rs := req.ResourceSpans[0]

	resourceAttrs := map[string]string{}
	for _, kv := range rs.Resource.Attributes {
		resourceAttrs[kv.Key] = kv.Value.GetStringValue()
	}
	if resourceAttrs["service.name"] != "tingly-box" {
		t.Errorf("resource service.name = %q, want tingly-box", resourceAttrs["service.name"])
	}

	if len(rs.ScopeSpans) == 0 {
		t.Fatal("missing scopeSpans")
	}
	spans := rs.ScopeSpans[0].Spans
	if len(spans) != 2 {
		t.Fatalf("expected 2 spans, got %d", len(spans))
	}

	// Batch order is child-first (spans export on End).
	child, root := spans[0], spans[1]
	if child.Name != "llm.upstream.call" || root.Name != "chat claude-sonnet-4-6" {
		t.Errorf("unexpected span names: %q, %q", child.Name, root.Name)
	}
	if string(child.TraceId) != string(root.TraceId) {
		t.Error("child and root must share one traceId")
	}
	if string(child.ParentSpanId) != string(root.SpanId) {
		t.Error("child.parentSpanId must equal root.spanId")
	}
	if child.Status.GetCode() != 2 { // STATUS_CODE_ERROR
		t.Errorf("child status = %v, want STATUS_CODE_ERROR", child.Status.GetCode())
	}
	exceptions := 0
	for _, ev := range child.Events {
		if ev.Name == "exception" {
			exceptions++
		}
	}
	if exceptions != 1 {
		t.Errorf("child span has %d exception events, want exactly 1", exceptions)
	}

	rootAttrs := map[string]bool{}
	for _, kv := range root.Attributes {
		rootAttrs[kv.Key] = true
	}
	for _, want := range []string{
		"gen_ai.operation.name", "gen_ai.provider.name", "gen_ai.request.model",
		"gen_ai.usage.input_tokens", "gen_ai.usage.output_tokens",
		"tingly.scenario", "tingly.rule.uuid", "tingly.streaming",
	} {
		if !rootAttrs[want] {
			t.Errorf("root span missing attribute %s", want)
		}
	}

}
