package client

import (
	"bufio"
	"bytes"
	"fmt"
	"io"
	"net/http"
	"runtime"
	"strings"

	"github.com/tidwall/gjson"

	"github.com/tingly-dev/tingly-box/internal/protocol/transform/ops"
)

const (
	// Claude Code client identification
	claudeCLIUserAgent      = "claude-cli/2.1.86 (external, cli)"
	claudeXApp              = "cli"
	stainlessHelperMethod   = "stream"
	stainlessRetryCount     = "0"
	stainlessRuntimeVersion = "v24.3.0"
	stainlessPackageVersion = "0.74.0"
	stainlessRuntime        = "node"
	stainlessLang           = "js"
	stainlessTimeout        = "3000"

	// Anthropic API headers
	anthropicBeta                         = "claude-code-20250219,oauth-2025-04-20,interleaved-thinking-2025-05-14,context-management-2025-06-27,prompt-caching-scope-2026-01-05,effort-2025-11-24,"
	anthropicOAuthBeta                    = "oauth-2025-04-20"
	anthropicDangerousDirectBrowserAccess = "true"
	anthropicVersion                      = "2023-06-01"

	// Model-specific beta flags
	anthropicContext1m = "context-1m-2025-08-07"

	// Content negotiation
	acceptHeader = "application/json"

	// Buffer sizes
	maxStreamingLineSize = 52_428_800 // 50MB max line size
)

// stainlessOS returns the OS name for the x-stainless-os header
func stainlessOS() string {
	return runtime.GOOS // e.g., "darwin", "linux", "windows"
}

// stainlessArch returns the architecture for the x-stainless-arch header
func stainlessArch() string {
	return runtime.GOARCH // e.g., "amd64", "arm64"
}

// claudeModelPrefixes that support context-1m beta flag.
var context1mModelPrefixes = []string{
	"claude-sonnet-4-6",
	"claude-opus-4-6",
}

// supportsContext1M checks if the model supports the context-1m-2025-08-07 beta flag.
func supportsContext1M(model string) bool {
	m := strings.ToLower(model)
	for _, prefix := range context1mModelPrefixes {
		if strings.HasPrefix(m, prefix) {
			return true
		}
	}
	return false
}

// extractSessionIDFromBody extracts the session_id from the request body's
// metadata.user_id field. The user_id field has two variants:
//   - JSON: {"device_id":"...","account_uuid":"...","session_id":"..."}
//   - Legacy string: user_{64hex}_account_{uuid}_session_{uuid}
func extractSessionIDFromBody(body []byte) string {
	if len(body) == 0 {
		return ""
	}
	raw := gjson.GetBytes(body, "metadata.user_id").String()
	if raw == "" {
		return ""
	}
	m := ops.ParseMetadataUserID(raw)
	if m == nil {
		return ""
	}
	return m.SessionID
}

// extractModelFromBody parses the "model" field from JSON body without full unmarshal.
func extractModelFromBody(body []byte) string {
	return gjson.GetBytes(body, "model").String()
}

// claudeRoundTripper wraps an http.RoundTripper to handle Claude Code OAuth
// specific request/response transformations:
// - Applies tool prefix to request body for OAuth tokens
// - Strips tool prefix from response (streaming and non-streaming)
// - Sets Claude Code specific headers
// - Manages conditional Authorization vs x-api-key header
type claudeRoundTripper struct {
	http.RoundTripper
}

