package probe

import (
	"context"
	"encoding/json"
	"strings"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/tingly-dev/tingly-box/internal/protocol"
	"github.com/tingly-dev/tingly-box/internal/typ"
)

// ---- ResolveAxes: orthogonal Stream/Tool ----

func boolPtr(b bool) *bool { return &b }

func TestResolveAxes(t *testing.T) {
	cases := []struct {
		name       string
		stream     *bool
		tool       *bool
		wantStream bool
		wantTool   bool
	}{
		{"stream", boolPtr(true), nil, true, false},
		{"tool", nil, boolPtr(true), false, true},
		{"tool+stream", boolPtr(true), boolPtr(true), true, true},
		{"absent", nil, nil, false, false},
		{"explicit false", boolPtr(false), boolPtr(false), false, false},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			req := &E2ERequest{Stream: tc.stream, Tool: tc.tool}
			gotStream, gotTool := req.ResolveAxes()
			assert.Equal(t, tc.wantStream, gotStream)
			assert.Equal(t, tc.wantTool, gotTool)
		})
	}
}

// ---- Validation of the new fields ----

func TestValidateE2ERequest_NewAxes(t *testing.T) {
	base := func() *E2ERequest {
		return &E2ERequest{
			TargetType:   E2ETargetProvider,
			ProviderUUID: "p-1",
			Model:        "m",
			Stream:       boolPtr(true),
			Tool:         boolPtr(false),
		}
	}

	t.Run("axes only, is valid", func(t *testing.T) {
		assert.NoError(t, ValidateE2ERequest(base()))
	})

	t.Run("invalid protocol rejected", func(t *testing.T) {
		req := base()
		req.Protocol = ProbeProtocol("google")
		err := ValidateE2ERequest(req)
		require.Error(t, err)
		assert.Contains(t, err.Error(), "protocol")
	})

	t.Run("any protocol accepted for rule targets", func(t *testing.T) {
		// The scenario only picks the default; whether TB serves the
		// protocol is the probe's answer, not a validation error.
		for _, scenario := range []string{"openai", "claude_code:p1", "team"} {
			for _, p := range []ProbeProtocol{ProtocolOpenAIChat, ProtocolOpenAIResponses, ProtocolAnthropic} {
				assert.NoError(t, ValidateE2ERequest(&E2ERequest{
					TargetType: E2ETargetRule,
					Scenario:   scenario,
					RuleUUID:   "r-1",
					Protocol:   p,
				}), "scenario %s protocol %s", scenario, p)
			}
		}
	})

	t.Run("all protocol values valid for provider targets", func(t *testing.T) {
		for _, p := range []ProbeProtocol{ProtocolOpenAIChat, ProtocolOpenAIResponses, ProtocolAnthropic} {
			req := base()
			req.Protocol = p
			assert.NoError(t, ValidateE2ERequest(req), "protocol %s", p)
		}
	})
}

func TestResolveOpenAIEndpointOverride(t *testing.T) {
	req := &E2ERequest{Protocol: ProtocolOpenAIResponses}
	assert.Equal(t, "responses", req.ResolveOpenAIEndpointOverride())

	req = &E2ERequest{Protocol: ProtocolOpenAIChat}
	assert.Equal(t, "chat", req.ResolveOpenAIEndpointOverride())

	req = &E2ERequest{Protocol: ProtocolAnthropic}
	assert.Equal(t, "", req.ResolveOpenAIEndpointOverride(),
		"anthropic protocol implies no openai endpoint override")

	req = &E2ERequest{}
	assert.Equal(t, "", req.ResolveOpenAIEndpointOverride(), "no protocol implies no override")
}

// ---- BuildCurl golden tests ----

func newCurlTestProber(t *testing.T) *E2EProber {
	t.Helper()
	cfg := newTestConfig(t)
	cfg.ServerPort = 18080
	cfg.ModelToken = "test-token"
	return &E2EProber{config: cfg}
}

func decodeBody(t *testing.T, raw string) map[string]any {
	t.Helper()
	var m map[string]any
	require.NoError(t, json.Unmarshal([]byte(raw), &m))
	return m
}

