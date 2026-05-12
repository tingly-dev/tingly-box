package client

import (
	"net/http"
	"testing"

	"github.com/stretchr/testify/assert"
)

// mockTransport is a minimal http.RoundTripper for testing.
type mockTransport struct {
	roundTrip func(req *http.Request) (*http.Response, error)
}

func (m *mockTransport) RoundTrip(req *http.Request) (*http.Response, error) {
	return m.roundTrip(req)
}

// Test that the SDK middleware approach still applies the correct headers
func TestClaudeSDKHeaders(t *testing.T) {
	// Since headers are now applied via SDK options, we test that
	// the constants and helper functions work correctly
	assert.Equal(t, "claude-cli/2.1.86 (external, cli)", claudeCLIUserAgent)
	assert.Contains(t, claudeCLIUserAgent, "2.1.86")
	assert.Equal(t, "v24.3.0", stainlessRuntimeVersion)
	assert.Equal(t, "cli", claudeXApp)
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

func TestApplyThinking(t *testing.T) {
	tests := []struct {
		name        string
		input       string
		contains    string
		notContains string
	}{
		{
			"removes thinking field",
			`{"thinking":"test","model":"claude-sonnet-4-6"}`,
			"output_config",
			"thinking",
		},
		{
			"adds output_config with medium effort",
			`{"model":"claude-sonnet-4-6"}`,
			"output_config",
			"",
		},
		{
			"preserves other fields",
			`{"model":"claude-sonnet-4-6","max_tokens":1024}`,
			"max_tokens",
			"",
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := applyThinking([]byte(tt.input))
			resultStr := string(result)
			assert.Contains(t, resultStr, tt.contains)
			if tt.notContains != "" {
				assert.NotContains(t, resultStr, tt.notContains)
			}
		})
	}
}
