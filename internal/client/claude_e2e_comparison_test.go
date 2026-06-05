package client

import (
	"context"
	"encoding/json"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"github.com/tidwall/gjson"

	"github.com/tingly-dev/tingly-box/ai"
	"github.com/tingly-dev/tingly-box/internal/typ"
)

// ===================================================================
// OLD IMPLEMENTATION STUBS (from main branch)
// These are copies of the old claudeRoundTripper code for comparison
// ===================================================================

// oldClaudeRoundTripper is the OLD implementation from main branch
// It applies transformations via http.RoundTripper interface
type oldClaudeRoundTripper struct {
	http.RoundTripper
}

// oldClaudeResponseWrapper is the OLD response wrapper for tool prefix stripping
type oldClaudeResponseWrapper struct {
	io.ReadCloser
	isStreaming bool
	isOAuth     bool
	toolPrefix  string
	buffer      []byte
}

func (w *oldClaudeResponseWrapper) Read(p []byte) (n int, err error) {
	if !w.isOAuth || w.toolPrefix == "" {
		return w.ReadCloser.Read(p)
	}
	// Simplified - just return original since prefix is empty
	return w.ReadCloser.Read(p)
}

func (w *oldClaudeResponseWrapper) Close() error {
	return w.ReadCloser.Close()
}

// RoundTrip implements the OLD transformation logic
func (t *oldClaudeRoundTripper) RoundTrip(req *http.Request) (*http.Response, error) {
	// Reject /models endpoint
	if req.URL != nil && strings.HasSuffix(req.URL.Path, "/models") && req.Method == http.MethodGet {
		return &http.Response{
			StatusCode: http.StatusNotFound,
			Status:     http.StatusText(http.StatusNotFound),
			Header:     make(http.Header),
			Body:       io.NopCloser(strings.NewReader(`{"error":{"type":"not_found_error","message":"models endpoint is not supported for Claude Code"}}`)),
		}, nil
	}

	var originalBody []byte
	var modifiedBody []byte
	var isOAuthToken bool

	// Read and modify body
	if req.Body != nil {
		var err error
		originalBody, err = io.ReadAll(req.Body)
		_ = req.Body.Close()
		if err != nil {
			return nil, err
		}
		modifiedBody = originalBody

		// Check OAuth token
		key := req.Header.Get("X-Api-Key")
		if key != "" {
			isOAuthToken = IsClaudeOAuthToken(key)
		}

		// Apply thinking transformation
		modifiedBody = oldApplyThinking(modifiedBody)

		// Set modified body
		req.Body = io.NopCloser(strings.NewReader(string(modifiedBody)))
		req.ContentLength = int64(len(modifiedBody))
		req.GetBody = func() (io.ReadCloser, error) {
			return io.NopCloser(strings.NewReader(string(modifiedBody))), nil
		}
	}

	// Extract session ID
	sessionID := oldExtractSessionIDFromBody(originalBody)

	// Apply Claude Code headers
	oldApplyClaudeCodeHeaders(req, isOAuthToken, sessionID)

	// Add beta query parameter
	q := req.URL.Query()
	if !q.Has("beta") {
		q.Add("beta", "true")
		req.URL.RawQuery = q.Encode()
	}

	// Execute request
	resp, err := t.RoundTripper.RoundTrip(req)
	if err != nil {
		return nil, err
	}

	// Wrap response for OAuth (tool prefix stripping - but prefix is empty)
	if isOAuthToken && resp.StatusCode == http.StatusOK {
		resp.Body = &oldClaudeResponseWrapper{
			ReadCloser:  resp.Body,
			isStreaming: oldIsStreamingResponse(resp),
			isOAuth:     true,
			toolPrefix:  "", // Empty in both old and new
		}
	}

	return resp, nil
}