func TestBuildCurl_ThroughTB_OpenAIChat_Stream(t *testing.T) {
	svc := newCurlTestProber(t)
	addProvider(t, svc.config, &typ.Provider{
		UUID:     "p-openai",
		Name:     "OpenAI",
		APIBase:  "https://api.openai.com/v1",
		APIStyle: protocol.APIStyleOpenAI,
		Enabled:  true,
		Models:   []string{"gpt-4"},
	})

	curl, err := svc.BuildCurl(context.Background(), &E2ERequest{
		TargetType:   E2ETargetProvider,
		ProviderUUID: "p-openai",
		Model:        "gpt-4",
		Stream:       boolPtr(true),
	})
	require.NoError(t, err)

	assert.Equal(t, "http://localhost:18080/tingly/openai/chat/completions", curl.URL)
	assert.Equal(t, "$TB_API_KEY", curl.KeyEnvVar)
	assert.Equal(t, "Bearer $TB_API_KEY", curl.Headers["Authorization"])

	body := decodeBody(t, curl.Body)
	assert.Equal(t, true, body["stream"], "streaming body must carry stream:true (SDK adds it via WithJSONSet)")
	assert.Equal(t, "gpt-4", body["model"])
	streamOptions, ok := body["stream_options"].(map[string]any)
	require.True(t, ok, "streaming chat must request include_usage, body: %s", curl.Body)
	assert.Equal(t, true, streamOptions["include_usage"])

	assert.Contains(t, curl.Command, curl.URL)
	// URL leads: it must appear before the first header flag.
	assert.Less(t, strings.Index(curl.Command, curl.URL), strings.Index(curl.Command, "-H "),
		"URL must lead the command, got: %s", curl.Command)
	// Single-quoted payload: JSON reads verbatim (no \" escapes) and is
	// pretty-printed under -d.
	assert.Contains(t, curl.Command, "-d '{")
	assert.Contains(t, curl.Command, `"model": "gpt-4"`)
	assert.NotContains(t, curl.Command, `\"`, "payload should not be backslash-escaped, got: %s", curl.Command)
	assert.Contains(t, curl.Command, "$TB_API_KEY")
}

func TestBuildCurl_ThroughTB_Anthropic_NonStream_Thinking(t *testing.T) {
	svc := newCurlTestProber(t)
	addProvider(t, svc.config, &typ.Provider{
		UUID:     "p-anthropic",
		Name:     "Anthropic",
		APIBase:  "https://api.anthropic.com",
		APIStyle: protocol.APIStyleAnthropic,
		Enabled:  true,
		Models:   []string{"claude-3-5-sonnet-20241022"},
	})

	curl, err := svc.BuildCurl(context.Background(), &E2ERequest{
		TargetType:   E2ETargetProvider,
		ProviderUUID: "p-anthropic",
		Model:        "claude-3-5-sonnet-20241022",
		Stream:       boolPtr(false),
		Thinking:     ThinkingHigh,
	})
	require.NoError(t, err)

	assert.Equal(t, "http://localhost:18080/tingly/anthropic/v1/messages", curl.URL)
	assert.Equal(t, "$TB_API_KEY", curl.Headers["x-api-key"])
	assert.Equal(t, "2023-06-01", curl.Headers["anthropic-version"])

	body := decodeBody(t, curl.Body)
	assert.NotContains(t, body, "stream", "non-stream body must not carry a stream member")
	// Thinking raises max_tokens above the budget (budget < max_tokens).
	maxTokens, ok := body["max_tokens"].(float64)
	require.True(t, ok)
	assert.Greater(t, maxTokens, float64(1024), "thinking must raise max_tokens above the default, body: %s", curl.Body)
	thinking, ok := body["thinking"].(map[string]any)
	require.True(t, ok, "thinking param must be present, body: %s", curl.Body)
	assert.Equal(t, "enabled", thinking["type"])
}

func TestBuildCurl_Direct_OpenAIResponses_ProtocolOverride(t *testing.T) {
	svc := newCurlTestProber(t)
	addProvider(t, svc.config, &typ.Provider{
		UUID:     "p-openai",
		Name:     "OpenAI",
		APIBase:  "https://api.openai.com/v1",
		APIStyle: protocol.APIStyleOpenAI,
		Enabled:  true,
		Models:   []string{"gpt-4"},
	})

	curl, err := svc.BuildCurl(context.Background(), &E2ERequest{
		TargetType:   E2ETargetProvider,
		ProviderUUID: "p-openai",
		Model:        "gpt-4",
		Direct:       true,
		Protocol:     ProtocolOpenAIResponses,
	})
	require.NoError(t, err)

	assert.Equal(t, "https://api.openai.com/v1/responses", curl.URL)
	assert.Equal(t, "$UPSTREAM_API_KEY", curl.KeyEnvVar)

	body := decodeBody(t, curl.Body)
	assert.Equal(t, "gpt-4", body["model"])
	assert.NotContains(t, body, "stream", "non-stream responses body must not carry stream")
}

func TestBuildCurl_Direct_DualBase_ProtocolOverride_SelectsAnthropicURL(t *testing.T) {
	svc := newCurlTestProber(t)
	addProvider(t, svc.config, &typ.Provider{
		UUID:             "p-dual",
		Name:             "Dual",
		APIBase:          "https://primary.example.com/v1",
		APIBaseOpenAI:    "https://openai.example.com/v1",
		APIBaseAnthropic: "https://anthropic.example.com",
		APIStyle:         protocol.APIStyleOpenAI,
		Enabled:          true,
		Models:           []string{"m-1"},
	})

	curl, err := svc.BuildCurl(context.Background(), &E2ERequest{
		TargetType:   E2ETargetProvider,
		ProviderUUID: "p-dual",
		Model:        "m-1",
		Direct:       true,
		Protocol:     ProtocolAnthropic,
	})
	require.NoError(t, err)

	assert.Equal(t, "https://anthropic.example.com/v1/messages", curl.URL)
	assert.Equal(t, "$UPSTREAM_API_KEY", curl.Headers["x-api-key"])
}

