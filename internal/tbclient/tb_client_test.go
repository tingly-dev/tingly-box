package tbclient

import (
	"context"
	"encoding/json"
	"os"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	serverconfig "github.com/tingly-dev/tingly-box/internal/server/config"
	"github.com/tingly-dev/tingly-box/internal/typ"
)

func TestNewTBClient(t *testing.T) {
	cfg := &serverconfig.Config{}
	client := NewTBClient(cfg)

	assert.NotNil(t, client)
	assert.Equal(t, cfg, client.config)
}

func TestTBClient_Types(t *testing.T) {
	var _ TBClient = (*TBClientImpl)(nil)
	var _ TBClient = (*HTTPTBClient)(nil)
}

func TestNewHTTPTBClient(t *testing.T) {
	client := NewHTTPTBClient("http://localhost:12580", "user-token", "model-token")

	assert.NotNil(t, client)
	assert.Equal(t, "http://localhost:12580", client.baseURL)
	assert.Equal(t, "user-token", client.userToken)
	assert.Equal(t, "model-token", client.modelToken)
}

func TestHTTPTBClient_GetHTTPEndpointForScenario(t *testing.T) {
	client := NewHTTPTBClient("http://localhost:12580", "user-tok", "model-tok")

	cfg, err := client.GetHTTPEndpointForScenario(context.Background(), typ.ScenarioSmartGuide)
	require.NoError(t, err)
	assert.Equal(t, "http://localhost:12580/tingly/_smart_guide", cfg.BaseURL)
	assert.Equal(t, "model-tok", cfg.APIKey)

	cfg, err = client.GetHTTPEndpointForScenario(context.Background(), typ.ScenarioClaudeCode)
	require.NoError(t, err)
	assert.Equal(t, "http://localhost:12580/tingly/claude_code", cfg.BaseURL)
}

func TestHTTPTBClient_TrailingSlashTrimmed(t *testing.T) {
	client := NewHTTPTBClient("http://localhost:12580/", "tok", "mtok")
	assert.Equal(t, "http://localhost:12580", client.baseURL)
}

func TestHTTPTBClient_EmptyProfileID(t *testing.T) {
	client := NewHTTPTBClient("http://localhost:12580", "tok", "mtok")
	path, err := client.GetClaudeCodeSettingsPathForProfile(context.Background(), "")
	require.NoError(t, err)
	assert.Empty(t, path)
}

func TestSmartGuideRuleUUID(t *testing.T) {
	uuid := serverconfig.SmartGuideRuleUUID("bot-123")
	assert.Equal(t, "_internal_smart_guide_bot-123", uuid)
}

// ccRule builds an active claude_code rule with the given UUID and request model.
func ccRule(uuid, requestModel string) typ.Rule {
	return typ.Rule{
		UUID:         uuid,
		Scenario:     typ.ScenarioClaudeCode,
		RequestModel: requestModel,
		Active:       true,
	}
}

// ccSeparateFlag returns a scenario config that puts claude_code in separate mode.
func ccSeparateFlag() typ.ScenarioConfig {
	return typ.ScenarioConfig{
		Scenario: typ.ScenarioClaudeCode,
		Flags:    typ.ScenarioFlags{Separate: true},
	}
}

func TestResolveClaudeCodeModels_UnifiedDefault(t *testing.T) {
	client := NewTBClient(&serverconfig.Config{})
	models := client.resolveClaudeCodeModels()

	assert.Equal(t, "tingly/cc", models.def)
	assert.Equal(t, "tingly/cc", models.haiku)
	assert.Equal(t, "tingly/cc", models.sonnet)
	assert.Equal(t, "tingly/cc", models.opus)
	assert.Equal(t, "tingly/cc", models.subagent)
}

func TestResolveClaudeCodeModels_UnifiedFromRule(t *testing.T) {
	cfg := &serverconfig.Config{
		Rules: []typ.Rule{ccRule("built-in-cc", "tingly/cc")},
	}
	client := NewTBClient(cfg)
	models := client.resolveClaudeCodeModels()

	assert.Equal(t, "tingly/cc", models.def)
	assert.Equal(t, "tingly/cc", models.opus)
	assert.Equal(t, "tingly/cc", models.subagent)
}

