package client

import (
	"net/http"

	"go.opentelemetry.io/otel"
	"go.opentelemetry.io/otel/propagation"
	"go.opentelemetry.io/otel/trace"
)

// propagatingTransport injects the W3C trace context (traceparent/baggage)
// into outbound upstream requests, continuing the gateway's request span
// across the provider hop. It sits directly on the base transport so the
// header hits the wire regardless of which vendor round tripper is layered
// above.
//
// Cost discipline: when tracing is disabled there is no span in the request
// context, so the wrapper delegates untouched — no clone, no headers.
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
	// RoundTrippers must not mutate the caller's request — clone before
	// injecting (Clone copies the header map).
	req = req.Clone(req.Context())
	otel.GetTextMapPropagator().Inject(req.Context(), propagation.HeaderCarrier(req.Header))
	return t.base.RoundTrip(req)
}
