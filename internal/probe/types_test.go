package probe

import (
	"encoding/json"
	"errors"
	"strings"
	"testing"

	"github.com/anthropics/anthropic-sdk-go"
	"github.com/openai/openai-go/v3"
	"github.com/openai/openai-go/v3/responses"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	protocol2 "github.com/tingly-dev/tingly-box/ai"
	"github.com/tingly-dev/tingly-box/internal/protocol"
)

func TestValidateE2ERequest(t *testing.T) {
	tests := []struct {
		name    string
		req     E2ERequest
		wantErr string // empty = expect no error; non-empty = expect this Field in the ValidationError
	}{
		{
			name: "rule target ok",
			req: E2ERequest{
				TargetType: E2ETargetRule,
				Scenario:   "anthropic",
				RuleUUID:   "rule-1",
				TestMode:   E2EModeSimple,
			},
		},
		{
			name: "rule target missing scenario",
			req: E2ERequest{
				TargetType: E2ETargetRule,
				RuleUUID:   "rule-1",
				TestMode:   E2EModeSimple,
			},
			wantErr: "scenario",
		},
		{
			name: "rule target missing rule_uuid",
			req: E2ERequest{
				TargetType: E2ETargetRule,
				Scenario:   "anthropic",
				TestMode:   E2EModeSimple,
			},
			wantErr: "rule_uuid",
		},
		{
			name: "provider target ok",
			req: E2ERequest{
				TargetType:   E2ETargetProvider,
				ProviderUUID: "p-1",
				Model:        "gpt-4",
				TestMode:     E2EModeStreaming,
			},
		},
		{
			name: "provider target missing provider_uuid",
			req: E2ERequest{
				TargetType: E2ETargetProvider,
				Model:      "gpt-4",
				TestMode:   E2EModeSimple,
			},
			wantErr: "provider_uuid",
		},
		{
			name: "provider target missing model",
			req: E2ERequest{
				TargetType:   E2ETargetProvider,
				ProviderUUID: "p-1",
				TestMode:     E2EModeSimple,
			},
			wantErr: "model",
		},
		{
			name: "provider_config target ok",
			req: E2ERequest{
				TargetType: E2ETargetProviderConfig,
				APIBase:    "https://api.openai.com/v1",
				APIStyle:   "openai",
				Token:      "sk-x",
				TestMode:   E2EModeTool,
			},
		},
		{
			name: "provider_config missing token",
			req: E2ERequest{
				TargetType: E2ETargetProviderConfig,
				APIBase:    "https://api.openai.com/v1",
				APIStyle:   "openai",
				TestMode:   E2EModeSimple,
			},
			wantErr: "token",
		},
		{
			name: "unknown target type",
			req: E2ERequest{
				TargetType: E2ETarget("nope"),
				TestMode:   E2EModeSimple,
			},
			wantErr: "target_type",
		},
		{
			name: "unknown test mode",
			req: E2ERequest{
				TargetType:   E2ETargetProvider,
				ProviderUUID: "p-1",
				Model:        "m",
				TestMode:     E2EMode("bogus"),
			},
			wantErr: "test_mode",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			err := ValidateE2ERequest(&tt.req)
			if tt.wantErr == "" {
				if err != nil {
					t.Fatalf("ValidateE2ERequest unexpected error: %v", err)
				}
				return
			}
			if err == nil {
				t.Fatalf("ValidateE2ERequest expected error for field %q, got nil", tt.wantErr)
			}
			var ve *ValidationError
			ok := errors.As(err, &ve)
			if !ok {
				t.Fatalf("ValidateE2ERequest returned %T, want *ValidationError", err)
			}
			if ve.Field != tt.wantErr {
				t.Errorf("ValidationError.Field = %q, want %q", ve.Field, tt.wantErr)
			}
		})
	}
}

func TestE2EMessage(t *testing.T) {
	if got := E2EMessage(E2EModeSimple, "custom!"); got != "custom!" {
		t.Errorf("custom override ignored: got %q", got)
	}
	if got := E2EMessage(E2EModeTool, ""); !strings.Contains(got, "bash tool") {
		t.Errorf("tool default should mention bash tool, got %q", got)
	}
	if got := E2EMessage(E2EModeSimple, ""); got == "" || strings.Contains(got, "bash tool") {
		t.Errorf("simple default should be a greeting, got %q", got)
	}
}

