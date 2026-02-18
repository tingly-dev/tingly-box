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

// requestModifier wraps an http.RoundTripper to apply hooks to each request:
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
	if req.URL.Host != "chatgpt.com" {
		return nil
	}

	originalPath := req.URL.Path
	newPath := rewriteCodexPath(originalPath)

	if newPath != originalPath {
		logrus.Debugf("[Codex] Rewriting URL path: %s -> %s", originalPath, newPath)
		req.URL.Path = newPath
	}

	req.Header.Set("OpenAI-Beta", "responses=experimental")
	req.Header.Set("originator", "tingly-box")

	if accountID := req.Header.Get("X-ChatGPT-Account-ID"); accountID != "" {
		req.Header.Set("ChatGPT-Account-ID", accountID)
		req.Header.Del("X-ChatGPT-Account-ID")
	}

	return nil
}

func rewriteCodexPath(path string) string {
	if strings.HasPrefix(path, "/backend-api/") {
		return rewriteBackendAPIPath(path)
	}
	if strings.HasPrefix(path, "/v1/") && !strings.Contains(path, "/codex/") {
		return strings.Replace(path, "/v1/", "/codex/", 1)
	}
	return path
}

func rewriteBackendAPIPath(path string) string {
	switch {
	case strings.HasPrefix(path, "/backend-api/chat/completions"):
		return "/backend-api/codex/responses"
	case path == "/backend-api/responses":
		return "/backend-api/codex/responses"
	case strings.HasPrefix(path, "/backend-api/v1/"):
		return strings.Replace(path, "/backend-api/v1/", "/backend-api/codex/", 1)
	default:
		return path
	}
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

// antigravityRoundTripper wraps an http.RoundTripper to handle Antigravity's
// custom request/response format
type antigravityRoundTripper struct {
	http.RoundTripper
	project, model string
	proxyURL       string
}

// isStreamingRequest checks if the request is for streaming generate content
func isStreamingRequest(req *http.Request) bool {
	return strings.Contains(req.URL.Path, ":streamGenerateContent")
}

// streamingUnwrapReader wraps an io.Reader to unwrap Antigravity's streaming response format
// Antigravity wraps each SSE event's data in a "response" field, e.g.:
//
//	data: {"response": {...actual google response...}}
//
// This reader unwraps it to:
//
//	data: {...actual google response...}
type streamingUnwrapReader struct {
	reader io.ReadCloser
	buffer []byte
	err    error
}

func (r *streamingUnwrapReader) Read(p []byte) (n int, err error) {
	// Return any buffered data first
	if len(r.buffer) > 0 {
		n = copy(p, r.buffer)
		r.buffer = r.buffer[n:]
		return n, nil
	}

	// Return previous error if any
	if r.err != nil {
		return 0, r.err
	}

	// Read data line by line from source
	buf := make([]byte, 4096)
	var lineBuffer bytes.Buffer

	for {
		nn, readErr := r.reader.Read(buf)
		if nn > 0 {
			lineBuffer.Write(buf[:nn])
		}
		if readErr != nil {
			if readErr == io.EOF {
				// Flush remaining buffer at EOF
				if lineBuffer.Len() > 0 {
					r.buffer = r.processBuffer(lineBuffer.Bytes())
					n = copy(p, r.buffer)
					r.buffer = r.buffer[n:]
					if len(r.buffer) == 0 {
						r.err = io.EOF
					}
					return n, nil
				}
				return 0, io.EOF
			}
			r.err = readErr
			return 0, readErr
		}

		// Process the buffer to extract complete lines
		processed := r.processBuffer(lineBuffer.Bytes())
		if len(processed) > 0 {
			// Return processed data
			n = copy(p, processed)
			r.buffer = processed[n:]
			return n, nil
		}

		// Check if we have enough data to work with
		if lineBuffer.Len() > 40960 {
			// Buffer too large, return as-is (fallback)
			n = copy(p, lineBuffer.Bytes())
			lineBuffer.Next(n)
			return n, nil
		}
	}
}

func (r *streamingUnwrapReader) processBuffer(data []byte) []byte {
	// Process SSE format, unwrapping "response" field in data lines
	lines := bytes.Split(data, []byte("\n"))
	var result bytes.Buffer

	for i, line := range lines {
		if bytes.HasPrefix(line, []byte("data:")) {
			// Extract JSON from data line
			jsonData := bytes.TrimSpace(line[5:]) // Skip "data:"
			if len(jsonData) == 0 {
				result.Write(line)
			} else {
				// Try to unwrap the JSON
				var wrapped map[string]any
				if err := json.Unmarshal(jsonData, &wrapped); err == nil {
					if innerResponse, ok := wrapped["response"]; ok {
						// Unwrap: extract inner "response" field
						unwrapped, err := json.Marshal(innerResponse)
						if err == nil {
							result.Write([]byte("data: "))
							result.Write(unwrapped)
						} else {
							// Failed to marshal, keep original
							result.Write(line)
						}
					} else {
						// No "response" field, keep original
						result.Write(line)
					}
				} else {
					// Not valid JSON, keep original
					result.Write(line)
				}
			}
		} else {
			result.Write(line)
		}

		// Add newline back (except for last line if original didn't end with \n)
		if i < len(lines)-1 || data[len(data)-1] == '\n' {
			result.WriteByte('\n')
		}
	}

	return result.Bytes()
}

func (r *streamingUnwrapReader) Close() error {
	return r.reader.Close()
}

func (t *antigravityRoundTripper) RoundTrip(req *http.Request) (*http.Response, error) {
	key := req.Header.Get("X-Goog-Api-Key")
	isStreaming := isStreamingRequest(req)

	// Rewrite URL path from standard Google format to Antigravity format
	originalPath := req.URL.Path
	newPath := originalPath
	model := ""

	if strings.Contains(newPath, ":generateContent") || strings.Contains(newPath, ":streamGenerateContent") {
		parts := strings.Split(newPath, ":")
		if len(parts) >= 2 {
			subparts := strings.Split(parts[0], "/")
			model = subparts[len(subparts)-1]
			operation := parts[1]
			newPath = fmt.Sprintf("/v1internal:%s", operation)
		}
	}

	if newPath != originalPath {
		logrus.Debugf("[Antigravity] Rewriting URL path: %s -> %s", originalPath, newPath)
		req.URL.Path = newPath
	}

	// Read and wrap request body
	if req.Body != nil && t.project != "" && model != "" {
		body, err := io.ReadAll(req.Body)
		if err != nil {
			return nil, fmt.Errorf("failed to read request body: %w", err)
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
				"project":     t.project,
				"requestId":   fmt.Sprintf("agent-%s", uuid.New().String()),
				"request":     cleanBody,
				"model":       model,
				"userAgent":   "antigravity",
				"requestType": "agent",
			}

			wrappedBody, err := json.Marshal(wrapped)
			if err != nil {
				return nil, fmt.Errorf("failed to marshal wrapped body: %w", err)
			}
			// Set GetBody to allow retries and redirects
			req.GetBody = func() (io.ReadCloser, error) {
				return io.NopCloser(bytes.NewReader(wrappedBody)), nil
			}
			req.Body = io.NopCloser(bytes.NewReader(wrappedBody))
			req.ContentLength = int64(len(wrappedBody))
		}
	}

	// Set headers
	req.Header = http.Header{}
	req.Header.Set("User-Agent", "antigravity/1.11.5 windows/amd64")
	req.Header.Set("Content-Type", "application/json")
	if req.ContentLength > 0 {
		req.Header.Set("Content-Length", fmt.Sprintf("%d", req.ContentLength))
	}
	if key != "" {
		req.Header.Set("Authorization", fmt.Sprintf("Bearer %s", key))
	}

	logrus.Debugf("[Antigravity] Sending request to %s, Content-Length=%d, isStreaming=%v", req.URL.Path, req.ContentLength, isStreaming)

	// Send request
	resp, err := t.RoundTripper.RoundTrip(req)
	if err != nil {
		logrus.Errorf("[Antigravity] Request failed: %v", err)
		return nil, err
	}

	logrus.Debugf("[Antigravity] Response received, status=%d", resp.StatusCode)

	// Unwrap response body
	// Antigravity wraps the response in a "response" field
	// For streaming: we need to unwrap each SSE event's data field
	// For non-streaming: we can unwrap the entire JSON response
	if resp.Body != nil {
		if isStreaming {
			// For streaming responses, wrap the body with streamingUnwrapReader
			// to unwrap each SSE event's data field on-the-fly
			resp.Body = &streamingUnwrapReader{reader: resp.Body}
		} else {
			// For non-streaming responses, read and unwrap the entire JSON body
			body, err := io.ReadAll(resp.Body)
			resp.Body.Close()
			if err != nil {
				return nil, fmt.Errorf("failed to read response body: %w", err)
			}

			// Try to unwrap the response
			var wrappedResponse map[string]any
			if err := json.Unmarshal(body, &wrappedResponse); err == nil {
				if innerResponse, ok := wrappedResponse["response"]; ok {
					// Unwrap: extract the inner "response" field
					unwrappedBody, err := json.Marshal(innerResponse)
					if err != nil {
						return nil, fmt.Errorf("failed to marshal unwrapped response: %w", err)
					}
					resp.Body = io.NopCloser(bytes.NewReader(unwrappedBody))
					resp.ContentLength = int64(len(unwrappedBody))
				}
			} else {
				// If parsing fails, return original body (might not be wrapped)
				resp.Body = io.NopCloser(bytes.NewReader(body))
				resp.ContentLength = int64(len(body))
			}
		}
	}

	return resp, nil
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
		// For Antigravity, create a specialized RoundTripper with provider-specific config
		if providerType == oauth.ProviderAntigravity && provider.OAuthDetail != nil {
			project, model := "", ""
			if provider.OAuthDetail.ExtraFields != nil {
				if p, ok := provider.OAuthDetail.ExtraFields["project_id"].(string); ok {
					project = p
				}
			}
			// Create a separate transport with proxy for Antigravity
			var antigravityTransport http.RoundTripper = transport
			if provider.ProxyURL != "" {
				// Use CreateHTTPClientWithProxy to create a transport with proxy
				proxyClient := CreateHTTPClientWithProxy(provider.ProxyURL)
				if proxyClient.Transport != nil {
					antigravityTransport = proxyClient.Transport
				}
			}

			// Use antigravityRoundTripper for both request wrapping and response unwrapping
			client.Transport = &antigravityRoundTripper{
				RoundTripper: antigravityTransport,
				project:      project,
				model:        model,
				proxyURL:     provider.ProxyURL,
			}
			logrus.Infof("Created Antigravity RoundTripper with project=%s, model=%s, proxy=%s", project, model, provider.ProxyURL)
		} else {
			// For other OAuth providers, use the hook-based approach
			hook := GetOAuthHook(providerType)
			if hook != nil {
				client.Transport = &requestModifier{
					RoundTripper: transport,
					hooks:        []HookFunc{hook},
				}
			}
		}
	}

	return client
}
