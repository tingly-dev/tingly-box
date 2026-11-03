package config

import (
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/tingly-dev/tingly-box/internal/loadbalance"
	"github.com/tingly-dev/tingly-box/internal/typ"
)

func profileScenario(id string) typ.RuleScenario {
	return typ.ProfiledScenarioName(typ.ScenarioClaudeCode, id)
}

func rulesOf(cfg *Config, scenario typ.RuleScenario) []typ.Rule {
	var out []typ.Rule
	for _, r := range cfg.Rules {
		if r.Scenario == scenario {
			out = append(out, r)
		}
	}
	return out
}

func TestDuplicateProfile_CopiesEverythingAndLeavesSource(t *testing.T) {
	cfg := newSlotTestConfig(t)
	src, err := cfg.CreateProfile(typ.ScenarioClaudeCode, "work")
	require.NoError(t, err)
	srcScenario := profileScenario(src.ID)
	cfg.findRuleByUUID(BuiltinRuleUUID(srcScenario, "cc")).Services = []*loadbalance.Service{{Provider: "p", Model: "big", Active: true}}
	_, err = cfg.SetClaudeCodeSlot(srcScenario, "haiku", true)
	require.NoError(t, err)
	custom := typ.Rule{UUID: "custom-1", Scenario: srcScenario, RequestModel: "my-model", Active: true,
		Services: []*loadbalance.Service{{Provider: "p", Model: "custom", Active: true}}}
	cfg.Rules = append(cfg.Rules, custom)
	require.NoError(t, cfg.UpdateClaudeCodeProfileConfig(typ.ScenarioClaudeCode, src.ID,
		&typ.ClaudeCodeProfileConfig{Env: map[string]string{"A": "1"}, UnsetEnv: []string{"B"}, DefaultMode: "plan"}))
	require.NoError(t, cfg.SetScenarioConfig(typ.ScenarioConfig{Scenario: srcScenario, Flags: typ.ScenarioFlags{SkipUsage: true}}))
	before := rulesOf(cfg, srcScenario)

	dup, err := cfg.DuplicateProfile(typ.ScenarioClaudeCode, src.ID, "")
	require.NoError(t, err)
	assert.Equal(t, "work-copy", dup.Name)
	assert.NotEqual(t, src.ID, dup.ID)
	assert.Equal(t, &typ.ClaudeCodeProfileConfig{Env: map[string]string{"A": "1"}, UnsetEnv: []string{"B"}, DefaultMode: "plan"}, dup.ClaudeCode)

	dstScenario := profileScenario(dup.ID)
	got := rulesOf(cfg, dstScenario)
	require.Len(t, got, len(before))

	// Built-in rules keep their builtin:<scenario>:<slot> identity.
	cc := cfg.findRuleByUUID(BuiltinRuleUUID(dstScenario, "cc"))
	require.NotNil(t, cc)
	require.Len(t, cc.Services, 1)
	assert.Equal(t, "big", cc.Services[0].Model)
	haiku := cfg.findRuleByUUID(BuiltinRuleUUID(dstScenario, "haiku"))
	require.NotNil(t, haiku)
	assert.True(t, haiku.Active)
	assert.Equal(t, []string{"haiku"}, cfg.ClaudeCodeSlots(dstScenario))
	// A custom rule is copied under a fresh UUID.
	var copiedCustom *typ.Rule
	for i := range got {
		if got[i].RequestModel == "my-model" {
			copiedCustom = &got[i]
		}
	}
	require.NotNil(t, copiedCustom)
	assert.NotEqual(t, "custom-1", copiedCustom.UUID)
	assert.Equal(t, "custom", copiedCustom.Services[0].Model)

	sc := cfg.GetScenarioConfig(dstScenario)
	require.NotNil(t, sc)
	assert.Equal(t, dstScenario, sc.Scenario)
	assert.True(t, sc.Flags.SkipUsage)

	// The source is unchanged, and the copy shares no mutable state with it.
	assert.Equal(t, before, rulesOf(cfg, srcScenario))
	srcAfter, _ := cfg.GetProfile(typ.ScenarioClaudeCode, src.ID)
	assert.Equal(t, "work", srcAfter.Name)
	cc.Services[0].Model = "changed"
	assert.Equal(t, "big", cfg.findRuleByUUID(BuiltinRuleUUID(srcScenario, "cc")).Services[0].Model)
	dupMeta, _ := cfg.GetProfile(typ.ScenarioClaudeCode, dup.ID)
	dupMeta.ClaudeCode.Env["A"] = "2"
	stored, _ := cfg.GetProfile(typ.ScenarioClaudeCode, src.ID)
	assert.Equal(t, "1", stored.ClaudeCode.Env["A"])
}

func TestDuplicateProfile_Naming(t *testing.T) {
	cfg := newSlotTestConfig(t)
	src, err := cfg.CreateProfile(typ.ScenarioClaudeCode, "work")
	require.NoError(t, err)

	names := []string{}
	for range 3 {
		dup, err := cfg.DuplicateProfile(typ.ScenarioClaudeCode, src.ID, "")
		require.NoError(t, err)
		names = append(names, dup.Name)
	}
	assert.Equal(t, []string{"work-copy", "work-copy-2", "work-copy-3"}, names)

	named, err := cfg.DuplicateProfile(typ.ScenarioClaudeCode, src.ID, "other")
	require.NoError(t, err)
	assert.Equal(t, "other", named.Name)

	_, err = cfg.DuplicateProfile(typ.ScenarioClaudeCode, src.ID, "work")
	assert.ErrorContains(t, err, "already exists")
	_, err = cfg.DuplicateProfile(typ.ScenarioClaudeCode, src.ID, "has space")
	assert.Error(t, err)
	assert.Len(t, cfg.GetProfiles(typ.ScenarioClaudeCode), 5, "refused duplicates leave no profile behind")

	// A legacy name that is not a valid alias still gets a valid copy name.
	cfg.Profiles["claude_code"][0].Name = "my work!"
	dup, err := cfg.DuplicateProfile(typ.ScenarioClaudeCode, src.ID, "")
	require.NoError(t, err)
	assert.Equal(t, "my-work-copy", dup.Name)
}

func TestDuplicateProfile_UnknownSource(t *testing.T) {
	cfg := newSlotTestConfig(t)
	_, err := cfg.DuplicateProfile(typ.ScenarioClaudeCode, "p9", "")
	assert.ErrorIs(t, err, ErrProfileNotFound)
}

func TestDuplicateProfile_SaveFailureRollsBack(t *testing.T) {
	cfg := newSlotTestConfig(t)
	src, err := cfg.CreateProfile(typ.ScenarioClaudeCode, "work")
	require.NoError(t, err)
	nRules, nScenarios := len(cfg.Rules), len(cfg.Scenarios)

	cfg.ConfigFile = "" // saveLocked fails
	_, err = cfg.DuplicateProfile(typ.ScenarioClaudeCode, src.ID, "")
	require.Error(t, err)
	assert.Len(t, cfg.Rules, nRules)
	assert.Len(t, cfg.Scenarios, nScenarios)
	assert.Len(t, cfg.GetProfiles(typ.ScenarioClaudeCode), 1)
}
