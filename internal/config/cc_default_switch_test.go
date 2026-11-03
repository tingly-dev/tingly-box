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

func TestSwitchClaudeCodeDefaultToProfile_CopiesRoutingAndKeepsProfile(t *testing.T) {
	cfg, meta := newSwitchTestConfig(t)
	profiled := typ.ProfiledScenarioName(typ.ScenarioClaudeCode, meta.ID)
	assert.False(t, cfg.GetClaudeCodeDefaultSwitch().Switched)

	status, err := cfg.SwitchClaudeCodeDefaultToProfile("work") // by name
	require.NoError(t, err)
	assert.Equal(t, ClaudeCodeDefaultSwitchStatus{Switched: true, ProfileID: meta.ID, ProfileName: "work", ProfileExists: true, InSync: true}, status)

	assert.Equal(t, "work-model", mainModel(cfg, "cc"))
	assert.Equal(t, []string{"subagent"}, cfg.ClaudeCodeSlots(typ.ScenarioClaudeCode), "the profile's split slots become the default's")
	assert.Equal(t, "work-cheap", mainModel(cfg, "subagent"))

	// Source untouched, and not aliased to the default.
	assert.Equal(t, "work-model", cfg.ccRuleLocked(profiled, "cc").Services[0].Model)
	cfg.ccRuleLocked(typ.ScenarioClaudeCode, "cc").Services[0].Model = "edited"
	assert.Equal(t, "work-model", cfg.ccRuleLocked(profiled, "cc").Services[0].Model, "services are copied, not shared")
	assert.Equal(t, []string{"subagent"}, cfg.ClaudeCodeSlots(profiled))
	st := cfg.GetClaudeCodeDefaultSwitch()
	assert.True(t, st.Switched)
	assert.False(t, st.InSync, "editing the default after the switch is reported")
}

func TestSwitchClaudeCodeDefaultToProfile_SlotsFollowProfile(t *testing.T) {
	cfg, _ := newSwitchTestConfig(t)
	// The default had haiku split; the profile does not split it.
	_, err := cfg.SetClaudeCodeSlot(typ.ScenarioClaudeCode, "haiku", true)
	require.NoError(t, err)
	assert.Equal(t, []string{"haiku"}, cfg.ClaudeCodeSlots(typ.ScenarioClaudeCode))

	_, err = cfg.SwitchClaudeCodeDefaultToProfile("p1")
	require.NoError(t, err)
	assert.Equal(t, []string{"subagent"}, cfg.ClaudeCodeSlots(typ.ScenarioClaudeCode))
}

func TestSwitchClaudeCodeDefaultToProfile_SwitchBetweenProfilesKeepsOriginal(t *testing.T) {
	cfg, _ := newSwitchTestConfig(t)
	other, err := cfg.CreateProfile(typ.ScenarioClaudeCode, "play")
	require.NoError(t, err)
	cfg.ccRuleLocked(typ.ProfiledScenarioName(typ.ScenarioClaudeCode, other.ID), "cc").Services = []*loadbalance.Service{switchSvc("play-prov", "play-model")}

	_, err = cfg.SwitchClaudeCodeDefaultToProfile("work")
	require.NoError(t, err)
	status, err := cfg.SwitchClaudeCodeDefaultToProfile("play")
	require.NoError(t, err)
	assert.Equal(t, "play", status.ProfileName)
	assert.Equal(t, "play-model", mainModel(cfg, "cc"))
	assert.Empty(t, cfg.ClaudeCodeSlots(typ.ScenarioClaudeCode))

	// Restore goes back to the default from before any switch, not to "work".
	require.NoError(t, cfg.RestoreClaudeCodeDefault())
	assert.Equal(t, "main", mainModel(cfg, "cc"))
	assert.Empty(t, cfg.ClaudeCodeSlots(typ.ScenarioClaudeCode))
	assert.False(t, cfg.GetClaudeCodeDefaultSwitch().Switched)
}

