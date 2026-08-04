package client

import (
	"io"
	"net/http"
	"sync"

	"go.opentelemetry.io/otel"
	"go.opentelemetry.io/otel/codes"
	"go.opentelemetry.io/otel/propagation"
	"go.opentelemetry.io/otel/trace"

	pkgotel "github.com/tingly-dev/tingly-box/pkg/otel"
)

// propagatingTransport is the trace seam on the upstream hop. It sits
// directly on the base transport — beneath every vendor round tripper — so
// it sees exactly one thing: the real provider call.
//
// It does two jobs there:
//   - injects the W3C trace context so the gateway's trace continues into
//     the provider (and into whatever the provider reports back);
//   - records the "upstream" span, the stage that accounts for nearly all
//     of a request's latency and the backbone of the WebUI journey view.
//
// Cost discipline: with tracing off there is no span in the request
// context, so RoundTrip delegates untouched — no clone, no headers, no span.
type propagatingTransport struct {
	base http.RoundTripper
}

func newPropagatingTransport(base http.RoundTripper) http.RoundTripper {
	return &propagatingTransport{base: base}
}

// Unwrap exposes the underlying transport so tests can assert the shape of
// the provider-specific chain beneath the protocol-neutral wrapper.
func (t *propagatingTransport) Unwrap() http.RoundTripper { return t.base }

func (t *propagatingTransport) RoundTrip(req *http.Request) (*http.Response, error) {
	if !trace.SpanContextFromContext(req.Context()).IsValid() {
		return t.base.RoundTrip(req)
	}

	ctx, span := otel.Tracer("tingly-box").Start(req.Context(), "upstream",
		trace.WithSpanKind(trace.SpanKindClient),
		trace.WithAttributes(
			pkgotel.AttrHTTPRequestMethod.String(req.Method),
			pkgotel.AttrServerAddress.String(req.URL.Host),
		),
	)

	// RoundTrippers must not mutate the caller's request — clone before
	// injecting (Clone copies the header map).
	req = req.Clone(ctx)
	otel.GetTextMapPropagator().Inject(ctx, propagation.HeaderCarrier(req.Header))

	resp, err := t.base.RoundTrip(req)
	if err != nil {
		span.RecordError(err)
		span.SetStatus(codes.Error, err.Error())
		span.End()
		return nil, err
	}

	span.SetAttributes(pkgotel.AttrHTTPResponseStatus.Int(resp.StatusCode))
	if resp.StatusCode >= http.StatusBadRequest {
		span.SetStatus(codes.Error, http.StatusText(resp.StatusCode))
	}

	// The span must cover the whole upstream exchange, not just the
	// headers: for a streamed completion RoundTrip returns as soon as the
	// response head arrives while tokens keep flowing for seconds. Ending
	// on body close makes the recorded duration the real upstream time.
	if resp.Body != nil {
		resp.Body = &spanEndingBody{ReadCloser: resp.Body, span: span}
	} else {
		span.End()
	}
	return resp, nil
}

// spanEndingBody ends the upstream span when the response body is closed or
// fully drained, whichever happens first. once guards the double signal
// (drained then closed) — a span must be ended exactly once.
type spanEndingBody struct {
	io.ReadCloser
	span trace.Span
	once sync.Once
}

func (b *spanEndingBody) end() {
	b.once.Do(func() { b.span.End() })
}

func (b *spanEndingBody) Read(p []byte) (int, error) {
	n, err := b.ReadCloser.Read(p)
	if err != nil {
		b.end()
	}
	return n, err
}

func (b *spanEndingBody) Close() error {
	b.end()
	return b.ReadCloser.Close()
}
