package probe

import (
	"context"
	"encoding/json"
	"errors"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/tingly-dev/tingly-box/internal/protocol"
	"github.com/tingly-dev/tingly-box/internal/typ"
)

func validationField(t *testing.T, err error) string {
	t.Helper()
	var ve *ValidationError
	require.True(t, errors.As(err, &ve), "expected a ValidationError, got %v", err)
	return ve.Field
}

// Raw client requests in each protocol, using every carrier the fixture
// lacks: multi-turn, a tool round trip, an image and a mid-conversation
// system turn. The probe only fills the model (and Anthropic max_tokens).
const (
	rawAnthropic = `{
	  "system": [{"type":"text","text":"be brief","cache_control":{"type":"ephemeral"}}],
	  "messages": [
	    {"role":"user","content":"What's the weather in Tokyo?"},
	    {"role":"assistant","content":[{"type":"tool_use","id":"t1","name":"get_weather","input":{"city":"Tokyo"}}]},
	    {"role":"user","content":[{"type":"tool_result","tool_use_id":"t1","content":"24C, clear"}]},
	    {"role":"system","content":"From now on answer only in JSON."},
	    {"role":"user","content":[{"type":"text","text":"And this picture?"},{"type":"image","source":{"type":"url","url":"https://example.com/sky.png"}}]}
	  ],
	  "tools": [{"name":"get_weather","input_schema":{"type":"object","properties":{"city":{"type":"string"}}}}],
	  "thinking": {"type":"enabled","budget_tokens":2048},
	  "max_tokens": 4096
	}`
	rawChat = `{
	  "messages": [
	    {"role":"system","content":"be brief"},
	    {"role":"user","content":[{"type":"text","text":"What is this?"},{"type":"image_url","image_url":{"url":"https://example.com/sky.png"}}]},
	    {"role":"assistant","tool_calls":[{"id":"c1","type":"function","function":{"name":"get_weather","arguments":"{\"city\":\"Tokyo\"}"}}]},
	    {"role":"tool","tool_call_id":"c1","content":"24C"},
	    {"role":"user","content":"thanks"}
	  ],
	  "temperature": 0.2,
	  "tools": [{"type":"function","function":{"name":"get_weather","parameters":{"type":"object"}}}]
	}`
	rawResponses = `{
	  "instructions": "be brief",
	  "input": [{"role":"user","content":[{"type":"input_text","text":"hi"},{"type":"input_image","image_url":"https://example.com/sky.png"}]}],
	  "store": false
	}`
)

func rawReq(protocol ProbeProtocol, body string) *E2ERequest {
	return &E2ERequest{TargetType: E2ETargetProvider, ProviderUUID: "p", Model: "m", Request: json.RawMessage(body), RequestProtocol: protocol}
}

