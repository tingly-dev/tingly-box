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
	assert.False(t, cfg.GetRuleByUUID(RuleUUIDCCSubagent).Active)

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
}

func TestSetClaudeCodeSlot_ModeWrites(t *testing.T) {
	cfg := newSlotTestConfig(t)
	_, err := cfg.SetClaudeCodeSlot(typ.ScenarioClaudeCode, "subagent", true)
	require.NoError(t, err)

	// A whole-record write that doesn't change the mode (e.g. the page's
	// GET-merge for a plugin toggle, without the slot field) keeps the slot.
	sc := *cfg.GetScenarioConfig(typ.ScenarioClaudeCode)
	sc.ClaudeCodeSlots = nil
	sc.Flags.SkipUsage = true
	require.NoError(t, cfg.SetScenarioConfig(sc))
	assert.Equal(t, []string{"subagent"}, cfg.ClaudeCodeSlots(typ.ScenarioClaudeCode))
	assert.True(t, cfg.GetRuleByUUID(RuleUUIDCCSubagent).Active)
	assert.False(t, cfg.GetRuleByUUID(RuleUUIDCCHaiku).Active)

	// Switching to separate and back merges every slot, as before.
	require.NoError(t, cfg.SetScenarioFlag(typ.ScenarioClaudeCode, constant.FlagSeparate, true))
	assert.Empty(t, cfg.ClaudeCodeSlots(typ.ScenarioClaudeCode))
	require.NoError(t, cfg.SetScenarioFlag(typ.ScenarioClaudeCode, constant.FlagUnified, true))
	assert.Empty(t, cfg.ClaudeCodeSlots(typ.ScenarioClaudeCode))
	assert.False(t, cfg.GetRuleByUUID(RuleUUIDCCSubagent).Active)
	assert.True(t, cfg.GetRuleByUUID(RuleUUIDCC).Active)
}

func TestSetClaudeCodeSlot_ActiveAloneDoesNotSplit(t *testing.T) {
	// `agent apply` switches every Claude Code rule on; that must not give the
	// slots their own rule — only SetClaudeCodeSlot does.
	cfg := newSlotTestConfig(t)
	for i := range cfg.Rules {
		if cfg.Rules[i].GetScenario() == typ.ScenarioClaudeCode {
			cfg.Rules[i].Active = true
		}
	}
	assert.Empty(t, cfg.ClaudeCodeSlots(typ.ScenarioClaudeCode))
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
