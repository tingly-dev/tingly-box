package protocol

import (
	"encoding/json"
	"testing"

	"github.com/anthropics/anthropic-sdk-go/v2"
	"github.com/stretchr/testify/require"
)

func TestAnthropicIngressExtras(t *testing.T) {
	const body = `{"model":"m","max_tokens":64,"stream":true,
		"messages":[{"role":"user","content":"hi"}],
		"temperature":0,"top_p":0.9,"top_k":40,"user":"u-123",
		"context_management":{"edits":[{"type":"clear_tool_uses_20250919"}]}}`
	for _, beta := range []bool{false, true} {
		t.Run(map[bool]string{false: "v1", true: "beta"}[beta], func(t *testing.T) {
			var req any = &AnthropicMessagesRequest{}
			if beta {
				req = &AnthropicBetaMessagesRequest{}
			}
			require.NoError(t, json.Unmarshal([]byte(body), req))
			out, err := json.Marshal(req)
			require.NoError(t, err)
			var wire map[string]any
			require.NoError(t, json.Unmarshal(out, &wire))
			require.NotContains(t, wire, "user")
			require.Equal(t, float64(0), wire["temperature"])
			require.Equal(t, 0.9, wire["top_p"])
			require.Equal(t, float64(40), wire["top_k"])
			require.Equal(t, true, wire["stream"])
			require.Len(t, wire["messages"], 1)
			if beta {
				require.Equal(t, map[string]any{"edits": []any{map[string]any{"type": "clear_tool_uses_20250919"}}}, wire["context_management"], "declared beta fields must survive")
			} else {
				require.NotContains(t, wire, "context_management", "v1 must retain its historical filtering")
			}
		})
	}
}

func TestAnthropicIngressRejectsUnknownRole(t *testing.T) {
	for _, beta := range []bool{false, true} {
		t.Run(map[bool]string{false: "v1", true: "beta"}[beta], func(t *testing.T) {
			var req any = &AnthropicMessagesRequest{}
			if beta {
				req = &AnthropicBetaMessagesRequest{}
			}
			err := json.Unmarshal([]byte(`{"model":"m","max_tokens":64,"messages":[
				{"role":"developer","content":"be brief"},{"role":"user","content":"hi"}]}`), req)
			require.ErrorContains(t, err, "messages[0].role")
			require.ErrorContains(t, err, "developer")
		})
	}
}

func TestAnthropicIngressCountTokens(t *testing.T) {
	const body = `{"model":"m","messages":[{"role":"user","content":"hi"}],
		"max_tokens":64,"stream":true,"temperature":0.2,"top_p":0.9,"top_k":40,"user":"u-123",
		"context_management":{"edits":[{"type":"clear_tool_uses_20250919"}]}}`
	var req anthropic.BetaMessageCountTokensParams
	require.NoError(t, UnmarshalAnthropicCountTokensRequest([]byte(body), &req))
	wire, err := json.Marshal(req)
	require.NoError(t, err)
	var out map[string]any
	require.NoError(t, json.Unmarshal(wire, &out))
	for _, key := range []string{"max_tokens", "stream", "temperature", "top_p", "top_k", "user"} {
		require.NotContains(t, out, key)
	}
	require.Equal(t, map[string]any{"edits": []any{map[string]any{"type": "clear_tool_uses_20250919"}}}, out["context_management"])
	require.Len(t, req.Messages, 1)
}

func TestAnthropicIngressRoleValidation(t *testing.T) {
	for _, messages := range []string{
		`null`, `[null]`, `["hi"]`,
		`[{"content":"hi"}]`,
		`[{"role":null,"content":"hi"}]`,
		`[{"role":42,"content":"hi"}]`,
		`[{"role":"user","content":"hi"},{"role":"developer","content":"instruction"}]`,
		`{"role":"user","content":"hi"}`,
	} {
		t.Run(messages, func(t *testing.T) {
			body := []byte(`{"model":"m","messages":` + messages + `}`)
			var v1 AnthropicMessagesRequest
			var beta AnthropicBetaMessagesRequest
			var count anthropic.BetaMessageCountTokensParams
			require.ErrorContains(t, json.Unmarshal(body, &v1), "messages")
			require.ErrorContains(t, json.Unmarshal(body, &beta), "messages")
			require.ErrorContains(t, UnmarshalAnthropicCountTokensRequest(body, &count), "messages")
		})
	}
}

func TestAnthropicIngressPreservesSupportedRoles(t *testing.T) {
	body := []byte(`{"model":"m","max_tokens":64,"messages":[
		{"role":"user","content":"hello"},{"role":"assistant","content":"hi"},
		{"role":"system","content":[{"type":"text","text":"instruction"}]}]}`)
	var v1 AnthropicMessagesRequest
	var beta AnthropicBetaMessagesRequest
	require.NoError(t, json.Unmarshal(body, &v1))
	require.NoError(t, json.Unmarshal(body, &beta))
	require.Len(t, v1.Messages, 3)
	require.Len(t, beta.Messages, 3)
	// An intentional internal extra is still supported after ingress. Filtering
	// must not run at marshal/egress and erase callers' explicit wire options.
	v1.SetExtraFields(map[string]any{"internal_extension": "kept"})
	wire, err := json.Marshal(&v1)
	require.NoError(t, err)
	require.Contains(t, string(wire), `"internal_extension":"kept"`)
}