func TestValidateE2ERequest_RawRequest(t *testing.T) {
	t.Run("each protocol parses with its SDK decoder", func(t *testing.T) {
		assert.NoError(t, ValidateE2ERequest(rawReq(ProtocolAnthropic, rawAnthropic)))
		assert.NoError(t, ValidateE2ERequest(rawReq(ProtocolOpenAIChat, rawChat)))
		assert.NoError(t, ValidateE2ERequest(rawReq(ProtocolOpenAIResponses, rawResponses)))
	})
	t.Run("protocol required, must match the body", func(t *testing.T) {
		req := rawReq("", rawChat)
		assert.Equal(t, "request_protocol", validationField(t, ValidateE2ERequest(req)))
		assert.Equal(t, "request", validationField(t, ValidateE2ERequest(rawReq(ProtocolAnthropic, `{"max_tokens":1}`))), "no messages")
		assert.Equal(t, "request", validationField(t, ValidateE2ERequest(rawReq(ProtocolOpenAIResponses, `{"model":"x"}`))), "no input")
		assert.Equal(t, "request", validationField(t, ValidateE2ERequest(rawReq(ProtocolOpenAIChat, `[1,2]`))), "not an object")
	})
	t.Run("fixture knobs are rejected alongside a raw request", func(t *testing.T) {
		req := rawReq(ProtocolOpenAIChat, rawChat)
		req.Message = "x"
		assert.Equal(t, "message", validationField(t, ValidateE2ERequest(req)))
		req.Message = ""
		req.Tool = boolPtr(true)
		assert.Equal(t, "tool", validationField(t, ValidateE2ERequest(req)))
		req.Tool = nil
		req.Vision = VisonUser
		assert.Equal(t, "vision", validationField(t, ValidateE2ERequest(req)))
		req.Vision = ""
		req.Thinking = ThinkingHigh
		assert.Equal(t, "thinking", validationField(t, ValidateE2ERequest(req)))
		req.Thinking = ""
		req.Stream = boolPtr(true)
		assert.NoError(t, ValidateE2ERequest(req), "stream still applies")
	})
	t.Run("provider: protocol must agree; rule: scenario family must match", func(t *testing.T) {
		req := rawReq(ProtocolOpenAIChat, rawChat)
		req.Protocol = ProtocolAnthropic
		assert.Equal(t, "protocol", validationField(t, ValidateE2ERequest(req)))
		req.Protocol = ProtocolOpenAIChat
		assert.NoError(t, ValidateE2ERequest(req))

		rule := &E2ERequest{TargetType: E2ETargetRule, Scenario: "claude_code", RuleUUID: "r", Request: json.RawMessage(rawChat), RequestProtocol: ProtocolOpenAIChat}
		assert.Equal(t, "request_protocol", validationField(t, ValidateE2ERequest(rule)))
		rule.Request, rule.RequestProtocol = json.RawMessage(rawAnthropic), ProtocolAnthropic
		assert.NoError(t, ValidateE2ERequest(rule))
		codex := &E2ERequest{TargetType: E2ETargetRule, Scenario: "codex", RuleUUID: "r", Request: json.RawMessage(rawResponses), RequestProtocol: ProtocolOpenAIResponses}
		assert.NoError(t, ValidateE2ERequest(codex))
	})
	t.Run("wire protocol follows the raw request", func(t *testing.T) {
		req := rawReq(ProtocolOpenAIResponses, rawResponses)
		assert.Equal(t, ProtocolOpenAIResponses, req.WireProtocol())
		assert.Equal(t, "responses", req.ResolveOpenAIEndpointOverride())
		assert.Equal(t, protocol.APIStyleOpenAI, req.ResolveClientStyle(protocol.APIStyleAnthropic))
	})
}

