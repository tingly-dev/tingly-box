package client

import (
	"context"
	"net/http"
	"sort"
)

// Probe header overrides: a name→value map the probe layer applies to the
// outgoing HTTP request right before it hits the transport — the last word
// on the headers that leave the process. An empty value removes the header.
// Only probe code attaches overrides (via the request context), so
// production traffic never sees them.

type probeHeaderOverridesKey struct{}

// WithProbeHeaderOverrides stores overrides in ctx so
// probeHeaderOverridesRoundTripper applies them to every SDK HTTP call
// carrying that context.
func WithProbeHeaderOverrides(ctx context.Context, overrides map[string]string) context.Context {
	return context.WithValue(ctx, probeHeaderOverridesKey{}, overrides)
}

// GetProbeHeaderOverrides returns the overrides stored by
// WithProbeHeaderOverrides, if any.
func GetProbeHeaderOverrides(ctx context.Context) (map[string]string, bool) {
	o, ok := ctx.Value(probeHeaderOverridesKey{}).(map[string]string)
	return o, ok && len(o) > 0
}

// ApplyHeaderOverrides sets each override on h in name order; an empty value
// removes the header instead.
func ApplyHeaderOverrides(h http.Header, overrides map[string]string) {
	names := make([]string, 0, len(overrides))
	for name := range overrides {
		names = append(names, name)
	}
	sort.Strings(names)
	for _, name := range names {
		if v := overrides[name]; v == "" {
			h.Del(name)
		} else {
			h.Set(name, v)
		}
	}
}

// probeHeaderOverridesRoundTripper applies the context's header overrides.
// It is layered innermost on probe clients so its edits win over everything
// the SDK and the other probe wrappers set. No-op without overrides.
type probeHeaderOverridesRoundTripper struct {
	inner http.RoundTripper
}

func (t *probeHeaderOverridesRoundTripper) RoundTrip(req *http.Request) (*http.Response, error) {
	inner := t.inner
	if inner == nil {
		inner = http.DefaultTransport
	}
	overrides, ok := GetProbeHeaderOverrides(req.Context())
	if !ok {
		return inner.RoundTrip(req)
	}
	req = req.Clone(req.Context())
	ApplyHeaderOverrides(req.Header, overrides)
	return inner.RoundTrip(req)
}

// ApplyProbeHeaderOverridesToClient layers probeHeaderOverridesRoundTripper
// onto a probe client's transport. Call it before the other probe wrappers
// so it sits innermost. Probe clients only — never production instances.
func ApplyProbeHeaderOverridesToClient(c interface{}) {
	applyTransportWrap(c, func(inner http.RoundTripper) http.RoundTripper {
		return &probeHeaderOverridesRoundTripper{inner: inner}
	})
}
