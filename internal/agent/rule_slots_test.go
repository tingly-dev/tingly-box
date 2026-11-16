package agent

import (
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	serverconfig "github.com/tingly-dev/tingly-box/internal/config"
	"github.com/tingly-dev/tingly-box/internal/loadbalance"
	"github.com/tingly-dev/tingly-box/internal/protocol"
	"github.com/tingly-dev/tingly-box/internal/typ"
)

func newApplyTestConfig(t *testing.T) (*serverconfig.Config, string) {
	t.Helper()
	cfg, err := serverconfig.NewConfigWithDir(t.TempDir())
	require.NoError(t, err)
	for _, name := range []string{"main", "cheap"} {
		require.NoError(t, cfg.AddProvider(&typ.Provider{
			UUID: name, Name: name, APIBase: "https://api.example.com",
			APIStyle: protocol.APIStyleAnthropic, AuthType: typ.AuthTypeAPIKey, Token: "t", Enabled: true,
		}))
	}
	return cfg, "main"
}

func TestApplyClaudeCodeRules_UnifiedKeepsSlots(t *testing.T) {
	cfg, provider := newApplyTestConfig(t)
	_, err := cfg.SetClaudeCodeSlot(typ.ScenarioClaudeCode, "subagent", true)
	require.NoError(t, err)
	sub := cfg.GetRuleByUUID(serverconfig.RuleUUIDCCSubagent)
	sub.Services = []*loadbalance.Service{{Provider: "cheap", Model: "small", Active: true}}
	require.NoError(t, cfg.UpdateRule(sub.UUID, *sub))

	_, updated, err := NewAgentApply(cfg, "localhost").createOrUpdateClaudeCodeRules(provider, "big")
	require.NoError(t, err)
	assert.Equal(t, 5, updated, "the main rule and every slot rule but the split subagent")

	main := cfg.GetRuleByUUID(serverconfig.RuleUUIDCC)
	assert.Equal(t, "big", main.Services[0].Model)
	assert.Equal(t, "small", cfg.GetRuleByUUID(serverconfig.RuleUUIDCCSubagent).Services[0].Model, "a split slot keeps its own model")
	assert.False(t, cfg.GetRuleByUUID(serverconfig.RuleUUIDCCHaiku).Active, "slots without their own rule stay off")
	assert.Equal(t, []string{"subagent"}, cfg.ClaudeCodeSlots(typ.ScenarioClaudeCode))
}

func TestApplyClaudeCodeRules_AllSlotsSplitKeepsEveryModel(t *testing.T) {
	cfg, provider := newApplyTestConfig(t)
	require.NoError(t, cfg.SetAllClaudeCodeSlots(typ.ScenarioClaudeCode, true))
	for _, slot := range serverconfig.CCSlots {
		r := cfg.GetRuleByUUID(cfg.ClaudeCodeRuleUUID(typ.ScenarioClaudeCode, slot))
		r.Services = []*loadbalance.Service{{Provider: "cheap", Model: "own-" + slot, Active: true}}
		require.NoError(t, cfg.UpdateRule(r.UUID, *r))
	}

	_, _, err := NewAgentApply(cfg, "localhost").createOrUpdateClaudeCodeRules(provider, "big")
	require.NoError(t, err)
	assert.Equal(t, "big", cfg.GetRuleByUUID(serverconfig.RuleUUIDCC).Services[0].Model)
	for _, slot := range serverconfig.CCSlots {
		r := cfg.GetRuleByUUID(cfg.ClaudeCodeRuleUUID(typ.ScenarioClaudeCode, slot))
		assert.True(t, r.Active, slot)
		assert.Equal(t, "own-"+slot, r.Services[0].Model, slot)
	}
}

func TestApplyClaudeCodeRules_ThenSplitStillRoutes(t *testing.T) {
	cfg, provider := newApplyTestConfig(t)
	_, _, err := NewAgentApply(cfg, "localhost").createOrUpdateClaudeCodeRules(provider, "big")
	require.NoError(t, err)
	assert.False(t, cfg.GetRuleByUUID(serverconfig.RuleUUIDCCHaiku).Active, "apply leaves slot rules off")

	// Splitting a slot later turns its rule on: it must route.
	require.NoError(t, cfg.SetAllClaudeCodeSlots(typ.ScenarioClaudeCode, true))
	for _, uuid := range []string{serverconfig.RuleUUIDCCHaiku, serverconfig.RuleUUIDCCSubagent} {
		r := cfg.GetRuleByUUID(uuid)
		require.True(t, r.Active, uuid)
		require.Len(t, r.Services, 1, uuid)
		assert.Equal(t, "big", r.Services[0].Model, uuid)
	}
}
