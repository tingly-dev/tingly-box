package client

import (
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"go.opentelemetry.io/otel"
	"go.opentelemetry.io/otel/propagation"
	sdktrace "go.opentelemetry.io/otel/sdk/trace"
	"go.opentelemetry.io/otel/sdk/trace/tracetest"
)

type captureRoundTripper struct {
	req  *http.Request
	resp *http.Response
}

func (c *captureRoundTripper) RoundTrip(req *http.Request) (*http.Response, error) {
	c.req = req
	if c.resp != nil {
		return c.resp, nil
	}
	return &http.Response{StatusCode: http.StatusOK, Body: http.NoBody}, nil
}

// withRecordingTracer installs a real tracer provider plus the W3C
// propagator and returns a context carrying a started root span, mirroring
// how the gateway middleware sets things up.
func withRecordingTracer(t *testing.T) (ctxSpanEnd func(), ctx *http.Request, exporter *tracetest.InMemoryExporter) {
	t.Helper()
	prev := otel.GetTextMapPropagator()
	otel.SetTextMapPropagator(propagation.TraceContext{})
	exporter = tracetest.NewInMemoryExporter()
	tp := sdktrace.NewTracerProvider(sdktrace.WithSyncer(exporter))
	prevTP := otel.GetTracerProvider()
	otel.SetTracerProvider(tp)

	spanCtx, root := tp.Tracer("test").Start(t.Context(), "root")
	req := httptest.NewRequest(http.MethodPost, "https://upstream.example/v1/messages", nil).WithContext(spanCtx)

	return func() {
		root.End()
		otel.SetTextMapPropagator(prev)
		otel.SetTracerProvider(prevTP)
	}, req, exporter
}

func upstreamSpans(exporter *tracetest.InMemoryExporter) []tracetest.SpanStub {
	var out []tracetest.SpanStub
	for _, s := range exporter.GetSpans() {
		if s.Name == "upstream" {
			out = append(out, s)
		}
	}
	return out
}

func TestPropagatingTransport_NoSpanUntouched(t *testing.T) {
	capture := &captureRoundTripper{}
	transport := newPropagatingTransport(capture)

	req := httptest.NewRequest(http.MethodPost, "https://upstream.example/v1/messages", nil)
	if _, err := transport.RoundTrip(req); err != nil {
		t.Fatal(err)
	}
	if capture.req != req {
		t.Error("request must pass through unchanged when no span is present")
	}
	if got := capture.req.Header.Get("traceparent"); got != "" {
		t.Errorf("unexpected traceparent header without a span: %q", got)
	}
}

func TestPropagatingTransport_NoSpanRecordedWithoutTracing(t *testing.T) {
	exporter := tracetest.NewInMemoryExporter()
	capture := &captureRoundTripper{}
	transport := newPropagatingTransport(capture)

	req := httptest.NewRequest(http.MethodPost, "https://upstream.example/v1/messages", nil)
	if _, err := transport.RoundTrip(req); err != nil {
		t.Fatal(err)
	}
	if got := len(exporter.GetSpans()); got != 0 {
		t.Errorf("expected no spans without a request-context span, got %d", got)
	}
}

func TestPropagatingTransport_InjectsTraceparent(t *testing.T) {
	cleanup, req, _ := withRecordingTracer(t)
	defer cleanup()

	capture := &captureRoundTripper{}
	transport := newPropagatingTransport(capture)

	if _, err := transport.RoundTrip(req); err != nil {
		t.Fatal(err)
	}

	if capture.req == req {
		t.Error("request must be cloned before header injection (RoundTripper contract)")
	}
	if got := capture.req.Header.Get("traceparent"); got == "" {
		t.Error("expected traceparent header on the outbound request")
	}
	if got := req.Header.Get("traceparent"); got != "" {
		t.Errorf("original request must not be mutated, found traceparent %q", got)
	}
}

func TestPropagatingTransport_UpstreamSpanEndsOnBodyClose(t *testing.T) {
	cleanup, req, exporter := withRecordingTracer(t)
	defer cleanup()

	capture := &captureRoundTripper{resp: &http.Response{
		StatusCode: http.StatusOK,
		Body:       io.NopCloser(strings.NewReader("streamed tokens")),
	}}
	transport := newPropagatingTransport(capture)

	resp, err := transport.RoundTrip(req)
	if err != nil {
		t.Fatal(err)
	}

	// A streamed response keeps producing tokens long after RoundTrip
	// returns, so the span must still be open here.
	if got := len(upstreamSpans(exporter)); got != 0 {
		t.Fatalf("upstream span ended before the body was consumed (got %d)", got)
	}

	_ = resp.Body.Close()

	spans := upstreamSpans(exporter)
	if len(spans) != 1 {
		t.Fatalf("expected 1 upstream span after body close, got %d", len(spans))
	}
	attrs := map[string]interface{}{}
	for _, kv := range spans[0].Attributes {
		attrs[string(kv.Key)] = kv.Value.AsInterface()
	}
	if attrs["http.response.status_code"] != int64(200) {
		t.Errorf("status attr = %v, want 200", attrs["http.response.status_code"])
	}
	if attrs["server.address"] != "upstream.example" {
		t.Errorf("server.address = %v, want upstream.example", attrs["server.address"])
	}
}

func TestPropagatingTransport_UpstreamSpanEndsOnceOnDrainThenClose(t *testing.T) {
	cleanup, req, exporter := withRecordingTracer(t)
	defer cleanup()

	capture := &captureRoundTripper{resp: &http.Response{
		StatusCode: http.StatusBadGateway,
		Body:       io.NopCloser(strings.NewReader("upstream failed")),
	}}
	transport := newPropagatingTransport(capture)

	resp, err := transport.RoundTrip(req)
	if err != nil {
		t.Fatal(err)
	}
	// Drain to EOF, then close — both signal completion; the span must be
	// ended exactly once.
	_, _ = io.ReadAll(resp.Body)
	_ = resp.Body.Close()

	spans := upstreamSpans(exporter)
	if len(spans) != 1 {
		t.Fatalf("expected exactly 1 upstream span, got %d", len(spans))
	}
	if spans[0].Status.Code.String() != "Error" {
		t.Errorf("502 upstream should carry error status, got %v", spans[0].Status.Code)
	}
}
