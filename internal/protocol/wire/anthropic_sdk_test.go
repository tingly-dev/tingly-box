package wire

import (
	"encoding/json"
	"testing"

	"github.com/anthropics/anthropic-sdk-go"
	"github.com/anthropics/anthropic-sdk-go/shared/constant"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func marshalToMap(t *testing.T, v any) map[string]any {
	t.Helper()
	data, err := json.Marshal(v)
	require.NoError(t, err)
	var m map[string]any
	require.NoError(t, json.Unmarshal(data, &m))
	return m
}

const realV1Response = `{"id":"msg_01ABC","type":"message","role":"assistant","model":"claude-sonnet-4-5","content":[{"type":"text","text":"Hello!"}],"stop_reason":"end_turn","stop_sequence":null,"usage":{"input_tokens":10,"output_tokens":5,"cache_creation_input_tokens":0,"cache_read_input_tokens":3,"service_tier":"standard"}}`

// TestAnthropicMessageMap_Passthrough verifies a real upstream response
// round-trips without gaining SDK zero-value junk (phantom stop_details
// refusal, container, web_search noise inside text blocks).
func TestAnthropicMessageMap_Passthrough(t *testing.T) {
	var msg anthropic.Message
	require.NoError(t, json.Unmarshal([]byte(realV1Response), &msg))
	msg.Model = "renamed-model" // proxy renames before serving

	out := marshalToMap(t, AnthropicMessageMap(&msg))

	assert.NotContains(t, out, "stop_details", "must not invent a refusal")
	assert.NotContains(t, out, "container")
	assert.Equal(t, "renamed-model", out["model"])
	assert.Equal(t, "end_turn", out["stop_reason"])
	assert.Nil(t, out["stop_sequence"], "stop_sequence must stay null")

	content := out["content"].([]any)
	require.Len(t, content, 1)
	block := content[0].(map[string]any)
	assert.Equal(t, map[string]any{"type": "text", "text": "Hello!"}, block,
		"text block must contain exactly type+text")

	usage := out["usage"].(map[string]any)
	assert.Equal(t, "standard", usage["service_tier"], "upstream usage preserved verbatim")
	assert.EqualValues(t, 3, usage["cache_read_input_tokens"])
}

// TestAnthropicMessageMap_GuardrailMutation verifies post-forward edits to
// content text (guardrails redaction) are reflected in the wire output.
func TestAnthropicMessageMap_GuardrailMutation(t *testing.T) {
	var msg anthropic.Message
	require.NoError(t, json.Unmarshal([]byte(realV1Response), &msg))
	msg.Content[0].Text = "[REDACTED]"

	out := marshalToMap(t, AnthropicMessageMap(&msg))
	block := out["content"].([]any)[0].(map[string]any)
	assert.Equal(t, "[REDACTED]", block["text"])
}

// TestAnthropicMessageMap_RealStopDetails verifies an upstream refusal's
// stop_details survives passthrough.
func TestAnthropicMessageMap_RealStopDetails(t *testing.T) {
	raw := `{"id":"msg_1","type":"message","role":"assistant","model":"m","content":[],"stop_reason":"refusal","stop_details":{"type":"refusal","category":"safety","explanation":"x"},"stop_sequence":null,"usage":{"input_tokens":1,"output_tokens":1}}`
	var msg anthropic.Message
	require.NoError(t, json.Unmarshal([]byte(raw), &msg))

	out := marshalToMap(t, AnthropicMessageMap(&msg))
	require.Contains(t, out, "stop_details")
	sd := out["stop_details"].(map[string]any)
	assert.Equal(t, "refusal", sd["type"])
	assert.Equal(t, "safety", sd["category"])
	assert.Equal(t, []any{}, out["content"], "empty content stays an array")
}

// TestAnthropicBetaMessageMap_HandBuilt verifies converter-built messages
// (Google→Anthropic, OpenAI→Anthropic, stream assembly) serialize cleanly.
func TestAnthropicBetaMessageMap_HandBuilt(t *testing.T) {
	msg := anthropic.BetaMessage{
		ID:    "msg_built",
		Type:  constant.Message("message"),
		Role:  constant.Assistant("assistant"),
		Model: "claude-x",
		Content: []anthropic.BetaContentBlockUnion{
			{Type: "text", Text: "hi"},
			{Type: "tool_use", ID: "toolu_1", Name: "get_weather", Input: json.RawMessage(`{"city":"SF"}`)},
			{Type: "thinking", Thinking: "hmm", Signature: "sig"},
		},
		StopReason: anthropic.BetaStopReasonToolUse,
	}
	msg.Usage.InputTokens = 7
	msg.Usage.OutputTokens = 2

	out := marshalToMap(t, AnthropicBetaMessageMap(&msg))

	assert.NotContains(t, out, "stop_details")
	assert.NotContains(t, out, "container")
	assert.NotContains(t, out, "context_management")
	assert.NotContains(t, out, "diagnostics")
	assert.Equal(t, "tool_use", out["stop_reason"])
	assert.Nil(t, out["stop_sequence"])

	content := out["content"].([]any)
	require.Len(t, content, 3)
	assert.Equal(t, map[string]any{"type": "text", "text": "hi"}, content[0])
	assert.Equal(t, map[string]any{
		"type": "tool_use", "id": "toolu_1", "name": "get_weather",
		"input": map[string]any{"city": "SF"},
	}, content[1])
	assert.Equal(t, map[string]any{"type": "thinking", "thinking": "hmm", "signature": "sig"}, content[2])

	usage := out["usage"].(map[string]any)
	assert.EqualValues(t, 7, usage["input_tokens"])
	assert.EqualValues(t, 2, usage["output_tokens"])
	assert.NotContains(t, usage, "service_tier")
}

// TestAnthropicBetaMessageMap_ExoticBlockPassthrough verifies block types not
// rebuilt field-by-field are passed through from their raw upstream JSON.
func TestAnthropicBetaMessageMap_ExoticBlockPassthrough(t *testing.T) {
	raw := `{"id":"msg_1","type":"message","role":"assistant","model":"m","content":[{"type":"web_search_tool_result","tool_use_id":"srvtoolu_1","content":[{"type":"web_search_result","url":"https://example.com","title":"t","encrypted_content":"abc"}]}],"stop_reason":"end_turn","stop_sequence":null,"usage":{"input_tokens":1,"output_tokens":1}}`
	var msg anthropic.BetaMessage
	require.NoError(t, json.Unmarshal([]byte(raw), &msg))

	out := marshalToMap(t, AnthropicBetaMessageMap(&msg))
	block := out["content"].([]any)[0].(map[string]any)
	assert.Equal(t, "web_search_tool_result", block["type"])
	assert.Equal(t, "srvtoolu_1", block["tool_use_id"])
	assert.NotContains(t, block, "text", "raw passthrough must not gain union noise")
	assert.NotContains(t, block, "input")
}

// TestAnthropicMessageMap_EmptyToolInput verifies empty tool input serializes
// as {} (Anthropic wire convention), not null.
func TestAnthropicMessageMap_EmptyToolInput(t *testing.T) {
	msg := anthropic.Message{
		ID:   "msg_1",
		Type: constant.Message("message"),
		Role: constant.Assistant("assistant"),
		Content: []anthropic.ContentBlockUnion{
			{Type: "tool_use", ID: "toolu_1", Name: "ping"},
		},
		StopReason: anthropic.StopReasonToolUse,
	}
	out := marshalToMap(t, AnthropicMessageMap(&msg))
	block := out["content"].([]any)[0].(map[string]any)
	assert.Equal(t, map[string]any{}, block["input"])
}

// TestAnthropicBetaMessageMap_ExtraFields verifies unknown upstream fields are
// preserved (forward compatibility with new API fields).
func TestAnthropicBetaMessageMap_ExtraFields(t *testing.T) {
	raw := `{"id":"msg_1","type":"message","role":"assistant","model":"m","content":[],"stop_reason":"end_turn","stop_sequence":null,"usage":{"input_tokens":1,"output_tokens":1},"future_field":{"a":1}}`
	var msg anthropic.BetaMessage
	require.NoError(t, json.Unmarshal([]byte(raw), &msg))

	out := marshalToMap(t, AnthropicBetaMessageMap(&msg))
	require.Contains(t, out, "future_field")
	assert.Equal(t, map[string]any{"a": float64(1)}, out["future_field"])
}
