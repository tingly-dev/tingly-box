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
	if !xcodeConfig.Flags.DisableStreamUsage {
		t.Error("Xcode should have DisableStreamUsage = true")
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

	// Create config with xcode scenario that has DisableStreamUsage=false (user override)
	cfg, err := NewConfig(WithConfigDir(tmpDir))
	if err != nil {
		t.Fatalf("NewConfig error: %v", err)
	}

	// Manually set xcode DisableStreamUsage to false (user explicitly disabled it)
	for i := range cfg.Scenarios {
		if cfg.Scenarios[i].Scenario == typ.ScenarioXcode {
			cfg.Scenarios[i].Flags.DisableStreamUsage = false
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

	// Verify that user's DisableStreamUsage=false is preserved
	xcodeConfig := cfg2.GetScenarioConfig(typ.ScenarioXcode)
	if xcodeConfig == nil {
		t.Fatal("Xcode scenario config should exist")
	}

	if xcodeConfig.Flags.DisableStreamUsage != false {
		t.Errorf("Expected user's DisableStreamUsage=false to be preserved, got %v",
			xcodeConfig.Flags.DisableStreamUsage)
	}

	// Verify that SessionAffinity still got the default value
	if xcodeConfig.Flags.SessionAffinity != 1800 {
		t.Errorf("Expected SessionAffinity=1800 to be set even with customized DisableStreamUsage, got %d",
			xcodeConfig.Flags.SessionAffinity)
	}
}