func TestBuildCurl_Tool_SendsToolsAndToolChoice(t *testing.T) {
	svc := newCurlTestProber(t)
	addProvider(t, svc.config, &typ.Provider{
		UUID:     "p-openai",
		Name:     "OpenAI",
		APIBase:  "https://api.openai.com/v1",
		APIStyle: protocol.APIStyleOpenAI,
		Enabled:  true,
		Models:   []string{"gpt-4"},
	})

	curl, err := svc.BuildCurl(context.Background(), &E2ERequest{
		TargetType:   E2ETargetProvider,
		ProviderUUID: "p-openai",
		Model:        "gpt-4",
		Direct:       true,
		Tool:         boolPtr(true),
	})
	require.NoError(t, err)

	body := decodeBody(t, curl.Body)
	tools, ok := body["tools"].([]any)
	require.True(t, ok, "tool body must carry tool definitions, body: %s", curl.Body)
	assert.NotEmpty(t, tools)
	assert.Equal(t, "auto", body["tool_choice"])
	// The default tool message asks for a bash invocation.
	messages := body["messages"].([]any)
	last := messages[len(messages)-1].(map[string]any)
	content := last["content"].(string)
	assert.True(t, strings.Contains(content, "bash"), "default tool message should mention bash, got: %s", content)
}

func TestBuildCurl_Google_Rejected(t *testing.T) {
	svc := newCurlTestProber(t)
	addProvider(t, svc.config, &typ.Provider{
		UUID:     "p-google",
		Name:     "Google",
		APIBase:  "https://generativelanguage.googleapis.com",
		APIStyle: protocol.APIStyleGoogle,
		Enabled:  true,
		Models:   []string{"gemini-2.0-flash"},
	})

	_, err := svc.BuildCurl(context.Background(), &E2ERequest{
		TargetType:   E2ETargetProvider,
		ProviderUUID: "p-google",
		Model:        "gemini-2.0-flash",
	})
	require.Error(t, err)
	assert.Contains(t, err.Error(), "Google")
}

func TestBuildCurl_RuleTarget_ThroughTB_AnthropicScenario(t *testing.T) {
	svc := newCurlTestProber(t)
	require.NoError(t, svc.config.AddRule(typ.Rule{
		UUID:         "r-1",
		Scenario:     "anthropic",
		RequestModel: "claude-3-5-sonnet-20241022",
	}))

	curl, err := svc.BuildCurl(context.Background(), &E2ERequest{
		TargetType: E2ETargetRule,
		Scenario:   "anthropic",
		RuleUUID:   "r-1",
	})
	require.NoError(t, err)

	assert.Equal(t, "http://localhost:18080/tingly/anthropic/v1/messages", curl.URL)
	assert.Equal(t, "$TB_API_KEY", curl.KeyEnvVar)
}

// A rule is probed on the protocol the caller picks; without one it keeps
// its scenario's default (Chat for team, Anthropic for claude_code).
func TestBuildCurl_RuleTarget_TeamProtocolOverride(t *testing.T) {
	svc := newCurlTestProber(t)
	require.NoError(t, svc.config.AddRule(typ.Rule{
		UUID:         "r-team",
		Scenario:     "team",
		RequestModel: "shared-model",
	}))

	cases := map[ProbeProtocol]string{
		"":                      "http://localhost:18080/tingly/team/chat/completions",
		ProtocolOpenAIChat:      "http://localhost:18080/tingly/team/chat/completions",
		ProtocolOpenAIResponses: "http://localhost:18080/tingly/team/responses",
		ProtocolAnthropic:       "http://localhost:18080/tingly/team/v1/messages",
	}
	for p, wantURL := range cases {
		curl, err := svc.BuildCurl(context.Background(), &E2ERequest{
			TargetType: E2ETargetRule,
			Scenario:   "team",
			RuleUUID:   "r-team",
			Protocol:   p,
		})
		require.NoError(t, err, "protocol %q", p)
		assert.Equal(t, wantURL, curl.URL, "protocol %q", p)
	}
}

func TestBuildCurl_RuleTarget_AnthropicScenarioOnOpenAI(t *testing.T) {
	svc := newCurlTestProber(t)
	require.NoError(t, svc.config.AddRule(typ.Rule{
		UUID:         "r-cc",
		Scenario:     "anthropic",
		RequestModel: "claude-x",
	}))

	curl, err := svc.BuildCurl(context.Background(), &E2ERequest{
		TargetType: E2ETargetRule,
		Scenario:   "anthropic",
		RuleUUID:   "r-cc",
		Protocol:   ProtocolOpenAIChat,
	})
	require.NoError(t, err)
	assert.Equal(t, "http://localhost:18080/tingly/anthropic/chat/completions", curl.URL)
	assert.Equal(t, "Bearer $TB_API_KEY", curl.Headers["Authorization"])
}
