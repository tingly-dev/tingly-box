package client

import (
	"bytes"
	"context"
	"io"
	"net/http"
)

// WireRecorder is the minimal interface the wire-level recording transport
// needs from a request-scoped recorder. Defined here (not imported from
// internal/protocolserver/recording) so internal/client stays free of any
// upward dependency — recording.ProtocolRecorder satisfies this interface
// structurally, the same duck-typing pattern typ.RuleFlags/GetRuleFlags uses
// to cross this same boundary.
type WireRecorder interface {
	// WantsUpstreamRequest reports whether the current request should capture
	// the final wire request (method/URL/body).
	WantsUpstreamRequest() bool
	// RecordWireRequest stores the request exactly as it is about to go out
	// on the wire. Headers are deliberately not captured for now — see
	// wireRecorderTransport's doc comment.
	RecordWireRequest(method, url string, body []byte)
}

type wireRecorderKey struct{}

// noopWireRecorder always declines — used by WithoutWireRecorder to shadow an
// inherited recorder rather than trying to store a literal nil (a stored nil
// interface value round-trips as "absent" through context.Value, which would
// NOT shadow the parent's value).
type noopWireRecorder struct{}

func (noopWireRecorder) WantsUpstreamRequest() bool               { return false }
func (noopWireRecorder) RecordWireRequest(string, string, []byte) {}

// WithWireRecorder attaches rec so wrapWithWireRecorder — mounted as the
// innermost transport layer, closest to the actual wire send — can capture
// the truly final outbound request: method and URL exactly as every outer
// layer (rule flags, vendor round trippers, session binding) has already
// mutated them. This is the corrected design point for upstream_request
// capture: the chain layer used to snapshot a pre-wire SDK struct with the
// wrong URL (see .design/recording.md).
//
// rec==nil is a no-op (recording disabled for this request).
func WithWireRecorder(ctx context.Context, rec WireRecorder) context.Context {
	if rec == nil {
		return ctx
	}
	return context.WithValue(ctx, wireRecorderKey{}, rec)
}

// WithoutWireRecorder shadows any inherited WireRecorder so a nested outbound
// call sharing the parent's context — the in-process advisor tool's own LLM
// call is the one case in this codebase today — never overwrites the parent
// request's captured wire request with its own. Any future code that issues
// its own outbound call from a context derived from the main request's ctx
// must call this first for the same reason.
func WithoutWireRecorder(ctx context.Context) context.Context {
	return context.WithValue(ctx, wireRecorderKey{}, WireRecorder(noopWireRecorder{}))
}

func wireRecorderFromContext(ctx context.Context) WireRecorder {
	rec, _ := ctx.Value(wireRecorderKey{}).(WireRecorder)
	return rec
}

// wireRecorderTransport is a read-only observer: it never mutates the
// request, so — unlike ruleFlagTransport — it is safe to mount on vendor
// chains too (it cannot corrupt a vendor handshake).
//
// Headers are deliberately not captured: request headers routinely carry
// credentials (Authorization, x-api-key, and whatever a rule's free-form
// extra_headers names), and records land in files on disk. Capturing only
// method/URL/body keeps this safe by construction instead of relying on a
// redaction policy — revisit if header capture becomes a real need.
type wireRecorderTransport struct {
	inner http.RoundTripper
}

// wrapWithWireRecorder mounts the wire-level request recorder. Mount it as
// the innermost layer on every transport chain (right around the actual
// wire-touching transport — createSessionBoundTransport or the pooled
// *http.Transport — before any header-rewriting layer wraps it from
// outside), so it observes the request after every other layer's mutations.
func wrapWithWireRecorder(inner http.RoundTripper) http.RoundTripper {
	return &wireRecorderTransport{inner: inner}
}

func (t *wireRecorderTransport) RoundTrip(req *http.Request) (*http.Response, error) {
	inner := t.inner
	if inner == nil {
		inner = http.DefaultTransport
	}

	rec := wireRecorderFromContext(req.Context())
	if rec == nil || !rec.WantsUpstreamRequest() {
		return inner.RoundTrip(req)
	}

	var bodyBytes []byte
	if req.Body != nil && req.Body != http.NoBody {
		b, err := io.ReadAll(req.Body)
		_ = req.Body.Close()
		if err == nil {
			bodyBytes = b
			// Restore so the actual send still has its body. This transport
			// is the innermost wrapper — nothing downstream reads Body again
			// except the real net/http.Transport that sends it.
			req.Body = io.NopCloser(bytes.NewReader(b))
		}
	}

	rec.RecordWireRequest(req.Method, req.URL.String(), bodyBytes)
	return inner.RoundTrip(req)
}