func TestResolveClaudeCodeModels_UnifiedCustomRequestModel(t *testing.T) {
	cfg := &serverconfig.Config{
		Rules: []typ.Rule{ccRule("built-in-cc", "team/coder[1m]")},
	}
	client := NewTBClient(cfg)
	models := client.resolveClaudeCodeModels()

	assert.Equal(t, "team/coder[1m]", models.def)
	assert.Equal(t, "team/coder[1m]", models.haiku)
	assert.Equal(t, "team/coder[1m]", models.sonnet)
	assert.Equal(t, "team/coder[1m]", models.opus)
	assert.Equal(t, "team/coder[1m]", models.subagent)
}

func TestResolveClaudeCodeModels_Separate(t *testing.T) {
	cfg := &serverconfig.Config{
		Scenarios: []typ.ScenarioConfig{ccSeparateFlag()},
		Rules: []typ.Rule{
			ccRule("builtin:claude_code:default", "tingly/cc-default"),
			ccRule("builtin:claude_code:haiku", "vendor/fast"),
			ccRule("builtin:claude_code:sonnet", "tingly/cc-sonnet"),
			ccRule("builtin:claude_code:opus", "vendor/smart"),
			ccRule("builtin:claude_code:subagent", "tingly/cc-subagent"),
		},
	}
	client := NewTBClient(cfg)
	models := client.resolveClaudeCodeModels()

	assert.Equal(t, "tingly/cc-default", models.def)
	assert.Equal(t, "vendor/fast", models.haiku)
	assert.Equal(t, "tingly/cc-sonnet", models.sonnet)
	assert.Equal(t, "vendor/smart", models.opus)
	assert.Equal(t, "tingly/cc-subagent", models.subagent)
}

func TestResolveClaudeCodeModels_SeparateMissingTierFallsBack(t *testing.T) {
	cfg := &serverconfig.Config{
		Scenarios: []typ.ScenarioConfig{ccSeparateFlag()},
		Rules: []typ.Rule{
			ccRule("built-in-cc-default", "vendor/default"),
		},
	}
	client := NewTBClient(cfg)
	models := client.resolveClaudeCodeModels()

	assert.Equal(t, "vendor/default", models.def)
	assert.Equal(t, "tingly/cc-haiku", models.haiku)
	assert.Equal(t, "tingly/cc-sonnet", models.sonnet)
	assert.Equal(t, "tingly/cc-opus", models.opus)
	assert.Equal(t, "tingly/cc-subagent", models.subagent)
}

func TestResolveClaudeCodeModels_ModernUUIDWinsOverLegacy(t *testing.T) {
	cfg := &serverconfig.Config{
		Rules: []typ.Rule{
			ccRule("builtin:claude_code:cc", "modern/model"),
			ccRule("built-in-cc", "legacy/model"),
		},
	}
	client := NewTBClient(cfg)
	models := client.resolveClaudeCodeModels()

	assert.Equal(t, "modern/model", models.def)
}

func TestResolveClaudeCodeModels_Context1MSuffix(t *testing.T) {
	flagged := ccRule("builtin:claude_code:cc", "tingly/cc")
	flagged.Flags.Context1M = true
	cfg := &serverconfig.Config{Rules: []typ.Rule{flagged}}
	client := NewTBClient(cfg)

	models := client.resolveClaudeCodeModels()
	assert.Equal(t, "tingly/cc[1m]", models.def)

	suffixed := ccRule("builtin:claude_code:cc", "team/coder[1m]")
	suffixed.Flags.Context1M = true
	cfg2 := &serverconfig.Config{Rules: []typ.Rule{suffixed}}
	models2 := NewTBClient(cfg2).resolveClaudeCodeModels()
	assert.Equal(t, "team/coder[1m]", models2.def)
}

func TestResolveClaudeCodeModels_InactiveRuleIgnored(t *testing.T) {
	inactive := ccRule("built-in-cc", "should/not/use")
	inactive.Active = false
	cfg := &serverconfig.Config{Rules: []typ.Rule{inactive}}
	client := NewTBClient(cfg)

	models := client.resolveClaudeCodeModels()
	assert.Equal(t, "tingly/cc", models.def)
}

