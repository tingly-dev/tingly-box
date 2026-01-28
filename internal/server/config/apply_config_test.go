package config

import (
	"encoding/json"
	"os"
	"path/filepath"
	"testing"
	"time"
)

func TestApplyClaudeSettings_NewFile(t *testing.T) {
	// Create a temporary directory
	tempDir, err := os.MkdirTemp("", "tingly-test")
	if err != nil {
		t.Fatalf("Failed to create temp dir: %v", err)
	}
	defer os.RemoveAll(tempDir)

	// Override UserHomeDir for this test
	originalHome := os.Getenv("HOME")
	os.Setenv("HOME", tempDir)
	defer os.Setenv("HOME", originalHome)

	// Create .claude directory
	claudeDir := filepath.Join(tempDir, ".claude")
	if err := os.MkdirAll(claudeDir, 0755); err != nil {
		t.Fatalf("Failed to create .claude dir: %v", err)
	}

	result, err := ApplyClaudeSettingsFromEnv(map[string]string{
		"ANTHROPIC_MODEL": "test-model",
		"ANTHROPIC_BASE_URL": "http://localhost:12580",
	})
	if err != nil {
		t.Fatalf("ApplyClaudeSettings failed: %v", err)
	}

	if !result.Success {
		t.Errorf("Expected success, got failure: %s", result.Message)
	}

	if !result.Created {
		t.Errorf("Expected Created to be true for new file")
	}

	if result.BackupPath != "" {
		t.Errorf("Expected no backup path for new file, got: %s", result.BackupPath)
	}

	// Verify file was created
	targetPath := filepath.Join(claudeDir, "settings.json")
	if _, err := os.Stat(targetPath); os.IsNotExist(err) {
		t.Errorf("Expected file to be created at %s", targetPath)
	}

	// Verify content
	data, err := os.ReadFile(targetPath)
	if err != nil {
		t.Fatalf("Failed to read created file: %v", err)
	}

	var config map[string]interface{}
	if err := json.Unmarshal(data, &config); err != nil {
		t.Fatalf("Failed to parse JSON: %v", err)
	}

	env, ok := config["env"].(map[string]interface{})
	if !ok {
		t.Fatal("Expected env section in config")
	}

	if env["ANTHROPIC_MODEL"] != "test-model" {
		t.Errorf("Expected ANTHROPIC_MODEL to be 'test-model', got: %v", env["ANTHROPIC_MODEL"])
	}
}

func TestApplyClaudeSettings_ExistingFile(t *testing.T) {
	// Create a temporary directory
	tempDir, err := os.MkdirTemp("", "tingly-test")
	if err != nil {
		t.Fatalf("Failed to create temp dir: %v", err)
	}
	defer os.RemoveAll(tempDir)

	// Override UserHomeDir for this test
	originalHome := os.Getenv("HOME")
	os.Setenv("HOME", tempDir)
	defer os.Setenv("HOME", originalHome)

	// Create .claude directory and existing settings.json
	claudeDir := filepath.Join(tempDir, ".claude")
	if err := os.MkdirAll(claudeDir, 0755); err != nil {
		t.Fatalf("Failed to create .claude dir: %v", err)
	}

	existingConfig := map[string]interface{}{
		"someKey": "someValue",
		"env": map[string]string{
			"OLD_KEY": "old_value",
		},
	}
	existingData, _ := json.MarshalIndent(existingConfig, "", "  ")
	targetPath := filepath.Join(claudeDir, "settings.json")
	if err := os.WriteFile(targetPath, existingData, 0644); err != nil {
		t.Fatalf("Failed to create existing file: %v", err)
	}

	result, err := ApplyClaudeSettingsFromEnv(map[string]string{
		"ANTHROPIC_MODEL": "test-model",
		"ANTHROPIC_BASE_URL": "http://localhost:12580",
	})
	if err != nil {
		t.Fatalf("ApplyClaudeSettings failed: %v", err)
	}

	if !result.Success {
		t.Errorf("Expected success, got failure: %s", result.Message)
	}

	if !result.Updated {
		t.Errorf("Expected Updated to be true for existing file")
	}

	if result.BackupPath == "" {
		t.Errorf("Expected backup path for existing file")
	}

	// Verify backup was created
	if _, err := os.Stat(result.BackupPath); os.IsNotExist(err) {
		t.Errorf("Expected backup file to be created at %s", result.BackupPath)
	}

	// Verify content - env should be replaced, other keys preserved
	data, err := os.ReadFile(targetPath)
	if err != nil {
		t.Fatalf("Failed to read updated file: %v", err)
	}

	var config map[string]interface{}
	if err := json.Unmarshal(data, &config); err != nil {
		t.Fatalf("Failed to parse JSON: %v", err)
	}

	// Check that someKey is preserved
	if config["someKey"] != "someValue" {
		t.Errorf("Expected someKey to be preserved")
	}

	// Check that env was replaced with the test values
	env, ok := config["env"].(map[string]interface{})
	if !ok {
		t.Fatal("Expected env section in config")
	}

	if env["ANTHROPIC_MODEL"] != "test-model" {
		t.Errorf("Expected ANTHROPIC_MODEL to be 'test-model', got: %v", env["ANTHROPIC_MODEL"])
	}
}

