package client

import (
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// mockTransport is a minimal http.RoundTripper for testing.
type mockTransport struct {
	roundTrip func(req *http.Request) (*http.Response, error)
}

func (m *mockTransport) RoundTrip(req *http.Request) (*http.Response, error) {
	return m.roundTrip(req)
}

func newTestRoundTripper() *claudeRoundTripper {
	return &claudeRoundTripper{
		RoundTripper: &mockTransport{
			roundTrip: func(req *http.Request) (*http.Response, error) {
				return &http.Response{StatusCode: 200, Body: http.NoBody}, nil
			},
		},
	}
}

func TestApplyClaudeCodeHeaders_VersionAndBeta(t *testing.T) {
	rt := newTestRoundTripper()

	req := httptest.NewRequest(http.MethodPost, "https://api.anthropic.com/v1/messages", strings.NewReader(`{}`))
	req.Header.Set("X-Api-Key", "sk-ant-test-key")

	_, _ = rt.RoundTrip(req)

	// Version
	assert.Equal(t, claudeCLIUserAgent, req.Header.Get("user-agent"))
	assert.Contains(t, claudeCLIUserAgent, "2.1.86")

	// Stainless
	assert.Equal(t, stainlessRuntimeVersion, req.Header.Get("x-stainless-runtime-version"))
	assert.Equal(t, "v24.3.0", stainlessRuntimeVersion)

	// Beta flags (no model → no context-1m)
	beta := req.Header.Get("anthropic-beta")
	assert.Contains(t, beta, "claude-code-20250219")
	assert.Contains(t, beta, "effort-2025-11-24")
	assert.True(t, strings.HasSuffix(beta, "oauth-2025-04-20"), "beta should end with oauth: %s", beta)
	assert.NotContains(t, beta, "context-1m-2025-08-07", "no model → no context-1m")
}

func TestApplyClaudeCodeHeaders_SessionID(t *testing.T) {
	t.Run("session_id from json metadata", func(t *testing.T) {
		rt := newTestRoundTripper()

		body := `{"metadata":{"user_id":"{\"device_id\":\"abc\",\"account_uuid\":\"def\",\"session_id\":\"550e8400-e29b-41d4-a716-446655440000\"}"}}`
		req := httptest.NewRequest(http.MethodPost, "https://api.anthropic.com/v1/messages", strings.NewReader(body))
		req.Header.Set("X-Api-Key", "sk-ant-test-key")

		resp, _ := rt.RoundTrip(req)
		assert.Equal(t, 200, resp.StatusCode)
		assert.Equal(t, "550e8400-e29b-41d4-a716-446655440000", req.Header.Get("X-Claude-Code-Session-Id"))
	})

	t.Run("session_id from legacy metadata", func(t *testing.T) {
		rt := newTestRoundTripper()

		body := `{"metadata":{"user_id":"user_0000000000000000000000000000000000000000000000000000000000000064_account_def-00000000-0000-0000-0000-000000000001_session_550e8400-e29b-41d4-a716-446655440000"}}`
		req := httptest.NewRequest(http.MethodPost, "https://api.anthropic.com/v1/messages", strings.NewReader(body))
		req.Header.Set("X-Api-Key", "sk-ant-test-key")

		resp, _ := rt.RoundTrip(req)
		assert.Equal(t, 200, resp.StatusCode)
		assert.Equal(t, "550e8400-e29b-41d4-a716-446655440000", req.Header.Get("X-Claude-Code-Session-Id"))
	})

	t.Run("no metadata", func(t *testing.T) {
		rt := newTestRoundTripper()

		req := httptest.NewRequest(http.MethodPost, "https://api.anthropic.com/v1/messages", strings.NewReader(`{}`))
		req.Header.Set("X-Api-Key", "sk-ant-test-key")

		resp, _ := rt.RoundTrip(req)
		assert.Equal(t, 200, resp.StatusCode)
		assert.Empty(t, req.Header.Get("X-Claude-Code-Session-Id"))
	})
}

func TestApplyClaudeCodeHeaders_Context1M_ModelDependent(t *testing.T) {
	tests := []struct {
		name   string
		body   string
		want1m bool
	}{
		{"sonnet_4_6 gets context-1m", `{"model":"claude-sonnet-4-6","max_tokens":1024}`, true},
		{"sonnet_4 gets context-1m", `{"model":"claude-sonnet-4-20250514","max_tokens":1024}`, true},
		{"opus_4_6 gets context-1m", `{"model":"claude-opus-4-6","max_tokens":1024}`, true},
		{"opus_4 gets context-1m", `{"model":"claude-opus-4-20250514","max_tokens":1024}`, true},
		{"haiku no context-1m", `{"model":"claude-3-5-haiku-20241022","max_tokens":1024}`, false},
		{"haiku_4 no context-1m", `{"model":"claude-haiku-4-5-20250115","max_tokens":1024}`, false},
		{"empty body no context-1m", `{}`, false},
		{"no model field no context-1m", `{"max_tokens":1024}`, false},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			rt := newTestRoundTripper()
			req := httptest.NewRequest(http.MethodPost, "https://api.anthropic.com/v1/messages", strings.NewReader(tt.body))
			req.Header.Set("X-Api-Key", "sk-ant-test-key")

			_, _ = rt.RoundTrip(req)

			beta := req.Header.Get("anthropic-beta")
			if tt.want1m {
				assert.Contains(t, beta, "context-1m-2025-08-07", "model should get context-1m: %s", tt.name)
			} else {
				assert.NotContains(t, beta, "context-1m-2025-08-07", "model should NOT get context-1m: %s", tt.name)
			}
			// oauth always last
			assert.True(t, strings.HasSuffix(beta, "oauth-2025-04-20"), "beta should end with oauth: %s", beta)
		})
	}
}