func (t *claudeRoundTripper) RoundTrip(req *http.Request) (*http.Response, error) {
	// Reject /models endpoint for Claude Code OAuth (by design)
	if req.URL != nil && strings.HasSuffix(req.URL.Path, "/models") && req.Method == http.MethodGet {
		return &http.Response{
			StatusCode: http.StatusNotFound,
			Status:     http.StatusText(http.StatusNotFound),
			Header:     make(http.Header),
			Body:       io.NopCloser(bytes.NewReader([]byte(`{"error":{"type":"not_found_error","message":"models endpoint is not supported for Claude Code"}}`))),
		}, nil
	}

	// claudeHook applies Claude Code OAuth specific request modifications:
	// - Detects OAuth token (sk-ant-oat prefix)
	// - Applies tool prefix to request body for OAuth tokens
	// - Sets Claude Code specific headers with conditional auth
	// - Adds beta query parameter

	// Extract and read request body for potential modification
	var originalBody []byte
	var modifiedBody []byte
	var isOAuthToken bool

	if req.Body != nil && req.Method == "POST" {
		var err error
		originalBody, err = io.ReadAll(req.Body)
		_ = req.Body.Close()
		if err != nil {
			return nil, fmt.Errorf("failed to read request body: %w", err)
		}

		modifiedBody = originalBody

		// Check if this is an OAuth token
		key := req.Header.Get("X-Api-Key")
		if key != "" {
			isOAuthToken = IsClaudeOAuthToken(key)

			// Apply tool prefix for OAuth tokens
			if isOAuthToken {
				modifiedBody = ApplyClaudeToolPrefix(originalBody, ClaudeToolPrefix)
			}
		}

		// Trim capacity to length to avoid excessive memory usage
		modifiedBody = append([]byte(nil), modifiedBody...)
		// Set GetBody to allow retries and redirects
		req.Body = io.NopCloser(bytes.NewReader(modifiedBody))
		req.ContentLength = int64(len(modifiedBody))
		req.GetBody = func() (io.ReadCloser, error) {
			return io.NopCloser(bytes.NewReader(modifiedBody)), nil
		}
	}

	// Extract model and session ID from request body
	model := extractModelFromBody(originalBody)
	sessionID := extractSessionIDFromBody(originalBody)

	// Set Claude Code specific headers
	t.applyClaudeCodeHeaders(req, isOAuthToken, model, sessionID)

	// Add beta query parameter if not already present
	q := req.URL.Query()
	if !q.Has("beta") {
		q.Add("beta", "true")
		req.URL.RawQuery = q.Encode()
	}

	// Execute the request
	resp, err := t.RoundTripper.RoundTrip(req)
	if err != nil {
		return nil, err
	}

	// Wrap response body to strip tool prefix for OAuth tokens
	if isOAuthToken && resp.StatusCode == http.StatusOK {
		resp.Body = &claudeResponseWrapper{
			ReadCloser:  resp.Body,
			isStreaming: isStreamingResponse(resp),
			isOAuth:     true,
			toolPrefix:  ClaudeToolPrefix,
		}
	}

	return resp, nil
}

// applyClaudeCodeHeaders sets all Claude Code specific headers
func (t *claudeRoundTripper) applyClaudeCodeHeaders(req *http.Request, isOAuthToken bool, model string, sessionID string) {
	key := req.Header.Get("X-Api-Key")
	if key == "" {
		return
	}

	// Check if target is Anthropic's API
	isAnthropicBase := req.URL != nil && strings.Contains(strings.ToLower(req.URL.Host), "api.anthropic.com")

	if isAnthropicBase && !isOAuthToken {
		req.Header.Del("X-Api-Key")
		req.Header.Set("X-Api-Key", key)
	} else {
		req.Header.Del("X-Api-Key")
		req.Header.Set("Authorization", fmt.Sprintf("Bearer %s", key))
	}

	// Set Claude Code specific headers
	req.Header.Set("accept", acceptHeader)

	// Build beta header with all required flags
	baseBetas := anthropicBeta

	// Add context-1m for models that support it (Sonnet/Opus, not Haiku)
	if model != "" && supportsContext1M(model) {
		baseBetas = strings.TrimRight(baseBetas, ",") + "," + anthropicContext1m
	}
	baseBetas = strings.TrimRight(baseBetas, ",")

	// If user provides custom betas, merge them while ensuring oauth is included
	if val := strings.TrimSpace(req.Header.Get("Anthropic-Beta")); val != "" {
		baseBetas = val
		if !strings.Contains(val, "oauth") {
			baseBetas = fmt.Sprintf("%s,%s", baseBetas, anthropicOAuthBeta)
		}
	}

	req.Header.Set("anthropic-beta", baseBetas)
	req.Header.Set("anthropic-dangerous-direct-browser-access", anthropicDangerousDirectBrowserAccess)
	req.Header.Set("anthropic-version", anthropicVersion)
	req.Header.Set("user-agent", claudeCLIUserAgent)
	req.Header.Set("x-app", claudeXApp)
	req.Header.Set("x-stainless-helper-method", stainlessHelperMethod)
	req.Header.Set("x-stainless-retry-count", stainlessRetryCount)
	req.Header.Set("x-stainless-runtime-version", stainlessRuntimeVersion)
	req.Header.Set("x-stainless-package-version", stainlessPackageVersion)
	req.Header.Set("x-stainless-runtime", stainlessRuntime)
	req.Header.Set("x-stainless-lang", stainlessLang)
	req.Header.Set("x-stainless-arch", stainlessArch())
	req.Header.Set("x-stainless-os", stainlessOS())
	req.Header.Set("x-stainless-timeout", stainlessTimeout)
	if sessionID != "" {
		req.Header.Set("X-Claude-Code-Session-Id", sessionID)
	}
}