func TestBuilders_RawRequest(t *testing.T) {
	t.Run("anthropic: verbatim, model and max_tokens filled by the probe", func(t *testing.T) {
		req := rawReq(ProtocolAnthropic, rawAnthropic)
		require.NoError(t, ValidateE2ERequest(req))
		p := req.probeParams("claude-x")
		b, err := json.Marshal(buildAnthropicMessageParams(p, false))
		require.NoError(t, err)
		body := decodeBody(t, string(b))
		assert.Equal(t, "claude-x", body["model"])
		assert.EqualValues(t, 4096, body["max_tokens"], "caller's max_tokens kept")
		assert.Len(t, body["messages"], 5)
		assert.Contains(t, string(b), `"cache_control"`)
		assert.Contains(t, string(b), `"tool_use"`)
		assert.Contains(t, string(b), `"budget_tokens":2048`)
		assert.Contains(t, string(b), `"role":"system"`, "mid-conversation system kept verbatim")

		// max_tokens defaults when the caller omits it (Anthropic requires it).
		minimal := rawReq(ProtocolAnthropic, `{"messages":[{"role":"user","content":"hi"}]}`)
		require.NoError(t, ValidateE2ERequest(minimal))
		mb, _ := json.Marshal(buildAnthropicMessageParams(minimal.probeParams("m"), false))
		assert.EqualValues(t, 1024, decodeBody(t, string(mb))["max_tokens"])
	})

	t.Run("openai chat: verbatim, model filled, usage on stream", func(t *testing.T) {
		req := rawReq(ProtocolOpenAIChat, rawChat)
		req.Stream = boolPtr(true)
		require.NoError(t, ValidateE2ERequest(req))
		b, err := json.Marshal(buildOpenAIChatParams(req.probeParams("gpt-x")))
		require.NoError(t, err)
		body := decodeBody(t, string(b))
		assert.Equal(t, "gpt-x", body["model"])
		assert.EqualValues(t, 0.2, body["temperature"])
		assert.Len(t, body["messages"], 5)
		assert.Contains(t, string(b), `"image_url"`)
		assert.Contains(t, string(b), `"tool_calls"`)
		assert.Contains(t, string(b), `"include_usage":true`)
	})

	t.Run("openai responses: verbatim, model filled", func(t *testing.T) {
		req := rawReq(ProtocolOpenAIResponses, rawResponses)
		require.NoError(t, ValidateE2ERequest(req))
		b, err := json.Marshal(buildOpenAIResponsesParams(req.probeParams("gpt-x")))
		require.NoError(t, err)
		body := decodeBody(t, string(b))
		assert.Equal(t, "gpt-x", body["model"])
		assert.Equal(t, "be brief", body["instructions"])
		assert.Equal(t, false, body["store"])
		assert.Contains(t, string(b), `"input_image"`)
	})

	t.Run("fixture unchanged without a raw request", func(t *testing.T) {
		b, _ := json.Marshal(buildOpenAIChatParams(probeParams{Model: "m", Message: "hello"}))
		assert.Contains(t, string(b), probeEchoInstruction)
		assert.Contains(t, string(b), `"hello"`)
	})
}

// Through TB, a raw request is sent on its own protocol's loopback endpoint —
// the wire a real client using that protocol would hit.
func TestBuildCurl_RawRequest_ThroughTB(t *testing.T) {
	prober := newCurlTestProber(t)
	addProvider(t, prober.config, &typ.Provider{
		UUID: "p-openai", Name: "oai", APIStyle: protocol.APIStyleOpenAI, APIBase: "https://api.example.com/v1",
		Token: "sk-upstream", Enabled: true, Models: []string{"gpt-x"},
	})
	addProvider(t, prober.config, &typ.Provider{
		UUID: "p-anthropic", Name: "anth", APIStyle: protocol.APIStyleAnthropic, APIBase: "https://api.example.com",
		Token: "sk-upstream", Enabled: true, Models: []string{"claude-x"},
	})

	t.Run("responses request against an openai provider", func(t *testing.T) {
		req := &E2ERequest{TargetType: E2ETargetProvider, ProviderUUID: "p-openai", Model: "gpt-x", Request: json.RawMessage(rawResponses), RequestProtocol: ProtocolOpenAIResponses}
		require.NoError(t, ValidateE2ERequest(req))
		curl, err := prober.BuildCurl(context.Background(), req)
		require.NoError(t, err)
		assert.Contains(t, curl.URL, "/tingly/openai/responses")
		body := decodeBody(t, curl.Body)
		assert.Equal(t, "gpt-x", body["model"])
		assert.Equal(t, "be brief", body["instructions"])
	})

	t.Run("anthropic request against an anthropic provider, streaming", func(t *testing.T) {
		req := &E2ERequest{TargetType: E2ETargetProvider, ProviderUUID: "p-anthropic", Model: "claude-x", Request: json.RawMessage(rawAnthropic), RequestProtocol: ProtocolAnthropic, Stream: boolPtr(true)}
		require.NoError(t, ValidateE2ERequest(req))
		curl, err := prober.BuildCurl(context.Background(), req)
		require.NoError(t, err)
		assert.Contains(t, curl.URL, "/tingly/anthropic/v1/messages")
		body := decodeBody(t, curl.Body)
		assert.Equal(t, "claude-x", body["model"])
		assert.Equal(t, true, body["stream"])
		assert.Len(t, body["messages"], 5)
	})
}
