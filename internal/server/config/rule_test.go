package config

import (
	"strings"
	"testing"

	"github.com/tingly-dev/tingly-box/internal/typ"
)

func TestAddRule_DuplicateNameSameScenario(t *testing.T) {
	cfg, err := NewConfig(WithConfigDir(t.TempDir()))
	if err != nil {
		t.Fatalf("NewConfig error: %v", err)
	}

	rule1 := typ.Rule{
		UUID:         "uuid-1",
		Scenario:     "openai",
		RequestModel: "gpt-4",
	}
	if err := cfg.AddRule(rule1); err != nil {
		t.Fatalf("first AddRule failed: %v", err)
	}

	rule2 := typ.Rule{
		UUID:         "uuid-2",
		Scenario:     "openai",
		RequestModel: "gpt-4",
	}
	err = cfg.AddRule(rule2)
	if err == nil {
		t.Fatal("expected error for duplicate name in same scenario, got nil")
	}
	if !strings.Contains(err.Error(), "already exists") {
		t.Errorf("unexpected error message: %v", err)
	}
}

func TestAddRule_DuplicateNameDifferentScenario(t *testing.T) {
	cfg, err := NewConfig(WithConfigDir(t.TempDir()))
	if err != nil {
		t.Fatalf("NewConfig error: %v", err)
	}

	rule1 := typ.Rule{
		UUID:         "uuid-1",
		Scenario:     "openai",
		RequestModel: "gpt-4",
	}
	if err := cfg.AddRule(rule1); err != nil {
		t.Fatalf("first AddRule failed: %v", err)
	}

	// Same request_model but different scenario — must succeed
	rule2 := typ.Rule{
		UUID:         "uuid-2",
		Scenario:     "anthropic",
		RequestModel: "gpt-4",
	}
	if err := cfg.AddRule(rule2); err != nil {
		t.Errorf("AddRule with same name in different scenario should succeed, got: %v", err)
	}
}

func TestAddRule_TeamSeedsCreateDefaults(t *testing.T) {
	cfg, err := NewConfig(WithConfigDir(t.TempDir()))
	if err != nil {
		t.Fatalf("NewConfig error: %v", err)
	}

	// A bare team rule (no flags) — as any non-HTTP path (CLI/TUI/import) would
	// build it — must come out with the team creation defaults seeded on.
	if err := cfg.AddRule(typ.Rule{UUID: "team-1", Scenario: typ.ScenarioTeam, RequestModel: "m"}); err != nil {
		t.Fatalf("AddRule failed: %v", err)
	}
	seeded := cfg.GetRuleByUUID("team-1")
	if seeded == nil {
		t.Fatal("team rule not found after AddRule")
	}
	if !seeded.Flags.ClaudeCodeCompat || !seeded.Flags.CleanHeader {
		t.Errorf("expected team defaults seeded, got %+v", seeded.Flags)
	}

	// An explicit flag set is left untouched — the defaults are not layered on.
	if err := cfg.AddRule(typ.Rule{UUID: "team-2", Scenario: typ.ScenarioTeam, RequestModel: "m2", Flags: typ.RuleFlags{SkipUsage: true}}); err != nil {
		t.Fatalf("AddRule failed: %v", err)
	}
	explicit := cfg.GetRuleByUUID("team-2")
	if explicit == nil {
		t.Fatal("explicit team rule not found after AddRule")
	}
	if !explicit.Flags.SkipUsage || explicit.Flags.ClaudeCodeCompat || explicit.Flags.CleanHeader {
		t.Errorf("explicit flags must not be overridden, got %+v", explicit.Flags)
	}

	// A non-team rule keeps its empty flag set.
	if err := cfg.AddRule(typ.Rule{UUID: "oai-1", Scenario: typ.ScenarioOpenAI, RequestModel: "m3"}); err != nil {
		t.Fatalf("AddRule failed: %v", err)
	}
	other := cfg.GetRuleByUUID("oai-1")
	if other == nil {
		t.Fatal("openai rule not found after AddRule")
	}
	if other.Flags != (typ.RuleFlags{}) {
		t.Errorf("non-team rule should keep empty flags, got %+v", other.Flags)
	}
}

func TestAddRule_DuplicateUUID(t *testing.T) {
	cfg, err := NewConfig(WithConfigDir(t.TempDir()))
	if err != nil {
		t.Fatalf("NewConfig error: %v", err)
	}

	rule1 := typ.Rule{
		UUID:         "uuid-1",
		Scenario:     "openai",
		RequestModel: "gpt-4",
	}
	if err := cfg.AddRule(rule1); err != nil {
		t.Fatalf("first AddRule failed: %v", err)
	}

	rule2 := typ.Rule{
		UUID:         "uuid-1", // same UUID, different model
		Scenario:     "openai",
		RequestModel: "gpt-3.5-turbo",
	}
	if err := cfg.AddRule(rule2); err == nil {
		t.Fatal("expected error for duplicate UUID, got nil")
	}
}
