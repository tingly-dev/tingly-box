package config

import (
	"testing"

	"github.com/tingly-dev/tingly-box/internal/constant"
	"github.com/tingly-dev/tingly-box/internal/db"
	"github.com/tingly-dev/tingly-box/internal/typ"
)

func TestDeleteTeam_RemovesItsRouting(t *testing.T) {
	cfg := newDuplicateTestConfig(t)
	team, err := cfg.StoreManager().Team().Create("Research")
	if err != nil {
		t.Fatal(err)
	}
	scope := db.TeamScenario(team.ID)
	cfg.Rules = append(cfg.Rules, typ.Rule{UUID: "r-team", Scenario: scope, RequestModel: "m", Active: true})
	if err := cfg.SetScenarioFlag(scope, constant.FlagSmartCompact, true); err != nil {
		t.Fatal(err)
	}

	// A refused delete (the default team) leaves everything in place.
	if err := cfg.DeleteTeam(db.DefaultTeamID); err == nil {
		t.Fatal("the default team must not be deletable")
	}
	if len(rulesIn(cfg, scope)) != 1 {
		t.Fatal("a refused delete removed routing")
	}

	if err := cfg.DeleteTeam(team.ID); err != nil {
		t.Fatalf("DeleteTeam: %v", err)
	}
	if len(rulesIn(cfg, scope)) != 0 {
		t.Error("the deleted team's rules are still there")
	}
	for _, sc := range cfg.GetScenarios() {
		if sc.Scenario == scope {
			t.Error("the deleted team's scenario config is still there")
		}
	}
	if _, err := cfg.StoreManager().Team().Get(team.ID); err == nil {
		t.Error("the team record is still there")
	}
}

// Routing of teams deleted before DeleteTeam cleaned up after itself is
// dropped at startup; live teams and the Default team's bare scope are kept.
func TestDropOrphanTeamScopes(t *testing.T) {
	cfg := newDuplicateTestConfig(t)
	live, err := cfg.StoreManager().Team().Create("Live")
	if err != nil {
		t.Fatal(err)
	}
	orphan := typ.RuleScenario("team:deleted-long-ago")
	cfg.Rules = append(cfg.Rules,
		typ.Rule{UUID: "r-default", Scenario: typ.ScenarioTeam, RequestModel: "a"},
		typ.Rule{UUID: "r-live", Scenario: db.TeamScenario(live.ID), RequestModel: "b"},
		typ.Rule{UUID: "r-orphan", Scenario: orphan, RequestModel: "c"},
	)
	cfg.Scenarios = append(cfg.Scenarios, typ.ScenarioConfig{Scenario: orphan})

	if !dropOrphanTeamScopes(cfg) {
		t.Fatal("expected the orphan to be dropped")
	}
	if len(rulesIn(cfg, orphan)) != 0 || cfg.GetScenarioConfig(orphan) != nil && cfg.GetScenarioConfig(orphan).Scenario == orphan {
		t.Error("orphaned routing survived")
	}
	if len(rulesIn(cfg, typ.ScenarioTeam)) == 0 || len(rulesIn(cfg, db.TeamScenario(live.ID))) != 1 {
		t.Error("live routing was dropped")
	}
	if dropOrphanTeamScopes(cfg) {
		t.Error("a second run should be a no-op")
	}
}
