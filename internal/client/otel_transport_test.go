package client

import (
	"net/http"
	"net/http/httptest"
	"testing"

	"go.opentelemetry.io/otel"
	"go.opentelemetry.io/otel/propagation"
	sdktrace "go.opentelemetry.io/otel/sdk/trace"
	"go.opentelemetry.io/otel/sdk/trace/tracetest"
)

type captureRoundTripper struct {
	req *http.Request
}

func (c *captureRoundTripper) RoundTrip(req *http.Request) (*http.Response, error) {
	c.req = req
	return &http.Response{StatusCode: http.StatusOK, Body: http.NoBody}, nil
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

func TestPropagatingTransport_InjectsTraceparent(t *testing.T) {
	prev := otel.GetTextMapPropagator()
	otel.SetTextMapPropagator(propagation.TraceContext{})
	defer otel.SetTextMapPropagator(prev)

	tp := sdktrace.NewTracerProvider(sdktrace.WithSyncer(tracetest.NewInMemoryExporter()))
	ctx, span := tp.Tracer("test").Start(t.Context(), "root")
	defer span.End()

	capture := &captureRoundTripper{}
	transport := newPropagatingTransport(capture)

	req := httptest.NewRequest(http.MethodPost, "https://upstream.example/v1/messages", nil)
	req = req.WithContext(ctx)
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
