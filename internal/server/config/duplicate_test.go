package config

import (
	"errors"
	"testing"

	"github.com/tingly-dev/tingly-box/internal/constant"
	"github.com/tingly-dev/tingly-box/internal/db"
	"github.com/tingly-dev/tingly-box/internal/loadbalance"
	"github.com/tingly-dev/tingly-box/internal/typ"
)

func newDuplicateTestConfig(t *testing.T) *Config {
	t.Helper()
	cfg, err := NewConfig(WithConfigDir(t.TempDir()))
	if err != nil {
		t.Fatalf("NewConfig: %v", err)
	}
	return cfg
}

func rulesIn(cfg *Config, scenario typ.RuleScenario) []typ.Rule {
	var out []typ.Rule
	for _, r := range cfg.Rules {
		if r.Scenario == scenario {
			out = append(out, r)
		}
	}
	return out
}

func TestDuplicateProfile_FromProfile(t *testing.T) {
	cfg := newDuplicateTestConfig(t)
	cc := typ.ScenarioClaudeCode
	src, err := cfg.CreateProfile(cc, "work", false)
	if err != nil {
		t.Fatal(err)
	}
	srcScenario := typ.ProfiledScenarioName(cc, src.ID)
	for i := range cfg.Rules {
		if cfg.Rules[i].UUID == BuiltinRuleUUID(srcScenario, "sonnet") {
			cfg.Rules[i].Services = []*loadbalance.Service{{Provider: "prov-1", Model: "m-1", Active: true}}
		}
	}
	if err := cfg.SetScenarioFlag(srcScenario, constant.FlagSmartCompact, true); err != nil {
		t.Fatal(err)
	}

	res, err := cfg.Duplicate(cc, src.ID, "work-copy")
	if err != nil {
		t.Fatalf("Duplicate: %v", err)
	}
	if meta, _ := cfg.GetProfile(cc, res.ID); meta.Unified {
		t.Error("mode not copied")
	}
	if got := rulesIn(cfg, res.Scenario); len(got) != 5 {
		t.Fatalf("got %d rules, want the 5 separate-mode tiers", len(got))
	}
	copied := cfg.GetRuleByUUID(BuiltinRuleUUID(res.Scenario, "sonnet"))
	if copied == nil || len(copied.Services) != 1 || copied.Services[0].Provider != "prov-1" {
		t.Fatalf("sonnet tier not copied under its canonical UUID: %+v", copied)
	}
	if !cfg.GetScenarioFlag(res.Scenario, constant.FlagSmartCompact) {
		t.Error("the profile's own flag was not copied")
	}

	var notFound ErrDuplicateSourceNotFound
	if _, err := cfg.Duplicate(cc, "p99", "x"); !errors.As(err, &notFound) {
		t.Errorf("unknown source: got %v", err)
	}
	if _, err := cfg.Duplicate(cc, src.ID, "work"); err == nil {
		t.Error("a taken name must be refused")
	}
}

// The main config keeps both rule sets and toggles them by mode; its copy
// takes the current mode's set under the UUIDs the profile's generated
// settings look each tier up by, and keeps inheriting the main flags.
func TestDuplicateProfile_FromMainConfiguration(t *testing.T) {
	for _, tc := range []struct {
		separate bool
		tiers    []string
	}{
		{false, []string{"cc"}},
		{true, []string{"default", "haiku", "sonnet", "opus", "subagent"}},
	} {
		cfg := newDuplicateTestConfig(t)
		cc := typ.ScenarioClaudeCode
		if err := cfg.SetScenarioFlag(cc, constant.FlagSeparate, tc.separate); err != nil {
			t.Fatal(err)
		}
		if err := cfg.SetScenarioFlag(cc, constant.FlagUnified, !tc.separate); err != nil {
			t.Fatal(err)
		}
		res, err := cfg.Duplicate(cc, MainScopeID, "main-copy")
		if err != nil {
			t.Fatalf("Duplicate: %v", err)
		}
		if meta, _ := cfg.GetProfile(cc, res.ID); meta.Unified == tc.separate {
			t.Errorf("separate=%v: profile Unified=%v", tc.separate, meta.Unified)
		}
		if got := rulesIn(cfg, res.Scenario); len(got) != len(tc.tiers) {
			t.Errorf("separate=%v: got %d rules, want %d", tc.separate, len(got), len(tc.tiers))
		}
		for _, tier := range tc.tiers {
			if r := cfg.GetRuleByUUID(BuiltinRuleUUID(res.Scenario, tier)); r == nil || !r.Active {
				t.Errorf("separate=%v: no active %s rule at the canonical profile UUID", tc.separate, tier)
			}
		}
		if err := cfg.SetScenarioFlag(cc, constant.FlagSmartCompact, true); err != nil {
			t.Fatal(err)
		}
		if !cfg.GetScenarioFlag(res.Scenario, constant.FlagSmartCompact) {
			t.Error("the copy stopped inheriting the main config's flags")
		}
	}
}

func TestDuplicateTeam(t *testing.T) {
	cfg := newDuplicateTestConfig(t)
	teams := cfg.StoreManager().Team()
	cfg.Rules = append(cfg.Rules, typ.Rule{UUID: "r-team", Scenario: typ.ScenarioTeam, RequestModel: "gpt-shared", Active: true})

	// MainScopeID is the Default team, whose config every team inherits live.
	res, err := cfg.Duplicate(typ.ScenarioTeam, MainScopeID, "Default copy")
	if err != nil {
		t.Fatalf("Duplicate: %v", err)
	}
	if res.Scenario != db.TeamScenario(res.ID) {
		t.Fatalf("result scenario %q", res.Scenario)
	}
	if got := rulesIn(cfg, res.Scenario); len(got) != 1 || got[0].RequestModel != "gpt-shared" || got[0].UUID == "r-team" {
		t.Fatalf("unexpected copied rules: %+v", got)
	}
	if err := cfg.SetScenarioFlag(typ.ScenarioTeam, constant.FlagSmartCompact, true); err != nil {
		t.Fatal(err)
	}
	if !cfg.GetScenarioFlag(res.Scenario, constant.FlagSmartCompact) {
		t.Error("the copy stopped inheriting the Default team's flags")
	}

	// A failed copy doesn't leave an empty team behind.
	before := len(teams.List())
	cfg.ConfigFile = "" // Save now fails
	if _, err := cfg.Duplicate(typ.ScenarioTeam, MainScopeID, "Broken"); err == nil {
		t.Fatal("expected the save failure to surface")
	}
	if after := len(teams.List()); after != before {
		t.Errorf("team count %d → %d after a failed duplicate", before, after)
	}
}
