package tui

import (
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	serverconfig "github.com/tingly-dev/tingly-box/internal/config"
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

	require.NoError(t, applyClaudeCodeChoice(cfg, "p", "small-model"))

	assert.Equal(t, []string{"subagent"}, cfg.ClaudeCodeSlots(typ.ScenarioClaudeCode))
	rule := cfg.GetRuleByUUID(serverconfig.RuleUUIDCCSubagent)
	require.Len(t, rule.Services, 1)
	assert.Equal(t, "small-model", rule.Services[0].Model)
	assert.True(t, rule.Active)
	assert.False(t, rule.SmartEnabled)
	assert.False(t, rule.Flags.Context1M)

	// Rerun, "Same as the default model": the split is undone.
	require.NoError(t, applyClaudeCodeChoice(cfg, "p", ""))
	assert.Empty(t, cfg.ClaudeCodeSlots(typ.ScenarioClaudeCode))
}

func TestApplyClaudeCodeChoice_KeepsOtherSlots(t *testing.T) {
	cfg := newQuickstartTestConfig(t)
	_, err := cfg.SetClaudeCodeSlot(typ.ScenarioClaudeCode, "haiku", true)
	require.NoError(t, err)

	require.NoError(t, applyClaudeCodeChoice(cfg, "p", "small-model"))
	assert.Equal(t, []string{"haiku", "subagent"}, cfg.ClaudeCodeSlots(typ.ScenarioClaudeCode))

	require.NoError(t, applyClaudeCodeChoice(cfg, "p", ""))
	assert.Equal(t, []string{"haiku"}, cfg.ClaudeCodeSlots(typ.ScenarioClaudeCode))
}

func TestIsClaudeCodeTierRule(t *testing.T) {
	cfg := newQuickstartTestConfig(t)
	// Wiring up a tier rule must not switch it on: that would
	// give its slot a rule of its own.
	assert.True(t, isClaudeCodeTierRule(cfg.GetRuleByUUID(serverconfig.RuleUUIDCCSubagent)))
	assert.True(t, isClaudeCodeTierRule(cfg.GetRuleByUUID(serverconfig.RuleUUIDCCHaiku)))
	assert.False(t, isClaudeCodeTierRule(cfg.GetRuleByUUID(serverconfig.RuleUUIDCC)))
	assert.False(t, isClaudeCodeTierRule(cfg.GetRuleByUUID(serverconfig.RuleUUIDOpenCode)))
}
