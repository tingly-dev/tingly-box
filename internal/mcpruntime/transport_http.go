package mcpruntime

import (
	"net/http"
	"net/url"
	"strings"
	"time"

	"github.com/tingly-dev/tingly-box/internal/typ"
)

// buildHTTPClient builds an http.Client for an MCP HTTP source,
// applying proxy URL.
func buildHTTPClient(source typ.MCPSourceConfig) *http.Client {
	transport := &http.Transport{
		// Default to system environment proxy when source-specific proxy is not set.
		Proxy: http.ProxyFromEnvironment,
	}

	// Use custom proxy if configured.
	if strings.TrimSpace(source.ProxyURL) != "" {
		if proxyURL, err := url.Parse(source.ProxyURL); err == nil {
			transport.Proxy = http.ProxyURL(proxyURL)
		}
	}

	return &http.Client{
		Transport: &headerInjectRoundTripper{Transport: transport, Headers: source.Headers},
		Timeout:   30 * time.Second, // default timeout; SDK respects context deadlines
	}
}

// headerInjectRoundTripper is an http.RoundTripper that injects custom headers
// before delegating to the underlying transport.
type headerInjectRoundTripper struct {
	Transport *http.Transport
	Headers   map[string]string
}

func (rt *headerInjectRoundTripper) RoundTrip(req *http.Request) (*http.Response, error) {
	// Inject custom headers.
	for k, v := range rt.Headers {
		if strings.TrimSpace(k) != "" {
			req.Header.Set(k, v)
		}
	}
	return rt.Transport.RoundTrip(req)
}
