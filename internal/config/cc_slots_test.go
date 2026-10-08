package config

import (
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/tingly-dev/tingly-box/internal/loadbalance"
	"github.com/tingly-dev/tingly-box/internal/typ"
)

func TestSetClaudeCodeSlot_MainScenario(t *testing.T) {
	cfg, err := NewConfigWithDir(t.TempDir())
	require.NoError(t, err)

	rule, err := cfg.SetClaudeCodeSlot(typ.ScenarioClaudeCode, "subagent", true)
	require.NoError(t, err)
	assert.Equal(t, RuleUUIDCCSubagent, rule.UUID, "the seeded slot rule is reused")
	assert.True(t, cfg.GetRuleByUUID(RuleUUIDCCSubagent).Active)
	assert.True(t, cfg.GetRuleByUUID(RuleUUIDCC).Active)
	assert.True(t, cfg.GetScenarioConfig(typ.ScenarioClaudeCode).Flags.Unified, "one slot on is not separate")

	_, err = cfg.SetClaudeCodeSlot(typ.ScenarioClaudeCode, "subagent", false)
	require.NoError(t, err)
	assert.False(t, cfg.GetRuleByUUID(RuleUUIDCCSubagent).Active)

	// Every slot on is what separate mode means.
	for _, slot := range CCSlots {
		_, err := cfg.SetClaudeCodeSlot(typ.ScenarioClaudeCode, slot, true)
		require.NoError(t, err)
	}
	flags := cfg.GetScenarioConfig(typ.ScenarioClaudeCode).Flags
	assert.True(t, flags.Separate)
	assert.False(t, flags.Unified)

	_, err = cfg.SetClaudeCodeSlot(typ.ScenarioClaudeCode, "default", false)
	assert.Error(t, err, "the default slot is the main rule, not a switch")
	_, err = cfg.SetClaudeCodeSlot(typ.ScenarioClaudeCode, "turbo", true)
	assert.Error(t, err)
	_, err = cfg.SetClaudeCodeSlot(typ.ScenarioOpenAI, "haiku", true)
	assert.Error(t, err)
}

func TestSetClaudeCodeSlot_UnifiedProfileSeedsFromMainRule(t *testing.T) {
	cfg, err := NewConfigWithDir(t.TempDir())
	require.NoError(t, err)
	meta, err := cfg.CreateProfile(typ.ScenarioClaudeCode, "solo", true)
	require.NoError(t, err)
	scenario := typ.ProfiledScenarioName(typ.ScenarioClaudeCode, meta.ID)
	cfg.findRuleByUUID(BuiltinRuleUUID(scenario, "cc")).Services = []*loadbalance.Service{{Provider: "p", Model: "big", Active: true}}

	rule, err := cfg.SetClaudeCodeSlot(scenario, "subagent", true)
	require.NoError(t, err)
	assert.Equal(t, BuiltinRuleUUID(scenario, "subagent"), rule.UUID)
	assert.Equal(t, "subagent", rule.RequestModel)
	require.Len(t, rule.Services, 1)
	assert.Equal(t, "big", rule.Services[0].Model, "a new slot rule starts as a copy of the main rule")
}

func TestSetClaudeCodeSlot_SeparateProfileFollowsItsDefaultRule(t *testing.T) {
	cfg, err := NewConfigWithDir(t.TempDir())
	require.NoError(t, err)
	meta, err := cfg.CreateProfile(typ.ScenarioClaudeCode, "work", false)
	require.NoError(t, err)
	scenario := typ.ProfiledScenarioName(typ.ScenarioClaudeCode, meta.ID)

	_, err = cfg.SetClaudeCodeSlot(scenario, "haiku", false)
	require.NoError(t, err)

	// A separate profile has no cc rule: its default rule is the main one.
	assert.Nil(t, cfg.GetRuleByUUID(BuiltinRuleUUID(scenario, "cc")))
	assert.True(t, cfg.GetRuleByUUID(BuiltinRuleUUID(scenario, "default")).Active)
	assert.False(t, cfg.GetRuleByUUID(BuiltinRuleUUID(scenario, "haiku")).Active)

	got, ok := cfg.GetProfile(typ.ScenarioClaudeCode, meta.ID)
	require.True(t, ok)
	assert.True(t, got.Unified, "not every slot has its own rule any more")
}

func TestSetClaudeCodeSlot_LegacySeparateKeepsDefaultRule(t *testing.T) {
	cfg, err := NewConfigWithDir(t.TempDir())
	require.NoError(t, err)
	require.NoError(t, cfg.SetScenarioFlag(typ.ScenarioClaudeCode, "separate", true))

	// Handing a slot back keeps the default slot where separate mode put it.
	_, err = cfg.SetClaudeCodeSlot(typ.ScenarioClaudeCode, "haiku", false)
	require.NoError(t, err)
	assert.False(t, cfg.GetRuleByUUID(RuleUUIDCC).Active)
	assert.True(t, cfg.GetRuleByUUID(RuleUUIDCCDefault).Active)
}
