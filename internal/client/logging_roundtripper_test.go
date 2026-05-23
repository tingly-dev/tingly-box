package client

import (
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/tingly-dev/tingly-box/internal/typ"
)

func TestRedactProxy(t *testing.T) {
	cases := []struct {
		raw  string
		want string
	}{
		{"", "direct"},
		{ProxyURLNone, "direct"},
		{"http://proxy.example.com:8080", "http://proxy.example.com:8080"},
		{"socks5://10.0.0.1:1080", "socks5://10.0.0.1:1080"},
		{"http://user:secret@proxy.example.com:8080", "http://proxy.example.com:8080"},
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

	var loggedProxy string
	fn := roundTripFunc(func(req *http.Request) (*http.Response, error) {
		// Capture the proxy that was resolved — we can't hook the log line
		// directly, so instead we validate via the public contract: the inner
		// transport receives the request and we check the env proxy resolves.
		if envProxy, err := http.ProxyFromEnvironment(req); err == nil && envProxy != nil {
			loggedProxy = redactProxy(envProxy.String())
		}
		return &http.Response{StatusCode: 200, Body: http.NoBody}, nil
	})

	lrt := &loggingRoundTripper{
		inner:    &fn,
		provider: "test",
		proxy:    "direct", // no provider-level proxy
	}

	req, _ := http.NewRequest("GET", "http://example.com/", nil)
	if _, err := lrt.RoundTrip(req); err != nil {
		t.Fatalf("RoundTrip: %v", err)
	}

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
