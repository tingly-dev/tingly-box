package tbclient

import (
	"context"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	serverconfig "github.com/tingly-dev/tingly-box/internal/server/config"
	"github.com/tingly-dev/tingly-box/internal/typ"
)

func TestNewTBClient(t *testing.T) {
	cfg := &serverconfig.Config{}

	// We can't easily mock the dependencies without interfaces,
	// so we'll test with nil values and check the constructor works
	client := NewTBClient(cfg, nil)

	assert.NotNil(t, client)
	assert.Equal(t, cfg, client.config)
}

func TestTBClient_BuildBaseURL(t *testing.T) {
	cfg := &serverconfig.Config{ServerPort: 8080}
	client := NewTBClient(cfg, nil)

	assert.Equal(t, "http://localhost:8080/tingly/claude_code", client.buildBaseURL())
}

func TestTBClient_BuildBaseURL_DefaultPort(t *testing.T) {
	cfg := &serverconfig.Config{} // ServerPort = 0
	client := NewTBClient(cfg, nil)

	assert.Equal(t, "http://localhost:12580/tingly/claude_code", client.buildBaseURL())
}

func TestProviderInfo_Structure(t *testing.T) {
	info := ProviderInfo{
		UUID:     "test-uuid",
		Name:     "test-provider",
		APIBase:  "https://api.test.com",
		APIStyle: "anthropic",
		Enabled:  true,
		Models:   []string{"model-1", "model-2"},
	}

	assert.Equal(t, "test-uuid", info.UUID)
	assert.Equal(t, "test-provider", info.Name)
	assert.Equal(t, "https://api.test.com", info.APIBase)
	assert.Equal(t, "anthropic", info.APIStyle)
	assert.True(t, info.Enabled)
	assert.Equal(t, []string{"model-1", "model-2"}, info.Models)
}

func TestModelSelectionRequest_Structure(t *testing.T) {
	req := ModelSelectionRequest{
		ProviderUUID: "prov-1",
		ModelID:      "model-1",
	}

	assert.Equal(t, "prov-1", req.ProviderUUID)
	assert.Equal(t, "model-1", req.ModelID)
}

func TestModelConfig_Structure(t *testing.T) {
	config := ModelConfig{
		ProviderUUID: "prov-1",
		ModelID:      "model-1",
		BaseURL:      "https://api.test.com",
		APIKey:       "test-key",
		APIStyle:     "anthropic",
	}

	assert.Equal(t, "prov-1", config.ProviderUUID)
	assert.Equal(t, "model-1", config.ModelID)
	assert.Equal(t, "https://api.test.com", config.BaseURL)
	assert.Equal(t, "test-key", config.APIKey)
	assert.Equal(t, "anthropic", config.APIStyle)
}

func TestConnectionConfig_Structure(t *testing.T) {
	config := ConnectionConfig{
		BaseURL: "http://localhost:12580/tingly/claude_code",
		APIKey:  "test-key",
	}

	assert.Equal(t, "http://localhost:12580/tingly/claude_code", config.BaseURL)
	assert.Equal(t, "test-key", config.APIKey)
}

func TestDefaultServiceConfig_Structure(t *testing.T) {
	config := DefaultServiceConfig{
		ProviderUUID: "prov-1",
		ProviderName: "Test Provider",
		ModelID:      "model-1",
		BaseURL:      "http://localhost:12580/tingly/claude_code",
		APIKey:       "test-key",
		APIStyle:     "anthropic",
	}

	assert.Equal(t, "prov-1", config.ProviderUUID)
	assert.Equal(t, "Test Provider", config.ProviderName)
	assert.Equal(t, "model-1", config.ModelID)
	assert.Equal(t, "http://localhost:12580/tingly/claude_code", config.BaseURL)
	assert.Equal(t, "test-key", config.APIKey)
	assert.Equal(t, "anthropic", config.APIStyle)
}

func TestTBClient_Types(t *testing.T) {
	// Test that TBClientImpl implements TBClient interface
	var _ TBClient = (*TBClientImpl)(nil)
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
	// No scenario config and no rules → unified mode with canonical fallback.
	client := NewTBClient(&serverconfig.Config{}, nil)

	models := client.resolveClaudeCodeModels()

	assert.Equal(t, "tingly/cc", models.def)
	assert.Equal(t, "tingly/cc", models.haiku)
	assert.Equal(t, "tingly/cc", models.sonnet)
	assert.Equal(t, "tingly/cc", models.opus)
	assert.Equal(t, "tingly/cc", models.subagent)
}

