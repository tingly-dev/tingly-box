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
	require.Nil(t, cfg.GetRuleByUUID(RuleUUIDCCDefault), "fresh installs have no default rule: cc is the default slot")

	rule, err := cfg.SetClaudeCodeSlot(typ.ScenarioClaudeCode, "subagent", true)
	require.NoError(t, err)
	assert.Equal(t, RuleUUIDCCSubagent, rule.UUID, "the seeded slot rule is reused")
	assert.True(t, cfg.GetRuleByUUID(RuleUUIDCCSubagent).Active)
	assert.True(t, cfg.GetRuleByUUID(RuleUUIDCC).Active)

	_, err = cfg.SetClaudeCodeSlot(typ.ScenarioClaudeCode, "subagent", false)
	require.NoError(t, err)
	assert.False(t, cfg.GetRuleByUUID(RuleUUIDCCSubagent).Active, "off keeps the rule for next time")
	assert.True(t, cfg.GetRuleByUUID(RuleUUIDCC).Active)

	_, err = cfg.SetClaudeCodeSlot(typ.ScenarioClaudeCode, "default", false)
	assert.Error(t, err, "the default slot is the main rule, not a switch")
	_, err = cfg.SetClaudeCodeSlot(typ.ScenarioClaudeCode, "turbo", true)
	assert.Error(t, err)
	_, err = cfg.SetClaudeCodeSlot(typ.ScenarioOpenAI, "haiku", true)
	assert.Error(t, err)
}

func TestSetClaudeCodeSlot_LegacyModeWritesSwitchEverySlot(t *testing.T) {
	cfg, err := NewConfigWithDir(t.TempDir())
	require.NoError(t, err)

	require.NoError(t, cfg.SetScenarioFlag(typ.ScenarioClaudeCode, "separate", true))
	for _, slot := range CCSlots {
		assert.True(t, cfg.GetRuleByUUID(BuiltinRuleUUID(typ.ScenarioClaudeCode, slot)).Active, slot)
	}
	assert.True(t, cfg.GetRuleByUUID(RuleUUIDCC).Active, "the main rule stays on")

	require.NoError(t, cfg.SetScenarioConfig(typ.ScenarioConfig{Scenario: typ.ScenarioClaudeCode, Flags: typ.ScenarioFlags{Unified: true}}))
	for _, slot := range CCSlots {
		assert.False(t, cfg.GetRuleByUUID(BuiltinRuleUUID(typ.ScenarioClaudeCode, slot)).Active, slot)
	}
	flags := cfg.GetScenarioConfig(typ.ScenarioClaudeCode).Flags
	assert.False(t, flags.Unified || flags.Separate, "no mode is stored")
}

func TestCreateProfile_StartsWithMainRule(t *testing.T) {
	cfg, err := NewConfigWithDir(t.TempDir())
	require.NoError(t, err)

	one, err := cfg.CreateProfile(typ.ScenarioClaudeCode, "solo", true)
	require.NoError(t, err)
	scenario := typ.ProfiledScenarioName(typ.ScenarioClaudeCode, one.ID)
	require.NotNil(t, cfg.GetRuleByUUID(BuiltinRuleUUID(scenario, "cc")))
	assert.Nil(t, cfg.GetRuleByUUID(BuiltinRuleUUID(scenario, "subagent")))

	// The legacy "separate" choice switches every slot on.
	all, err := cfg.CreateProfile(typ.ScenarioClaudeCode, "all", false)
	require.NoError(t, err)
	scenario = typ.ProfiledScenarioName(typ.ScenarioClaudeCode, all.ID)
	require.NotNil(t, cfg.GetRuleByUUID(BuiltinRuleUUID(scenario, "cc")))
	for _, slot := range CCSlots {
		r := cfg.GetRuleByUUID(BuiltinRuleUUID(scenario, slot))
		require.NotNil(t, r, slot)
		assert.True(t, r.Active, slot)
		assert.Equal(t, slot, r.RequestModel)
	}
}

