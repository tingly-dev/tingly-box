package config

import (
	"testing"

	typ "github.com/tingly-dev/tingly-box/internal/typ"
)

func ccRule(uuid, profileID, requestModel string) typ.Rule {
	return typ.Rule{
		UUID:         uuid,
		Scenario:     typ.ProfiledScenarioName(typ.ScenarioClaudeCode, profileID),
		RequestModel: requestModel,
		Active:       true,
	}
}

func TestMigrate20260519_UnifiedSingleRule(t *testing.T) {
	c := &Config{
		Profiles: map[string][]typ.ProfileMeta{
			string(typ.ScenarioClaudeCode): {
				{ID: "p1", Name: "my-unified", Unified: true},
			},
		},
		Rules: []typ.Rule{
			ccRule("random-uuid-1", "p1", "cc"),
		},
	}

	migrate20260519(c)

	if len(c.Rules) != 1 {
		t.Fatalf("expected 1 rule, got %d", len(c.Rules))
	}
	want := "builtin:claude_code:p1:cc"
	if c.Rules[0].UUID != want {
		t.Errorf("UUID = %q, want %q", c.Rules[0].UUID, want)
	}
	if c.Rules[0].RequestModel != "cc" {
		t.Errorf("RequestModel = %q, want %q", c.Rules[0].RequestModel, "cc")
	}
}

func TestMigrate20260519_SeparateFiveRules(t *testing.T) {
	// Rules in mixed order to verify match-by-RequestModel logic.
	c := &Config{
		Profiles: map[string][]typ.ProfileMeta{
			string(typ.ScenarioClaudeCode): {
				{ID: "p2", Name: "my-separate", Unified: false},
			},
		},
		Rules: []typ.Rule{
			ccRule("uuid-haiku", "p2", "haiku"),
			ccRule("uuid-default", "p2", "default"),
			ccRule("uuid-subagent", "p2", "subagent"),
			ccRule("uuid-opus", "p2", "opus"),
			ccRule("uuid-sonnet", "p2", "sonnet"),
		},
	}

	migrate20260519(c)

	want := map[string]string{
		"default":  "builtin:claude_code:p2:default",
		"haiku":    "builtin:claude_code:p2:haiku",
		"sonnet":   "builtin:claude_code:p2:sonnet",
		"opus":     "builtin:claude_code:p2:opus",
		"subagent": "builtin:claude_code:p2:subagent",
	}
	got := map[string]string{}
	for _, r := range c.Rules {
		got[r.RequestModel] = r.UUID
	}
	if len(got) != len(want) {
		t.Fatalf("got %d distinct models, want %d (rules=%+v)", len(got), len(want), c.Rules)
	}
	for k, v := range want {
		if got[k] != v {
			t.Errorf("model %s: UUID = %q, want %q", k, got[k], v)
		}
	}
}

func TestMigrate20260519_SeparateOverflowDropped(t *testing.T) {
	c := &Config{
		Profiles: map[string][]typ.ProfileMeta{
			string(typ.ScenarioClaudeCode): {
				{ID: "p3", Name: "overflow", Unified: false},
			},
		},
		Rules: []typ.Rule{
			ccRule("u1", "p3", "default"),
			ccRule("u2", "p3", "haiku"),
			ccRule("u3", "p3", "sonnet"),
			ccRule("u4", "p3", "opus"),
			ccRule("u5", "p3", "subagent"),
			ccRule("u6", "p3", "extra"),
			ccRule("u7", "p3", "another"),
		},
	}

	migrate20260519(c)

	if len(c.Rules) != 5 {
		t.Fatalf("expected 5 rules after overflow drop, got %d", len(c.Rules))
	}
	models := map[string]bool{}
	for _, r := range c.Rules {
		models[r.RequestModel] = true
	}
	for _, m := range []string{"default", "haiku", "sonnet", "opus", "subagent"} {
		if !models[m] {
			t.Errorf("missing canonical model %q", m)
		}
	}
}

func TestMigrate20260519_SeparateFillsMissing(t *testing.T) {
	// Only 2 rules but separate mode → must be filled to 5.
	c := &Config{
		Profiles: map[string][]typ.ProfileMeta{
			string(typ.ScenarioClaudeCode): {
				{ID: "p4", Name: "partial", Unified: false},
			},
		},
		Rules: []typ.Rule{
			ccRule("u1", "p4", "haiku"),
			ccRule("u2", "p4", "opus"),
		},
	}

	migrate20260519(c)

	if len(c.Rules) != 5 {
		t.Fatalf("expected 5 rules after fill, got %d", len(c.Rules))
	}
	want := map[string]string{
		"default":  "builtin:claude_code:p4:default",
		"haiku":    "builtin:claude_code:p4:haiku",
		"sonnet":   "builtin:claude_code:p4:sonnet",
		"opus":     "builtin:claude_code:p4:opus",
		"subagent": "builtin:claude_code:p4:subagent",
	}
	got := map[string]string{}
	for _, r := range c.Rules {
		got[r.RequestModel] = r.UUID
	}
	for k, v := range want {
		if got[k] != v {
			t.Errorf("model %s: UUID = %q, want %q", k, got[k], v)
		}
	}
}

func TestMigrate20260519_OrphanedRulesStillMigrated(t *testing.T) {
	// ProfileMeta is gone but rules remain with scenario claude_code:p9.
	c := &Config{
		Profiles: map[string][]typ.ProfileMeta{},
		Rules: []typ.Rule{
			ccRule("orphan-1", "p9", "cc"),
		},
	}

	migrate20260519(c)

	if len(c.Rules) != 1 {
		t.Fatalf("expected 1 rule, got %d", len(c.Rules))
	}
	want := "builtin:claude_code:p9:cc"
	if c.Rules[0].UUID != want {
		t.Errorf("orphan UUID = %q, want %q", c.Rules[0].UUID, want)
	}
}

func TestMigrate20260519_IdempotentOnAlreadyMigrated(t *testing.T) {
	c := &Config{
		Profiles: map[string][]typ.ProfileMeta{
			string(typ.ScenarioClaudeCode): {
				{ID: "p1", Name: "u", Unified: true},
			},
		},
		Rules: []typ.Rule{
			ccRule("builtin:claude_code:p1:cc", "p1", "cc"),
		},
	}

	migrate20260519(c)
	migrate20260519(c)

	if c.Rules[0].UUID != "builtin:claude_code:p1:cc" {
		t.Errorf("UUID changed unexpectedly: %q", c.Rules[0].UUID)
	}
	if len(c.Rules) != 1 {
		t.Errorf("rule count changed: %d", len(c.Rules))
	}
}

func TestMigrate20260519_IgnoresNonClaudeCode(t *testing.T) {
	c := &Config{
		Profiles: map[string][]typ.ProfileMeta{
			string(typ.ScenarioClaudeCode): {
				{ID: "p1", Name: "u", Unified: true},
			},
		},
		Rules: []typ.Rule{
			{
				UUID:         "openai-uuid",
				Scenario:     typ.ScenarioOpenAI,
				RequestModel: "gpt-4",
				Active:       true,
			},
			ccRule("cc-uuid", "p1", "cc"),
		},
	}

	migrate20260519(c)

	for _, r := range c.Rules {
		if r.Scenario == typ.ScenarioOpenAI && r.UUID != "openai-uuid" {
			t.Errorf("non-claude_code rule was modified: %+v", r)
		}
	}
}
