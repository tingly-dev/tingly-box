package client

import (
	"fmt"
	"net/http"
	"net/url"
	"strings"

	"github.com/sirupsen/logrus"
	"golang.org/x/net/proxy"

	"github.com/tingly-dev/tingly-box/internal/typ"
	"github.com/tingly-dev/tingly-box/pkg/oauth"
)

// HookFunc is a function that can modify the request before it's sent
type HookFunc func(req *http.Request) error

// oauthHookFunctions defines custom hooks for OAuth providers based on provider type
// Each hook handles custom headers, query params, and any special request modifications
var oauthHookFunctions = map[oauth.ProviderType]HookFunc{
	oauth.ProviderClaudeCode:  claudeCodeHook,
	oauth.ProviderAntigravity: antigravityHook,
	oauth.ProviderCodex:       codexHook,
}

// providerHookFunctions defines custom hooks for API key providers based on API base URL
// These hooks handle URL path rewriting and other provider-specific modifications
// for providers that use API key authentication (not OAuth)
var providerHookFunctions = map[string]HookFunc{
	"https://api.minimaxi.com/v1": minimaxHook,
	"https://api.minimax.io/v1":   minimaxHook,
}

func antigravityHook(req *http.Request) error {
	key := req.Header.Get("X-Goog-Api-Key")

	req.Header = http.Header{}
	req.Header.Set("User-Agent", "antigravity/1.11.3 Darwin/arm64")
	req.Header.Set("Content-Type", "application/json")
	if key != "" {
		req.Header.Set("Authorization", fmt.Sprintf("Bearer %s", key))
	}
	return nil
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

// codexHook applies ChatGPT/Codex OAuth specific request modifications:
// - Rewrites URL paths from /v1/... to /codex/... for ChatGPT backend API
// - Handles special cases for responses endpoint
// - Adds required ChatGPT backend API headers
// - Transforms X-ChatGPT-Account-ID to ChatGPT-Account-ID header
func codexHook(req *http.Request) error {
	// Only process requests to chatgpt.com
	if req.URL.Host != "chatgpt.com" {
		return nil
	}

	originalPath := req.URL.Path
	newPath := req.URL.Path

	// Pattern 1: Rewrite /backend-api/v1/... to /backend-api/codex/...
	// Example: /backend-api/v1/chat/completions → /backend-api/codex/chat/completions
	if strings.HasPrefix(newPath, "/backend-api/v1/") {
		newPath = strings.Replace(newPath, "/backend-api/v1/", "/backend-api/codex/", 1)
	}

	// Pattern 2: Rewrite /backend-api/responses to /backend-api/codex/responses
	// The Responses API may use a different URL structure than chat completions
	if newPath == "/backend-api/responses" {
		newPath = "/backend-api/codex/responses"
	}

	// Pattern 3: Rewrite /v1/... to /codex/... (if base URL doesn't include /backend-api)
	// Example: /v1/chat/completions → /codex/chat/completions
	if strings.HasPrefix(newPath, "/v1/") && !strings.Contains(newPath, "/codex/") {
		newPath = strings.Replace(newPath, "/v1/", "/codex/", 1)
	}

	// Apply the rewrite if the path was changed
	if newPath != originalPath {
		logrus.Debugf("[Codex] Rewriting URL path: %s -> %s", originalPath, newPath)
		req.URL.Path = newPath
	}

	// Add required ChatGPT backend API headers
	req.Header.Set("OpenAI-Beta", "responses=experimental")
	req.Header.Set("originator", "tingly-box")

	// Transform X-ChatGPT-Account-ID to ChatGPT-Account-ID if present
	// (The X- prefix header is set by the client setup via option.WithHeader)
	if accountID := req.Header.Get("X-ChatGPT-Account-ID"); accountID != "" {
		req.Header.Set("ChatGPT-Account-ID", accountID)
		req.Header.Del("X-ChatGPT-Account-ID")
	}

	return nil
}

// minimaxHook applies Minimax provider specific request modifications:
// - Rewrites URL paths from /chat/completions to /text/chatcompletion_v2
// Minimax API documentation: https://platform.minimaxi.com/docs/api-reference/text-chat
func minimaxHook(req *http.Request) error {
	// Only process requests to minimax domains
	if !strings.Contains(req.URL.Host, "minimax.") {
		return nil
	}

	originalPath := req.URL.Path

	// Pattern: Rewrite /chat/completions to /text/chatcompletion_v2
	// Example: https://api.minimaxi.com/v1/chat/completions → https://api.minimaxi.com/v1/text/chatcompletion_v2
	if strings.HasSuffix(originalPath, "/chat/completions") {
		newPath := strings.Replace(originalPath, "/chat/completions", "/text/chatcompletion_v2", 1)
		logrus.Debugf("[Minimax] Rewriting URL path: %s -> %s", originalPath, newPath)
		req.URL.Path = newPath
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
func GetOAuthHook(providerType oauth.ProviderType) HookFunc {
	hook, ok := oauthHookFunctions[providerType]
	if !ok {
		return nil
	}
	return hook
}

// GetProviderHook returns the hook function for the given provider ID
// Provider IDs are from provider_templates.json (e.g., "minimax", "minimax-intl")
func GetProviderHook(providerID string) HookFunc {
	hook, ok := providerHookFunctions[providerID]
	if !ok {
		return nil
	}
	return hook
}

// GetProviderHookByAPIBase returns the hook function for the given API base URL
// This matches the API base against known provider patterns and returns the appropriate hook
func GetProviderHookByAPIBase(apiBase string) HookFunc {
	// Normalize API base
	apiBase = strings.ToLower(strings.TrimSuffix(apiBase, "/"))

	// Match API base to provider ID
	for providerID, hook := range providerHookFunctions {
		if getProviderAPIBase(providerID) == apiBase {
			return hook
		}
	}

	return nil
}

// getProviderAPIBase returns the expected API base for a given provider ID
// This maps provider template IDs to their API base URLs
func getProviderAPIBase(providerID string) string {
	switch providerID {
	case "minimax":
		return "https://api.minimaxi.com/v1"
	case "minimax-intl":
		return "https://api.minimax.io/v1"
	default:
		return ""
	}
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
// Uses transport pool for connection reuse and applies OAuth/provider-specific hooks
//
// For OAuth providers: applies OAuth hooks (e.g., Claude Code headers)
// For API key providers: applies provider-specific hooks (e.g., Minimax URL rewriting)
func CreateHTTPClientForProvider(provider *typ.Provider) *http.Client {
	var providerType oauth.ProviderType
	if provider.OAuthDetail != nil {
		providerType = oauth.ProviderType(provider.OAuthDetail.ProviderType)
	}

	// Get shared transport from transport pool
	transport := GetGlobalTransportPool().GetTransport(provider.APIBase, provider.ProxyURL, providerType)

	client := &http.Client{
		Transport: transport,
	}

	var hook HookFunc

	// For OAuth providers, get OAuth hook
	if provider.AuthType == typ.AuthTypeOAuth {
		hook = GetOAuthHook(providerType)
	} else {
		// For API key providers, get provider-specific hook by API base
		hook = GetProviderHookByAPIBase(provider.APIBase)
	}

	// Apply the hook if found
	if hook != nil {
		client.Transport = &requestModifier{
			RoundTripper: transport,
			hooks:        []HookFunc{hook},
		}
	}

	return client
}