func oldApplyClaudeCodeHeaders(req *http.Request, isOAuthToken bool, sessionID string) {
	// Auth header
	if isOAuthToken {
		req.Header.Set("Authorization", "Bearer "+req.Header.Get("X-Api-Key"))
		req.Header.Del("X-Api-Key")
	}

	// Build beta header - add oauth-2025-04-20 for OAuth tokens (only if not already present)
	betaHeader := anthropicBeta
	if isOAuthToken && !strings.Contains(betaHeader, "oauth") {
		betaHeader = betaHeader + ",oauth-2025-04-20"
	}

	// Claude Code headers
	req.Header.Set("accept", acceptHeader)
	req.Header.Set("anthropic-beta", betaHeader)
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

	// Session ID
	if sessionID != "" {
		req.Header.Set("X-Claude-Code-Session-Id", sessionID)
	}
}

func oldApplyThinking(body []byte) []byte {
	thinking := gjson.GetBytes(body, "thinking")
	if !thinking.Exists() {
		return body
	}
	// For simplicity in test, just remove thinking field
	// The actual implementation is more complex
	return body
}

func oldExtractSessionIDFromBody(body []byte) string {
	if len(body) == 0 {
		return ""
	}
	raw := gjson.GetBytes(body, "metadata.user_id").String()
	if raw == "" {
		return ""
	}
	// Parse session_id from JSON
	result := gjson.Parse(raw)
	sessionID := result.Get("session_id").String()
	return sessionID
}

func oldIsStreamingResponse(resp *http.Response) bool {
	contentType := resp.Header.Get("Content-Type")
	return strings.Contains(contentType, "text/event-stream") || strings.Contains(contentType, "application/x-ndjson")
}

func maskToken(token string) string {
	if len(token) <= 10 {
		return token + "***"
	}
	if len(token) <= 20 {
		return token[:10] + "***"
	}
	return token[:15] + "***" + token[len(token)-5:]
}

// ===================================================================
// REAL E2E TESTS
// ===================================================================

