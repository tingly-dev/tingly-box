package config

import (
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/tingly-dev/tingly-box/internal/loadbalance"
	"github.com/tingly-dev/tingly-box/internal/typ"
)

func newSlotTestConfig(t *testing.T) (*Config, string) {
	t.Helper()
	dir := t.TempDir()
	cfg, err := NewConfigWithDir(dir)
	require.NoError(t, err)
	return cfg, dir
}

// slotModels maps slot → request model for compact assertions.
func slotModels(slots []CCSlotResolution) map[string]string {
	out := map[string]string{}
	for _, s := range slots {
		out[s.Slot] = s.RequestModel
	}
	return out
}

func allSlots(model string) map[string]string {
	out := map[string]string{}
	for _, slot := range CCSlots {
		out[slot] = model
	}
	return out
}

func TestClaudeCodeSlots_FreshConfigIsUnified(t *testing.T) {
	cfg, _ := newSlotTestConfig(t)

	slots := cfg.ResolveClaudeCodeSlots(typ.ScenarioClaudeCode)
	assert.Equal(t, allSlots("tingly/cc"), slotModels(slots))
	assert.True(t, ClaudeCodeSlotsUnified(slots))
	for _, s := range slots {
		assert.False(t, s.Bound, s.Slot)
	}
}

func TestClaudeCodeSlots_SplitOnlySubagent(t *testing.T) {
	cfg, dir := newSlotTestConfig(t)

	rule, err := cfg.CreateClaudeCodeSlotRule(typ.ScenarioClaudeCode, CCSlotSubagent)
	require.NoError(t, err)
	assert.Equal(t, RuleUUIDCCSubagent, rule.UUID, "the seeded built-in rule is reused")
	assert.True(t, rule.Active)

	want := allSlots("tingly/cc")
	want[CCSlotSubagent] = "tingly/cc-subagent"
	assert.Equal(t, want, slotModels(cfg.ResolveClaudeCodeSlots(typ.ScenarioClaudeCode)))

	// The legacy flags follow: not unified any more.
	flags := cfg.GetScenarioConfig(typ.ScenarioClaudeCode).Flags
	assert.False(t, flags.Unified)
	assert.True(t, flags.Separate)

	// Bindings survive a reload.
	reloaded, err := NewConfigWithDir(dir)
	require.NoError(t, err)
	assert.Equal(t, want, slotModels(reloaded.ResolveClaudeCodeSlots(typ.ScenarioClaudeCode)))

	// Merging it back keeps the rule (and its config) but routes through cc.
	require.NoError(t, cfg.SetClaudeCodeSlot(typ.ScenarioClaudeCode, CCSlotSubagent, ""))
	assert.Equal(t, allSlots("tingly/cc"), slotModels(cfg.ResolveClaudeCodeSlots(typ.ScenarioClaudeCode)))
	assert.NotNil(t, cfg.GetRuleByUUID(RuleUUIDCCSubagent))
	assert.True(t, cfg.GetScenarioConfig(typ.ScenarioClaudeCode).Flags.Unified)
}

func TestClaudeCodeSlots_SharedRuleAcrossSlots(t *testing.T) {
	cfg, _ := newSlotTestConfig(t)
	fast, err := cfg.CreateClaudeCodeSlotRule(typ.ScenarioClaudeCode, CCSlotHaiku)
	require.NoError(t, err)
	require.NoError(t, cfg.SetClaudeCodeSlot(typ.ScenarioClaudeCode, CCSlotSubagent, fast.UUID))

	want := allSlots("tingly/cc")
	want[CCSlotHaiku] = "tingly/cc-haiku"
	want[CCSlotSubagent] = "tingly/cc-haiku"
	assert.Equal(t, want, slotModels(cfg.ResolveClaudeCodeSlots(typ.ScenarioClaudeCode)))
}

func TestClaudeCodeSlots_BindRejectsForeignRule(t *testing.T) {
	cfg, _ := newSlotTestConfig(t)
	assert.Error(t, cfg.SetClaudeCodeSlot(typ.ScenarioClaudeCode, CCSlotSubagent, "no-such-rule"))
	assert.Error(t, cfg.SetClaudeCodeSlot(typ.ScenarioClaudeCode, "turbo", ""))
	assert.Error(t, cfg.SetClaudeCodeSlot(typ.ScenarioOpenAI, CCSlotSubagent, ""))
}

func TestClaudeCodeSlots_Presets(t *testing.T) {
	cfg, _ := newSlotTestConfig(t)

	require.NoError(t, cfg.ApplyClaudeCodeSlotPreset(typ.ScenarioClaudeCode, CCSlotPresetSeparate))
	assert.Equal(t, map[string]string{
		CCSlotDefault:  "tingly/cc-default",
		CCSlotHaiku:    "tingly/cc-haiku",
		CCSlotSonnet:   "tingly/cc-sonnet",
		CCSlotOpus:     "tingly/cc-opus",
		CCSlotFable:    "tingly/cc-fable",
		CCSlotSubagent: "tingly/cc-subagent",
	}, slotModels(cfg.ResolveClaudeCodeSlots(typ.ScenarioClaudeCode)))

	require.NoError(t, cfg.ApplyClaudeCodeSlotPreset(typ.ScenarioClaudeCode, CCSlotPresetUnified))
	assert.Equal(t, allSlots("tingly/cc"), slotModels(cfg.ResolveClaudeCodeSlots(typ.ScenarioClaudeCode)))
	// Rules are never switched off by a preset: a Claude Code still running
	// on the separate env keeps routing.
	assert.True(t, cfg.GetRuleByUUID(RuleUUIDCCHaiku).Active)
	assert.True(t, cfg.GetRuleByUUID(RuleUUIDCC).Active)
}

