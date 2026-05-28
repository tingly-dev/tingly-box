package client

import (
	"encoding/json"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"github.com/tidwall/gjson"
)

func TestStripKimiPrefix(t *testing.T) {
	tests := []struct {
		name     string
		input    string
		expected string
	}{
		{
			name:     "strip lowercase kimi prefix",
			input:    "kimi-k2",
			expected: "k2",
		},
		{
			name:     "preserve case after prefix",
			input:    "kimi-K2",
			expected: "K2",
		},
		{
			name:     "no prefix - already stripped",
			input:    "k2",
			expected: "k2",
		},
		{
			name:     "case insensitive prefix check",
			input:    "Kimi-K2",
			expected: "K2",
		},
		{
			name:     "empty string",
			input:    "",
			expected: "",
		},
		{
			name:     "whitespace trimmed",
			input:    "  kimi-k2  ",
			expected: "k2",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := stripKimiPrefix(tt.input)
			assert.Equal(t, tt.expected, result)
		})
	}
}

func TestNormalizeRequest(t *testing.T) {
	tests := []struct {
		name     string
		input    string
		expected string
	}{
		{
			name:     "strip kimi prefix from model",
			input:    `{"model":"kimi-k2","messages":[{"role":"user","content":"hi"}]}`,
			expected: `{"model":"k2","messages":[{"role":"user","content":"hi"}]}`,
		},
		{
			name:     "preserve case in model name",
			input:    `{"model":"kimi-K2","messages":[]}`,
			expected: `{"model":"K2","messages":[]}`,
		},
		{
			name:     "no change for non-kimi model",
			input:    `{"model":"gpt-4","messages":[]}`,
			expected: `{"model":"gpt-4","messages":[]}`,
		},
		{
			name:     "filter empty assistant message",
			input:    `{"model":"k2","messages":[{"role":"assistant","content":""},{"role":"user","content":"hi"}]}`,
			expected: `{"model":"k2","messages":[{"role":"user","content":"hi"}]}`,
		},
		{
			name:     "keep assistant message with tool calls and add reasoning_content",
			input:    `{"model":"k2","messages":[{"role":"assistant","content":"","tool_calls":[{"id":"123","function":{"name":"test"}}]},{"role":"user","content":"hi"}]}`,
			expected: `{"model":"k2","messages":[{"role":"assistant","content":"","tool_calls":[{"id":"123","function":{"name":"test"}}],"reasoning_content":""},{"role":"user","content":"hi"}]}`,
		},
		{
			name:     "invalid json - return as is",
			input:    `not valid json`,
			expected: `not valid json`,
		},
		{
			name:     "empty input - return as is",
			input:    ``,
			expected: ``,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result, err := NormalizeRequest([]byte(tt.input))
			require.NoError(t, err)
			// For invalid JSON, compare raw strings
			if tt.name == "invalid json - return as is" || tt.name == "empty input - return as is" {
				assert.Equal(t, tt.expected, string(result))
			} else {
				assert.JSONEq(t, tt.expected, string(result))
			}
		})
	}
}

func TestNormalizeToolMessages_FixToolCallID(t *testing.T) {
	tests := []struct {
		name     string
		input    string
		expected string
	}{
		{
			name:     "fix call_id to tool_call_id (keeps both fields)",
			input:    `{"model":"k2","messages":[{"role":"assistant","tool_calls":[{"id":"tc1"}]},{"role":"tool","call_id":"tc1","content":"result"}]}`,
			expected: `{"model":"k2","messages":[{"role":"assistant","tool_calls":[{"id":"tc1"}],"reasoning_content":""},{"role":"tool","call_id":"tc1","tool_call_id":"tc1","content":"result"}]}`,
		},
		{
			name:     "infer missing tool_call_id from single pending",
			input:    `{"model":"k2","messages":[{"role":"assistant","tool_calls":[{"id":"tc1"}]},{"role":"tool","content":"result"}]}`,
			expected: `{"model":"k2","messages":[{"role":"assistant","tool_calls":[{"id":"tc1"}],"reasoning_content":""},{"role":"tool","tool_call_id":"tc1","content":"result"}]}`,
		},
		{
			name:     "add reasoning_content to assistant with tool calls",
			input:    `{"model":"k2","messages":[{"role":"assistant","tool_calls":[{"id":"tc1","function":{"name":"test"}}]}]}`,
			expected: `{"model":"k2","messages":[{"role":"assistant","tool_calls":[{"id":"tc1","function":{"name":"test"}}],"reasoning_content":""}]}`,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result, err := NormalizeRequest([]byte(tt.input))
			require.NoError(t, err)
			assert.JSONEq(t, tt.expected, string(result))
		})
	}
}

func TestStripKimiPrefixFromBody(t *testing.T) {
	tests := []struct {
		name     string
		input    string
		expected string
	}{
		{
			name:     "strip prefix",
			input:    `{"model":"kimi-k2"}`,
			expected: `{"model":"k2"}`,
		},
		{
			name:     "no model field",
			input:    `{"messages":[]}`,
			expected: `{"messages":[]}`,
		},
		{
			name:     "invalid json",
			input:    `not json`,
			expected: `not json`,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := stripKimiPrefixFromBody([]byte(tt.input))
			if tt.input == "not json" {
				assert.Equal(t, tt.input, string(result))
			} else {
				assert.JSONEq(t, tt.expected, string(result))
			}
		})
	}
}

func TestFilterEmptyAssistantMessages(t *testing.T) {
	tests := []struct {
		name            string
		input           string
		expectedDropped int
		expectedMsg     string
	}{
		{
			name:            "drop empty assistant",
			input:           `[{"role":"assistant","content":""},{"role":"user","content":"hi"}]`,
			expectedDropped: 1,
			expectedMsg:     `[{"role":"user","content":"hi"}]`,
		},
		{
			name:            "keep assistant with tool calls",
			input:           `[{"role":"assistant","tool_calls":[{"id":"1"}]}]`,
			expectedDropped: 0,
			expectedMsg:     `[{"role":"assistant","tool_calls":[{"id":"1"}]}]`,
		},
		{
			name:            "keep assistant with reasoning",
			input:           `[{"role":"assistant","reasoning_content":"thinking"}]`,
			expectedDropped: 0,
			expectedMsg:     `[{"role":"assistant","reasoning_content":"thinking"}]`,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			body := []byte(`{"model":"k2","messages":` + tt.input + `}`)
			result, dropped, err := filterEmptyAssistantMessages(body, parseMessages(t, tt.input))
			require.NoError(t, err)
			assert.Equal(t, tt.expectedDropped, dropped)
			// Check messages array in result
			assert.JSONEq(t, `{"model":"k2","messages":`+tt.expectedMsg+`}`, string(result))
		})
	}
}

// parseMessages is a test helper to parse JSON messages array
func parseMessages(t *testing.T, jsonMsgs string) []gjson.Result {
	t.Helper()
	var msgs []interface{}
	err := json.Unmarshal([]byte(jsonMsgs), &msgs)
	require.NoError(t, err)

	raw, _ := json.Marshal(msgs)
	result := gjson.ParseBytes(raw)
	return result.Array()
}
