package probe

import (
	"context"
	"encoding/json"
	"errors"
	"strings"
	"testing"

	"github.com/anthropics/anthropic-sdk-go"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/tingly-dev/tingly-box/internal/protocol"
	"github.com/tingly-dev/tingly-box/internal/protocol/ops"
	"github.com/tingly-dev/tingly-box/internal/typ"
)

func shapingBase() *E2ERequest {
	return &E2ERequest{TargetType: E2ETargetRule, Scenario: "claude_code", RuleUUID: "r1"}
}

func validationField(t *testing.T, err error) string {
	t.Helper()
	var ve *ValidationError
	require.True(t, errors.As(err, &ve), "expected a ValidationError, got %v", err)
	return ve.Field
}

func TestValidateE2ERequest_Conversation(t *testing.T) {
	t.Run("messages ok", func(t *testing.T) {
		req := shapingBase()
		req.Messages = []ProbeMessage{{Role: "user", Text: "a"}, {Role: "assistant", Text: "b"}, {Role: "system", Text: "c"}, {Role: "user", Text: "d"}}
		assert.NoError(t, ValidateE2ERequest(req))
	})
	t.Run("last must be user", func(t *testing.T) {
		req := shapingBase()
		req.Messages = []ProbeMessage{{Role: "user", Text: "a"}, {Role: "assistant", Text: "b"}}
		assert.Equal(t, "messages", validationField(t, ValidateE2ERequest(req)))
	})
	t.Run("bad role / empty text", func(t *testing.T) {
		req := shapingBase()
		req.Messages = []ProbeMessage{{Role: "tool", Text: "a"}}
		assert.Equal(t, "messages", validationField(t, ValidateE2ERequest(req)))
		req.Messages = []ProbeMessage{{Role: "user", Text: "  "}}
		assert.Equal(t, "messages", validationField(t, ValidateE2ERequest(req)))
	})
	t.Run("exclusive with message and vision", func(t *testing.T) {
		req := shapingBase()
		req.Messages = []ProbeMessage{{Role: "user", Text: "a"}}
		req.Message = "x"
		assert.Equal(t, "messages", validationField(t, ValidateE2ERequest(req)))
		req.Message = ""
		req.Vision = VisonUser
		assert.Equal(t, "messages", validationField(t, ValidateE2ERequest(req)))
	})
}

func TestValidateE2ERequest_Client(t *testing.T) {
	req := shapingBase()
	req.Client = ClientClaudeCode
	assert.NoError(t, ValidateE2ERequest(req))

	req.Client = ProbeClient("cursor")
	assert.Equal(t, "client", validationField(t, ValidateE2ERequest(req)))

	direct := &E2ERequest{TargetType: E2ETargetProvider, ProviderUUID: "p", Model: "m", Direct: true, Client: ClientClaudeCode}
	assert.Equal(t, "client", validationField(t, ValidateE2ERequest(direct)))

	cfg := &E2ERequest{TargetType: E2ETargetProviderConfig, APIBase: "https://x", APIStyle: "anthropic", Token: "t", Model: "m", Client: ClientClaudeCode}
	assert.Equal(t, "client", validationField(t, ValidateE2ERequest(cfg)))
}

func TestCheckClientSimulation(t *testing.T) {
	req := &E2ERequest{Client: ClientClaudeCode}
	anthropicP := &typ.Provider{APIStyle: protocol.APIStyleAnthropic}
	openaiP := &typ.Provider{APIStyle: protocol.APIStyleOpenAI}
	pins := map[string]string{"X-Tingly-Probe-Service": "p:m"}

	assert.NoError(t, checkClientSimulation(req, anthropicP, pins))
	assert.Error(t, checkClientSimulation(req, anthropicP, nil), "no loopback")
	assert.Error(t, checkClientSimulation(req, openaiP, pins), "wrong protocol")
	assert.NoError(t, checkClientSimulation(&E2ERequest{}, openaiP, nil), "no client → nothing to check")
}

func TestBuilders_CustomConversation(t *testing.T) {
	p := probeParams{
		Model:  "m",
		System: "be brief",
		Messages: []ProbeMessage{
			{Role: ProbeRoleUser, Text: "q1"},
			{Role: ProbeRoleAssistant, Text: "a1"},
			{Role: ProbeRoleSystem, Text: "mid"},
			{Role: ProbeRoleUser, Text: "q2"},
		},
	}

	t.Run("openai chat", func(t *testing.T) {
		b, err := json.Marshal(buildOpenAIChatParams(p))
		require.NoError(t, err)
		var body struct {
			Messages []struct {
				Role    string `json:"role"`
				Content string `json:"content"`
			} `json:"messages"`
		}
		require.NoError(t, json.Unmarshal(b, &body))
		require.Len(t, body.Messages, 5)
		assert.Equal(t, "system", body.Messages[0].Role)
		assert.Equal(t, "be brief", body.Messages[0].Content)
		assert.Equal(t, []string{"user", "assistant", "system", "user"},
			[]string{body.Messages[1].Role, body.Messages[2].Role, body.Messages[3].Role, body.Messages[4].Role})
	})

	t.Run("openai responses", func(t *testing.T) {
		b, err := json.Marshal(buildOpenAIResponsesParams(p))
		require.NoError(t, err)
		var body struct {
			Instructions string `json:"instructions"`
			Input        []struct {
				Role string `json:"role"`
			} `json:"input"`
		}
		require.NoError(t, json.Unmarshal(b, &body))
		assert.Equal(t, "be brief", body.Instructions)
		require.Len(t, body.Input, 4)
		assert.Equal(t, "system", body.Input[2].Role)
	})

	t.Run("anthropic keeps mid-conversation system role verbatim", func(t *testing.T) {
		b, err := json.Marshal(buildAnthropicMessageParams(p, false))
		require.NoError(t, err)
		var body struct {
			System   []anthropic.TextBlockParam `json:"system"`
			Messages []struct {
				Role string `json:"role"`
			} `json:"messages"`
		}
		require.NoError(t, json.Unmarshal(b, &body))
		require.Len(t, body.System, 1)
		assert.Equal(t, "be brief", body.System[0].Text)
		require.Len(t, body.Messages, 4)
		assert.Equal(t, "system", body.Messages[2].Role)
	})

	t.Run("default fixture unchanged without messages", func(t *testing.T) {
		b, _ := json.Marshal(buildOpenAIChatParams(probeParams{Model: "m", Message: "hello"}))
		assert.Contains(t, string(b), probeEchoInstruction)
		assert.Contains(t, string(b), `"hello"`)
	})
}