func TestSetClaudeCodeSlot_ProfileSeedsFromMainRule(t *testing.T) {
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

func ccTestRule(uuid string, scenario typ.RuleScenario, model string, active bool, svcModel string) typ.Rule {
	r := typ.Rule{UUID: uuid, Scenario: scenario, RequestModel: model, Active: active}
	if svcModel != "" {
		r.Services = []*loadbalance.Service{{Provider: "p", Model: svcModel, Active: true}}
	}
	return r
}

func TestCollapseClaudeCodeMainRule_Unified(t *testing.T) {
	cfg := &Config{
		Rules: []typ.Rule{
			ccTestRule(RuleUUIDCC, typ.ScenarioClaudeCode, "tingly/cc", true, "main"),
			ccTestRule(RuleUUIDCCDefault, typ.ScenarioClaudeCode, "tingly/cc-default", false, "old"),
			ccTestRule(RuleUUIDCCHaiku, typ.ScenarioClaudeCode, "tingly/cc-haiku", false, ""),
		},
		Scenarios: []typ.ScenarioConfig{{Scenario: typ.ScenarioClaudeCode, Flags: typ.ScenarioFlags{Unified: true}}},
	}
	assert.True(t, collapseClaudeCodeMainRuleOnce(cfg))

	cc := cfg.findRuleByUUID(RuleUUIDCC)
	assert.Equal(t, "tingly/cc", cc.RequestModel, "the env keeps requesting tingly/cc")
	assert.Equal(t, "main", cc.Services[0].Model)
	assert.Nil(t, cfg.findRuleByUUID(RuleUUIDCCDefault))
	assert.False(t, cfg.findRuleByUUID(RuleUUIDCCHaiku).Active)
	assert.False(t, cfg.Scenarios[0].Flags.Unified)

	assert.False(t, collapseClaudeCodeMainRuleOnce(cfg), "runs once")
}

func TestCollapseClaudeCodeMainRule_Separate(t *testing.T) {
	cfg := &Config{
		Rules: []typ.Rule{
			ccTestRule(RuleUUIDCC, typ.ScenarioClaudeCode, "tingly/cc", false, "stale"),
			ccTestRule(RuleUUIDCCDefault, typ.ScenarioClaudeCode, "tingly/cc-default", true, "def"),
			ccTestRule(RuleUUIDCCHaiku, typ.ScenarioClaudeCode, "tingly/cc-haiku", true, "fast"),
		},
		Scenarios: []typ.ScenarioConfig{{Scenario: typ.ScenarioClaudeCode, Flags: typ.ScenarioFlags{Separate: true}}},
	}
	collapseClaudeCodeMainRuleOnce(cfg)

	cc := cfg.findRuleByUUID(RuleUUIDCC)
	require.NotNil(t, cc)
	assert.True(t, cc.Active)
	assert.Equal(t, "tingly/cc-default", cc.RequestModel, "ANTHROPIC_MODEL in the applied env keeps routing")
	assert.Equal(t, "def", cc.Services[0].Model)
	assert.Nil(t, cfg.findRuleByUUID(RuleUUIDCCDefault))
	haiku := cfg.findRuleByUUID(RuleUUIDCCHaiku)
	assert.True(t, haiku.Active)
	assert.Equal(t, "fast", haiku.Services[0].Model)
	assert.False(t, cfg.Scenarios[0].Flags.Separate)
}

func TestCollapseClaudeCodeMainRule_SeparateProfile(t *testing.T) {
	p1 := typ.ProfiledScenarioName(typ.ScenarioClaudeCode, "p1")
	cfg := &Config{
		Rules: []typ.Rule{
			ccTestRule(BuiltinRuleUUID(p1, "default"), p1, "default", true, "def"),
			ccTestRule(BuiltinRuleUUID(p1, "subagent"), p1, "subagent", true, "cheap"),
		},
		Profiles: map[string][]typ.ProfileMeta{"claude_code": {{ID: "p1", Name: "work", Unified: false}}},
	}
	collapseClaudeCodeMainRuleOnce(cfg)

	cc := cfg.findRuleByUUID(BuiltinRuleUUID(p1, "cc"))
	require.NotNil(t, cc, "the separate profile gets its main rule")
	assert.True(t, cc.Active)
	assert.Equal(t, "default", cc.RequestModel)
	assert.Equal(t, "def", cc.Services[0].Model)
	assert.Equal(t, p1, cc.Scenario)
	assert.Nil(t, cfg.findRuleByUUID(BuiltinRuleUUID(p1, "default")))
	assert.True(t, cfg.findRuleByUUID(BuiltinRuleUUID(p1, "subagent")).Active)
	assert.True(t, cfg.Profiles["claude_code"][0].Unified, "kept true for a downgraded binary")
}