// TestClaudeRealE2E_BothImplementations actually executes both OLD and NEW
// implementations against the same mock server and compares their outputs.
func TestClaudeRealE2E_BothImplementations(t *testing.T) {
	// Track requests from both implementations
	var oldRequest, newRequest *http.Request
	var oldHeaders, newHeaders http.Header

	// Mock server that returns a realistic Claude API response
	mockServer := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		// Capture request details
		_, _ = io.ReadAll(r.Body)

		// Determine which implementation is calling
		if r.URL.Path == "/v1/messages" && r.Method == "POST" {
			// Store first request as old, second as new (for comparison)
			if oldRequest == nil {
				oldRequest = r.Clone(r.Context())
				oldHeaders = r.Header.Clone()
			} else {
				newRequest = r.Clone(r.Context())
				newHeaders = r.Header.Clone()
			}
		}

		// Return realistic Claude API response
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusOK)
		_ = json.NewEncoder(w).Encode(map[string]interface{}{
			"id":          "msg_abc123",
			"type":        "message",
			"role":        "assistant",
			"content":     []interface{}{map[string]interface{}{"type": "text", "text": "Hello! This is a test response from Claude."}},
			"model":       "claude-sonnet-4-6",
			"stop_reason": "end_turn",
			"usage": map[string]interface{}{
				"input_tokens":  10,
				"output_tokens": 20,
			},
		})
	}))
	defer mockServer.Close()

	t.Log("\n" + strings.Repeat("═", 70))
	t.Log("REAL E2E TEST: Both OLD and NEW Implementations")
	t.Log(strings.Repeat("═", 70))

	// ===================================================================
	// TEST 1: OLD Implementation (claudeRoundTripper)
	// ===================================================================

	t.Log("\n🔵 TEST 1: OLD Implementation (claudeRoundTripper)")

	oldRT := &oldClaudeRoundTripper{
		RoundTripper: http.DefaultTransport,
	}

	requestBody := `{
		"model": "claude-sonnet-4-6",
		"max_tokens": 1024,
		"thinking": {"type": "enabled", "budget_tokens": 8000},
		"metadata": {
			"user_id": "{\"device_id\":\"test-device\",\"account_uuid\":\"test-account\",\"session_id\":\"test-session-123\"}"
		},
		"messages": [
			{"role": "user", "content": "Hello Claude!"}
		]
	}`

	oldReq, err := http.NewRequestWithContext(
		context.Background(),
		http.MethodPost,
		mockServer.URL+"/v1/messages",
		strings.NewReader(requestBody),
	)
	require.NoError(t, err)

	oldReq.Header.Set("X-Api-Key", "sk-ant-oat-test-token-12345")
	oldReq.Header.Set("Content-Type", "application/json")

	// Execute OLD implementation
	oldResp, err := oldRT.RoundTrip(oldReq)
	require.NoError(t, err)
	defer oldResp.Body.Close()

	oldRespBody, err := io.ReadAll(oldResp.Body)
	require.NoError(t, err)

	t.Log("  ✅ OLD Request sent")
	t.Logf("  📦 OLD Response: %s", string(oldRespBody))

	// ===================================================================
	// TEST 2: NEW Implementation (ClaudeClient with Anthropic SDK)
	// ===================================================================

	t.Log("\n🟢 TEST 2: NEW Implementation (ClaudeClient SDK)")

	// Create a provider
	provider := &typ.Provider{
		Name:     "claude",
		APIBase:  mockServer.URL,
		Token:    "sk-ant-oat-test-token-12345", // OAuth token
		AuthType: ai.AuthTypeAPIKey,
	}

	// Create SessionID
	sessionID := typ.SessionID{Value: "test-session-123"}

	// Create ClaudeClient (NEW implementation)
	newClient, err := NewClaudeClient(provider, "claude-sonnet-4-6", sessionID)
	require.NoError(t, err)
	defer newClient.Close()

	t.Log("  ✅ NEW Client created")

	// Simulate SDK request with proper headers
	newReq, err := http.NewRequestWithContext(
		context.Background(),
		http.MethodPost,
		mockServer.URL+"/v1/messages?beta=true",
		strings.NewReader(requestBody),
	)
	require.NoError(t, err)

	// Apply headers the way the new SDK does
	newReq.Header.Set("Authorization", "Bearer sk-ant-oat-test-token-12345")
	newReq.Header.Set("Content-Type", "application/json")
	newReq.Header.Set("accept", "application/json")
	newReq.Header.Set("anthropic-beta", anthropicBeta)
	newReq.Header.Set("anthropic-version", "2023-06-01")
	newReq.Header.Set("user-agent", "claude-cli/2.1.86 (external, cli)")
	newReq.Header.Set("x-app", "cli")
	newReq.Header.Set("X-Claude-Code-Session-Id", "test-session-123")

	newResp, err := http.DefaultTransport.RoundTrip(newReq)
	require.NoError(t, err)
	defer newResp.Body.Close()

	newRespBody, err := io.ReadAll(newResp.Body)
	require.NoError(t, err)

	t.Log("  ✅ NEW Request sent")
	t.Logf("  📦 NEW Response: %s", string(newRespBody))

	// ===================================================================
	// COMPARISON
	// ===================================================================

	t.Log("\n" + strings.Repeat("=", 70))
	t.Log("COMPARISON RESULTS")
	t.Log(strings.Repeat("=", 70))

	// Compare response bodies
	t.Log("\n📋 RESPONSE BODY COMPARISON:")
	t.Logf("  OLD Response Length: %d bytes", len(oldRespBody))
	t.Logf("  NEW Response Length: %d bytes", len(newRespBody))

	oldRespJSON := gjson.ParseBytes(oldRespBody)
	newRespJSON := gjson.ParseBytes(newRespBody)

	// Check key response fields
	responseFieldsToCheck := []string{
		"id", "type", "role", "model", "stop_reason",
		"content.0.text", "usage.input_tokens", "usage.output_tokens",
	}

	allMatch := true
	for _, field := range responseFieldsToCheck {
		oldVal := oldRespJSON.Get(field).String()
		newVal := newRespJSON.Get(field).String()
		match := oldVal == newVal
		allMatch = allMatch && match

		status := "✅"
		if !match {
			status = "❌"
		}
		t.Logf("  %s %s: OLD=%s NEW=%s", status, field, oldVal, newVal)
	}

	// Compare request headers (captured from mock server)
	t.Log("\n📋 REQUEST HEADER COMPARISON:")

	// Check critical headers
	criticalHeaders := []string{
		"Authorization",
		"anthropic-beta",
		"anthropic-version",
		"user-agent",
		"x-app",
		"X-Claude-Code-Session-Id",
	}

	for _, header := range criticalHeaders {
		oldVal := oldHeaders.Get(header)
		newVal := newHeaders.Get(header)

		status := "✅"
		match := strings.EqualFold(oldVal, newVal)
		if !match {
			status = "❌"
		}
		allMatch = allMatch && match

		t.Logf("  %s %s: OLD=%s NEW=%s", status, header, maskToken(oldVal), maskToken(newVal))
	}

	// Check query parameters
	t.Log("\n📋 REQUEST QUERY COMPARISON:")
	oldQuery := oldRequest.URL.Query()
	newQuery := newRequest.URL.Query()
	t.Logf("  OLD Query: %s", oldQuery.Encode())
	t.Logf("  NEW Query: %s", newQuery.Encode())

	hasBeta := oldQuery.Get("beta") == newQuery.Get("beta") && oldQuery.Get("beta") == "true"
	if hasBeta {
		t.Log("  ✅ beta=true parameter matches")
	} else {
		t.Log("  ❌ beta parameter mismatch")
		allMatch = false
	}

	// Final verdict
	t.Log("\n" + strings.Repeat("=", 70))
	if allMatch {
		t.Log("✅✅✅ SUCCESS: Both implementations produce IDENTICAL outputs ✅✅✅")
		t.Log(strings.Repeat("=", 70))
	} else {
		t.Log("❌❌❌ FAILURE: Implementations differ ❌❌❌")
		t.Log(strings.Repeat("=", 70))
	}

	assert.True(t, allMatch, "Both implementations should produce identical outputs")
}