func TestGetClaudeCodeEnv_RoutesThroughGateway(t *testing.T) {
	cfg := &serverconfig.Config{
		ServerPort: 9000,
		Rules:      []typ.Rule{ccRule("built-in-cc", "tingly/cc")},
	}
	client := NewTBClient(cfg)

	env, err := client.GetClaudeCodeEnv(context.Background())
	require.NoError(t, err)

	kv := map[string]string{}
	for _, e := range env {
		for i := 0; i < len(e); i++ {
			if e[i] == '=' {
				kv[e[:i]] = e[i+1:]
				break
			}
		}
	}

	assert.Equal(t, "http://localhost:9000/tingly/claude_code", kv["ANTHROPIC_BASE_URL"])
	assert.Equal(t, "tingly/cc", kv["ANTHROPIC_MODEL"])
	assert.Equal(t, "tingly/cc", kv["ANTHROPIC_DEFAULT_OPUS_MODEL"])
	_, hasToken := kv["ANTHROPIC_AUTH_TOKEN"]
	assert.True(t, hasToken)
}

func TestGetClaudeCodeSettingsPathForProfile_EmptyIDReturnsNoPath(t *testing.T) {
	cfg := &serverconfig.Config{ServerPort: 9000}
	client := NewTBClient(cfg)

	path, err := client.GetClaudeCodeSettingsPathForProfile(context.Background(), "")
	require.NoError(t, err)
	assert.Empty(t, path)
}

func TestGetClaudeCodeSettingsPathForProfile_UnknownProfile(t *testing.T) {
	cfg := &serverconfig.Config{ServerPort: 9000}
	client := NewTBClient(cfg)

	_, err := client.GetClaudeCodeSettingsPathForProfile(context.Background(), "ghost")
	require.Error(t, err)
	assert.Contains(t, err.Error(), "ghost")
}

func TestGetClaudeCodeSettingsPathForProfile_MaterializesProfileSettings(t *testing.T) {
	home := t.TempDir()
	t.Setenv("HOME", home)
	t.Setenv("USERPROFILE", home)

	profiledRule := ccRule(serverconfig.BuiltinRuleUUID(typ.RuleScenario("claude_code:p1"), "cc"), "team/coder")
	profiledRule.Scenario = "claude_code:p1"
	cfg := &serverconfig.Config{
		ServerPort: 9000,
		Profiles: map[string][]typ.ProfileMeta{
			string(typ.ScenarioClaudeCode): {{ID: "p1", Name: "work", Unified: true}},
		},
		Rules: []typ.Rule{profiledRule},
	}
	client := NewTBClient(cfg)

	path, err := client.GetClaudeCodeSettingsPathForProfile(context.Background(), "p1")
	require.NoError(t, err)
	require.NotEmpty(t, path)

	data, err := os.ReadFile(path)
	require.NoError(t, err)

	var written struct {
		Env map[string]string `json:"env"`
	}
	require.NoError(t, json.Unmarshal(data, &written))

	assert.Equal(t, "http://localhost:9000/tingly/claude_code:p1", written.Env["ANTHROPIC_BASE_URL"])
	assert.Equal(t, "team/coder", written.Env["ANTHROPIC_MODEL"])
}

func TestGetScenarioEndpointPath(t *testing.T) {
	tests := []struct {
		scenario typ.RuleScenario
		want     string
	}{
		{typ.ScenarioClaudeCode, "/tingly/claude_code"},
		{"claude_code:p1", "/tingly/claude_code"},
		{"claude_code:profile-abc", "/tingly/claude_code"},
		{typ.ScenarioOpenCode, "/tingly/opencode"},
		{"opencode:p1", "/tingly/opencode"},
		{typ.ScenarioXcode, "/tingly/xcode"},
		{typ.ScenarioVSCode, "/tingly/vscode"},
		{typ.ScenarioTeam, "/tingly/team"},
		{"team:p1", "/tingly/team"},
		{typ.ScenarioSmartGuide, "/tingly/_smart_guide"},
		{typ.ScenarioOpenAI, "/tingly/openai"},
	}
	for _, tt := range tests {
		got := GetScenarioEndpointPath(tt.scenario)
		if got != tt.want {
			t.Errorf("GetScenarioEndpointPath(%q) = %q, want %q", tt.scenario, got, tt.want)
		}
	}
}
