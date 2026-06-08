package client

import (
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/sirupsen/logrus"
	"github.com/tingly-dev/tingly-box/internal/typ"
)

// captureProxyHook collects the `proxy` field value from any logrus entry
// emitted while it is installed, so tests can assert what the round-tripper
// actually logs without depending on http.ProxyFromEnvironment's internal
// env cache (which is set on first call and ignores later t.Setenv).
type captureProxyHook struct{ proxies []string }

func (h *captureProxyHook) Levels() []logrus.Level { return logrus.AllLevels }
func (h *captureProxyHook) Fire(e *logrus.Entry) error {
	if v, ok := e.Data["proxy"]; ok {
		if s, ok := v.(string); ok {
			h.proxies = append(h.proxies, s)
		}
	}
	return nil
}

func TestRedactProxy(t *testing.T) {
	cases := []struct {
		raw  string
		want string
	}{
		{"", "direct"},
		{ProxyURLNone, "direct"},
		{"http://proxy.example.com:8080", "http://proxy.example.com:8080"},
		{"socks5://10.0.0.1:1080", "socks5://10.0.0.1:1080"},
		{"http://user:secret@proxy.example.com:8080", "http://***@proxy.example.com:8080"},
		{"://bad", "proxy(set)"},
	}
	for _, c := range cases {
		got := redactProxy(c.raw)
		if got != c.want {
			t.Errorf("redactProxy(%q) = %q, want %q", c.raw, got, c.want)
		}
		// Hard guarantee: credentials must never appear in the logged value.
		if strings.Contains(got, "secret") {
			t.Errorf("redactProxy(%q) leaked credentials: %q", c.raw, got)
		}
	}
}

// TestLoggingRoundTripper_EnvProxy verifies that when no provider proxy is
// configured but HTTP_PROXY is set in the environment, the round-tripper
// resolves and logs the real env proxy rather than "direct".
func TestLoggingRoundTripper_EnvProxy(t *testing.T) {
	// Stand up a minimal proxy stub so http.ProxyFromEnvironment can resolve it.
	proxyStub := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {}))
	defer proxyStub.Close()

	t.Setenv("HTTP_PROXY", proxyStub.URL)
	t.Setenv("HTTPS_PROXY", proxyStub.URL)

	// http.ProxyFromEnvironment caches its env config on first call (sync.Once
	// inside net/http). When another test in this package already triggered
	// the cache with no HTTP_PROXY set, our t.Setenv is ignored and the
	// round-tripper logs "direct". Skip in that case rather than flake —
	// TestRedactProxy still covers the redaction contract.
	probeReq, _ := http.NewRequest("GET", "http://example.com/", nil)
	if envProxy, _ := http.ProxyFromEnvironment(probeReq); envProxy == nil {
		t.Skip("http.ProxyFromEnvironment env cache initialized without HTTP_PROXY; env-proxy path unreachable in this run")
	}

	hook := &captureProxyHook{}
	logrus.AddHook(hook)
	t.Cleanup(func() {
		logrus.StandardLogger().ReplaceHooks(logrus.LevelHooks{})
	})

	fn := roundTripFunc(func(req *http.Request) (*http.Response, error) {
		return &http.Response{StatusCode: 200, Body: http.NoBody}, nil
	})

	lrt := &loggingRoundTripper{
		inner:    &fn,
		provider: &typ.Provider{Name: "test"},
		proxy:    "direct", // no provider-level proxy
	}

	req, _ := http.NewRequest("GET", "http://example.com/", nil)
	if _, err := lrt.RoundTrip(req); err != nil {
		t.Fatalf("RoundTrip: %v", err)
	}

	if len(hook.proxies) == 0 {
		t.Fatalf("no log entry with proxy field captured")
	}
	loggedProxy := hook.proxies[len(hook.proxies)-1]
	if loggedProxy == "direct" || loggedProxy == "" {
		t.Errorf("expected env proxy to be detected, got %q", loggedProxy)
	}

	// Credentials must never appear.
	if strings.Contains(loggedProxy, "@") {
		t.Errorf("proxy value contains userinfo: %q", loggedProxy)
	}
}

// TestLoggingRoundTripper_ProviderProxyTakesPrecedence ensures that an
// explicitly configured provider proxy is used as-is and env vars are ignored.
func TestLoggingRoundTripper_ProviderProxyTakesPrecedence(t *testing.T) {
	t.Setenv("HTTP_PROXY", "http://env-proxy.example.com:3128")

	lrt := wrapWithLogging(http.DefaultTransport, &typ.Provider{
		Name:     "test",
		ProxyURL: "http://provider-proxy.example.com:8080",
	}).(*loggingRoundTripper)

	if lrt.proxy != "http://provider-proxy.example.com:8080" {
		t.Errorf("expected provider proxy, got %q", lrt.proxy)
	}
}

// roundTripFunc is a one-shot http.RoundTripper backed by a function.
type roundTripFunc func(*http.Request) (*http.Response, error)

func (f *roundTripFunc) RoundTrip(req *http.Request) (*http.Response, error) { return (*f)(req) }