// isStreamingResponse checks if the response is a streaming SSE response
func isStreamingResponse(resp *http.Response) bool {
	contentType := resp.Header.Get("Content-Type")
	return strings.Contains(contentType, "text/event-stream") || strings.Contains(contentType, "application/x-ndjson")
}

// claudeResponseWrapper wraps response body to strip tool prefix from Claude Code OAuth responses
type claudeResponseWrapper struct {
	io.ReadCloser
	isStreaming bool
	isOAuth     bool
	toolPrefix  string
	buffer      []byte         // Processed data for non-streaming
	scanner     *bufio.Scanner // Scanner for streaming
}

// Read implements io.Reader for tool prefix stripping
func (w *claudeResponseWrapper) Read(p []byte) (n int, err error) {
	if !w.isOAuth || w.toolPrefix == "" {
		return w.ReadCloser.Read(p)
	}

	if w.isStreaming {
		return w.readStreaming(p)
	}
	return w.readNonStreaming(p)
}

// readStreaming handles streaming response (SSE format) using bufio.Scanner
func (w *claudeResponseWrapper) readStreaming(p []byte) (n int, err error) {
	// Initialize scanner on first use
	if w.scanner == nil {
		w.scanner = bufio.NewScanner(w.ReadCloser)
		w.scanner.Buffer(nil, maxStreamingLineSize)
	}

	// Scan next line
	if !w.scanner.Scan() {
		if err := w.scanner.Err(); err != nil {
			return 0, err
		}
		return 0, io.EOF
	}

	// Get line and strip tool prefix
	line := w.scanner.Bytes()
	stripped := StripClaudeToolPrefixFromStreamLine(line, w.toolPrefix)

	// Copy to output, preserving newline (scanner consumes \n, we add it back)
	n = copy(p, stripped)
	if n < len(p) {
		p[n] = '\n'
		n++
	}
	return n, nil
}

// readNonStreaming handles non-streaming response using io.ReadAll
func (w *claudeResponseWrapper) readNonStreaming(p []byte) (n int, err error) {
	// Return buffered data first
	if len(w.buffer) > 0 {
		n = copy(p, w.buffer)
		w.buffer = w.buffer[n:]
		if len(w.buffer) == 0 {
			w.buffer = nil
		}
		return n, nil
	}

	// Read entire response at once
	data, err := io.ReadAll(w.ReadCloser)
	if err != nil {
		return 0, err
	}

	// Strip tool prefix from complete response
	w.buffer = StripClaudeToolPrefixFromResponse(data, w.toolPrefix)

	// Return buffered data
	n = copy(p, w.buffer)
	w.buffer = w.buffer[n:]
	if len(w.buffer) == 0 {
		return n, io.EOF
	}
	return n, nil
}

// Close closes the underlying reader
func (w *claudeResponseWrapper) Close() error {
	return w.ReadCloser.Close()
}
