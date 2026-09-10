package config

import (
	"encoding/json"
	"os"
	"path/filepath"
	"testing"
)

func TestApplyClaudeSettings_DefaultMode(t *testing.T) {
	targetPath := filepath.Join(t.TempDir(), "settings.json")

	result, err := ApplyClaudeSettingsToPath(targetPath, map[string]string{
		"ANTHROPIC_MODEL": "test-model",
	}, WithDefaultMode("acceptEdits"), WithBackup(false))
	if err != nil {
		t.Fatalf("ApplyClaudeSettingsToPath failed: %v", err)
	}
	if !result.Success {
		t.Fatalf("expected success, got: %s", result.Message)
	}

	data, err := os.ReadFile(targetPath)
	if err != nil {
		t.Fatalf("read settings: %v", err)
	}
	var settings map[string]interface{}
	if err := json.Unmarshal(data, &settings); err != nil {
		t.Fatalf("unmarshal settings: %v", err)
	}
	if settings["defaultMode"] != "acceptEdits" {
		t.Fatalf("legacy top-level defaultMode = %v, want acceptEdits", settings["defaultMode"])
	}
	permissions, ok := settings["permissions"].(map[string]interface{})
	if !ok || permissions["defaultMode"] != "acceptEdits" {
		t.Fatalf("permissions.defaultMode = %v, want acceptEdits", settings["permissions"])
	}
}

func TestApplyClaudeSettings_ShowThinkingSummaries(t *testing.T) {
	targetPath := filepath.Join(t.TempDir(), "settings.json")

	result, err := ApplyClaudeSettingsToPath(targetPath, map[string]string{
		"ANTHROPIC_MODEL": "test-model",
	}, WithShowThinkingSummaries(false), WithBackup(false))
	if err != nil {
		t.Fatalf("ApplyClaudeSettingsToPath failed: %v", err)
	}
	if !result.Success {
		t.Fatalf("expected success, got: %s", result.Message)
	}

	data, err := os.ReadFile(targetPath)
	if err != nil {
		t.Fatalf("read settings: %v", err)
	}
	var settings map[string]interface{}
	if err := json.Unmarshal(data, &settings); err != nil {
		t.Fatalf("unmarshal settings: %v", err)
	}
	if settings["showThinkingSummaries"] != false {
		t.Fatalf("showThinkingSummaries = %v, want false", settings["showThinkingSummaries"])
	}
}

func TestApplyClaudeSettings_ShowThinkingSummariesOmittedWhenUnset(t *testing.T) {
	targetPath := filepath.Join(t.TempDir(), "settings.json")

	result, err := ApplyClaudeSettingsToPath(targetPath, map[string]string{
		"ANTHROPIC_MODEL": "test-model",
	}, WithBackup(false))
	if err != nil {
		t.Fatalf("ApplyClaudeSettingsToPath failed: %v", err)
	}
	if !result.Success {
		t.Fatalf("expected success, got: %s", result.Message)
	}

	data, err := os.ReadFile(targetPath)
	if err != nil {
		t.Fatalf("read settings: %v", err)
	}
	var settings map[string]interface{}
	if err := json.Unmarshal(data, &settings); err != nil {
		t.Fatalf("unmarshal settings: %v", err)
	}
	if _, present := settings["showThinkingSummaries"]; present {
		t.Fatalf("showThinkingSummaries should be omitted when WithShowThinkingSummaries is not used, got: %v", settings["showThinkingSummaries"])
	}
}

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
		"ANTHROPIC_MODEL":    "test-model",
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
		"ANTHROPIC_MODEL":    "test-model",
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
		"someKey":      "preserved",
		"otherSetting": 123,
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

func TestApplyClaudeSettingsToPath_WithBackupDisabled(t *testing.T) {
	tempDir, err := os.MkdirTemp("", "tingly-test")
	if err != nil {
		t.Fatalf("Failed to create temp dir: %v", err)
	}
	defer os.RemoveAll(tempDir)

	// Create existing file
	targetPath := filepath.Join(tempDir, "settings.json")
	existingConfig := map[string]interface{}{
		"someKey": "someValue",
	}
	existingData, _ := json.MarshalIndent(existingConfig, "", "  ")
	if err := os.WriteFile(targetPath, existingData, 0644); err != nil {
		t.Fatalf("Failed to create existing file: %v", err)
	}

	// Apply with backup disabled
	result, err := ApplyClaudeSettingsToPath(targetPath, map[string]string{
		"ANTHROPIC_MODEL": "test-model",
	}, WithBackup(false))
	if err != nil {
		t.Fatalf("ApplyClaudeSettingsToPath failed: %v", err)
	}

	if !result.Success {
		t.Errorf("Expected success, got failure: %s", result.Message)
	}

	// Verify no backup was created
	if result.BackupPath != "" {
		t.Errorf("Expected no backup when disabled, got: %s", result.BackupPath)
	}

	backupDir := filepath.Join(filepath.Dir(targetPath), "backup")
	entries, _ := os.ReadDir(backupDir)
	if len(entries) > 0 {
		t.Errorf("Expected backup directory to be empty, found %d entries", len(entries))
	}
}

