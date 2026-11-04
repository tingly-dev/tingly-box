package config

import (
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/tingly-dev/tingly-box/internal/loadbalance"
	"github.com/tingly-dev/tingly-box/internal/typ"
)

func switchSvc(provider, model string) *loadbalance.Service {
	return &loadbalance.Service{Provider: provider, Model: model, Active: true, Weight: 1}
}

// newSwitchTestConfig has a routed default and one routed profile "work" that
// splits the subagent slot.
func newSwitchTestConfig(t *testing.T) (*Config, typ.ProfileMeta) {
	t.Helper()
	cfg := newSlotTestConfig(t)
	cfg.findRuleByUUID(RuleUUIDCC).Services = []*loadbalance.Service{switchSvc("p", "main")}

	meta, err := cfg.CreateProfile(typ.ScenarioClaudeCode, "work")
	require.NoError(t, err)
	profiled := typ.ProfiledScenarioName(typ.ScenarioClaudeCode, meta.ID)
	cfg.ccRuleLocked(profiled, "cc").Services = []*loadbalance.Service{switchSvc("work-prov", "work-model")}
	_, err = cfg.SetClaudeCodeSlot(profiled, "subagent", true)
	require.NoError(t, err)
	cfg.ccRuleLocked(profiled, "subagent").Services = []*loadbalance.Service{switchSvc("work-prov", "work-cheap")}
	return cfg, meta
}

func mainModel(cfg *Config, slot string) string {
	r := cfg.ccRuleLocked(typ.ScenarioClaudeCode, slot)
	if r == nil || len(r.Services) == 0 {
		return ""
	}
	return r.Services[0].Model
}

func TestUseProfileAsDefault_OverwritesWithACopy(t *testing.T) {
	cfg, meta := newSwitchTestConfig(t)
	profiled := typ.ProfiledScenarioName(typ.ScenarioClaudeCode, meta.ID)

	require.NoError(t, cfg.UseProfileAsDefault("work")) // by name

	assert.Equal(t, "work-model", mainModel(cfg, "cc"))
	assert.Equal(t, []string{"subagent"}, cfg.ClaudeCodeSlots(typ.ScenarioClaudeCode), "the profile's split slots become the default's")
	assert.Equal(t, "work-cheap", mainModel(cfg, "subagent"))

	// The profile is untouched, and nothing is shared with the default.
	assert.Equal(t, "work-model", cfg.ccRuleLocked(profiled, "cc").Services[0].Model)
	assert.Equal(t, []string{"subagent"}, cfg.ClaudeCodeSlots(profiled))
	cfg.ccRuleLocked(typ.ScenarioClaudeCode, "cc").Services[0].Model = "edited"
	assert.Equal(t, "work-model", cfg.ccRuleLocked(profiled, "cc").Services[0].Model, "services are copied, not shared")
}

func TestUseProfileAsDefault_SlotsFollowTheProfile(t *testing.T) {
	cfg, _ := newSwitchTestConfig(t)
	// The default had haiku split; the profile does not split it.
	_, err := cfg.SetClaudeCodeSlot(typ.ScenarioClaudeCode, "haiku", true)
	require.NoError(t, err)
	require.Equal(t, []string{"haiku"}, cfg.ClaudeCodeSlots(typ.ScenarioClaudeCode))

	require.NoError(t, cfg.UseProfileAsDefault("p1")) // by id
	assert.Equal(t, []string{"subagent"}, cfg.ClaudeCodeSlots(typ.ScenarioClaudeCode))
}

func TestUseProfileAsDefault_SecondUseReplacesTheFirst(t *testing.T) {
	cfg, _ := newSwitchTestConfig(t)
	other, err := cfg.CreateProfile(typ.ScenarioClaudeCode, "play")
	require.NoError(t, err)
	cfg.ccRuleLocked(typ.ProfiledScenarioName(typ.ScenarioClaudeCode, other.ID), "cc").Services = []*loadbalance.Service{switchSvc("play-prov", "play-model")}

	require.NoError(t, cfg.UseProfileAsDefault("work"))
	require.NoError(t, cfg.UseProfileAsDefault("play"))
	assert.Equal(t, "play-model", mainModel(cfg, "cc"))
	assert.Empty(t, cfg.ClaudeCodeSlots(typ.ScenarioClaudeCode))
}

func TestUseProfileAsDefault_PersistsAcrossReload(t *testing.T) {
	dir := t.TempDir()
	cfg, err := NewConfigWithDir(dir)
	require.NoError(t, err)
	meta, err := cfg.CreateProfile(typ.ScenarioClaudeCode, "work")
	require.NoError(t, err)
	cfg.ccRuleLocked(typ.ProfiledScenarioName(typ.ScenarioClaudeCode, meta.ID), "cc").Services = []*loadbalance.Service{switchSvc("w", "work-model")}
	require.NoError(t, cfg.UseProfileAsDefault(meta.ID))

	reloaded, err := NewConfigWithDir(dir)
	require.NoError(t, err)
	assert.Equal(t, "work-model", mainModel(reloaded, "cc"))
}

func TestUseProfileAsDefault_Rejects(t *testing.T) {
	cfg, _ := newSwitchTestConfig(t)

	err := cfg.UseProfileAsDefault("nope")
	assert.ErrorIs(t, err, ErrProfileNotFound)

	empty, err := cfg.CreateProfile(typ.ScenarioClaudeCode, "empty")
	require.NoError(t, err)
	assert.Error(t, cfg.UseProfileAsDefault(empty.ID), "a profile without routing would leave the default unrouted")

	assert.Equal(t, "main", mainModel(cfg, "cc"), "a rejected switch changes nothing")
	assert.Empty(t, cfg.ClaudeCodeSlots(typ.ScenarioClaudeCode))
}

func TestUseProfileAsDefault_FailedSaveLeavesTheDefaultAlone(t *testing.T) {
	cfg, _ := newSwitchTestConfig(t)
	nRules := len(cfg.Rules)
	cfg.ConfigFile = "" // saveLocked refuses an empty path

	require.Error(t, cfg.UseProfileAsDefault("work"))
	assert.Equal(t, "main", mainModel(cfg, "cc"))
	assert.Empty(t, cfg.ClaudeCodeSlots(typ.ScenarioClaudeCode))
	assert.Len(t, cfg.Rules, nRules, "rules created by the failed switch are dropped again")
}