func TestBuildAnthropicMessageParams_AsClaudeCode(t *testing.T) {
	params := buildAnthropicMessageParams(probeParams{Model: "m", Message: "hi", Client: ClientClaudeCode}, false)
	b, err := json.Marshal(params)
	require.NoError(t, err)
	var body struct {
		System []struct {
			Text         string          `json:"text"`
			CacheControl json.RawMessage `json:"cache_control"`
		} `json:"system"`
		Metadata struct {
			UserID string `json:"user_id"`
		} `json:"metadata"`
	}
	require.NoError(t, json.Unmarshal(b, &body))
	require.Len(t, body.System, 3, "identity + billing + echo")
	assert.Contains(t, body.System[0].Text, "You are Claude Code")
	assert.Contains(t, string(body.System[0].CacheControl), "ephemeral")
	assert.True(t, strings.HasPrefix(body.System[1].Text, "x-anthropic-billing-header:"))
	assert.Equal(t, probeEchoInstruction, body.System[2].Text)

	meta := ops.ParseMetadataUserID(body.Metadata.UserID)
	require.NotNil(t, meta, "the Claude Code client requires a parseable metadata.user_id")
	assert.Len(t, meta.DeviceID, 64)
	assert.NotEmpty(t, meta.SessionID)

	// A plain probe (no client) has no metadata and no billing block.
	plain, _ := json.Marshal(buildAnthropicMessageParams(probeParams{Model: "m", Message: "hi"}, false))
	assert.NotContains(t, string(plain), "x-anthropic-billing-header")
	assert.NotContains(t, string(plain), "user_id")
}

// The cURL for a send-as-Claude-Code probe is captured from TB's own Claude
// Code client, so it carries that client's headers without any list being
// kept in the probe.
func TestBuildCurl_AsClaudeCode_ThroughTB(t *testing.T) {
	prober := newCurlTestProber(t)
	addProvider(t, prober.config, &typ.Provider{
		UUID:     "p-anthropic",
		Name:     "anth",
		APIStyle: protocol.APIStyleAnthropic,
		APIBase:  "https://api.example.com",
		Token:    "sk-upstream",
		Enabled:  true,
		Models:   []string{"claude-x"},
	})
	for _, stream := range []bool{false, true} {
		req := &E2ERequest{TargetType: E2ETargetProvider, ProviderUUID: "p-anthropic", Model: "claude-x", Client: ClientClaudeCode, Stream: boolPtr(stream), System: "be brief"}
		require.NoError(t, ValidateE2ERequest(req))
		curl, err := prober.BuildCurl(context.Background(), req)
		require.NoError(t, err)

		assert.Contains(t, curl.URL, "/tingly/anthropic/v1/messages")
		assert.Contains(t, curl.URL, "beta=true", "Claude Code adds the beta query")
		assert.Contains(t, curl.Headers["User-Agent"], "claude-cli/")
		assert.Equal(t, "cli", curl.Headers["X-App"])
		assert.Contains(t, curl.Headers["Anthropic-Beta"], "oauth-2025-04-20")
		assert.NotEmpty(t, curl.Headers["X-Claude-Code-Session-Id"], "set by the client guard from metadata.user_id")
		assert.Equal(t, "$TB_API_KEY", curl.Headers["X-Api-Key"], "gateway token placeholder")
		assert.Equal(t, "p-anthropic:claude-x", curl.Headers["X-Tingly-Probe-Service"], "transport-level pin still rendered")

		body := decodeBody(t, curl.Body)
		assert.Equal(t, stream, body["stream"] == true)
		system, _ := body["system"].([]any)
		require.Len(t, system, 3)
		assert.Contains(t, curl.Body, "x-anthropic-billing-header")
		assert.Contains(t, curl.Body, `"user_id"`)
		thinking, _ := body["thinking"].(map[string]any)
		assert.Equal(t, "disabled", thinking["type"], "Claude Code's default thinking shape, applied by the client guard")
		assert.Contains(t, curl.Command, "claude-cli/")
	}
}
