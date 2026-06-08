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

	// Check that IDE/Agent scenarios have affinity
	expectedAffinityScenarios := []typ.RuleScenario{
		typ.ScenarioClaudeCode,
		typ.ScenarioClaudeDesktop,
		typ.ScenarioVSCode,
		typ.ScenarioAgent,
		typ.ScenarioCodex,
		typ.ScenarioOpenCode,
	}

	for _, scenario := range expectedAffinityScenarios {
		t.Run(string(scenario), func(t *testing.T) {
			scenarioConfig := cfg.GetScenarioConfig(scenario)
			if scenarioConfig == nil {
				t.Errorf("Scenario config not found for %s", scenario)
				return
			}
			if scenarioConfig.Flags.SessionAffinity != 1800 {
				t.Errorf("Scenario %s: expected SessionAffinity = 1800, got %d",
					scenario, scenarioConfig.Flags.SessionAffinity)
			}
		})
	}

	// Check xcode scenario
	xcodeConfig := cfg.GetScenarioConfig(typ.ScenarioXcode)
	if xcodeConfig == nil {
		t.Error("Xcode scenario config not found")
		return
	}
	if !xcodeConfig.Flags.SkipUsage {
		t.Error("Xcode should have SkipUsage = true")
	}
	if xcodeConfig.Flags.SessionAffinity != 1800 {
		t.Errorf("Xcode: expected SessionAffinity = 1800, got %d",
			xcodeConfig.Flags.SessionAffinity)
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

	// Verify defaults were set
	claudeCodeConfig1 := cfg1.GetScenarioConfig(typ.ScenarioClaudeCode)
	if claudeCodeConfig1 == nil {
		t.Fatal("Claude Code scenario config should exist after migration")
	}
	if claudeCodeConfig1.Flags.SessionAffinity != 1800 {
		t.Errorf("Expected default SessionAffinity = 1800 after migration, got %d",
			claudeCodeConfig1.Flags.SessionAffinity)
	}

	// Now user explicitly changes the value
	for i := range cfg1.Scenarios {
		if cfg1.Scenarios[i].Scenario == typ.ScenarioClaudeCode {
			cfg1.Scenarios[i].Flags.SessionAffinity = 3600
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

	// Debug: print migrations completed
	t.Logf("After second NewConfig, MigrationsCompleted: %v", cfg2.MigrationsCompleted)

	// Verify migration is still marked as completed
	if !cfg2.hasMigrationCompleted("20260606") {
		t.Error("Migration should still be marked as completed")
	}

	claudeCodeConfig2 := cfg2.GetScenarioConfig(typ.ScenarioClaudeCode)
	if claudeCodeConfig2 == nil {
		t.Fatal("Claude Code scenario config should still exist")
	}

	// Should preserve user's explicit 3600 value
	if claudeCodeConfig2.Flags.SessionAffinity != 3600 {
		t.Errorf("Expected to preserve user's explicit SessionAffinity = 3600, got %d",
			claudeCodeConfig2.Flags.SessionAffinity)
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

	// Verify that SessionAffinity still got the default value
	if xcodeConfig.Flags.SessionAffinity != 1800 {
		t.Errorf("Expected SessionAffinity=1800 to be set even with customized SkipUsage, got %d",
			xcodeConfig.Flags.SessionAffinity)
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

func TestMigrate20260609_DefaultsCleanHeader(t *testing.T) {
	c := &Config{
		Rules: []typ.Rule{
			{UUID: "built-in-cc", Scenario: typ.ScenarioClaudeCode},
			{UUID: "cc-profile", Scenario: typ.RuleScenario("claude_code:p1")},
			{UUID: "already-on", Scenario: typ.ScenarioClaudeCode, Flags: typ.RuleFlags{CleanHeader: true}},
			{UUID: "desktop", Scenario: typ.ScenarioClaudeDesktop},
			{UUID: "openai", Scenario: typ.ScenarioOpenAI},
		},
	}

	migrate20260609(c)

	if !c.hasMigrationCompleted("20260609") {
		t.Fatal("migration should be marked completed")
	}
	want := map[string]bool{
		"built-in-cc": true,  // claude_code base → defaulted on
		"cc-profile":  true,  // claude_code:<profile> → defaulted on
		"already-on":  true,  // unchanged
		"desktop":     false, // claude_desktop — not in scope for this migration
		"openai":      false, // non-claude_code → untouched
	}
	for _, r := range c.Rules {
		if got := r.Flags.CleanHeader; got != want[r.UUID] {
			t.Errorf("rule %q CleanHeader = %v, want %v", r.UUID, got, want[r.UUID])
		}
	}
}

func TestMigrate20260609_OneTime_PreservesUserOff(t *testing.T) {
	c := &Config{
		Rules: []typ.Rule{{UUID: "built-in-cc", Scenario: typ.ScenarioClaudeCode}},
	}

	migrate20260609(c)
	if !c.Rules[0].Flags.CleanHeader {
		t.Fatal("first run should default the flag on")
	}

	// User turns it off; a later boot must not re-enable it.
	c.Rules[0].Flags.CleanHeader = false
	migrate20260609(c)
	if c.Rules[0].Flags.CleanHeader {
		t.Error("migration re-enabled a user-disabled flag; one-time gate is broken")
	}
}
