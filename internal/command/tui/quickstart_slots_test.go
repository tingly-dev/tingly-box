package tui

import (
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	serverconfig "github.com/tingly-dev/tingly-box/internal/config"
	"github.com/tingly-dev/tingly-box/internal/constant"
	"github.com/tingly-dev/tingly-box/internal/loadbalance"
	"github.com/tingly-dev/tingly-box/internal/protocol"
	"github.com/tingly-dev/tingly-box/internal/typ"
)

func newQuickstartTestConfig(t *testing.T) *serverconfig.Config {
	t.Helper()
	cfg, err := serverconfig.NewConfigWithDir(t.TempDir())
	require.NoError(t, err)
	require.NoError(t, cfg.AddProvider(&typ.Provider{
		UUID: "p", Name: "p", APIBase: "https://api.example.com",
		APIStyle: protocol.APIStyleAnthropic, AuthType: typ.AuthTypeAPIKey, Token: "t", Enabled: true,
	}))
	return cfg
}

func TestApplyClaudeCodeChoice_SubagentModel(t *testing.T) {
	cfg := newQuickstartTestConfig(t)
	// The main rule has smart routing and 1M; the subagent must get exactly
	// the chosen model.
	main := cfg.GetRuleByUUID(serverconfig.RuleUUIDCC)
	main.Services = []*loadbalance.Service{{Active: true, Provider: "p", Model: "big"}}
	main.SmartEnabled = true
	main.Flags.Context1M = true
	require.NoError(t, cfg.UpdateRule(main.UUID, *main))

	require.NoError(t, applyClaudeCodeChoice(cfg, true, "p", "small-model"))

	assert.Equal(t, []string{"subagent"}, cfg.ClaudeCodeSlots(typ.ScenarioClaudeCode))
	rule := cfg.GetRuleByUUID(serverconfig.RuleUUIDCCSubagent)
	require.Len(t, rule.Services, 1)
	assert.Equal(t, "small-model", rule.Services[0].Model)
	assert.True(t, rule.Active)
	assert.False(t, rule.SmartEnabled)
	assert.False(t, rule.Flags.Context1M)

	// Rerun, "Same as the default model": the split is undone.
	require.NoError(t, applyClaudeCodeChoice(cfg, true, "p", ""))
	assert.Empty(t, cfg.ClaudeCodeSlots(typ.ScenarioClaudeCode))
}

func TestApplyClaudeCodeChoice_AlignsMode(t *testing.T) {
	cfg := newQuickstartTestConfig(t)
	require.NoError(t, cfg.SetScenarioFlag(typ.ScenarioClaudeCode, constant.FlagSeparate, true))

	// Unified chosen on a separate scenario: the scenario follows, so the
	// subagent can be split and the unified env routes.
	require.NoError(t, applyClaudeCodeChoice(cfg, true, "p", "small-model"))
	assert.False(t, cfg.GetScenarioFlag(typ.ScenarioClaudeCode, constant.FlagSeparate))
	assert.True(t, cfg.GetRuleByUUID(serverconfig.RuleUUIDCC).Active)
	assert.Equal(t, []string{"subagent"}, cfg.ClaudeCodeSlots(typ.ScenarioClaudeCode))

	require.NoError(t, applyClaudeCodeChoice(cfg, false, "p", ""))
	assert.True(t, cfg.GetScenarioFlag(typ.ScenarioClaudeCode, constant.FlagSeparate))
}
