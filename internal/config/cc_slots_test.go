package config

import (
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/tingly-dev/tingly-box/internal/constant"
	"github.com/tingly-dev/tingly-box/internal/loadbalance"
	"github.com/tingly-dev/tingly-box/internal/typ"
)

func newSlotTestConfig(t *testing.T) *Config {
	t.Helper()
	cfg, err := NewConfigWithDir(t.TempDir())
	require.NoError(t, err)
	// The main rule is routed somewhere, as it is for a configured user.
	cfg.findRuleByUUID(RuleUUIDCC).Services = []*loadbalance.Service{{Provider: "p", Model: "main", Active: true}}
	return cfg
}

func TestSetClaudeCodeSlot_SplitAndMerge(t *testing.T) {
	cfg := newSlotTestConfig(t)
	assert.Empty(t, cfg.ClaudeCodeSlots(typ.ScenarioClaudeCode), "a fresh install has no split slots")

	rule, err := cfg.SetClaudeCodeSlot(typ.ScenarioClaudeCode, "subagent", true)
	require.NoError(t, err)
	assert.Equal(t, RuleUUIDCCSubagent, rule.UUID, "the seeded slot rule is reused")
	assert.True(t, rule.Active)
	// The seeded rule had no routing: it starts as a copy of the main rule,
	// so turning the slot on changes nothing until the user edits it.
	require.Len(t, rule.Services, 1)
	assert.Equal(t, "main", rule.Services[0].Model)
	assert.Equal(t, []string{"subagent"}, cfg.ClaudeCodeSlots(typ.ScenarioClaudeCode))

	// The user points it elsewhere; merging back keeps that for next time.
	cfg.findRuleByUUID(RuleUUIDCCSubagent).Services = []*loadbalance.Service{{Provider: "p", Model: "cheap", Active: true}}
	_, err = cfg.SetClaudeCodeSlot(typ.ScenarioClaudeCode, "subagent", false)
	require.NoError(t, err)
	assert.Empty(t, cfg.ClaudeCodeSlots(typ.ScenarioClaudeCode))

	rule, err = cfg.SetClaudeCodeSlot(typ.ScenarioClaudeCode, "subagent", true)
	require.NoError(t, err)
	assert.Equal(t, "cheap", rule.Services[0].Model, "turning the slot on again restores its rule")
}

func TestSetClaudeCodeSlot_Rejects(t *testing.T) {
	cfg := newSlotTestConfig(t)

	_, err := cfg.SetClaudeCodeSlot(typ.ScenarioClaudeCode, "default", true)
	assert.Error(t, err, "the default slot is the main rule")
	_, err = cfg.SetClaudeCodeSlot(typ.ScenarioOpenAI, "haiku", true)
	assert.Error(t, err)
	_, err = cfg.SetClaudeCodeSlot("claude_code:p99", "haiku", false)
	assert.Error(t, err, "unknown profile")
	assert.Nil(t, cfg.GetRuleByUUID("builtin:claude_code:p99:cc"), "no orphan rules for an unknown profile")

	require.NoError(t, cfg.SetScenarioFlag(typ.ScenarioClaudeCode, constant.FlagSeparate, true))
	_, err = cfg.SetClaudeCodeSlot(typ.ScenarioClaudeCode, "haiku", true)
	assert.Error(t, err, "separate mode already gives every slot its own rule")
	assert.Empty(t, cfg.ClaudeCodeSlots(typ.ScenarioClaudeCode), "separate mode reports no split slots")
}