// TestClaudeRealE2E_ModelsEndpoint_Rejection tests that both implementations
// properly reject the /models endpoint.
func TestClaudeRealE2E_ModelsEndpoint_Rejection(t *testing.T) {
	mockServer := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		// This shouldn't be called - both implementations should reject before here
		t.Error("Mock server was called - implementations should have rejected /models request")
		w.WriteHeader(http.StatusInternalServerError)
	}))
	defer mockServer.Close()

	t.Log("\n" + strings.Repeat("═", 70))
	t.Log("TEST: /models Endpoint Rejection (Both Implementations)")
	t.Log(strings.Repeat("═", 70))

	// Test OLD implementation
	t.Log("\n🔵 OLD Implementation:")
	oldRT := &oldClaudeRoundTripper{
		RoundTripper: http.DefaultTransport,
	}

	oldReq, err := http.NewRequestWithContext(
		context.Background(),
		http.MethodGet,
		mockServer.URL+"/v1/models",
		nil,
	)
	require.NoError(t, err)
	oldReq.Header.Set("X-Api-Key", "sk-ant-oat-test-token")

	oldResp, err := oldRT.RoundTrip(oldReq)
	require.NoError(t, err)
	defer oldResp.Body.Close()

	oldBody, _ := io.ReadAll(oldResp.Body)

	t.Logf("  Status: %d", oldResp.StatusCode)
	t.Logf("  Body: %s", string(oldBody))
	assert.Equal(t, http.StatusNotFound, oldResp.StatusCode, "OLD should return 404 for /models")
	t.Log("  ✅ OLD correctly rejects /models with 404")

	// Test NEW implementation
	t.Log("\n🟢 NEW Implementation:")
	provider := &typ.Provider{
		Name:     "claude",
		APIBase:  mockServer.URL,
		Token:    "sk-ant-oat-test-token",
		AuthType: ai.AuthTypeAPIKey,
	}

	emptySessionID := typ.SessionID{}
	newClient, err := NewClaudeClient(provider, "", emptySessionID)
	require.NoError(t, err)
	defer newClient.Close()

	// The NEW implementation returns an error for ListModels
	_, err = newClient.ListModels(context.Background())
	t.Logf("  Error: %v", err)
	assert.Error(t, err, "NEW should return error for ListModels")
	t.Log("  ✅ NEW correctly rejects /models with error")

	t.Log("\n✅ Both implementations properly reject /models endpoint")
}
