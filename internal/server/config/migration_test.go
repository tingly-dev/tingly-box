package config

import (
	"testing"

	"github.com/tingly-dev/tingly-box/internal/loadbalance"
	"github.com/tingly-dev/tingly-box/internal/typ"
)

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

func TestMigrate20260513_SeedsDesktopRules(t *testing.T) {
	c := &Config{
		Rules: []typ.Rule{
			{UUID: RuleUUIDBuiltinCC, Scenario: typ.ScenarioClaudeCode, Services: []*loadbalance.Service{svc("p1")}},
			// One desktop rule already present — must be left untouched / not duplicated.
			{UUID: RuleUUIDBuiltinClaudeDesktopSonnet46, Scenario: typ.ScenarioClaudeDesktop},
		},
	}

	migrate20260513(c)

	for _, uuid := range []string{
		RuleUUIDBuiltinClaudeDesktopSonnet46,
		RuleUUIDBuiltinClaudeDesktopOpus46,
		RuleUUIDBuiltinClaudeDesktopOpus47,
	} {
		if c.findRuleByUUID(uuid) == nil {
			t.Errorf("expected desktop rule %q to exist", uuid)
		}
	}
	if n := countRules(c, RuleUUIDBuiltinClaudeDesktopSonnet46); n != 1 {
		t.Errorf("sonnet rule duplicated: count = %d", n)
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
	if n := countRules(c, RuleUUIDBuiltinClaudeDesktopHaiku45); n != 1 {
		t.Errorf("haiku rule duplicated: count = %d", n)
	}
}

func TestMigrate20260606_DefaultsXcodeSkipUsage(t *testing.T) {
	// NewConfig runs Migrate, which applies migrate20260606.
	cfg, err := NewConfig(WithConfigDir(t.TempDir()))
	if err != nil {
		t.Fatalf("NewConfig error: %v", err)
	}

	if !cfg.hasMigrationCompleted("20260606") {
		t.Error("migration 20260606 should be marked completed")
	}
	xcode := cfg.GetScenarioConfig(typ.ScenarioXcode)
	if xcode == nil {
		t.Fatal("Xcode scenario config not found")
	}
	if !xcode.Flags.SkipUsage {
		t.Error("Xcode should default SkipUsage = true")
	}
}

func TestMigrate20260606_PreservesUserOverride(t *testing.T) {
	tmpDir := t.TempDir()

	// First boot applies the default.
	cfg, err := NewConfig(WithConfigDir(tmpDir))
	if err != nil {
		t.Fatalf("NewConfig error: %v", err)
	}
	if x := cfg.GetScenarioConfig(typ.ScenarioXcode); x == nil || !x.Flags.SkipUsage {
		t.Fatalf("expected SkipUsage defaulted on after first boot, got %+v", x)
	}

	// User explicitly disables it.
	for i := range cfg.Scenarios {
		if cfg.Scenarios[i].Scenario == typ.ScenarioXcode {
			cfg.Scenarios[i].Flags.SkipUsage = false
			cfg.Save()
			break
		}
	}

	// Second boot: migration is marker-gated, so the user's choice survives.
	cfg2, err := NewConfig(WithConfigDir(tmpDir))
	if err != nil {
		t.Fatalf("NewConfig error (reload): %v", err)
	}
	if !cfg2.hasMigrationCompleted("20260606") {
		t.Error("migration should still be marked completed after reload")
	}
	if x := cfg2.GetScenarioConfig(typ.ScenarioXcode); x == nil || x.Flags.SkipUsage {
		t.Errorf("user's SkipUsage=false should be preserved across reload, got %+v", x)
	}
}

func TestMigrate20260610_SeedsRuleFlags(t *testing.T) {
	c := &Config{
		Rules: []typ.Rule{
			{UUID: "built-in-cc", Scenario: typ.ScenarioClaudeCode},
			{UUID: "cc-profile", Scenario: typ.RuleScenario("claude_code:p1")},
			{UUID: "cc-compat-on", Scenario: typ.ScenarioClaudeCode, Flags: typ.RuleFlags{ClaudeCodeCompat: true}},
			{UUID: "cc-affinity-set", Scenario: typ.ScenarioClaudeCode, Flags: typ.RuleFlags{SessionAffinity: 900}},
			{UUID: "desktop", Scenario: typ.ScenarioClaudeDesktop},
			{UUID: "codex", Scenario: typ.ScenarioCodex},
			{UUID: "openai", Scenario: typ.ScenarioOpenAI},
		},
	}

	migrate20260610(c)

	if !c.hasMigrationCompleted("20260610") {
		t.Fatal("migration should be marked completed")
	}

	type want struct {
		compat   bool
		clean    bool
		affinity int
	}
	const aff = defaultSessionAffinitySeconds
	wants := map[string]want{
		"built-in-cc":     {compat: true, clean: true, affinity: aff},  // claude_code base → all defaulted on
		"cc-profile":      {compat: true, clean: true, affinity: aff},  // claude_code:<profile> → covered
		"cc-compat-on":    {compat: true, clean: true, affinity: aff},  // already-on compat unchanged, others seeded
		"cc-affinity-set": {compat: true, clean: true, affinity: 900},  // user affinity preserved
		"desktop":         {compat: true, clean: true, affinity: aff},  // claude_desktop → all defaulted on
		"codex":           {compat: false, clean: false, affinity: aff}, // codex → affinity only
		"openai":          {compat: false, clean: false, affinity: 0},   // out of scope → untouched
	}
	for _, r := range c.Rules {
		w := wants[r.UUID]
		if r.Flags.ClaudeCodeCompat != w.compat {
			t.Errorf("rule %q ClaudeCodeCompat = %v, want %v", r.UUID, r.Flags.ClaudeCodeCompat, w.compat)
		}
		if r.Flags.CleanHeader != w.clean {
			t.Errorf("rule %q CleanHeader = %v, want %v", r.UUID, r.Flags.CleanHeader, w.clean)
		}
		if r.Flags.SessionAffinity != w.affinity {
			t.Errorf("rule %q SessionAffinity = %d, want %d", r.UUID, r.Flags.SessionAffinity, w.affinity)
		}
	}
}

func TestMigrate20260610_OneTime_PreservesUserOff(t *testing.T) {
	c := &Config{
		Rules: []typ.Rule{
			{UUID: "built-in-cc", Scenario: typ.ScenarioClaudeCode},
			{UUID: "codex", Scenario: typ.ScenarioCodex},
		},
	}

	migrate20260610(c)
	if !c.Rules[0].Flags.ClaudeCodeCompat || !c.Rules[0].Flags.CleanHeader ||
		c.Rules[0].Flags.SessionAffinity != defaultSessionAffinitySeconds {
		t.Fatal("first run should seed the default flags on the CC rule")
	}
	if c.Rules[1].Flags.SessionAffinity != defaultSessionAffinitySeconds {
		t.Fatal("first run should seed affinity on the Codex rule")
	}

	// User turns everything off; a later boot must not re-enable any of it.
	c.Rules[0].Flags.ClaudeCodeCompat = false
	c.Rules[0].Flags.CleanHeader = false
	c.Rules[0].Flags.SessionAffinity = 0
	c.Rules[1].Flags.SessionAffinity = 0
	migrate20260610(c)
	if c.Rules[0].Flags.ClaudeCodeCompat || c.Rules[0].Flags.CleanHeader ||
		c.Rules[0].Flags.SessionAffinity != 0 || c.Rules[1].Flags.SessionAffinity != 0 {
		t.Error("migration re-enabled user-disabled flags; one-time gate is broken")
	}
}
