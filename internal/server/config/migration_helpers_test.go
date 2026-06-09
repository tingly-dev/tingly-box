package config

import (
	"testing"

	"github.com/tingly-dev/tingly-box/internal/loadbalance"
	"github.com/tingly-dev/tingly-box/internal/typ"
)

func svc(provider string) *loadbalance.Service {
	return &loadbalance.Service{Provider: provider, Model: "m", Active: true}
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

	t.Run("cloneServices", func(t *testing.T) {
		if got := cloneServices(nil); got != nil {
			t.Errorf("cloneServices(nil) = %v, want nil", got)
		}
		src := []*loadbalance.Service{svc("p1")}
		got := cloneServices(src)
		if len(got) != 1 {
			t.Fatalf("cloneServices len = %d, want 1", len(got))
		}
		// Distinct slice header (mutating the copy must not touch the source).
		got[0] = svc("changed")
		if src[0].Provider != "p1" {
			t.Error("cloneServices returned an aliasing slice")
		}
	})

	t.Run("referenceServicesFor prefers first match in arg order over rule order", func(t *testing.T) {
		// claude_code (rule b) and codex (rule c) both have services; the first
		// rule in c.Rules that matches any requested scenario wins.
		got := c.referenceServicesFor(typ.ScenarioCodex, typ.ScenarioClaudeCode)
		if len(got) != 1 || got[0].Provider != "p1" {
			t.Fatalf("referenceServicesFor = %v, want services from rule b (p1)", got)
		}
		if c.referenceServicesFor(typ.ScenarioAnthropic) != nil {
			t.Error("expected nil when no rule matches")
		}
	})
}

func TestMigrate20260513_SeedsDesktopRules(t *testing.T) {
	c := &Config{
		Rules: []typ.Rule{
			{UUID: "built-in-cc", Scenario: typ.ScenarioClaudeCode, Services: []*loadbalance.Service{svc("p1")}},
			// One desktop rule already present — must be left untouched / not duplicated.
			{UUID: RuleUUIDBuiltinClaudeDesktopSonnet46, Scenario: typ.ScenarioClaudeDesktop},
		},
	}

	migrate20260513(c)

	// Sonnet already existed (not duplicated); Opus46 + Opus47 added with copied services.
	for _, uuid := range []string{
		RuleUUIDBuiltinClaudeDesktopSonnet46,
		RuleUUIDBuiltinClaudeDesktopOpus46,
		RuleUUIDBuiltinClaudeDesktopOpus47,
	} {
		if c.findRuleByUUID(uuid) == nil {
			t.Errorf("expected desktop rule %q to exist", uuid)
		}
	}
	count := 0
	for _, r := range c.Rules {
		if r.UUID == RuleUUIDBuiltinClaudeDesktopSonnet46 {
			count++
		}
	}
	if count != 1 {
		t.Errorf("sonnet rule duplicated: count = %d", count)
	}
	opus := c.findRuleByUUID(RuleUUIDBuiltinClaudeDesktopOpus46)
	if opus == nil || len(opus.Services) != 1 || opus.Services[0].Provider != "p1" {
		t.Errorf("opus46 should inherit reference services, got %+v", opus)
	}
}

func TestMigrate20260521_AddsHaikuFromTemplate(t *testing.T) {
	c := &Config{
		Rules: []typ.Rule{
			{UUID: RuleUUIDBuiltinClaudeDesktopSonnet46, Scenario: typ.ScenarioClaudeDesktop, Services: []*loadbalance.Service{svc("pd")}},
		},
	}

	migrate20260521(c)

	haiku := c.findRuleByUUID(RuleUUIDBuiltinClaudeDesktopHaiku45)
	if haiku == nil {
		t.Fatal("expected haiku-4-5 desktop rule to be added")
	}
	// Pulled from the init.go template, so it carries the built-in desktop flags.
	if !haiku.Flags.ClaudeCodeCompat || !haiku.Flags.CleanHeader {
		t.Errorf("haiku rule missing built-in flags: %+v", haiku.Flags)
	}
	// Services mirrored from the existing desktop rule.
	if len(haiku.Services) != 1 || haiku.Services[0].Provider != "pd" {
		t.Errorf("haiku rule should inherit desktop reference services, got %+v", haiku.Services)
	}

	// Idempotent: running again does not duplicate it.
	migrate20260521(c)
	count := 0
	for _, r := range c.Rules {
		if r.UUID == RuleUUIDBuiltinClaudeDesktopHaiku45 {
			count++
		}
	}
	if count != 1 {
		t.Errorf("haiku rule duplicated: count = %d", count)
	}
}

func TestMigrate20260110_CopiesCCServices(t *testing.T) {
	c := &Config{
		Rules: []typ.Rule{
			{UUID: RuleUUIDBuiltinCC, Scenario: typ.ScenarioClaudeCode, Services: []*loadbalance.Service{svc("cc")}},
			{UUID: RuleUUIDBuiltinCCHaiku, Scenario: typ.ScenarioClaudeCode}, // empty → should inherit
			{UUID: RuleUUIDBuiltinCCSonnet, Scenario: typ.ScenarioClaudeCode, Services: []*loadbalance.Service{svc("own")}}, // own → kept
		},
	}

	migrate20260110(c)

	if !c.hasMigrationCompleted("20260110") {
		t.Error("migration should be marked completed")
	}
	haiku := c.findRuleByUUID(RuleUUIDBuiltinCCHaiku)
	if haiku == nil || len(haiku.Services) != 1 || haiku.Services[0].Provider != "cc" {
		t.Errorf("haiku should inherit built-in-cc services, got %+v", haiku)
	}
	sonnet := c.findRuleByUUID(RuleUUIDBuiltinCCSonnet)
	if sonnet == nil || len(sonnet.Services) != 1 || sonnet.Services[0].Provider != "own" {
		t.Errorf("sonnet's own services must be preserved, got %+v", sonnet)
	}
}