func TestApplyClaudeOnboarding_NewFile(t *testing.T) {
	tempDir, err := os.MkdirTemp("", "tingly-test")
	if err != nil {
		t.Fatalf("Failed to create temp dir: %v", err)
	}
	defer os.RemoveAll(tempDir)

	originalHome := os.Getenv("HOME")
	os.Setenv("HOME", tempDir)
	defer os.Setenv("HOME", originalHome)

	payload := map[string]interface{}{
		"hasCompletedOnboarding": true,
	}

	result, err := ApplyClaudeOnboarding(payload)
	if err != nil {
		t.Fatalf("ApplyClaudeOnboarding failed: %v", err)
	}

	if !result.Success {
		t.Errorf("Expected success, got failure: %s", result.Message)
	}

	if !result.Created {
		t.Errorf("Expected Created to be true")
	}
}

func TestApplyClaudeOnboarding_ExistingFile(t *testing.T) {
	tempDir, err := os.MkdirTemp("", "tingly-test")
	if err != nil {
		t.Fatalf("Failed to create temp dir: %v", err)
	}
	defer os.RemoveAll(tempDir)

	originalHome := os.Getenv("HOME")
	os.Setenv("HOME", tempDir)
	defer os.Setenv("HOME", originalHome)

	// Create existing .claude.json
	existingConfig := map[string]interface{}{
		"someKey":        "preserved",
		"otherSetting":   123,
	}
	existingData, _ := json.MarshalIndent(existingConfig, "", "  ")
	targetPath := filepath.Join(tempDir, ".claude.json")
	if err := os.WriteFile(targetPath, existingData, 0644); err != nil {
		t.Fatalf("Failed to create existing file: %v", err)
	}

	payload := map[string]interface{}{
		"hasCompletedOnboarding": true,
	}

	result, err := ApplyClaudeOnboarding(payload)
	if err != nil {
		t.Fatalf("ApplyClaudeOnboarding failed: %v", err)
	}

	if !result.Success {
		t.Errorf("Expected success, got failure: %s", result.Message)
	}

	if !result.Updated {
		t.Errorf("Expected Updated to be true")
	}

	if result.BackupPath == "" {
		t.Errorf("Expected backup path")
	}

	// Verify existing keys are preserved
	data, _ := os.ReadFile(targetPath)
	var config map[string]interface{}
	json.Unmarshal(data, &config)

	if config["someKey"] != "preserved" {
		t.Errorf("Expected someKey to be preserved")
	}

	if config["hasCompletedOnboarding"] != true {
		t.Errorf("Expected hasCompletedOnboarding to be true")
	}
}

func TestApplyOpenCodeConfig_NewFile(t *testing.T) {
	tempDir, err := os.MkdirTemp("", "tingly-test")
	if err != nil {
		t.Fatalf("Failed to create temp dir: %v", err)
	}
	defer os.RemoveAll(tempDir)

	originalHome := os.Getenv("HOME")
	os.Setenv("HOME", tempDir)
	defer os.Setenv("HOME", originalHome)

	payload := map[string]interface{}{
		"provider": map[string]interface{}{
			"tingly-box": map[string]interface{}{
				"name": "tingly-box",
				"npm":  "@ai-sdk/anthropic",
				"options": map[string]interface{}{
					"baseURL": "http://localhost:12580/tingly/opencode",
				},
				"models": map[string]interface{}{
					"test-model": map[string]interface{}{
						"name": "test-model",
					},
				},
			},
		},
	}

	result, err := ApplyOpenCodeConfig(payload)
	if err != nil {
		t.Fatalf("ApplyOpenCodeConfig failed: %v", err)
	}

	if !result.Success {
		t.Errorf("Expected success, got failure: %s", result.Message)
	}

	if !result.Created {
		t.Errorf("Expected Created to be true")
	}
}

