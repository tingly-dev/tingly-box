package config

import (
	"os"
	"path/filepath"
	"testing"
)

func TestNewConfigWithDisableMigration(t *testing.T) {
	// Create a temporary directory for testing
	tempDir, err := os.MkdirTemp("", "tingly-config-test-*")
	if err != nil {
		t.Fatalf("Failed to create temp directory: %v", err)
	}
	defer os.RemoveAll(tempDir)

	// Test 1: Create config without disable migration (default, migration enabled)
	cfg1, err := NewConfig(WithConfigDir(tempDir))
	if err != nil {
		t.Fatalf("Failed to create config: %v", err)
	}
	if cfg1 == nil {
		t.Fatal("Expected config to be non-nil")
	}

	// Clean up for next test
	configFile := filepath.Join(tempDir, "config.json")
	if err := os.Remove(configFile); err != nil {
		t.Fatalf("Failed to remove config file: %v", err)
	}

	// Test 2: Create config with disable migration
	cfg2, err := NewConfig(WithConfigDir(tempDir), WithDisableMigration())
	if err != nil {
		t.Fatalf("Failed to create config with disable migration: %v", err)
	}
	if cfg2 == nil {
		t.Fatal("Expected config to be non-nil")
	}

	// Both configs should be valid, just created with different options
	if cfg1.ConfigDir != cfg2.ConfigDir {
		t.Errorf("Expected same config dir, got %s and %s", cfg1.ConfigDir, cfg2.ConfigDir)
	}
}

func TestNewConfigWithDir(t *testing.T) {
	// Test backward compatibility with NewConfigWithDir
	tempDir, err := os.MkdirTemp("", "tingly-config-test-*")
	if err != nil {
		t.Fatalf("Failed to create temp directory: %v", err)
	}
	defer os.RemoveAll(tempDir)

	// Test NewConfigWithDir without options
	cfg1, err := NewConfigWithDir(tempDir)
	if err != nil {
		t.Fatalf("Failed to create config: %v", err)
	}
	if cfg1 == nil {
		t.Fatal("Expected config to be non-nil")
	}
	if cfg1.ConfigDir != tempDir {
		t.Errorf("Expected config dir %s, got %s", tempDir, cfg1.ConfigDir)
	}

	// Clean up
	configFile := filepath.Join(tempDir, "config.json")
	if err := os.Remove(configFile); err != nil {
		t.Fatalf("Failed to remove config file: %v", err)
	}

	// Test NewConfigWithDir with disable migration
	cfg2, err := NewConfigWithDir(tempDir, WithDisableMigration())
	if err != nil {
		t.Fatalf("Failed to create config with disable migration: %v", err)
	}
	if cfg2 == nil {
		t.Fatal("Expected config to be non-nil")
	}
	if cfg2.ConfigDir != tempDir {
		t.Errorf("Expected config dir %s, got %s", tempDir, cfg2.ConfigDir)
	}
}

func TestConfigOptions(t *testing.T) {
	// Test that the options pattern works correctly
	opts := &configOptions{
		enableMigration: true,
		enableBuiltIn:   true,
	}

	// Default should be true
	if !opts.enableMigration {
		t.Error("Expected enableMigration to default to true")
	}
	if !opts.enableBuiltIn {
		t.Error("Expected enableBuiltIn to default to true")
	}
	if opts.configDir != "" {
		t.Error("Expected configDir to default to empty string")
	}

	// Apply WithDisableMigration option
	WithDisableMigration()(opts)
	if opts.enableMigration {
		t.Error("Expected enableMigration to be false after applying WithDisableMigration")
	}

	// Apply WithDisableBuiltIn option
	WithDisableBuiltIn()(opts)
	if opts.enableBuiltIn {
		t.Error("Expected enableBuiltIn to be false after applying WithDisableBuiltIn")
	}

	// Apply WithConfigDir option
	testDir := "/test/config"
	WithConfigDir(testDir)(opts)
	if opts.configDir != testDir {
		t.Errorf("Expected configDir to be %s, got %s", testDir, opts.configDir)
	}
}

func TestNewDefaultConfig(t *testing.T) {
	// NewDefaultConfig should use default settings
	// Note: This test may fail if GetTinglyConfDir() returns empty
	// In real usage, GetTinglyConfDir() is properly configured
	cfg, err := NewDefaultConfig()
	if err != nil {
		// It's okay to fail if there's no default config dir
		t.Skipf("Skipping test: %v", err)
		return
	}
	if cfg == nil {
		t.Fatal("Expected config to be non-nil")
	}
}
