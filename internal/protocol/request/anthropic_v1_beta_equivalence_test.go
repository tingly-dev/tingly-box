package request

import (
	"encoding/json"
	"testing"

	"github.com/anthropics/anthropic-sdk-go"
	"github.com/stretchr/testify/require"
)

// Anthropic Beta is planned to become the only internal Anthropic protocol,
// with V1 handled by upgrade (client edge) and downgrade (provider edge); see
// .design/protocol-stage-v2.md §4. That rests on one property: a V1 request
// survives V1 -> Beta -> V1 byte for byte, and its Beta form serializes to the
// same wire JSON. This corpus pins the property across the V1 feature surface.
var v1WireCorpus = map[string]string{
	"text": `{"model":"m","max_tokens":64,"messages":[{"role":"user","content":"hi"}]}`,

	"system_blocks_cache_control": `{"model":"m","max_tokens":64,
		"system":[{"type":"text","text":"sys","cache_control":{"type":"ephemeral","ttl":"1h"}}],
		"messages":[{"role":"user","content":[{"type":"text","text":"hi","cache_control":{"type":"ephemeral"}}]}]}`,

	"sampling_and_metadata": `{"model":"m","max_tokens":64,"temperature":0.2,"top_p":0.9,"top_k":40,
		"stop_sequences":["END"],"metadata":{"user_id":"u-1"},"service_tier":"auto",
		"messages":[{"role":"user","content":"hi"}]}`,

	"images": `{"model":"m","max_tokens":64,"messages":[{"role":"user","content":[
		{"type":"image","source":{"type":"base64","media_type":"image/png","data":"iVBORw0KGgo="}},
		{"type":"image","source":{"type":"url","url":"https://example.com/a.png"}},
		{"type":"text","text":"describe"}]}]}`,

	"documents": `{"model":"m","max_tokens":64,"messages":[{"role":"user","content":[
		{"type":"document","source":{"type":"base64","media_type":"application/pdf","data":"JVBERi0="},"title":"t","citations":{"enabled":true}},
		{"type":"document","source":{"type":"text","media_type":"text/plain","data":"plain body"}},
		{"type":"text","text":"summarize"}]}]}`,

	"tools_and_choice": `{"model":"m","max_tokens":64,
		"tools":[
			{"name":"get_weather","description":"w","input_schema":{"type":"object","properties":{"location":{"type":"string"}},"required":["location"]},"cache_control":{"type":"ephemeral"}},
			{"type":"web_search_20250305","name":"web_search","max_uses":3}],
		"tool_choice":{"type":"tool","name":"get_weather","disable_parallel_tool_use":true},
		"messages":[{"role":"user","content":"weather?"}]}`,

	"tool_round_trip": `{"model":"m","max_tokens":64,"messages":[
		{"role":"user","content":"weather in Paris?"},
		{"role":"assistant","content":[{"type":"text","text":"checking"},{"type":"tool_use","id":"toolu_1","name":"get_weather","input":{"location":"Paris"}}]},
		{"role":"user","content":[
			{"type":"tool_result","tool_use_id":"toolu_1","content":"18C"},
			{"type":"tool_result","tool_use_id":"toolu_2","is_error":true,"content":[
				{"type":"text","text":"failed"},
				{"type":"image","source":{"type":"base64","media_type":"image/png","data":"iVBORw0KGgo="}}]}]}]}`,

	"thinking": `{"model":"m","max_tokens":2048,"thinking":{"type":"enabled","budget_tokens":1024},"messages":[
		{"role":"user","content":"think"},
		{"role":"assistant","content":[
			{"type":"thinking","thinking":"hmm","signature":"sig=="},
			{"type":"redacted_thinking","data":"opaque"},
			{"type":"text","text":"answer"}]},
		{"role":"user","content":"again"}]}`,
}

func canonicalWire(t *testing.T, v any) string {
	t.Helper()
	raw, err := json.Marshal(v)
	require.NoError(t, err)
	var generic any
	require.NoError(t, json.Unmarshal(raw, &generic))
	out, err := json.Marshal(generic) // map keys sorted
	require.NoError(t, err)
	return string(out)
}

func TestAnthropicV1BetaWireEquivalence(t *testing.T) {
	for name, raw := range v1WireCorpus {
		t.Run(name, func(t *testing.T) {
			var v1 anthropic.MessageNewParams
			require.NoError(t, json.Unmarshal([]byte(raw), &v1))
			v1Wire := canonicalWire(t, &v1)

			// Every corpus field must survive V1 decoding itself, or the case
			// would not exercise what it claims to. (The V1 SDK does normalize
			// string content into text blocks; that is today's behavior too.)
			var input, decoded map[string]any
			require.NoError(t, json.Unmarshal([]byte(raw), &input))
			require.NoError(t, json.Unmarshal([]byte(v1Wire), &decoded))
			for key := range input {
				require.Contains(t, decoded, key, "V1 SDK drops field %q", key)
			}

			// Upgrade at the client edge.
			beta, err := ConvertAnthropicV1ToBetaRequestWithError(&v1)
			require.NoError(t, err)
			require.Equal(t, v1Wire, canonicalWire(t, beta), "V1 -> Beta changed the wire request")

			// Downgrade at the provider edge (no Beta-only content present).
			back, err := ConvertAnthropicBetaToV1Request(beta)
			require.NoError(t, err)
			require.Equal(t, v1Wire, canonicalWire(t, back), "V1 -> Beta -> V1 changed the wire request")
		})
	}
}

// The downgrade refuses what V1 cannot carry instead of dropping it, so a
// Beta-only addition reaching a V1-wire provider surfaces as an error.
func TestAnthropicBetaToV1DowngradeRefusesBetaOnly(t *testing.T) {
	var withField anthropic.BetaMessageNewParams
	require.NoError(t, json.Unmarshal([]byte(`{"model":"m","max_tokens":64,
		"messages":[{"role":"user","content":"hi"}],
		"mcp_servers":[{"type":"url","name":"s","url":"https://example.com/mcp"}]}`), &withField))
	_, err := ConvertAnthropicBetaToV1Request(&withField)
	require.ErrorContains(t, err, "mcp_servers")

	withBetas := anthropic.BetaMessageNewParams{
		Model:     "m",
		MaxTokens: 64,
		Messages:  []anthropic.BetaMessageParam{anthropic.NewBetaUserMessage(anthropic.NewBetaTextBlock("hi"))},
		Betas:     []anthropic.AnthropicBeta{anthropic.AnthropicBetaMCPClient2025_04_04},
	}
	_, err = ConvertAnthropicBetaToV1Request(&withBetas)
	require.ErrorContains(t, err, "anthropic-beta")
}
