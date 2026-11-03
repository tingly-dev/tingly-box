package config

import (
	"strings"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/tingly-dev/tingly-box/internal/db"
	"github.com/tingly-dev/tingly-box/internal/loadbalance"
	"github.com/tingly-dev/tingly-box/internal/typ"
)

// newTeamWithRouting creates a team with a rule, a custom-UUID rule and its
// own scenario config.
func newTeamWithRouting(t *testing.T, cfg *Config, name string) *db.TeamRecord {
	t.Helper()
	team, err := cfg.StoreManager().Team().Create(name)
	require.NoError(t, err)
	scope := db.TeamScenario(team.ID)
	cfg.Rules = append(cfg.Rules,
		typ.Rule{UUID: "rule-" + team.ID, Scenario: scope, RequestModel: "gpt-x", Active: true,
			Services: []*loadbalance.Service{{Provider: "p", Model: "big", Active: true}}},
	)
	require.NoError(t, cfg.SetScenarioConfig(typ.ScenarioConfig{Scenario: scope, Flags: typ.ScenarioFlags{SkipUsage: true}}))
	return team
}

func TestDuplicateTeam_CopiesRoutingNotKeys(t *testing.T) {
	cfg := newSlotTestConfig(t)
	src := newTeamWithRouting(t, cfg, "Research")
	srcScope := db.TeamScenario(src.ID)
	_, err := cfg.StoreManager().APIToken().CreateTokenForTeam("u", "tok-1", src.ID, "key", "u", nil)
	require.NoError(t, err)
	before := rulesOf(cfg, srcScope)

	dup, err := cfg.DuplicateTeam(src.ID, "")
	require.NoError(t, err)
	assert.Equal(t, "Research copy", dup.Name)
	assert.NotEqual(t, src.ID, dup.ID)
	assert.True(t, dup.Enabled)

	dstScope := db.TeamScenario(dup.ID)
	got := rulesOf(cfg, dstScope)
	require.Len(t, got, 1)
	assert.Equal(t, "gpt-x", got[0].RequestModel)
	assert.NotEqual(t, "rule-"+src.ID, got[0].UUID)
	assert.Equal(t, "big", got[0].Services[0].Model)
	assert.True(t, cfg.GetScenarioConfig(dstScope).Flags.SkipUsage)

	// Sharing keys are credentials: the copy starts without any.
	_, total, err := cfg.StoreManager().APIToken().ListTokensForTeam("u", dup.ID, nil, 10, 0)
	require.NoError(t, err)
	assert.Zero(t, total)
	_, total, err = cfg.StoreManager().APIToken().ListTokensForTeam("u", src.ID, nil, 10, 0)
	require.NoError(t, err)
	assert.EqualValues(t, 1, total, "the source keeps its key")

	// The source is untouched, and the copy shares no state with it.
	assert.Equal(t, before, rulesOf(cfg, srcScope))
	got[0].Services[0].Model = "changed"
	assert.Equal(t, "big", cfg.findRuleByUUID("rule-" + src.ID).Services[0].Model)

	second, err := cfg.DuplicateTeam(src.ID, "")
	require.NoError(t, err)
	assert.Equal(t, "Research copy 2", second.Name)
}

func TestDuplicateTeam_DefaultTeamInheritsFlags(t *testing.T) {
	cfg := newSlotTestConfig(t)
	cfg.Rules = append(cfg.Rules, typ.Rule{UUID: "default-team-rule", Scenario: typ.ScenarioTeam, RequestModel: "m", Active: true})
	require.NoError(t, cfg.SetScenarioConfig(typ.ScenarioConfig{Scenario: typ.ScenarioTeam, Flags: typ.ScenarioFlags{SkipUsage: true}}))
	nScenarios := len(cfg.Scenarios)

	dup, err := cfg.DuplicateTeam(db.DefaultTeamID, "From default")
	require.NoError(t, err)
	assert.Equal(t, "From default", dup.Name)
	require.Len(t, rulesOf(cfg, db.TeamScenario(dup.ID)), 1)
	assert.Len(t, cfg.Scenarios, nScenarios, "the Default team's config is inherited, not snapshotted")
	assert.True(t, cfg.GetScenarioConfig(db.TeamScenario(dup.ID)).Flags.SkipUsage)
	assert.Len(t, rulesOf(cfg, typ.ScenarioTeam), 1, "the Default team keeps its rules")
}

