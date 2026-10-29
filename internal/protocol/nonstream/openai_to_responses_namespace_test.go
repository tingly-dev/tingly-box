package nonstream

import (
	"encoding/json"
	"testing"

	"github.com/anthropics/anthropic-sdk-go"
	"github.com/openai/openai-go/v3"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/tingly-dev/tingly-box/internal/protocol/toolnamespace"
)

var linearNamespaces = toolnamespace.Index{
	"mcp__linear__get_issue": {Namespace: "mcp__linear__", Name: "get_issue"},
}

func TestConvertChatToResponsesWire_RestoresNamespace(t *testing.T) {
	var resp openai.ChatCompletion
	require.NoError(t, json.Unmarshal([]byte(`{
	  "id":"c1","object":"chat.completion","model":"m",
	  "choices":[{"index":0,"finish_reason":"tool_calls","message":{"role":"assistant","content":"","tool_calls":[
	    {"id":"call_1","type":"function","function":{"name":"mcp__linear__get_issue","arguments":"{}"}},
	    {"id":"call_2","type":"function","function":{"name":"shell","arguments":"{}"}}
	  ]}}]}`), &resp))

	out := ConvertChatToResponsesWire(&resp, "gpt-5", "m", linearNamespaces).Output
	require.Len(t, out, 2)
	assert.Equal(t, "mcp__linear__", out[0].Namespace)
	assert.Equal(t, "get_issue", out[0].Name)
	assert.Empty(t, out[1].Namespace)
	assert.Equal(t, "shell", out[1].Name)

	// Without an index nothing changes.
	plain := ConvertChatToResponsesWire(&resp, "gpt-5", "m", nil).Output
	assert.Empty(t, plain[0].Namespace)
	assert.Equal(t, "mcp__linear__get_issue", plain[0].Name)
}

func TestConvertAnthropicBetaToResponsesWire_RestoresNamespace(t *testing.T) {
	var msg anthropic.BetaMessage
	require.NoError(t, json.Unmarshal([]byte(`{
	  "id":"m1","type":"message","role":"assistant","model":"claude","stop_reason":"tool_use",
	  "content":[{"type":"tool_use","id":"toolu_1","name":"mcp__linear__get_issue","input":{}}],
	  "usage":{"input_tokens":1,"output_tokens":1}}`), &msg))

	out := ConvertAnthropicBetaToResponsesWire(&msg, "gpt-5", "claude", linearNamespaces).Output
	require.Len(t, out, 1)
	assert.Equal(t, "mcp__linear__", out[0].Namespace)
	assert.Equal(t, "get_issue", out[0].Name)
}