func TestSetClaudeCodeSlot_ModeWrites(t *testing.T) {
	cfg := newSlotTestConfig(t)
	_, err := cfg.SetClaudeCodeSlot(typ.ScenarioClaudeCode, "subagent", true)
	require.NoError(t, err)

	// Whole-record writes and flag sets that don't change the mode keep the slot.
	require.NoError(t, cfg.SetScenarioConfig(typ.ScenarioConfig{
		Scenario: typ.ScenarioClaudeCode,
		Flags:    typ.ScenarioFlags{Unified: true, SkipUsage: true},
	}))
	require.NoError(t, cfg.SetScenarioFlag(typ.ScenarioClaudeCode, constant.FlagUnified, true))
	assert.Equal(t, []string{"subagent"}, cfg.ClaudeCodeSlots(typ.ScenarioClaudeCode))

	// Switching to separate and back merges every slot.
	require.NoError(t, cfg.SetScenarioFlag(typ.ScenarioClaudeCode, constant.FlagSeparate, true))
	require.NoError(t, cfg.SetScenarioFlag(typ.ScenarioClaudeCode, constant.FlagUnified, true))
	assert.Empty(t, cfg.ClaudeCodeSlots(typ.ScenarioClaudeCode))
	assert.True(t, cfg.GetRuleByUUID(RuleUUIDCC).Active)
}

func TestSetClaudeCodeSlot_UnifiedProfile(t *testing.T) {
	cfg := newSlotTestConfig(t)
	meta, err := cfg.CreateProfile(typ.ScenarioClaudeCode, "solo", true)
	require.NoError(t, err)
	scenario := typ.ProfiledScenarioName(typ.ScenarioClaudeCode, meta.ID)
	cfg.findRuleByUUID(BuiltinRuleUUID(scenario, "cc")).Services = []*loadbalance.Service{{Provider: "p", Model: "big", Active: true}}

	// The profile doesn't inherit the main scenario's slots.
	_, err = cfg.SetClaudeCodeSlot(typ.ScenarioClaudeCode, "haiku", true)
	require.NoError(t, err)
	assert.Empty(t, cfg.ClaudeCodeSlots(scenario))

	rule, err := cfg.SetClaudeCodeSlot(scenario, "subagent", true)
	require.NoError(t, err)
	assert.Equal(t, BuiltinRuleUUID(scenario, "subagent"), rule.UUID)
	assert.Equal(t, "subagent", rule.RequestModel)
	require.Len(t, rule.Services, 1)
	assert.Equal(t, "big", rule.Services[0].Model)
	assert.Equal(t, []string{"subagent"}, cfg.ClaudeCodeSlots(scenario))
	assert.Equal(t, []string{"haiku"}, cfg.ClaudeCodeSlots(typ.ScenarioClaudeCode))

	separate, err := cfg.CreateProfile(typ.ScenarioClaudeCode, "multi", false)
	require.NoError(t, err)
	_, err = cfg.SetClaudeCodeSlot(typ.ProfiledScenarioName(typ.ScenarioClaudeCode, separate.ID), "haiku", true)
	assert.Error(t, err, "separate profiles already have a rule per slot")
}

func TestResetUnifiedSlotRulesOnce(t *testing.T) {
	// Before slots, agent apply switched every tier rule on whatever the mode;
	// that must not read as "every slot has its own rule" once it does.
	cfg := newSlotTestConfig(t)
	cfg.MigrationsCompleted = nil
	for i := range cfg.Rules {
		if cfg.Rules[i].GetScenario() == typ.ScenarioClaudeCode {
			cfg.Rules[i].Active = true
		}
	}
	assert.NotEmpty(t, cfg.ClaudeCodeSlots(typ.ScenarioClaudeCode))

	require.True(t, resetUnifiedSlotRulesOnce(cfg))
	assert.Empty(t, cfg.ClaudeCodeSlots(typ.ScenarioClaudeCode))
	assert.True(t, cfg.GetRuleByUUID(RuleUUIDCC).Active)

	// Once only: a slot the user splits afterwards survives later boots.
	_, err := cfg.SetClaudeCodeSlot(typ.ScenarioClaudeCode, "opus", true)
	require.NoError(t, err)
	assert.False(t, resetUnifiedSlotRulesOnce(cfg))
	assert.Equal(t, []string{"opus"}, cfg.ClaudeCodeSlots(typ.ScenarioClaudeCode))
}