func TestDuplicateTeam_Errors(t *testing.T) {
	cfg := newSlotTestConfig(t)
	src := newTeamWithRouting(t, cfg, "Research")

	_, err := cfg.DuplicateTeam("no-such-team", "")
	assert.ErrorContains(t, err, "not found")

	_, err = cfg.DuplicateTeam(src.ID, "Research")
	assert.ErrorContains(t, err, "already exists")
	assert.Len(t, cfg.StoreManager().Team().List(), 2, "a refused copy leaves no team behind")
}

func TestDuplicateTeam_FailedSaveLeavesNothing(t *testing.T) {
	cfg := newSlotTestConfig(t)
	src := newTeamWithRouting(t, cfg, "Research")
	nRules, nScenarios := len(cfg.Rules), len(cfg.Scenarios)
	nTeams := len(cfg.StoreManager().Team().List())

	cfg.ConfigFile = "" // saveLocked fails
	_, err := cfg.DuplicateTeam(src.ID, "")
	require.Error(t, err)
	assert.Len(t, cfg.Rules, nRules)
	assert.Len(t, cfg.Scenarios, nScenarios)
	assert.Len(t, cfg.StoreManager().Team().List(), nTeams, "the team row is taken back out")
}

func TestFreeTeamCopyName(t *testing.T) {
	taken := []db.TeamRecord{{Name: "A copy"}, {Name: "A copy 2"}}
	assert.Equal(t, "A copy 3", freeTeamCopyName("A", taken))
	assert.Equal(t, "B copy", freeTeamCopyName("B", taken))

	long := strings.Repeat("é", 100) // 200 bytes
	name := freeTeamCopyName(long, nil)
	assert.LessOrEqual(t, len(name), maxTeamNameBytes)
	assert.True(t, strings.HasSuffix(name, " copy"))
}

func TestDeleteTeam_RemovesRouting(t *testing.T) {
	cfg := newSlotTestConfig(t)
	gone := newTeamWithRouting(t, cfg, "Gone")
	kept := newTeamWithRouting(t, cfg, "Kept")
	nBase := len(rulesOf(cfg, typ.ScenarioTeam))

	require.NoError(t, cfg.DeleteTeam(gone.ID))
	assert.Empty(t, rulesOf(cfg, db.TeamScenario(gone.ID)))
	assert.Nil(t, cfg.GetScenarioConfig(db.TeamScenario(gone.ID)))
	assert.Len(t, rulesOf(cfg, db.TeamScenario(kept.ID)), 1, "other teams keep their routing")
	assert.Len(t, rulesOf(cfg, typ.ScenarioTeam), nBase)
}

func TestDeleteTeam_RefusedKeepsRouting(t *testing.T) {
	cfg := newSlotTestConfig(t)
	src := newTeamWithRouting(t, cfg, "Keyed")
	_, err := cfg.StoreManager().APIToken().CreateTokenForTeam("u", "tok-1", src.ID, "key", "u", nil)
	require.NoError(t, err)

	assert.ErrorContains(t, cfg.DeleteTeam(src.ID), "sharing key")
	assert.Len(t, rulesOf(cfg, db.TeamScenario(src.ID)), 1, "a refused delete changes nothing")

	assert.Error(t, cfg.DeleteTeam(db.DefaultTeamID))
}

func TestDropOrphanTeamScopes(t *testing.T) {
	cfg := newSlotTestConfig(t)
	live := newTeamWithRouting(t, cfg, "Live")
	cfg.Rules = append(cfg.Rules, typ.Rule{UUID: "orphan", Scenario: "team:deleted-long-ago", RequestModel: "m"})
	cfg.Scenarios = append(cfg.Scenarios, typ.ScenarioConfig{Scenario: "team:also-deleted"})
	baseRules := len(rulesOf(cfg, typ.ScenarioTeam))

	require.True(t, dropOrphanTeamScopes(cfg))
	assert.Empty(t, rulesOf(cfg, "team:deleted-long-ago"))
	assert.Nil(t, cfg.GetScenarioConfig("team:also-deleted"))
	assert.Len(t, rulesOf(cfg, db.TeamScenario(live.ID)), 1, "live teams keep routing")
	assert.Len(t, rulesOf(cfg, typ.ScenarioTeam), baseRules, "the Default team's bare scope is never an orphan")
	assert.False(t, dropOrphanTeamScopes(cfg), "a no-op once nothing is orphaned")
}
