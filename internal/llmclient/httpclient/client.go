package httpclient

import (
	"fmt"
	"net/http"
	"net/url"

	oauth2 "tingly-box/pkg/oauth"

	"github.com/sirupsen/logrus"
	"golang.org/x/net/proxy"
)

// HookFunc is a function that can modify the request before it's sent
type HookFunc func(req *http.Request) error

// oauthHookFunctions defines custom hooks for OAuth providers based on provider type
// Each hook handles custom headers, query params, and any special request modifications
var oauthHookFunctions = map[oauth2.ProviderType]HookFunc{
	oauth2.ProviderClaudeCode: claudeCodeHook,
}

// claudeCodeHook applies Claude Code OAuth specific request modifications:
// - Converts X-Api-Key header to Authorization header
// - Adds required Claude Code specific headers
// - Adds beta query parameter
func claudeCodeHook(req *http.Request) error {
	// Convert X-Api-Key to Authorization header
	key := req.Header.Get("X-Api-Key")
	if key != "" {
		req.Header.Set("Authorization", fmt.Sprintf("Bearer %s", key))
		req.Header.Del("X-Api-Key")
	}

	// Set Claude Code specific headers
	req.Header.Set("accept", "application/json")
	req.Header.Set("anthropic-beta", "claude-code-20250219,oauth-2025-04-20,interleaved-thinking-2025-05-14")
	req.Header.Set("anthropic-dangerous-direct-browser-access", "true")
	req.Header.Set("anthropic-version", "2023-06-01")
	req.Header.Set("user-agent", "claude-cli/2.0.76 (external, cli)")
	req.Header.Set("x-app", "cli")
	req.Header.Set("x-stainless-helper-method", "stream")
	req.Header.Set("x-stainless-retry-count", "0")
	req.Header.Set("x-stainless-runtime-version", "v25.2.1")
	req.Header.Set("x-stainless-package-version", "0.70.0")
	req.Header.Set("x-stainless-runtime", "node")
	req.Header.Set("x-stainless-lang", "js")
	req.Header.Set("x-stainless-arch", "arm64")
	req.Header.Set("x-stainless-os", "MacOS")
	req.Header.Set("x-stainless-timeout", "3000")

	// Add beta query parameter if not already present
	q := req.URL.Query()
	if !q.Has("beta") {
		q.Add("beta", "true")
		req.URL.RawQuery = q.Encode()
	}

	return nil
}

// requestModifier wraps an http.RoundTripper to apply hooks to each request
type requestModifier struct {
	http.RoundTripper
	hooks []HookFunc
}

func (t *requestModifier) RoundTrip(req *http.Request) (*http.Response, error) {
	// Execute hooks in order
	for _, hook := range t.hooks {
		if err := hook(req); err != nil {
			return nil, err
		}
	}
	return t.RoundTripper.RoundTrip(req)
}

// GetOAuthHook returns the hook function for the given provider type
func GetOAuthHook(providerType oauth2.ProviderType) HookFunc {
	hook, ok := oauthHookFunctions[providerType]
	if !ok {
		return nil
	}
	return hook
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
// It handles proxy and OAuth hooks if applicable
//
// providerType: the OAuth provider type (e.g., oauth2.ProviderClaudeCode)
// proxyURL: optional proxy URL (can be empty)
// isOAuth: whether this is an OAuth provider
//
// Returns a configured http.Client
func CreateHTTPClientForProvider(providerType oauth2.ProviderType, proxyURL string, isOAuth bool) *http.Client {
	client := CreateHTTPClientWithProxy(proxyURL)

	if isOAuth {
		hook := GetOAuthHook(providerType)

		if hook != nil {
			// Use the client's transport, or default transport if nil (http.DefaultClient has nil Transport)
			transport := client.Transport
			if transport == nil {
				transport = http.DefaultTransport
			}

			client.Transport = &requestModifier{
				RoundTripper: transport,
				hooks:        []HookFunc{hook},
			}
		}
	}

	return client
}
