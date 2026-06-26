package config

import (
	"testing"

	"github.com/tingly-dev/tingly-box/internal/loadbalance"
	"github.com/tingly-dev/tingly-box/internal/typ"
)

// svc builds a minimal load-balancing service for migration tests.
func svc(provider string) *loadbalance.Service {
	return &loadbalance.Service{Provider: provider, Model: "m", Active: true}
}

// countRules counts how many rules carry the given UUID (duplicate detection).
func countRules(c *Config, uuid string) int {
	n := 0
	for _, r := range c.Rules {
		if r.UUID == uuid {
			n++
		}
	}
	return n
}

func TestMigrationHelpers(t *testing.T) {
	c := &Config{
		Rules: []typ.Rule{
			{UUID: "a", Scenario: typ.ScenarioOpenAI},
			{UUID: "b", Scenario: typ.ScenarioClaudeCode, Services: []*loadbalance.Service{svc("p1")}},
			{UUID: "c", Scenario: typ.ScenarioCodex, Services: []*loadbalance.Service{svc("p2")}},
		},
	}

	t.Run("findRuleByUUID", func(t *testing.T) {
		if got := c.findRuleByUUID("b"); got == nil || got.UUID != "b" {
			t.Fatalf("findRuleByUUID(b) = %v", got)
		}
		if got := c.findRuleByUUID("missing"); got != nil {
			t.Fatalf("findRuleByUUID(missing) = %v, want nil", got)
		}
	})

	t.Run("defaultRuleByUUID", func(t *testing.T) {
		if _, ok := defaultRuleByUUID(RuleUUIDBuiltinCC); !ok {
			t.Error("expected built-in-cc in DefaultRules")
		}
		if _, ok := defaultRuleByUUID("nope"); ok {
			t.Error("did not expect unknown UUID in DefaultRules")
		}
	})

	t.Run("cloneServices returns a distinct slice", func(t *testing.T) {
		if got := cloneServices(nil); got != nil {
			t.Errorf("cloneServices(nil) = %v, want nil", got)
		}
		src := []*loadbalance.Service{svc("p1")}
		got := cloneServices(src)
		got[0] = svc("changed")
		if src[0].Provider != "p1" {
			t.Error("cloneServices returned an aliasing slice")
		}
	})

	t.Run("referenceServicesFor matches in arg order", func(t *testing.T) {
		// Both rule b (claude_code) and rule c (codex) have services; the first
		// rule in c.Rules matching any requested scenario wins.
		got := c.referenceServicesFor(typ.ScenarioCodex, typ.ScenarioClaudeCode)
		if len(got) != 1 || got[0].Provider != "p1" {
			t.Fatalf("referenceServicesFor = %v, want services from rule b (p1)", got)
		}
		if c.referenceServicesFor(typ.ScenarioAnthropic) != nil {
			t.Error("expected nil when no rule matches")
		}
	})
}

func TestSeedBuiltinRuleIfMissing(t *testing.T) {
	c := &Config{}

	newRule, ok := c.seedBuiltinRuleIfMissing(RuleUUIDBuiltinCodex, []*loadbalance.Service{svc("p1")})
	if !ok {
		t.Fatal("expected missing Codex rule to be seeded")
	}
	if newRule.UUID != RuleUUIDCodex {
		t.Fatalf("seeded rule UUID = %q, want %q", newRule.UUID, RuleUUIDCodex)
	}
	if len(c.Rules) != 1 || c.Rules[0].UUID != RuleUUIDCodex {
		t.Fatalf("config rules after seed = %+v", c.Rules)
	}
	if len(c.Rules[0].Services) != 1 || c.Rules[0].Services[0].Provider != "p1" {
		t.Fatalf("seeded services = %+v", c.Rules[0].Services)
	}

	// The seeded rule must not alias the caller's slice header.
	services := []*loadbalance.Service{svc("p2")}
	newRule, ok = c.seedBuiltinRuleIfMissing(RuleUUIDBuiltinClaudeDesktopHaiku45, services)
	if !ok {
		t.Fatal("expected missing desktop haiku rule to be seeded")
	}
	services[0] = svc("changed")
	seeded := c.findRuleByUUID(RuleUUIDBuiltinClaudeDesktopHaiku45)
	if seeded == nil || len(seeded.Services) != 1 || seeded.Services[0].Provider != "p2" {
		t.Fatalf("seeded services should be cloned, got %+v (returned %+v)", seeded, newRule)
	}

	if _, ok := c.seedBuiltinRuleIfMissing(RuleUUIDBuiltinCodex, nil); ok {
		t.Fatal("existing Codex rule should not be seeded twice")
	}
	if len(c.Rules) != 2 {
		t.Fatalf("rule count after duplicate seed = %d, want 2", len(c.Rules))
	}

	if _, ok := c.seedBuiltinRuleIfMissing("unknown-template", nil); ok {
		t.Fatal("unknown template should be a no-op")
	}
}
