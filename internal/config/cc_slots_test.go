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

	_, err = cfg.SetClaudeCodeSlot(typ.ScenarioClaudeCode, "bogus", true)
	assert.Error(t, err, "unknown slot")
}

func TestSetClaudeCodeSlot_ScenarioWritesKeepSlots(t *testing.T) {
	cfg := newSlotTestConfig(t)
	_, err := cfg.SetClaudeCodeSlot(typ.ScenarioClaudeCode, "subagent", true)
	require.NoError(t, err)

	// Whole-record writes and the legacy "unified" flag leave a split slot alone.
	require.NoError(t, cfg.SetScenarioConfig(typ.ScenarioConfig{
		Scenario: typ.ScenarioClaudeCode,
		Flags:    typ.ScenarioFlags{Unified: true, SkipUsage: true},
	}))
	assert.Equal(t, []string{"subagent"}, cfg.ClaudeCodeSlots(typ.ScenarioClaudeCode))
	assert.True(t, cfg.GetRuleByUUID(RuleUUIDCC).Active)
}

func TestLegacyModeFlagsAreSlotShortcuts(t *testing.T) {
	cfg := newSlotTestConfig(t)
	assert.True(t, cfg.GetScenarioFlag(typ.ScenarioClaudeCode, constant.FlagUnified))
	assert.False(t, cfg.GetScenarioFlag(typ.ScenarioClaudeCode, constant.FlagSeparate))

	require.NoError(t, cfg.SetScenarioFlag(typ.ScenarioClaudeCode, constant.FlagSeparate, true))
	assert.Equal(t, CCSlots, cfg.ClaudeCodeSlots(typ.ScenarioClaudeCode), "separate splits every slot")
	assert.True(t, cfg.GetScenarioFlag(typ.ScenarioClaudeCode, constant.FlagSeparate))

	require.NoError(t, cfg.SetScenarioFlag(typ.ScenarioClaudeCode, constant.FlagUnified, true))
	assert.Empty(t, cfg.ClaudeCodeSlots(typ.ScenarioClaudeCode), "unified hands every slot back")
	assert.True(t, cfg.GetRuleByUUID(RuleUUIDCC).Active)
}

