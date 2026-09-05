package client

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"sort"

	"github.com/tidwall/sjson"
)

// ProbeRewrite describes edits the probe layer applies to an outgoing HTTP
// request AFTER the SDK has serialized it — the last word on what leaves the
// process. Body maps a JSON path (sjson syntax: "temperature",
// "messages.0.content", "metadata.user_id") to the raw JSON to put there;
// a JSON null deletes the path. Headers sets a header, or removes it when
// the value is empty. Only probe code attaches a rewrite (via the request
// context), so production traffic never sees one.
type ProbeRewrite struct {
	Body    map[string]json.RawMessage
	Headers map[string]string
}

// Empty reports whether the rewrite would change nothing.
func (r *ProbeRewrite) Empty() bool {
	return r == nil || (len(r.Body) == 0 && len(r.Headers) == 0)
}

type probeRewriteKey struct{}

// WithProbeRewrite stores rw in ctx so probeRewriteRoundTripper applies it to
// every SDK HTTP call carrying that context.
func WithProbeRewrite(ctx context.Context, rw *ProbeRewrite) context.Context {
	return context.WithValue(ctx, probeRewriteKey{}, rw)
}

// GetProbeRewrite returns the rewrite stored by WithProbeRewrite, if any.
func GetProbeRewrite(ctx context.Context) (*ProbeRewrite, bool) {
	rw, ok := ctx.Value(probeRewriteKey{}).(*ProbeRewrite)
	return rw, ok && !rw.Empty()
}

// sortedKeys returns m's keys in sorted order so overrides apply
// deterministically (a set and a delete on overlapping paths must not
// depend on map iteration order).
func sortedKeys[V any](m map[string]V) []string {
	keys := make([]string, 0, len(m))
	for k := range m {
		keys = append(keys, k)
	}
	sort.Strings(keys)
	return keys
}

// ApplyBodyOverrides returns body with each override applied in key order.
// Shared by the probe transport (real request) and the cURL builder
// (rendered request), so the two cannot disagree.
func ApplyBodyOverrides(body []byte, overrides map[string]json.RawMessage) ([]byte, error) {
	if len(overrides) == 0 {
		return body, nil
	}
	out := body
	var err error
	for _, path := range sortedKeys(overrides) {
		raw := bytes.TrimSpace(overrides[path])
		if bytes.Equal(raw, []byte("null")) {
			out, err = sjson.DeleteBytes(out, path)
		} else {
			if !json.Valid(raw) {
				return nil, fmt.Errorf("body override %q: value is not valid JSON", path)
			}
			out, err = sjson.SetRawBytes(out, path, raw)
		}
		if err != nil {
			return nil, fmt.Errorf("body override %q: %w", path, err)
		}
	}
	return out, nil
}

// ApplyHeaderOverrides sets each override on h; an empty value removes the
// header instead.
func ApplyHeaderOverrides(h http.Header, overrides map[string]string) {
	for _, name := range sortedKeys(overrides) {
		if v := overrides[name]; v == "" {
			h.Del(name)
		} else {
			h.Set(name, v)
		}
	}
}

// probeRewriteRoundTripper applies the context's ProbeRewrite to the request
// right before it hits the transport. It is layered innermost on probe
// clients so its edits win over everything the SDK and the other probe
// wrappers set. No-op for requests without a rewrite.
type probeRewriteRoundTripper struct {
	inner http.RoundTripper
}

func (t *probeRewriteRoundTripper) RoundTrip(req *http.Request) (*http.Response, error) {
	inner := t.inner
	if inner == nil {
		inner = http.DefaultTransport
	}
	rw, ok := GetProbeRewrite(req.Context())
	if !ok {
		return inner.RoundTrip(req)
	}
	req = req.Clone(req.Context())
	ApplyHeaderOverrides(req.Header, rw.Headers)
	if len(rw.Body) > 0 && req.Body != nil {
		raw, err := io.ReadAll(req.Body)
		_ = req.Body.Close()
		if err != nil {
			return nil, fmt.Errorf("probe rewrite: read body: %w", err)
		}
		edited, err := ApplyBodyOverrides(raw, rw.Body)
		if err != nil {
			return nil, fmt.Errorf("probe rewrite: %w", err)
		}
		req.Body = io.NopCloser(bytes.NewReader(edited))
		req.ContentLength = int64(len(edited))
		req.GetBody = func() (io.ReadCloser, error) {
			return io.NopCloser(bytes.NewReader(edited)), nil
		}
	}
	return inner.RoundTrip(req)
}

// ApplyProbeRewriteToClient layers probeRewriteRoundTripper onto a probe
// client's transport. Call it before the other probe wrappers so it sits
// innermost. Probe clients only — never production instances.
func ApplyProbeRewriteToClient(c interface{}) {
	applyTransportWrap(c, func(inner http.RoundTripper) http.RoundTripper {
		return &probeRewriteRoundTripper{inner: inner}
	})
}
