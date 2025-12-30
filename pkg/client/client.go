package client

import (
	"fmt"
	"net/http"
	"net/url"

	oauth2 "tingly-box/pkg/oauth"

	"github.com/sirupsen/logrus"
	"golang.org/x/net/proxy"
)

// oauthCustomHeaders defines custom headers for OAuth providers based on provider type
var oauthCustomHeaders = map[oauth2.ProviderType]map[string]string{
	oauth2.ProviderClaudeCode: {
		"anthropic-beta": "claude-code-20250219,oauth-2025-04-20,interleaved-thinking-2025-05-14,fine-grained-tool-streaming-2025-05-14",
		"anthropic-dangerous-direct-browser-access": "true",
		"anthropic-version":                         "2023-06-01",
		"user-agent":                                "claude-cli/1.0.0 (external, cli)",
		"x-app":                                     "cli",
	},
}

// oauthCustomParams defines custom query params for OAuth providers based on provider type
var oauthCustomParams = map[oauth2.ProviderType]map[string]string{
	oauth2.ProviderClaudeCode: {
		"beta": "true",
	},
}

// requestModifier wraps an http.RoundTripper to add custom headers and query params to each request
type requestModifier struct {
	http.RoundTripper
	headers map[string]string
	params  map[string]string
}

func (t *requestModifier) RoundTrip(req *http.Request) (*http.Response, error) {
	// Add custom headers
	for k, v := range t.headers {
		req.Header.Set(k, v)
	}
	// Add custom query params
	if len(t.params) > 0 {
		q := req.URL.Query()
		for k, v := range t.params {
			q.Add(k, v)
		}
		req.URL.RawQuery = q.Encode()
	}
	key := req.Header.Get("X-Api-Key")
	req.Header.Set("Authorization", fmt.Sprintf("Bearer %s", key))
	delete(req.Header, "X-Api-Key")
	return t.RoundTripper.RoundTrip(req)
}

// GetOAuthCustomHeaders returns custom headers for the given provider type
func GetOAuthCustomHeaders(providerType oauth2.ProviderType) map[string]string {
	headers, ok := oauthCustomHeaders[providerType]
	if !ok {
		return nil
	}
	return headers
}

// GetOAuthCustomParams returns custom query params for the given provider type
func GetOAuthCustomParams(providerType oauth2.ProviderType) map[string]string {
	params, ok := oauthCustomParams[providerType]
	if !ok {
		return nil
	}
	return params
}

// CreateHTTPClientWithProxy creates an HTTP client with proxy support
func CreateHTTPClientWithProxy(proxyURL string) *http.Client {
	if proxyURL == "" {
		return http.DefaultClient
	}

	// Parse the proxy URL
	parsedURL, err := url.Parse(proxyURL)
	if err != nil {
		logrus.Errorf("Failed to parse proxy URL %s: %v, using default client", proxyURL, err)
		return http.DefaultClient
	}

	// Create transport with proxy
	transport := &http.Transport{}

	switch parsedURL.Scheme {
	case "http", "https":
		transport.Proxy = http.ProxyURL(parsedURL)
	case "socks5":
		dialer, err := proxy.SOCKS5("tcp", parsedURL.Host, nil, proxy.Direct)
		if err != nil {
			logrus.Errorf("Failed to create SOCKS5 proxy dialer: %v, using default client", err)
			return http.DefaultClient
		}
		dialContext, ok := dialer.(proxy.ContextDialer)
		if ok {
			transport.DialContext = dialContext.DialContext
		} else {
			return http.DefaultClient
		}
	default:
		logrus.Errorf("Unsupported proxy scheme %s, supported schemes are http, https, socks5", parsedURL.Scheme)
		return http.DefaultClient
	}

	return &http.Client{
		Transport: transport,
	}
}

// CreateHTTPClientForProvider creates an HTTP client configured for the given provider
// It handles proxy and OAuth custom headers/params if applicable
//
// providerType: the OAuth provider type (e.g., oauth2.ProviderClaudeCode)
// proxyURL: optional proxy URL (can be empty)
// isOAuth: whether this is an OAuth provider
//
// Returns a configured http.Client
func CreateHTTPClientForProvider(providerType oauth2.ProviderType, proxyURL string, isOAuth bool) *http.Client {
	client := CreateHTTPClientWithProxy(proxyURL)

	if isOAuth {
		headers := GetOAuthCustomHeaders(providerType)
		params := GetOAuthCustomParams(providerType)
		if len(headers) > 0 || len(params) > 0 {
			// Use the client's transport, or default transport if nil (http.DefaultClient has nil Transport)
			transport := client.Transport
			if transport == nil {
				transport = http.DefaultTransport
			}
			client.Transport = &requestModifier{
				RoundTripper: transport,
				headers:      headers,
				params:       params,
			}
		}
	}

	return client
}