func TestScenarioEndpoint(t *testing.T) {
	tests := []struct {
		scenario string
		wantPath string
		wantAPI  protocol.APIStyle
	}{
		{"anthropic", "/tingly/anthropic", protocol.APIStyleAnthropic},
		{"claude_code", "/tingly/claude_code", protocol.APIStyleAnthropic},
		{"claude_code:p1", "/tingly/claude_code:p1", protocol.APIStyleAnthropic},
		{"opencode", "/tingly/opencode", protocol.APIStyleAnthropic},
		{"openai", "/tingly/openai", protocol.APIStyleOpenAI},
		{"unknown-scenario", "/tingly/unknown-scenario", protocol.APIStyleOpenAI},
	}
	for _, tt := range tests {
		t.Run(tt.scenario, func(t *testing.T) {
			gotPath, gotAPI := ScenarioEndpoint(tt.scenario)
			if gotPath != tt.wantPath {
				t.Errorf("ScenarioEndpoint(%q) path = %q, want %q", tt.scenario, gotPath, tt.wantPath)
			}
			if gotAPI != tt.wantAPI {
				t.Errorf("ScenarioEndpoint(%q) apiStyle = %q, want %q", tt.scenario, gotAPI, tt.wantAPI)
			}
		})
	}
}

func TestValidationErrorMessage(t *testing.T) {
	ve := &ValidationError{Field: "scenario", Message: "scenario is required"}
	if got := ve.Error(); got != "scenario: scenario is required" {
		t.Errorf("ValidationError.Error() = %q", got)
	}
}

// ---- toProbeResult ----

func TestToProbeResult_SetsSuccessAndUsage(t *testing.T) {
	// Canonical TokenUsage is passed through unchanged — no derived/renamed
	// fields. Input 10, output 5, cache-read 2.
	u := protocol2.NewTokenUsageFull(10, 5, 2, 0, 0)
	r := toProbeResult("body", 42, "https://x/y", false, u, nil)

	assert.True(t, r.Success, "toProbeResult must set Success=true")
	assert.Equal(t, int64(42), r.LatencyMs)
	assert.False(t, r.Stream)
	assert.Same(t, u, r.Usage, "Usage must be the canonical TokenUsage, passed through")
	assert.Equal(t, 10, r.Usage.InputTokens)
	assert.Equal(t, 5, r.Usage.OutputTokens)
	assert.Equal(t, 2, r.Usage.CacheReadTokens)
}

func TestToProbeResult_NilUsageStaysNil(t *testing.T) {
	r := toProbeResult("body", 1, "url", true, nil, nil)
	assert.True(t, r.Success)
	assert.Nil(t, r.Usage)
	assert.True(t, r.Stream)
}

// ---- tool-call extractors ----

func TestToolCallsFromOpenAIChat(t *testing.T) {
	// AsFunction() reads from the union's raw JSON, so construct via unmarshal
	// (struct literals don't populate the raw JSON the accessor needs).
	raw := `[{"id":"call_1","type":"function",
	          "function":{"name":"ls","arguments":"{\"dir\":\"/tmp\"}"}}]`
	var calls []openai.ChatCompletionMessageToolCallUnion
	require.NoError(t, json.Unmarshal([]byte(raw), &calls))
	msg := openai.ChatCompletionMessage{ToolCalls: calls}

	got := toolCallsFromOpenAIChat(msg)
	assert.Len(t, got, 1)
	assert.Equal(t, ToolCall{ID: "call_1", Name: "ls", Input: map[string]any{"dir": "/tmp"}}, got[0])
}

func TestToolCallsFromOpenAIChat_InvalidJSONBecomesEmptyInput(t *testing.T) {
	raw := `[{"type":"function","function":{"name":"ls","arguments":"not-json"}}]`
	var calls []openai.ChatCompletionMessageToolCallUnion
	require.NoError(t, json.Unmarshal([]byte(raw), &calls))
	msg := openai.ChatCompletionMessage{ToolCalls: calls}

	got := toolCallsFromOpenAIChat(msg)
	assert.Len(t, got, 1)
	assert.Equal(t, "ls", got[0].Name)
	assert.Empty(t, got[0].Input)
}

func TestToolCallsFromOpenAIResponses(t *testing.T) {
	// The Responses output-item union uses nested inline wrappers; build it via
	// JSON unmarshal, exactly as it arrives from the API.
	raw := `[{"type":"function_call","id":"fc_1","name":"get_weather","arguments":"{\"city\":\"SF\"}"},
	         {"type":"message","id":"msg_1"}]`
	var output []responses.ResponseOutputItemUnion
	require.NoError(t, json.Unmarshal([]byte(raw), &output))

	got := toolCallsFromOpenAIResponses(output)
	assert.Len(t, got, 1)
	assert.Equal(t, "get_weather", got[0].Name)
	assert.Equal(t, "SF", got[0].Input["city"])
}

func TestToolCallsFromAnthropic(t *testing.T) {
	raw := `[{"type":"tool_use","id":"tu_1","name":"list_dir","input":{"path":"/"}}]`
	var content []anthropic.ContentBlockUnion
	require.NoError(t, json.Unmarshal([]byte(raw), &content))

	got := toolCallsFromAnthropic(content)
	assert.Len(t, got, 1)
	assert.Equal(t, ToolCall{ID: "tu_1", Name: "list_dir", Input: map[string]any{"path": "/"}}, got[0])
}
