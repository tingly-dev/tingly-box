package client

import (
	"net/http"
	"testing"

	"github.com/anthropics/anthropic-sdk-go"
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
	assert.Equal(t, "claude-cli/2.1.86 (external, cli)", claudeCLIUserAgent)
	assert.Contains(t, claudeCLIUserAgent, "2.1.86")
	assert.Equal(t, "v24.3.0", stainlessRuntimeVersion)
	assert.Equal(t, "cli", claudeXApp)
	assert.Equal(t, "600", stainlessTimeout)
}

func TestAnthropicBetaFlags(t *testing.T) {
	for _, flag := range []string{
		"claude-code-20250219",
		"oauth-2025-04-20",
		"interleaved-thinking-2025-05-14",
		"structured-outputs-2025-12-15",
		"fast-mode-2026-02-01",
		"redact-thinking-2026-02-12",
		"token-efficient-tools-2026-03-28",
	} {
		assert.Contains(t, anthropicBeta, flag, "anthropicBeta should contain %s", flag)
	}
}

func TestMergeBetaFlags(t *testing.T) {
	tests := []struct {
		name     string
		required []string
		upstream []string
		oauth    string
		want     string
	}{
		{
			name:     "no upstream — required preserved, oauth dedupes",
			required: []string{"claude-code-20250219", "oauth-2025-04-20"},
			upstream: nil,
			oauth:    "oauth-2025-04-20",
			want:     "claude-code-20250219,oauth-2025-04-20",
		},
		{
			name:     "allowed upstream flag (context-1m) passes through",
			required: []string{"claude-code-20250219", "oauth-2025-04-20"},
			upstream: []string{"context-1m-2025-08-07,oauth-2025-04-20"},
			oauth:    "oauth-2025-04-20",
			want:     "claude-code-20250219,oauth-2025-04-20,context-1m-2025-08-07",
		},
		{
			name:     "multiple upstream header values — only allowlisted kept",
			required: []string{"claude-code-20250219"},
			upstream: []string{"context-1m-2025-08-07,pdfs-2024-09-25", "managed-agents-2026-04-01"},
			oauth:    "oauth-2025-04-20",
			want:     "claude-code-20250219,context-1m-2025-08-07,oauth-2025-04-20",
		},
		{
			name:     "appends oauth when missing",
			required: []string{"claude-code-20250219"},
			upstream: nil,
			oauth:    "oauth-2025-04-20",
			want:     "claude-code-20250219,oauth-2025-04-20",
		},
		{
			name:     "upstream duplicate of required is a no-op",
			required: []string{"claude-code-20250219", "oauth-2025-04-20"},
			upstream: []string{" claude-code-20250219 , "},
			oauth:    "oauth-2025-04-20",
			want:     "claude-code-20250219,oauth-2025-04-20",
		},
		{
			name:     "drops non-allowlisted SDK flags",
			required: []string{"claude-code-20250219"},
			upstream: []string{"message-batches-2024-09-24,mcp-client-2025-04-04,bad flag,oauth-2025-04-20"},
			oauth:    "oauth-2025-04-20",
			want:     "claude-code-20250219,oauth-2025-04-20",
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			assert.Equal(t, tt.want, mergeBetaFlags(tt.required, tt.upstream, tt.oauth))
		})
	}
}

