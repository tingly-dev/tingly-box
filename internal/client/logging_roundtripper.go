package client

import (
	"net/http"
	"net/url"
	"time"

	"github.com/sirupsen/logrus"
	"github.com/tingly-dev/tingly-box/internal/obs"
	"github.com/tingly-dev/tingly-box/internal/protocol"
	"github.com/tingly-dev/tingly-box/internal/typ"
)

// loggingRoundTripper emits one log line per upstream request capturing the
// final outcome of the client stage — which proxy was used, the HTTP status and
// the latency. It correlates via the request context (request_id), so the line
// lands in the per-request model_request pipeline timeline. Proxy credentials
// are never logged.
type loggingRoundTripper struct {
	inner    http.RoundTripper
	provider *typ.Provider
	proxy    string // redacted (scheme://host) or "direct"
}

// wrapWithLogging wraps a transport so every provider's upstream call is logged
// uniformly. It is the single place that surfaces proxy + outcome per request.
// Logging only — rule flags are applied by wrapWithRuleFlags, mounted
// explicitly by the pass-through constructors (never on vendor chains).
func wrapWithLogging(inner http.RoundTripper, provider *typ.Provider) http.RoundTripper {
	var proxyRaw string
	if provider != nil {
		proxyRaw = provider.ProxyURL
	}
	return &loggingRoundTripper{
		inner:    inner,
		provider: provider,
		proxy:    redactProxy(proxyRaw),
	}
}

func (t *loggingRoundTripper) RoundTrip(req *http.Request) (*http.Response, error) {
	start := time.Now()

	// When no provider proxy is configured, resolve the env proxy at request
	// time so HTTP_PROXY/HTTPS_PROXY vars are reflected accurately in logs.
	proxy := t.proxy
	if proxy == "direct" {
		if envProxy, err := http.ProxyFromEnvironment(req); err == nil && envProxy != nil {
			proxy = redactProxy(envProxy.String())
		}
	}

	resp, err := t.inner.RoundTrip(req)
	latencyMs := time.Since(start).Milliseconds()

	// Build fields with provider information
	providerName := ""
	var apiStyle, baseURL string
	if t.provider != nil {
		providerName = t.provider.Name
		apiStyle = string(t.provider.APIStyle)
		baseURL = t.provider.APIBase
	}

	fields := logrus.Fields{
		"stage":      "upstream",
		"provider":   providerName,
		"proxy":      proxy,
		"method":     req.Method,
		"host":       req.URL.Host,
		"latency_ms": latencyMs,
	}
	if apiStyle != "" {
		fields["api_style"] = apiStyle
	}
	if baseURL != "" {
		fields["base_url"] = baseURL
	}

	entry := logrus.WithContext(req.Context()).WithFields(fields)
	if err != nil {
		// A transport-level failure (DNS, TCP connect, TLS, timeout) never
		// reached the upstream, so err is a raw Go net/url error — usually
		// readable, but not categorized, which is what makes it slow to
		// eyeball in a log stream. reason surfaces that category as its own
		// field so entries are filterable/greppable without parsing the
		// message text; the full err still goes out via WithError for the
		// exact underlying detail.
		if reason, ok := protocol.ClassifyTransportError(err); ok {
			entry = entry.WithField("fail_reason", string(reason))
		}
		entry.WithError(err).Errorf("upstream call failed via %s", proxy)
		return resp, err
	}
	// The provider did respond, so RoundTrip returned no error — but a 4xx/5xx
	// body is just as much a failure as the transport case above, and without
	// this it only ever reaches Info level, indistinguishable from a 200 to
	// anyone filtering logs by severity. The response body (the actual error
	// reason) isn't read here — the SDK layer above still owns parsing it
	// into a typed error — so this only classifies by status, the same way
	// the HTTP access log does (obs.LevelForStatus). Logf defers the
	// Sprintf until logrus confirms the level is enabled, same as Errorf did
	// on the branch above.
	entry.WithField("status", resp.StatusCode).
		Logf(obs.LevelForStatus(resp.StatusCode), "upstream %d via %s", resp.StatusCode, proxy)
	return resp, nil
}

// redactProxy returns a safe description of a proxy URL for logging.
// Credentials are masked as "***" rather than dropped so it remains clear that
// authentication is in use: "scheme://***@host:port". Without the mask,
// "scheme://host" looks identical to an unauthenticated proxy, which is
// misleading. Returns "direct" when no proxy is configured.
func redactProxy(raw string) string {
	if raw == "" || raw == ProxyURLNone {
		return "direct"
	}
	u, err := url.Parse(raw)
	if err != nil || u.Host == "" {
		// Unparseable — never echo the raw string (may contain credentials).
		return "proxy(set)"
	}
	if u.User != nil {
		return u.Scheme + "://***@" + u.Host
	}
	return u.Scheme + "://" + u.Host
}