func TestResolveClaudeCodeModels_UnifiedFromRule(t *testing.T) {
	// Unified mode reads the built-in-cc rule's request_model for every tier.
	cfg := &serverconfig.Config{
		Rules: []typ.Rule{ccRule("built-in-cc", "tingly/cc")},
	}
	client := NewTBClient(cfg, nil)

	models := client.resolveClaudeCodeModels()

	assert.Equal(t, "tingly/cc", models.def)
	assert.Equal(t, "tingly/cc", models.opus)
	assert.Equal(t, "tingly/cc", models.subagent)
}

func TestResolveClaudeCodeModels_UnifiedCustomRequestModel(t *testing.T) {
	// A customized request_model on the unified rule must propagate to all tiers.
	cfg := &serverconfig.Config{
		Rules: []typ.Rule{ccRule("built-in-cc", "team/coder[1m]")},
	}
	client := NewTBClient(cfg, nil)

	models := client.resolveClaudeCodeModels()

	assert.Equal(t, "team/coder[1m]", models.def)
	assert.Equal(t, "team/coder[1m]", models.haiku)
	assert.Equal(t, "team/coder[1m]", models.sonnet)
	assert.Equal(t, "team/coder[1m]", models.opus)
	assert.Equal(t, "team/coder[1m]", models.subagent)
}

func TestResolveClaudeCodeModels_Separate(t *testing.T) {
	// Separate mode reads each tier from its own built-in rule's request_model.
	cfg := &serverconfig.Config{
		Scenarios: []typ.ScenarioConfig{ccSeparateFlag()},
		Rules: []typ.Rule{
			ccRule("built-in-cc-default", "tingly/cc-default"),
			ccRule("built-in-cc-haiku", "vendor/fast"),
			ccRule("built-in-cc-sonnet", "tingly/cc-sonnet"),
			ccRule("built-in-cc-opus", "vendor/smart"),
			ccRule("built-in-cc-subagent", "tingly/cc-subagent"),
		},
	}
	client := NewTBClient(cfg, nil)

	models := client.resolveClaudeCodeModels()

	assert.Equal(t, "tingly/cc-default", models.def)
	assert.Equal(t, "vendor/fast", models.haiku)
	assert.Equal(t, "tingly/cc-sonnet", models.sonnet)
	assert.Equal(t, "vendor/smart", models.opus)
	assert.Equal(t, "tingly/cc-subagent", models.subagent)
}

func TestResolveClaudeCodeModels_SeparateMissingTierFallsBack(t *testing.T) {
	// Separate mode with a missing tier rule falls back to the canonical name.
	cfg := &serverconfig.Config{
		Scenarios: []typ.ScenarioConfig{ccSeparateFlag()},
		Rules: []typ.Rule{
			ccRule("built-in-cc-default", "vendor/default"),
			// no haiku/sonnet/opus/subagent rules
		},
	}
	client := NewTBClient(cfg, nil)

	models := client.resolveClaudeCodeModels()

	assert.Equal(t, "vendor/default", models.def)
	assert.Equal(t, "tingly/cc-haiku", models.haiku)
	assert.Equal(t, "tingly/cc-sonnet", models.sonnet)
	assert.Equal(t, "tingly/cc-opus", models.opus)
	assert.Equal(t, "tingly/cc-subagent", models.subagent)
}

func TestResolveClaudeCodeModels_InactiveRuleIgnored(t *testing.T) {
	// An inactive built-in-cc rule must not override the canonical fallback.
	inactive := ccRule("built-in-cc", "should/not/use")
	inactive.Active = false
	cfg := &serverconfig.Config{Rules: []typ.Rule{inactive}}
	client := NewTBClient(cfg, nil)

	models := client.resolveClaudeCodeModels()

	assert.Equal(t, "tingly/cc", models.def)
}

func TestGetClaudeCodeEnv_RoutesThroughGateway(t *testing.T) {
	cfg := &serverconfig.Config{
		ServerPort: 9000,
		Rules:      []typ.Rule{ccRule("built-in-cc", "tingly/cc")},
	}
	client := NewTBClient(cfg, nil)

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
