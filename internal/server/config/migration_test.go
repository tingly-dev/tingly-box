package config

import (
	"testing"

	"github.com/tingly-dev/tingly-box/internal/typ"
)

func TestMigrate20260606(t *testing.T) {
	tmpDir := t.TempDir()
	cfg, err := NewConfig(WithConfigDir(tmpDir))
	if err != nil {
		t.Fatalf("NewConfig error: %v", err)
	}

	// Trigger migration manually since it already ran during NewConfig
	migrate20260606(cfg)

	// Verify migration completed flag
	if !cfg.hasMigrationCompleted("20260606") {
		t.Error("Migration 20260606 should be marked as completed")
	}

	// migrate20260606 now only defaults SkipUsage on for the Xcode scenario;
	// session_affinity has moved to a rule-only flag (migrate20260610).
	xcodeConfig := cfg.GetScenarioConfig(typ.ScenarioXcode)
	if xcodeConfig == nil {
		t.Error("Xcode scenario config not found")
		return
	}
	if !xcodeConfig.Flags.SkipUsage {
		t.Error("Xcode should have SkipUsage = true")
	}
}

func TestMigrate20260606_Idempotent(t *testing.T) {
	tmpDir := t.TempDir()

	// First run - migration should add defaults
	cfg1, err := NewConfig(WithConfigDir(tmpDir))
	if err != nil {
		t.Fatalf("NewConfig error: %v", err)
	}

	// Debug: print migrations completed
	t.Logf("After first NewConfig, MigrationsCompleted: %v", cfg1.MigrationsCompleted)

	// Verify migration ran
	if !cfg1.hasMigrationCompleted("20260606") {
		t.Error("Migration should have completed after first NewConfig")
	}

	// Verify the Xcode SkipUsage default was set
	xcodeConfig1 := cfg1.GetScenarioConfig(typ.ScenarioXcode)
	if xcodeConfig1 == nil {
		t.Fatal("Xcode scenario config should exist after migration")
	}
	if !xcodeConfig1.Flags.SkipUsage {
		t.Error("Expected default SkipUsage = true after migration")
	}

	// Now user explicitly disables it
	for i := range cfg1.Scenarios {
		if cfg1.Scenarios[i].Scenario == typ.ScenarioXcode {
			cfg1.Scenarios[i].Flags.SkipUsage = false
			cfg1.Save()
			break
		}
	}

	// Second run - migration should NOT run again (already completed)
	// and should NOT overwrite user's explicit setting
	cfg2, err := NewConfig(WithConfigDir(tmpDir))
	if err != nil {
		t.Fatalf("NewConfig error (second run): %v", err)
	}

	// Verify migration is still marked as completed
	if !cfg2.hasMigrationCompleted("20260606") {
		t.Error("Migration should still be marked as completed")
	}

	xcodeConfig2 := cfg2.GetScenarioConfig(typ.ScenarioXcode)
	if xcodeConfig2 == nil {
		t.Fatal("Xcode scenario config should still exist")
	}

	// Should preserve user's explicit choice to disable
	if xcodeConfig2.Flags.SkipUsage {
		t.Error("Expected to preserve user's explicit SkipUsage = false")
	}
}

func TestMigrate20260606_PartialCustomization(t *testing.T) {
	tmpDir := t.TempDir()

	// Create config with xcode scenario that has SkipUsage=false (user override)
	cfg, err := NewConfig(WithConfigDir(tmpDir))
	if err != nil {
		t.Fatalf("NewConfig error: %v", err)
	}

	// Manually set xcode SkipUsage to false (user explicitly disabled it)
	for i := range cfg.Scenarios {
		if cfg.Scenarios[i].Scenario == typ.ScenarioXcode {
			cfg.Scenarios[i].Flags.SkipUsage = false
			cfg.Save()
			break
		}
	}

	// Reload config
	cfg2, err := NewConfig(WithConfigDir(tmpDir))
	if err != nil {
		t.Fatalf("NewConfig error (reload): %v", err)
	}

	// Verify that migration completed but didn't re-run
	if !cfg2.hasMigrationCompleted("20260606") {
		t.Error("Migration should still be marked as completed")
	}

	// Verify that user's SkipUsage=false is preserved
	xcodeConfig := cfg2.GetScenarioConfig(typ.ScenarioXcode)
	if xcodeConfig == nil {
		t.Fatal("Xcode scenario config should exist")
	}

	if xcodeConfig.Flags.SkipUsage != false {
		t.Errorf("Expected user's SkipUsage=false to be preserved, got %v",
			xcodeConfig.Flags.SkipUsage)
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