func TestSetClaudeCodeSlot_Profile(t *testing.T) {
	cfg := newSlotTestConfig(t)
	meta, err := cfg.CreateProfile(typ.ScenarioClaudeCode, "solo")
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

func TestFoldClaudeCodeSeparate(t *testing.T) {
	cfg := newSlotTestConfig(t)
	cfg.MigrationsCompleted = nil
	routed := func(model string) []*loadbalance.Service {
		return []*loadbalance.Service{{Provider: "p", Model: model, Active: true}}
	}
	// A separate-mode config: scenario flag, the old default rule, slot rules on.
	cfg.Scenarios = []typ.ScenarioConfig{{Scenario: typ.ScenarioClaudeCode, Flags: typ.ScenarioFlags{Separate: true}}}
	cfg.Rules = append(cfg.Rules, typ.Rule{
		UUID: RuleUUIDCCDefault, Scenario: typ.ScenarioClaudeCode, RequestModel: "tingly/cc-default",
		Services: routed("old-default"), Active: true,
	})
	cfg.findRuleByUUID(RuleUUIDCC).Active = false
	for _, slot := range CCSlots {
		r := cfg.findRuleByUUID(BuiltinRuleUUID(typ.ScenarioClaudeCode, slot))
		r.Active = true
		r.Services = routed("own-" + slot)
	}
	// A separate profile: default and haiku rules, no cc rule yet.
	cfg.Profiles = map[string][]typ.ProfileMeta{string(typ.ScenarioClaudeCode): {{ID: "p1", Name: "multi", Unified: false}}}
	profile := typ.ProfiledScenarioName(typ.ScenarioClaudeCode, "p1")
	prDefault := newCCProfileRule(profile, "default", "default")
	prDefault.Services = routed("profile-default")
	prHaiku := newCCProfileRule(profile, "haiku", "haiku")
	prHaiku.Services = routed("profile-haiku")
	cfg.Rules = append(cfg.Rules, prDefault, prHaiku)

	require.True(t, foldClaudeCodeSeparateOnce(cfg))

	assert.Nil(t, cfg.findRuleByUUID(RuleUUIDCCDefault), "the default rule is gone")
	cc := cfg.findRuleByUUID(RuleUUIDCC)
	assert.True(t, cc.Active)
	assert.Equal(t, "old-default", cc.Services[0].Model, "the default slot took the default rule's routing")
	assert.Equal(t, CCSlots, cfg.ClaudeCodeSlots(typ.ScenarioClaudeCode), "every slot keeps its own rule")
	assert.Equal(t, "own-haiku", cfg.findRuleByUUID(RuleUUIDCCHaiku).Services[0].Model)
	assert.False(t, cfg.GetScenarioConfig(typ.ScenarioClaudeCode).Flags.Separate)

	assert.Nil(t, cfg.findRuleByUUID(BuiltinRuleUUID(profile, "default")))
	pcc := cfg.findRuleByUUID(BuiltinRuleUUID(profile, "cc"))
	require.NotNil(t, pcc, "the profile gets a default slot rule")
	assert.Equal(t, "profile-default", pcc.Services[0].Model)
	assert.Equal(t, []string{"haiku"}, cfg.ClaudeCodeSlots(profile))
	assert.True(t, cfg.Profiles[string(typ.ScenarioClaudeCode)][0].Unified)

	// Once only.
	assert.False(t, foldClaudeCodeSeparateOnce(cfg))
}

func TestFoldClaudeCodeSeparate_UnifiedConfigKeepsDefaultSlot(t *testing.T) {
	cfg := newSlotTestConfig(t)
	cfg.MigrationsCompleted = nil
	cfg.Rules = append(cfg.Rules, typ.Rule{
		UUID: RuleUUIDCCDefault, Scenario: typ.ScenarioClaudeCode, RequestModel: "tingly/cc-default",
		Services: []*loadbalance.Service{{Provider: "p", Model: "stale", Active: true}},
	})

	require.True(t, foldClaudeCodeSeparateOnce(cfg))

	assert.Nil(t, cfg.findRuleByUUID(RuleUUIDCCDefault))
	assert.Equal(t, "main", cfg.findRuleByUUID(RuleUUIDCC).Services[0].Model, "an unused default rule never overrides the unified one")
}

func TestLegacyDefaultRequestModelRoutesToDefaultSlot(t *testing.T) {
	cfg := newSlotTestConfig(t)
	got := cfg.MatchRuleByModelAndScenario("tingly/cc-default", typ.ScenarioClaudeCode)
	require.NotNil(t, got)
	assert.Equal(t, RuleUUIDCC, got.UUID)

	meta, err := cfg.CreateProfile(typ.ScenarioClaudeCode, "solo")
	require.NoError(t, err)
	profile := typ.ProfiledScenarioName(typ.ScenarioClaudeCode, meta.ID)
	got = cfg.MatchRuleByModelAndScenario("default", profile)
	require.NotNil(t, got)
	assert.Equal(t, BuiltinRuleUUID(profile, "cc"), got.UUID)
}

func TestShortenClaudeCodeRequestModelsOnce(t *testing.T) {
	cfg := newSlotTestConfig(t)
	cfg.MigrationsCompleted = nil
	for slot, old := range map[string]string{"cc": "tingly/cc", "sonnet": "tingly/cc-sonnet", "subagent": "tingly/cc-subagent"} {
		cfg.ccRuleLocked(typ.ScenarioClaudeCode, slot).RequestModel = old
	}
	// A rule the user already renamed is left alone.
	cfg.ccRuleLocked(typ.ScenarioClaudeCode, "opus").RequestModel = "vendor/big"

	require.True(t, shortenClaudeCodeRequestModelsOnce(cfg))

	assert.Equal(t, "cc", cfg.ccRuleLocked(typ.ScenarioClaudeCode, "cc").RequestModel)
	assert.Equal(t, "sonnet", cfg.ccRuleLocked(typ.ScenarioClaudeCode, "sonnet").RequestModel)
	assert.Equal(t, "subagent", cfg.ccRuleLocked(typ.ScenarioClaudeCode, "subagent").RequestModel)
	assert.Equal(t, "vendor/big", cfg.ccRuleLocked(typ.ScenarioClaudeCode, "opus").RequestModel)
	assert.False(t, shortenClaudeCodeRequestModelsOnce(cfg), "once only")
}

func TestLegacyRequestModelsRouteToShortNamedRules(t *testing.T) {
	cfg := newSlotTestConfig(t)
	for old, slot := range map[string]string{
		"tingly/cc": "cc", "tingly/cc[1m]": "cc", "tingly/cc-sonnet": "sonnet", "tingly/cc-subagent": "subagent",
	} {
		got := cfg.MatchRuleByModelAndScenario(old, typ.ScenarioClaudeCode)
		require.NotNil(t, got, old)
		assert.Equal(t, BuiltinRuleUUID(typ.ScenarioClaudeCode, slot), got.UUID, old)
	}
}