func TestApplyClaudeSettingsToPath_WithBackupEnabled(t *testing.T) {
	tempDir, err := os.MkdirTemp("", "tingly-test")
	if err != nil {
		t.Fatalf("Failed to create temp dir: %v", err)
	}
	defer os.RemoveAll(tempDir)

	// Create existing file
	targetPath := filepath.Join(tempDir, "settings.json")
	existingConfig := map[string]interface{}{
		"someKey": "someValue",
	}
	existingData, _ := json.MarshalIndent(existingConfig, "", "  ")
	if err := os.WriteFile(targetPath, existingData, 0644); err != nil {
		t.Fatalf("Failed to create existing file: %v", err)
	}

	// Apply with backup enabled (default)
	result, err := ApplyClaudeSettingsToPath(targetPath, map[string]string{
		"ANTHROPIC_MODEL": "test-model",
	}, WithBackup(true))
	if err != nil {
		t.Fatalf("ApplyClaudeSettingsToPath failed: %v", err)
	}

	if !result.Success {
		t.Errorf("Expected success, got failure: %s", result.Message)
	}

	// Verify backup was created
	if result.BackupPath == "" {
		t.Errorf("Expected backup path when enabled")
	}

	if _, err := os.Stat(result.BackupPath); os.IsNotExist(err) {
		t.Errorf("Expected backup file to exist at %s", result.BackupPath)
	}
}

func TestApplyClaudeSettingsToPath_WithExtra(t *testing.T) {
	tempDir, err := os.MkdirTemp("", "tingly-test")
	if err != nil {
		t.Fatalf("Failed to create temp dir: %v", err)
	}
	defer os.RemoveAll(tempDir)

	targetPath := filepath.Join(tempDir, "settings.json")

	// Apply with extra statusLine config
	statusLine := map[string]any{
		"type":    "command",
		"command": "/path/to/script.sh",
	}
	result, err := ApplyClaudeSettingsToPath(targetPath, map[string]string{
		"ANTHROPIC_MODEL": "test-model",
	}, WithExtra("statusLine", statusLine), WithBackup(false))
	if err != nil {
		t.Fatalf("ApplyClaudeSettingsToPath failed: %v", err)
	}

	if !result.Success {
		t.Errorf("Expected success, got failure: %s", result.Message)
	}

	// Verify statusLine was added
	data, err := os.ReadFile(targetPath)
	if err != nil {
		t.Fatalf("Failed to read file: %v", err)
	}

	var config map[string]interface{}
	if err := json.Unmarshal(data, &config); err != nil {
		t.Fatalf("Failed to parse JSON: %v", err)
	}

	sl, ok := config["statusLine"].(map[string]interface{})
	if !ok {
		t.Fatal("Expected statusLine section in config")
	}

	if sl["type"] != "command" {
		t.Errorf("Expected statusLine type to be 'command', got: %v", sl["type"])
	}

	if sl["command"] != "/path/to/script.sh" {
		t.Errorf("Expected statusLine command to be '/path/to/script.sh', got: %v", sl["command"])
	}
}

func TestApplyClaudeSettingsToPath_MultipleWithExtra(t *testing.T) {
	tempDir, err := os.MkdirTemp("", "tingly-test")
	if err != nil {
		t.Fatalf("Failed to create temp dir: %v", err)
	}
	defer os.RemoveAll(tempDir)

	targetPath := filepath.Join(tempDir, "settings.json")

	// Apply with multiple extras using multiple WithExtra calls
	result, err := ApplyClaudeSettingsToPath(targetPath, map[string]string{
		"ANTHROPIC_MODEL": "test-model",
	},
		WithExtra("key1", "value1"),
		WithExtra("key2", "value2"),
		WithExtra("statusLine", map[string]any{
			"type":    "command",
			"command": "/path/to/script.sh",
		}),
		WithBackup(false),
	)
	if err != nil {
		t.Fatalf("ApplyClaudeSettingsToPath failed: %v", err)
	}

	if !result.Success {
		t.Errorf("Expected success, got failure: %s", result.Message)
	}

	// Verify all extras were added
	data, err := os.ReadFile(targetPath)
	if err != nil {
		t.Fatalf("Failed to read file: %v", err)
	}

	var config map[string]interface{}
	if err := json.Unmarshal(data, &config); err != nil {
		t.Fatalf("Failed to parse JSON: %v", err)
	}

	if config["key1"] != "value1" {
		t.Errorf("Expected key1 to be 'value1', got: %v", config["key1"])
	}
	if config["key2"] != "value2" {
		t.Errorf("Expected key2 to be 'value2', got: %v", config["key2"])
	}

	sl, ok := config["statusLine"].(map[string]interface{})
	if !ok {
		t.Fatal("Expected statusLine section in config")
	}

	if sl["command"] != "/path/to/script.sh" {
		t.Errorf("Expected statusLine command to be '/path/to/script.sh', got: %v", sl["command"])
	}
}

func TestApplyClaudeSettingsToPath_DefaultBackupBehavior(t *testing.T) {
	tempDir, err := os.MkdirTemp("", "tingly-test")
	if err != nil {
		t.Fatalf("Failed to create temp dir: %v", err)
	}
	defer os.RemoveAll(tempDir)

	// Create existing file
	targetPath := filepath.Join(tempDir, "settings.json")
	existingConfig := map[string]interface{}{
		"someKey": "someValue",
	}
	existingData, _ := json.MarshalIndent(existingConfig, "", "  ")
	if err := os.WriteFile(targetPath, existingData, 0644); err != nil {
		t.Fatalf("Failed to create existing file: %v", err)
	}

	// Apply without specifying backup option (should default to true)
	result, err := ApplyClaudeSettingsToPath(targetPath, map[string]string{
		"ANTHROPIC_MODEL": "test-model",
	})
	if err != nil {
		t.Fatalf("ApplyClaudeSettingsToPath failed: %v", err)
	}

	if !result.Success {
		t.Errorf("Expected success, got failure: %s", result.Message)
	}

	// Verify backup was created by default
	if result.BackupPath == "" {
		t.Errorf("Expected backup path by default")
	}

	if _, err := os.Stat(result.BackupPath); os.IsNotExist(err) {
		t.Errorf("Expected backup file to exist at %s by default", result.BackupPath)
	}
}