func TestApplyOpenCodeConfig_ExistingFile(t *testing.T) {
	tempDir, err := os.MkdirTemp("", "tingly-test")
	if err != nil {
		t.Fatalf("Failed to create temp dir: %v", err)
	}
	defer os.RemoveAll(tempDir)

	originalHome := os.Getenv("HOME")
	os.Setenv("HOME", tempDir)
	defer os.Setenv("HOME", originalHome)

	// Create existing config directory and file
	configDir := filepath.Join(tempDir, ".config", "opencode")
	if err := os.MkdirAll(configDir, 0755); err != nil {
		t.Fatalf("Failed to create config dir: %v", err)
	}

	existingConfig := map[string]interface{}{
		"$schema": "https://opencode.ai/config.json",
		"provider": map[string]interface{}{
			"other-provider": map[string]interface{}{
				"name": "other-provider",
			},
		},
	}
	existingData, _ := json.MarshalIndent(existingConfig, "", "  ")
	targetPath := filepath.Join(configDir, "opencode.json")
	if err := os.WriteFile(targetPath, existingData, 0644); err != nil {
		t.Fatalf("Failed to create existing file: %v", err)
	}

	payload := map[string]interface{}{
		"provider": map[string]interface{}{
			"tingly-box": map[string]interface{}{
				"name": "tingly-box",
				"npm":  "@ai-sdk/anthropic",
			},
		},
	}

	result, err := ApplyOpenCodeConfig(payload)
	if err != nil {
		t.Fatalf("ApplyOpenCodeConfig failed: %v", err)
	}

	if !result.Success {
		t.Errorf("Expected success, got failure: %s", result.Message)
	}

	if !result.Updated {
		t.Errorf("Expected Updated to be true")
	}

	// Verify other provider is preserved and tingly-box is added
	data, _ := os.ReadFile(targetPath)
	var config map[string]interface{}
	json.Unmarshal(data, &config)

	providers, ok := config["provider"].(map[string]interface{})
	if !ok {
		t.Fatal("Expected provider section")
	}

	if providers["other-provider"] == nil {
		t.Errorf("Expected other-provider to be preserved")
	}

	if providers["tingly-box"] == nil {
		t.Errorf("Expected tingly-box to be added")
	}
}

func TestBackupFileNaming(t *testing.T) {
	tempDir, err := os.MkdirTemp("", "tingly-test")
	if err != nil {
		t.Fatalf("Failed to create temp dir: %v", err)
	}
	defer os.RemoveAll(tempDir)

	// Create a test file
	testFile := filepath.Join(tempDir, "test.json")
	if err := os.WriteFile(testFile, []byte("{}"), 0644); err != nil {
		t.Fatalf("Failed to create test file: %v", err)
	}

	// Generate backup path
	backupPath := generateBackupPath(testFile)

	// Verify it contains timestamp
	expectedSuffix := ".json.bak-" + time.Now().Format("20060102-")
	if len(backupPath) < len(expectedSuffix) {
		t.Errorf("Backup path too short: %s", backupPath)
	}

	// Verify backup doesn't exist yet
	if _, err := os.Stat(backupPath); !os.IsNotExist(err) {
		t.Errorf("Backup should not exist yet: %s", backupPath)
	}
}

func TestEnsureDir(t *testing.T) {
	tempDir, err := os.MkdirTemp("", "tingly-test")
	if err != nil {
		t.Fatalf("Failed to create temp dir: %v", err)
	}
	defer os.RemoveAll(tempDir)

	nestedPath := filepath.Join(tempDir, "a", "b", "c", "file.json")

	// Ensure directory exists
	if err := ensureDir(nestedPath); err != nil {
		t.Fatalf("ensureDir failed: %v", err)
	}

	// Verify directory was created
	if _, err := os.Stat(filepath.Dir(nestedPath)); os.IsNotExist(err) {
		t.Errorf("Expected directory to be created")
	}
}
