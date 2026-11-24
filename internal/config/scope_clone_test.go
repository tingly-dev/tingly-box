package config

import (
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/tingly-dev/tingly-box/internal/loadbalance"
	"github.com/tingly-dev/tingly-box/internal/typ"
)

func TestCloneScope_RulesAndUUIDs(t *testing.T) {
	cfg := newSlotTestConfig(t)
	src, dst := typ.RuleScenario("team:a"), typ.RuleScenario("team:b")
	cfg.Rules = append(cfg.Rules,
		typ.Rule{UUID: BuiltinRuleUUID(src, "x"), Scenario: src, RequestModel: "x", Active: true,
			Services: []*loadbalance.Service{{Provider: "p", Model: "m", Active: true}}},
		typ.Rule{UUID: "custom-1", Scenario: src, RequestModel: "y"},
		typ.Rule{UUID: "elsewhere", Scenario: "team:other", RequestModel: "z"},
	)
	nRules := len(cfg.Rules)

	undo, err := cfg.cloneScopeLocked(src, dst)
	require.NoError(t, err)
	require.Len(t, cfg.Rules, nRules+2, "only the source scope's rules are copied")

	var builtin, custom *typ.Rule
	for i := range cfg.Rules {
		r := &cfg.Rules[i]
		if r.Scenario != dst {
			continue
		}
		switch r.RequestModel {
		case "x":
			builtin = r
		case "y":
			custom = r
		}
	}
	require.NotNil(t, builtin)
	require.NotNil(t, custom)
	assert.Equal(t, BuiltinRuleUUID(dst, "x"), builtin.UUID, "built-in identity moves to the new scope")
	assert.NotEqual(t, "custom-1", custom.UUID)
	assert.NotEmpty(t, custom.UUID)

	// Services are copies, not shared pointers.
	builtin.Services[0].Model = "changed"
	assert.Equal(t, "m", cfg.findRuleByUUID(BuiltinRuleUUID(src, "x")).Services[0].Model)

	undo()
	assert.Len(t, cfg.Rules, nRules)
}

func TestCloneScope_LegacyMainClaudeCodeUUID(t *testing.T) {
	dst := typ.ProfiledScenarioName(typ.ScenarioClaudeCode, "p1")
	assert.Equal(t, BuiltinRuleUUID(dst, "haiku"), clonedRuleUUID(RuleUUIDBuiltinCCHaiku, typ.ScenarioClaudeCode, dst))
	assert.Equal(t, BuiltinRuleUUID(dst, "cc"), clonedRuleUUID(RuleUUIDCC, typ.ScenarioClaudeCode, dst))
	// A legacy UUID is only a Claude Code main-scope thing.
	assert.NotEqual(t, BuiltinRuleUUID("team:b", "haiku"), clonedRuleUUID(RuleUUIDBuiltinCCHaiku, "team:a", "team:b"))
}

func TestCloneScope_ScenarioConfig(t *testing.T) {
	cfg := newSlotTestConfig(t)
	flags := typ.ScenarioFlags{SkipUsage: true}

	// A scope with its own config: the copy gets its own snapshot.
	require.NoError(t, cfg.SetScenarioConfig(typ.ScenarioConfig{Scenario: "team:a", Flags: flags}))
	_, err := cfg.cloneScopeLocked("team:a", "team:b")
	require.NoError(t, err)
	got := cfg.GetScenarioConfig("team:b")
	require.NotNil(t, got)
	assert.Equal(t, typ.RuleScenario("team:b"), got.Scenario)
	assert.True(t, got.Flags.SkipUsage)

	// A base scope: the copy inherits it live instead of freezing a snapshot.
	require.NoError(t, cfg.SetScenarioConfig(typ.ScenarioConfig{Scenario: typ.ScenarioTeam, Flags: flags}))
	_, err = cfg.cloneScopeLocked(typ.ScenarioTeam, "team:c")
	require.NoError(t, err)
	for _, sc := range cfg.Scenarios {
		assert.NotEqual(t, typ.RuleScenario("team:c"), sc.Scenario, "base config is inherited, not copied")
	}
	assert.True(t, cfg.GetScenarioConfig("team:c").Flags.SkipUsage, "...and still visible through inheritance")
}

func TestCloneScope_RefusesNonEmptyDestination(t *testing.T) {
	cfg := newSlotTestConfig(t)
	cfg.Rules = append(cfg.Rules, typ.Rule{UUID: "r", Scenario: "team:b", RequestModel: "x"})
	nRules := len(cfg.Rules)
	_, err := cfg.cloneScopeLocked("team:a", "team:b")
	assert.ErrorContains(t, err, "not empty")
	assert.Len(t, cfg.Rules, nRules)
}