func TestClaudeCodeSlots_LegacyFlagWritesActAsPresets(t *testing.T) {
	cfg, _ := newSlotTestConfig(t)
	_, err := cfg.CreateClaudeCodeSlotRule(typ.ScenarioClaudeCode, CCSlotSubagent)
	require.NoError(t, err)

	// A whole-record write that doesn't know about slots (and doesn't change
	// the mode) leaves the bindings alone.
	sc := *cfg.GetScenarioConfig(typ.ScenarioClaudeCode)
	sc.ClaudeCodeSlots = nil
	require.NoError(t, cfg.SetScenarioConfig(sc))
	assert.Equal(t, "tingly/cc-subagent", slotModels(cfg.ResolveClaudeCodeSlots(typ.ScenarioClaudeCode))[CCSlotSubagent])

	// Switching to unified through the old flag merges every slot.
	sc.Flags = typ.ScenarioFlags{Unified: true}
	require.NoError(t, cfg.SetScenarioConfig(sc))
	assert.Equal(t, allSlots("tingly/cc"), slotModels(cfg.ResolveClaudeCodeSlots(typ.ScenarioClaudeCode)))
}

func TestClaudeCodeSlots_LegacySeparateScenarioMigratesLazily(t *testing.T) {
	cfg := &Config{
		Rules: []typ.Rule{
			{UUID: RuleUUIDCC, Scenario: typ.ScenarioClaudeCode, RequestModel: "tingly/cc"},
			{UUID: RuleUUIDCCDefault, Scenario: typ.ScenarioClaudeCode, RequestModel: "tingly/cc-default", Active: true},
			{UUID: RuleUUIDCCOpus, Scenario: typ.ScenarioClaudeCode, RequestModel: "vendor/smart", Active: true},
		},
		Scenarios: []typ.ScenarioConfig{{Scenario: typ.ScenarioClaudeCode, Flags: typ.ScenarioFlags{Separate: true}}},
	}
	want := allSlots("tingly/cc-default")
	want[CCSlotOpus] = "vendor/smart"
	assert.Equal(t, want, slotModels(cfg.ResolveClaudeCodeSlots(typ.ScenarioClaudeCode)))
}

func TestClaudeCodeSlots_ProfileSeparateUnbindDefaultSeedsMainRule(t *testing.T) {
	cfg, _ := newSlotTestConfig(t)
	meta, err := cfg.CreateProfile(typ.ScenarioClaudeCode, "work", false)
	require.NoError(t, err)
	scenario := typ.ProfiledScenarioName(typ.ScenarioClaudeCode, meta.ID)

	// Give the profile's default rule a service to check it gets copied.
	cfg.findRuleByUUID(BuiltinRuleUUID(scenario, CCSlotDefault)).Services = []*loadbalance.Service{{Provider: "p", Model: "m", Active: true}}

	assert.Equal(t, "subagent", slotModels(cfg.ResolveClaudeCodeSlots(scenario))[CCSlotSubagent])

	// Merge everything into a main rule the separate profile never had.
	require.NoError(t, cfg.ApplyClaudeCodeSlotPreset(scenario, CCSlotPresetUnified))
	assert.Equal(t, allSlots("cc"), slotModels(cfg.ResolveClaudeCodeSlots(scenario)))
	main := cfg.GetRuleByUUID(BuiltinRuleUUID(scenario, "cc"))
	require.NotNil(t, main)
	require.Len(t, main.Services, 1)
	assert.Equal(t, "m", main.Services[0].Model)

	got, ok := cfg.GetProfile(typ.ScenarioClaudeCode, meta.ID)
	require.True(t, ok)
	assert.True(t, got.Unified, "profile mode is derived from its slots")

	// The base scenario is untouched.
	assert.Equal(t, allSlots("tingly/cc"), slotModels(cfg.ResolveClaudeCodeSlots(typ.ScenarioClaudeCode)))
}

func TestClaudeCodeSlots_ProfileUnifiedSplitSeedsFromCurrentRule(t *testing.T) {
	cfg, _ := newSlotTestConfig(t)
	meta, err := cfg.CreateProfile(typ.ScenarioClaudeCode, "solo", true)
	require.NoError(t, err)
	scenario := typ.ProfiledScenarioName(typ.ScenarioClaudeCode, meta.ID)

	cfg.findRuleByUUID(BuiltinRuleUUID(scenario, "cc")).Services = []*loadbalance.Service{{Provider: "p", Model: "big", Active: true}}

	rule, err := cfg.CreateClaudeCodeSlotRule(scenario, CCSlotSubagent)
	require.NoError(t, err)
	assert.Equal(t, BuiltinRuleUUID(scenario, CCSlotSubagent), rule.UUID)
	assert.Equal(t, "subagent", rule.RequestModel)
	require.Len(t, rule.Services, 1)
	assert.Equal(t, "big", rule.Services[0].Model, "a new slot rule starts as a copy of the rule the slot used")

	want := allSlots("cc")
	want[CCSlotSubagent] = "subagent"
	assert.Equal(t, want, slotModels(cfg.ResolveClaudeCodeSlots(scenario)))
}