func TestRestoreClaudeCodeDefault(t *testing.T) {
	cfg, _ := newSwitchTestConfig(t)
	assert.Error(t, cfg.RestoreClaudeCodeDefault(), "nothing to restore before a switch")

	// A previously configured slot rule is brought back, and the rule the
	// switch created from nothing is removed again.
	_, err := cfg.SetClaudeCodeSlot(typ.ScenarioClaudeCode, "haiku", true)
	require.NoError(t, err)
	cfg.ccRuleLocked(typ.ScenarioClaudeCode, "haiku").Services = []*loadbalance.Service{switchSvc("p", "haiku-own")}
	cfg.Rules = deleteRuleByUUID(cfg.Rules, BuiltinRuleUUID(typ.ScenarioClaudeCode, "subagent"))
	require.Nil(t, cfg.ccRuleLocked(typ.ScenarioClaudeCode, "subagent"))

	_, err = cfg.SwitchClaudeCodeDefaultToProfile("work")
	require.NoError(t, err)
	require.NotNil(t, cfg.ccRuleLocked(typ.ScenarioClaudeCode, "subagent"))

	require.NoError(t, cfg.RestoreClaudeCodeDefault())
	assert.Equal(t, "main", mainModel(cfg, "cc"))
	assert.Equal(t, []string{"haiku"}, cfg.ClaudeCodeSlots(typ.ScenarioClaudeCode))
	assert.Equal(t, "haiku-own", mainModel(cfg, "haiku"))
	assert.Nil(t, cfg.ccRuleLocked(typ.ScenarioClaudeCode, "subagent"))
	assert.Error(t, cfg.RestoreClaudeCodeDefault(), "restoring consumes the saved default")
}

func TestSwitchClaudeCodeDefaultToProfile_Rejects(t *testing.T) {
	cfg, _ := newSwitchTestConfig(t)
	_, err := cfg.SwitchClaudeCodeDefaultToProfile("nope")
	assert.Error(t, err)

	empty, err := cfg.CreateProfile(typ.ScenarioClaudeCode, "empty")
	require.NoError(t, err)
	_, err = cfg.SwitchClaudeCodeDefaultToProfile(empty.ID)
	assert.Error(t, err, "a profile without routing would leave the default unrouted")

	assert.Equal(t, "main", mainModel(cfg, "cc"), "a rejected switch changes nothing")
	assert.False(t, cfg.GetClaudeCodeDefaultSwitch().Switched)
}

func TestClaudeCodeDefaultSwitch_ProfileDeletedAndPersisted(t *testing.T) {
	dir := t.TempDir()
	cfg, err := NewConfigWithDir(dir)
	require.NoError(t, err)
	cfg.findRuleByUUID(RuleUUIDCC).Services = []*loadbalance.Service{switchSvc("p", "main")}
	meta, err := cfg.CreateProfile(typ.ScenarioClaudeCode, "work")
	require.NoError(t, err)
	cfg.ccRuleLocked(typ.ProfiledScenarioName(typ.ScenarioClaudeCode, meta.ID), "cc").Services = []*loadbalance.Service{switchSvc("w", "work-model")}
	_, err = cfg.SwitchClaudeCodeDefaultToProfile(meta.ID)
	require.NoError(t, err)

	// The state survives a reload.
	reloaded, err := NewConfigWithDir(dir)
	require.NoError(t, err)
	st := reloaded.GetClaudeCodeDefaultSwitch()
	assert.True(t, st.Switched)
	assert.Equal(t, "work", st.ProfileName)

	// Deleting the profile keeps the default as switched, restorable, flagged.
	require.NoError(t, reloaded.DeleteProfile(typ.ScenarioClaudeCode, meta.ID))
	st = reloaded.GetClaudeCodeDefaultSwitch()
	assert.True(t, st.Switched)
	assert.False(t, st.ProfileExists)
	assert.False(t, st.InSync)
	assert.Equal(t, "work-model", mainModel(reloaded, "cc"))

	require.NoError(t, reloaded.RestoreClaudeCodeDefault())
	again, err := NewConfigWithDir(dir)
	require.NoError(t, err)
	assert.False(t, again.GetClaudeCodeDefaultSwitch().Switched, "clearing the state sticks across a reload")
}

func deleteRuleByUUID(rules []typ.Rule, uuid string) []typ.Rule {
	out := rules[:0:0]
	for _, r := range rules {
		if r.UUID != uuid {
			out = append(out, r)
		}
	}
	return out
}
