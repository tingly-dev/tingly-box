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