func TestSupportsContext1M(t *testing.T) {
	tests := []struct {
		model string
		want  bool
	}{
		{"claude-sonnet-4-6", true},
		{"claude-sonnet-4-20250514", false},
		{"claude-opus-4-6", true},
		{"claude-opus-4-20250514", false},
		{"claude-3-5-haiku-20241022", false},
		{"claude-haiku-4-5-20250115", false},
		{"", false},
		{"some-other-model", false},
	}
	for _, tt := range tests {
		t.Run(tt.model, func(t *testing.T) {
			assert.Equal(t, tt.want, supportsContext1M(tt.model))
		})
	}
}

func TestExtractModelFromBody(t *testing.T) {
	tests := []struct {
		name string
		body string
		want string
	}{
		{"standard", `{"model":"claude-sonnet-4-6","max_tokens":1024}`, "claude-sonnet-4-6"},
		{"model first", `{"model":"claude-opus-4-6"}`, "claude-opus-4-6"},
		{"empty body", ``, ""},
		{"invalid json", `not json`, ""},
		{"no model field", `{"max_tokens":1024}`, ""},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			assert.Equal(t, tt.want, extractModelFromBody([]byte(tt.body)))
		})
	}
}

func TestRejectModelsEndpoint(t *testing.T) {
	rt := newTestRoundTripper()

	t.Run("GET /v1/models returns 404", func(t *testing.T) {
		req := httptest.NewRequest(http.MethodGet, "https://api.anthropic.com/v1/models", nil)
		req.Header.Set("X-Api-Key", "sk-ant-oat-test")

		resp, err := rt.RoundTrip(req)
		require.NoError(t, err)
		assert.Equal(t, http.StatusNotFound, resp.StatusCode)

		body, _ := io.ReadAll(resp.Body)
		assert.Contains(t, string(body), "not supported for Claude Code")
	})

	t.Run("POST /v1/models not rejected", func(t *testing.T) {
		req := httptest.NewRequest(http.MethodPost, "https://api.anthropic.com/v1/models", strings.NewReader(`{}`))
		req.Header.Set("X-Api-Key", "sk-ant-test-key")

		resp, err := rt.RoundTrip(req)
		require.NoError(t, err)
		assert.Equal(t, 200, resp.StatusCode)
	})

	t.Run("GET /v1/messages not rejected", func(t *testing.T) {
		req := httptest.NewRequest(http.MethodGet, "https://api.anthropic.com/v1/messages", nil)
		req.Header.Set("X-Api-Key", "sk-ant-test-key")

		resp, err := rt.RoundTrip(req)
		require.NoError(t, err)
		assert.Equal(t, 200, resp.StatusCode)
	})
}

func TestExtractSessionIDFromBody(t *testing.T) {
	tests := []struct {
		name string
		body string
		want string
	}{
		{
			"json format",
			`{"metadata":{"user_id":"{\"device_id\":\"abc\",\"account_uuid\":\"def\",\"session_id\":\"550e8400-e29b-41d4-a716-446655440000\"}"}}`,
			"550e8400-e29b-41d4-a716-446655440000",
		},
		{
			"json format with session_id only",
			`{"metadata":{"user_id":"{\"session_id\":\"aaaa0000-bbbb-cccc-dddd-eeeeeeeeeeee\"}"}}`,
			"aaaa0000-bbbb-cccc-dddd-eeeeeeeeeeee",
		},
		{
			"legacy format",
			`{"metadata":{"user_id":"user_0000000000000000000000000000000000000000000000000000000000000064_account_def-00000000-0000-0000-0000-000000000001_session_550e8400-e29b-41d4-a716-446655440000"}}`,
			"550e8400-e29b-41d4-a716-446655440000",
		},
		{
			"no metadata field",
			`{"model":"claude-sonnet-4-6"}`,
			"",
		},
		{
			"empty user_id",
			`{"metadata":{"user_id":""}}`,
			"",
		},
		{
			"empty body",
			``,
			"",
		},
		{
			"invalid user_id format",
			`{"metadata":{"user_id":"not-valid"}}`,
			"",
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			assert.Equal(t, tt.want, extractSessionIDFromBody([]byte(tt.body)))
		})
	}
}
