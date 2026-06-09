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
	// session_affinity has moved to a rule-only flag (migrate20260609*).
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

func TestMigrate20260608_DefaultsClaudeCodeCompat(t *testing.T) {
	c := &Config{
		Rules: []typ.Rule{
			{UUID: "built-in-cc", Scenario: typ.ScenarioClaudeCode},
			{UUID: "cc-profile", Scenario: typ.RuleScenario("claude_code:p1")},
			{UUID: "already-on", Scenario: typ.ScenarioClaudeCode, Flags: typ.RuleFlags{ClaudeCodeCompat: true}},
			{UUID: "openai", Scenario: typ.ScenarioOpenAI},
		},
	}

	migrate20260608(c)

	if !c.hasMigrationCompleted("20260608") {
		t.Fatal("migration should be marked completed")
	}
	want := map[string]bool{
		"built-in-cc": true,  // claude_code base → defaulted on
		"cc-profile":  true,  // claude_code:<profile> → defaulted on
		"already-on":  true,  // unchanged
		"openai":      false, // non-claude_code → untouched
	}
	for _, r := range c.Rules {
		if got := r.Flags.ClaudeCodeCompat; got != want[r.UUID] {
			t.Errorf("rule %q ClaudeCodeCompat = %v, want %v", r.UUID, got, want[r.UUID])
		}
	}
}

func TestMigrate20260608_OneTime_PreservesUserOff(t *testing.T) {
	c := &Config{
		Rules: []typ.Rule{{UUID: "built-in-cc", Scenario: typ.ScenarioClaudeCode}},
	}

	migrate20260608(c)
	if !c.Rules[0].Flags.ClaudeCodeCompat {
		t.Fatal("first run should default the flag on")
	}

	// User turns it off; a later boot must not re-enable it.
	c.Rules[0].Flags.ClaudeCodeCompat = false
	migrate20260608(c)
	if c.Rules[0].Flags.ClaudeCodeCompat {
		t.Error("migration re-enabled a user-disabled flag; one-time gate is broken")
	}
}

func TestMigrate20260608_2_DefaultsCleanHeader(t *testing.T) {
	c := &Config{
		Rules: []typ.Rule{
			{UUID: "built-in-cc", Scenario: typ.ScenarioClaudeCode},
			{UUID: "cc-profile", Scenario: typ.RuleScenario("claude_code:p1")},
			{UUID: "already-on", Scenario: typ.ScenarioClaudeCode, Flags: typ.RuleFlags{CleanHeader: true}},
			{UUID: "desktop", Scenario: typ.ScenarioClaudeDesktop},
			{UUID: "openai", Scenario: typ.ScenarioOpenAI},
		},
	}

	migrate20260608_2(c)

	if !c.hasMigrationCompleted("20260608_2") {
		t.Fatal("migration should be marked completed")
	}
	want := map[string]bool{
		"built-in-cc": true,  // claude_code base → defaulted on
		"cc-profile":  true,  // claude_code:<profile> → defaulted on
		"already-on":  true,  // unchanged
		"desktop":     false, // claude_desktop — handled by migrate20260608_3
		"openai":      false, // non-claude_code → untouched
	}
	for _, r := range c.Rules {
		if got := r.Flags.CleanHeader; got != want[r.UUID] {
			t.Errorf("rule %q CleanHeader = %v, want %v", r.UUID, got, want[r.UUID])
		}
	}
}

func TestMigrate20260608_2_OneTime_PreservesUserOff(t *testing.T) {
	c := &Config{
		Rules: []typ.Rule{{UUID: "built-in-cc", Scenario: typ.ScenarioClaudeCode}},
	}

	migrate20260608_2(c)
	if !c.Rules[0].Flags.CleanHeader {
		t.Fatal("first run should default the flag on")
	}

	// User turns it off; a later boot must not re-enable it.
	c.Rules[0].Flags.CleanHeader = false
	migrate20260608_2(c)
	if c.Rules[0].Flags.CleanHeader {
		t.Error("migration re-enabled a user-disabled flag; one-time gate is broken")
	}
}

func TestMigrate20260609_DefaultsSessionAffinity(t *testing.T) {
	c := &Config{
		Rules: []typ.Rule{
			{UUID: "built-in-cc", Scenario: typ.ScenarioClaudeCode},
			{UUID: "cc-profile", Scenario: typ.RuleScenario("claude_code:p1")},
			{UUID: "already-set", Scenario: typ.ScenarioClaudeCode, Flags: typ.RuleFlags{SessionAffinity: 900}},
			{UUID: "desktop", Scenario: typ.ScenarioClaudeDesktop},
			{UUID: "codex", Scenario: typ.ScenarioCodex},
			{UUID: "openai", Scenario: typ.ScenarioOpenAI},
		},
	}

	migrate20260609(c)   // claude_code
	migrate20260609_2(c) // claude_desktop
	migrate20260609_3(c) // codex

	for _, marker := range []string{"20260609", "20260609_2", "20260609_3"} {
		if !c.hasMigrationCompleted(marker) {
			t.Fatalf("migration %s should be marked completed", marker)
		}
	}
	want := map[string]int{
		"built-in-cc": defaultSessionAffinitySeconds, // claude_code base → defaulted on
		"cc-profile":  defaultSessionAffinitySeconds, // claude_code:<profile> → defaulted on
		"already-set": 900,                           // user value preserved
		"desktop":     defaultSessionAffinitySeconds, // claude_desktop → defaulted on
		"codex":       defaultSessionAffinitySeconds, // codex → defaulted on
		"openai":      0,                             // out of scope → untouched
	}
	for _, r := range c.Rules {
		if got := r.Flags.SessionAffinity; got != want[r.UUID] {
			t.Errorf("rule %q SessionAffinity = %d, want %d", r.UUID, got, want[r.UUID])
		}
	}
}

func TestMigrate20260609_OneTime_PreservesUserDisabled(t *testing.T) {
	c := &Config{
		Rules: []typ.Rule{{UUID: "built-in-cc", Scenario: typ.ScenarioClaudeCode}},
	}

	migrate20260609(c)
	if c.Rules[0].Flags.SessionAffinity != defaultSessionAffinitySeconds {
		t.Fatal("first run should default session affinity on")
	}

	// User disables it (0); a later boot must not re-enable it.
	c.Rules[0].Flags.SessionAffinity = 0
	migrate20260609(c)
	if c.Rules[0].Flags.SessionAffinity != 0 {
		t.Error("migration re-enabled a user-disabled affinity; one-time gate is broken")
	}
}
