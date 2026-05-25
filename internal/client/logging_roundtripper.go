package client

import (
	"net/http"
	"net/url"
	"time"

	"github.com/sirupsen/logrus"
	"github.com/tingly-dev/tingly-box/internal/typ"
)

// loggingRoundTripper emits one log line per upstream request capturing the
// final outcome of the client stage — which proxy was used, the HTTP status and
// the latency. It correlates via the request context (request_id), so the line
// lands in the per-request model_request pipeline timeline. Proxy credentials
// are never logged.
type loggingRoundTripper struct {
	inner    http.RoundTripper
	provider string
	proxy    string // redacted (scheme://host) or "direct"
}

// wrapWithLogging wraps a transport so every provider's upstream call is logged
// uniformly. It is the single place that surfaces proxy + outcome per request.
func wrapWithLogging(inner http.RoundTripper, provider *typ.Provider) http.RoundTripper {
	var proxyRaw, name string
	if provider != nil {
		proxyRaw = provider.ProxyURL
		name = provider.Name
	}
	return &loggingRoundTripper{
		inner:    inner,
		provider: name,
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

	entry := logrus.WithContext(req.Context()).WithFields(logrus.Fields{
		"stage":      "upstream",
		"provider":   t.provider,
		"proxy":      proxy,
		"method":     req.Method,
		"host":       req.URL.Host,
		"latency_ms": latencyMs,
	})
	if err != nil {
		entry.WithError(err).Errorf("upstream call failed via %s", proxy)
		return resp, err
	}
	entry.WithField("status", resp.StatusCode).Infof("upstream %d via %s", resp.StatusCode, proxy)
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
