package client

import (
	"bytes"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"strings"

	"github.com/google/uuid"
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

func antigravityHook(req *http.Request) error {
	key := req.Header.Get("X-Goog-Api-Key")

	// Rewrite URL path from standard Google format to Antigravity format
	// Standard: /v1beta/models/{model}:generateContent
	// Antigravity: /v1internal:generateContent
	originalPath := req.URL.Path
	newPath := originalPath

	// Check if this is a generateContent request
	if strings.Contains(newPath, ":generateContent") {
		// Extract the operation name (generateContent, streamGenerateContent, etc.)
		parts := strings.Split(newPath, ":")
		if len(parts) >= 2 {
			operation := parts[1]
			// Rewrite to Antigravity format
			newPath = fmt.Sprintf("/v1internal:%s", operation)
		}
	}

	// Apply the path rewrite if changed
	if newPath != originalPath {
		logrus.Debugf("[Antigravity] Rewriting URL path: %s -> %s", originalPath, newPath)
		req.URL.Path = newPath
	}

	// Set headers (will be applied after URL rewrite)
	req.Header = http.Header{}
	req.Header.Set("User-Agent", "antigravity/1.11.5 windows/amd64")
	req.Header.Set("Content-Type", "application/json")
	if key != "" {
		req.Header.Set("Authorization", fmt.Sprintf("Bearer %s", key))
	}
	return nil
}

// newAntigravityHookWithConfig creates an Antigravity hook with provider-specific configuration
// This hook handles both URL rewriting and request body wrapping
func newAntigravityHookWithConfig(project, model string) HookFunc {
	return func(req *http.Request) error {
		key := req.Header.Get("X-Goog-Api-Key")

		// Rewrite URL path from standard Google format to Antigravity format
		originalPath := req.URL.Path
		newPath := originalPath

		if strings.Contains(newPath, ":generateContent") {
			parts := strings.Split(newPath, ":")
			if len(parts) >= 2 {
				operation := parts[1]
				newPath = fmt.Sprintf("/v1internal:%s", operation)
			}
		}

		if newPath != originalPath {
			logrus.Debugf("[Antigravity] Rewriting URL path: %s -> %s", originalPath, newPath)
			req.URL.Path = newPath
		}

		// Read and wrap request body
		if req.Body != nil && project != "" && model != "" {
			body, err := io.ReadAll(req.Body)
			if err != nil {
				return fmt.Errorf("failed to read request body: %w", err)
			}
			req.Body.Close()

			// Parse original body
			var originalBody map[string]any
			if err := json.Unmarshal(body, &originalBody); err == nil {
				// Remove model from original body
				cleanBody := make(map[string]any)
				for k, v := range originalBody {
					if k != "model" {
						cleanBody[k] = v
					}
				}

				// Wrap in Antigravity format
				wrapped := map[string]any{
					"project":     project,
					"requestId":   fmt.Sprintf("agent-%s", uuid.New().String()),
					"request":     cleanBody,
					"model":       model,
					"userAgent":   "antigravity",
					"requestType": "agent",
				}

				wrappedBody, err := json.Marshal(wrapped)
				if err != nil {
					return fmt.Errorf("failed to marshal wrapped body: %w", err)
				}
				req.Body = io.NopCloser(bytes.NewReader(wrappedBody))
				req.ContentLength = int64(len(wrappedBody))
			}
		}

		// Set headers
		req.Header = http.Header{}
		req.Header.Set("User-Agent", "antigravity/1.11.5 windows/amd64")
		req.Header.Set("Content-Type", "application/json")
		if key != "" {
			req.Header.Set("Authorization", fmt.Sprintf("Bearer %s", key))
		}
		return nil
	}
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
// Returns a configured http.Client
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

	if provider.AuthType == typ.AuthTypeOAuth {
		var hook HookFunc

		// For Antigravity, create a specialized hook with provider-specific config
		if providerType == oauth.ProviderAntigravity && provider.OAuthDetail != nil {
			project, model := "", ""
			if provider.OAuthDetail.ExtraFields != nil {
				if p, ok := provider.OAuthDetail.ExtraFields["project"].(string); ok {
					project = p
				}
				if m, ok := provider.OAuthDetail.ExtraFields["model"].(string); ok {
					model = m
				}
			}
			hook = newAntigravityHookWithConfig(project, model)
			logrus.Infof("Created Antigravity hook with project=%s, model=%s", project, model)
		} else {
			hook = GetOAuthHook(providerType)
		}

		if hook != nil {
			// Wrap the transport with request modifier for OAuth hooks
			client.Transport = &requestModifier{
				RoundTripper: transport,
				hooks:        []HookFunc{hook},
			}
		}
	}

	return client
}