func TestClassifyUpstreamBetaFlag(t *testing.T) {
	cases := []struct {
		flag       string
		wantKeep   bool
		wantReason string
	}{
		{"claude-code-20250219", true, ""},                  // required baseline
		{"context-1m-2025-08-07", true, ""},                 // allowed upstream addition
		{"message-batches-2024-09-24", false, "not-fingerprint-safe"},
		{"managed-agents-2026-04-01", false, "not-fingerprint-safe"},
		{"not-a-real-flag", false, "unknown"},
		{"bad flag", false, "unknown"},
		{"", false, "unknown"},
	}
	for _, c := range cases {
		t.Run(c.flag, func(t *testing.T) {
			keep, reason := classifyUpstreamBetaFlag(c.flag)
			assert.Equal(t, c.wantKeep, keep)
			assert.Equal(t, c.wantReason, reason)
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

func TestRemapOAuthToolNames(t *testing.T) {
	tests := []struct {
		name            string
		input           string
		wantContains    string
		wantNotContains string
		wantReverse     map[string]string
	}{
		{
			name:            "renames bash to Bash in tools array",
			input:           `{"tools":[{"name":"bash","description":"run bash"}]}`,
			wantContains:    `"Bash"`,
			wantNotContains: `"bash"`,
			wantReverse:     map[string]string{"Bash": "bash"},
		},
		{
			name:            "renames glob to Glob in tools array",
			input:           `{"tools":[{"name":"glob","description":"find files"}]}`,
			wantContains:    `"Glob"`,
			wantNotContains: `"glob"`,
			wantReverse:     map[string]string{"Glob": "glob"},
		},
		{
			name:            "leaves already-TitleCase names unchanged",
			input:           `{"tools":[{"name":"Bash","description":"run bash"}]}`,
			wantContains:    `"Bash"`,
			wantNotContains: "",
			wantReverse:     map[string]string{},
		},
		{
			name:         "leaves built-in tools (with type field) unchanged",
			input:        `{"tools":[{"type":"computer_use","name":"bash"}]}`,
			wantContains: `"bash"`,
			wantReverse:  map[string]string{},
		},
		{
			name:            "renames tool_use in messages",
			input:           `{"messages":[{"role":"assistant","content":[{"type":"tool_use","name":"bash","id":"x"}]}]}`,
			wantContains:    `"Bash"`,
			wantNotContains: `"bash"`,
			wantReverse:     map[string]string{"Bash": "bash"},
		},
		{
			name:            "renames tool_choice.name",
			input:           `{"tool_choice":{"type":"tool","name":"bash"}}`,
			wantContains:    `"Bash"`,
			wantNotContains: `"bash"`,
			wantReverse:     map[string]string{"Bash": "bash"},
		},
		{
			name:         "unknown tools are passed through unchanged",
			input:        `{"tools":[{"name":"my_custom_tool"}]}`,
			wantContains: `"my_custom_tool"`,
			wantReverse:  map[string]string{},
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result, reverseMap := remapOAuthToolNames([]byte(tt.input))
			resultStr := string(result)
			assert.Contains(t, resultStr, tt.wantContains)
			if tt.wantNotContains != "" {
				assert.NotContains(t, resultStr, tt.wantNotContains)
			}
			assert.Equal(t, tt.wantReverse, reverseMap)
		})
	}
}

func TestReverseRemapOAuthToolNames(t *testing.T) {
	tests := []struct {
		name         string
		input        string
		reverseMap   map[string]string
		wantContains string
	}{
		{
			name:         "restores Bash to bash in response",
			input:        `{"content":[{"type":"tool_use","name":"Bash","id":"x"}]}`,
			reverseMap:   map[string]string{"Bash": "bash"},
			wantContains: `"bash"`,
		},
		{
			name:         "noop with empty reverse map",
			input:        `{"content":[{"type":"tool_use","name":"Bash","id":"x"}]}`,
			reverseMap:   map[string]string{},
			wantContains: `"Bash"`,
		},
		{
			name:         "does not restore names not in reverse map",
			input:        `{"content":[{"type":"tool_use","name":"Read","id":"x"}]}`,
			reverseMap:   map[string]string{"Bash": "bash"},
			wantContains: `"Read"`,
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := reverseRemapOAuthToolNames([]byte(tt.input), tt.reverseMap)
			assert.Contains(t, string(result), tt.wantContains)
		})
	}
}

func TestRemapToolNames(t *testing.T) {
	t.Run("renames bash to Bash in OfTool", func(t *testing.T) {
		tools := []anthropic.ToolUnionParam{
			{OfTool: &anthropic.ToolParam{Name: "bash"}},
		}
		rev := remapToolNames(tools)
		assert.Equal(t, "Bash", tools[0].OfTool.Name)
		assert.Equal(t, map[string]string{"Bash": "bash"}, rev)
	})

	t.Run("skips built-in tools (OfTool is nil)", func(t *testing.T) {
		tools := []anthropic.ToolUnionParam{
			{OfBashTool20250124: &anthropic.ToolBash20250124Param{}},
		}
		rev := remapToolNames(tools)
		assert.Empty(t, rev)
	})

	t.Run("already TitleCase — no rename", func(t *testing.T) {
		tools := []anthropic.ToolUnionParam{
			{OfTool: &anthropic.ToolParam{Name: "Bash"}},
		}
		rev := remapToolNames(tools)
		assert.Equal(t, "Bash", tools[0].OfTool.Name)
		assert.Empty(t, rev)
	})

	t.Run("unknown tool — passed through unchanged", func(t *testing.T) {
		tools := []anthropic.ToolUnionParam{
			{OfTool: &anthropic.ToolParam{Name: "my_custom_tool"}},
		}
		rev := remapToolNames(tools)
		assert.Equal(t, "my_custom_tool", tools[0].OfTool.Name)
		assert.Empty(t, rev)
	})
}

func TestRestoreToolNamesInMessage(t *testing.T) {
	t.Run("restores tool_use name", func(t *testing.T) {
		msg := &anthropic.Message{
			Content: []anthropic.ContentBlockUnion{
				{Type: "tool_use", Name: "Bash"},
			},
		}
		restoreToolNamesInMessage(msg, map[string]string{"Bash": "bash"})
		assert.Equal(t, "bash", msg.Content[0].Name)
	})

	t.Run("noop for nil reverseMap", func(t *testing.T) {
		msg := &anthropic.Message{
			Content: []anthropic.ContentBlockUnion{
				{Type: "tool_use", Name: "Bash"},
			},
		}
		restoreToolNamesInMessage(msg, nil)
		assert.Equal(t, "Bash", msg.Content[0].Name)
	})

	t.Run("does not touch non-tool_use blocks", func(t *testing.T) {
		msg := &anthropic.Message{
			Content: []anthropic.ContentBlockUnion{
				{Type: "text", Name: ""},
			},
		}
		restoreToolNamesInMessage(msg, map[string]string{"Bash": "bash"})
		assert.Equal(t, "", msg.Content[0].Name)
	})
}
