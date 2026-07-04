package config

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"testing"
	"time"

	"github.com/pelletier/go-toml/v2"
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
		t.Fatalf("defaultMode = %v, want acceptEdits", settings["defaultMode"])
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

// writeFakeBackup synthesizes a backup file at <dir>/backup/ with a controlled
// timestamp embedded in its filename. Used by rotation tests to avoid the
// 1-second granularity of the real timestamping path.
func writeFakeBackup(t *testing.T, originalPath string, ts time.Time, content string) string {
	t.Helper()
	dir := filepath.Dir(originalPath)
	base := filepath.Base(originalPath)
	ext := filepath.Ext(originalPath)
	backupDir := filepath.Join(dir, "backup")
	if err := os.MkdirAll(backupDir, 0755); err != nil {
		t.Fatalf("mkdir backup: %v", err)
	}
	name := fmt.Sprintf("%s.bak-%s%s", base, ts.Format(backupTimestampLayout), ext)
	path := filepath.Join(backupDir, name)
	if err := os.WriteFile(path, []byte(content), 0644); err != nil {
		t.Fatalf("write fake backup: %v", err)
	}
	return path
}

func TestBackupRotation_KeepsLatestN(t *testing.T) {
	tempDir, err := os.MkdirTemp("", "tingly-test")
	if err != nil {
		t.Fatalf("Failed to create temp dir: %v", err)
	}
	defer os.RemoveAll(tempDir)

	targetPath := filepath.Join(tempDir, "settings.json")
	if err := os.WriteFile(targetPath, []byte(`{}`), 0644); err != nil {
		t.Fatalf("seed: %v", err)
	}

	// Synthesize 6 backups spaced 10 seconds apart.
	base := time.Date(2026, 1, 1, 12, 0, 0, 0, time.Local)
	var paths []string
	for i := 0; i < 6; i++ {
		paths = append(paths, writeFakeBackup(t, targetPath, base.Add(time.Duration(i)*10*time.Second), fmt.Sprintf("v%d", i)))
	}

	if err := rotateBackups(targetPath, 3); err != nil {
		t.Fatalf("rotateBackups: %v", err)
	}

	backups, err := ListBackups(targetPath)
	if err != nil {
		t.Fatalf("ListBackups: %v", err)
	}
	if len(backups) != 3 {
		t.Fatalf("Expected 3 backups after rotation, got %d", len(backups))
	}
	// The 3 newest must be the last 3 we created.
	expected := map[string]bool{paths[3]: true, paths[4]: true, paths[5]: true}
	for _, b := range backups {
		if !expected[b.Path] {
			t.Errorf("Unexpected backup retained: %s", b.Path)
		}
	}
	// Order is newest-first.
	for i := 1; i < len(backups); i++ {
		if !backups[i-1].Timestamp.After(backups[i].Timestamp) {
			t.Errorf("Backups not in descending order")
		}
	}
}

func TestBackupRotation_DoesNotTouchOtherBaseFiles(t *testing.T) {
	tempDir, err := os.MkdirTemp("", "tingly-test")
	if err != nil {
		t.Fatalf("Failed to create temp dir: %v", err)
	}
	defer os.RemoveAll(tempDir)

	settings := filepath.Join(tempDir, "settings.json")
	other := filepath.Join(tempDir, "other.json")
	if err := os.WriteFile(settings, []byte(`{}`), 0644); err != nil {
		t.Fatalf("seed settings: %v", err)
	}
	if err := os.WriteFile(other, []byte(`{}`), 0644); err != nil {
		t.Fatalf("seed other: %v", err)
	}

	base := time.Date(2026, 1, 1, 12, 0, 0, 0, time.Local)
	otherBackup := writeFakeBackup(t, other, base, "x")
	for i := 0; i < 5; i++ {
		writeFakeBackup(t, settings, base.Add(time.Duration(i)*10*time.Second), fmt.Sprintf("s%d", i))
	}

	if err := rotateBackups(settings, defaultBackupRetention); err != nil {
		t.Fatalf("rotateBackups: %v", err)
	}

	if _, err := os.Stat(otherBackup); err != nil {
		t.Errorf("Rotation of settings.json removed unrelated backup %s: %v", otherBackup, err)
	}

	settingsBackups, err := ListBackups(settings)
	if err != nil {
		t.Fatalf("ListBackups(settings): %v", err)
	}
	if len(settingsBackups) != defaultBackupRetention {
		t.Errorf("Expected %d settings backups, got %d", defaultBackupRetention, len(settingsBackups))
	}
}

func TestRestoreLatestBackup_RoundTrip(t *testing.T) {
	tempDir, err := os.MkdirTemp("", "tingly-test")
	if err != nil {
		t.Fatalf("Failed to create temp dir: %v", err)
	}
	defer os.RemoveAll(tempDir)

	targetPath := filepath.Join(tempDir, "settings.json")
	original := []byte(`{"version":"original"}`)
	mutated := []byte(`{"version":"mutated"}`)

	// Seed a synthetic backup that holds the "original" content.
	base := time.Date(2026, 1, 1, 12, 0, 0, 0, time.Local)
	backupPath := writeFakeBackup(t, targetPath, base, string(original))

	// Live file holds the mutated state we want to undo.
	if err := os.WriteFile(targetPath, mutated, 0644); err != nil {
		t.Fatalf("seed live: %v", err)
	}

	result, err := RestoreLatestBackup(targetPath)
	if err != nil {
		t.Fatalf("RestoreLatestBackup: %v", err)
	}
	if !result.Success {
		t.Fatalf("Restore not successful: %s", result.Message)
	}
	if result.RestoredFrom != backupPath {
		t.Errorf("Restored from %q, want %q", result.RestoredFrom, backupPath)
	}
	if result.PreRestoreBackup == "" {
		t.Errorf("Expected a pre-restore backup to be created")
	}

	got, err := os.ReadFile(targetPath)
	if err != nil {
		t.Fatalf("read restored: %v", err)
	}
	if string(got) != string(original) {
		t.Errorf("Restored content mismatch: got %s, want %s", got, original)
	}

	preData, err := os.ReadFile(result.PreRestoreBackup)
	if err != nil {
		t.Fatalf("read pre-restore backup: %v", err)
	}
	if string(preData) != string(mutated) {
		t.Errorf("Pre-restore backup mismatch: got %s, want %s", preData, mutated)
	}
}

func TestRestoreLatestBackup_NoBackup(t *testing.T) {
	tempDir, err := os.MkdirTemp("", "tingly-test")
	if err != nil {
		t.Fatalf("Failed to create temp dir: %v", err)
	}
	defer os.RemoveAll(tempDir)

	targetPath := filepath.Join(tempDir, "settings.json")
	if err := os.WriteFile(targetPath, []byte(`{}`), 0644); err != nil {
		t.Fatalf("seed: %v", err)
	}

	result, err := RestoreLatestBackup(targetPath)
	if err == nil {
		t.Fatalf("Expected error when no backup exists, got nil")
	}
	if result == nil || result.Success {
		t.Errorf("Expected unsuccessful result, got %+v", result)
	}
}

func TestListBackups_MissingDir(t *testing.T) {
	tempDir, err := os.MkdirTemp("", "tingly-test")
	if err != nil {
		t.Fatalf("Failed to create temp dir: %v", err)
	}
	defer os.RemoveAll(tempDir)

	backups, err := ListBackups(filepath.Join(tempDir, "settings.json"))
	if err != nil {
		t.Fatalf("ListBackups should not error on missing backup dir: %v", err)
	}
	if len(backups) != 0 {
		t.Errorf("Expected no backups, got %d", len(backups))
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

// ============================================================================
// ApplyCodexConfig tests
//
// Contract: writing ~/.codex/config.toml must MERGE — only fields we manage
// are overwritten. Unrelated top-level keys, user-defined providers, and
// user-defined profiles must survive.
// ============================================================================

func loadCodexConfigForTest(t *testing.T, path string) map[string]interface{} {
	t.Helper()
	data, err := os.ReadFile(path)
	if err != nil {
		t.Fatalf("read %s: %v", path, err)
	}
	out := map[string]interface{}{}
	if err := toml.Unmarshal(data, &out); err != nil {
		t.Fatalf("unmarshal toml: %v\n--- content ---\n%s", err, data)
	}
	return out
}

func TestApplyCodexConfig_NewFile_WritesManagedFields(t *testing.T) {
	tempDir := t.TempDir()
	t.Setenv("HOME", tempDir)

	result, err := ApplyCodexConfig("http://localhost:12580/tingly/codex", []string{"tingly-codex", "tingly-gpt5"}, DefaultCodexPrefs(), true)
	if err != nil {
		t.Fatalf("ApplyCodexConfig: %v", err)
	}
	if !result.Success || !result.Created {
		t.Fatalf("expected success+created, got %+v", result)
	}

	cfg := loadCodexConfigForTest(t, filepath.Join(tempDir, ".codex", "config.toml"))
	if cfg["model"] != "tingly-codex" {
		t.Errorf("model = %v, want tingly-codex", cfg["model"])
	}
	if cfg["model_provider"] != "tingly-box" {
		t.Errorf("model_provider = %v, want tingly-box", cfg["model_provider"])
	}
	providers, _ := cfg["model_providers"].(map[string]interface{})
	tb, _ := providers["tingly-box"].(map[string]interface{})
	if tb["base_url"] != "http://localhost:12580/tingly/codex" {
		t.Errorf("base_url = %v", tb["base_url"])
	}
	if tb["wire_api"] != "responses" {
		t.Errorf("wire_api = %v, want responses", tb["wire_api"])
	}
	profiles, _ := cfg["profiles"].(map[string]interface{})
	if len(profiles) != 2 {
		t.Fatalf("profiles = %d, want 2: %#v", len(profiles), profiles)
	}
	for _, model := range []string{"tingly-codex", "tingly-gpt5"} {
		p, ok := profiles[model].(map[string]interface{})
		if !ok {
			t.Fatalf("missing profile %q in %#v", model, profiles)
		}
		if p["model"] != model {
			t.Errorf("profile %s.model = %v", model, p["model"])
		}
		if p["model_provider"] != "tingly-box" {
			t.Errorf("profile %s.model_provider = %v", model, p["model_provider"])
		}
	}
}

func TestApplyCodexConfig_PreservesUserTopLevelFields(t *testing.T) {
	tempDir := t.TempDir()
	t.Setenv("HOME", tempDir)

	codexDir := filepath.Join(tempDir, ".codex")
	if err := os.MkdirAll(codexDir, 0o755); err != nil {
		t.Fatal(err)
	}
	existing := `approval_policy = "untrusted"
disable_response_storage = true
model = "user-custom-model"
hide_agent_reasoning = false

[shell_environment_policy]
inherit = "all"
`
	if err := os.WriteFile(filepath.Join(codexDir, "config.toml"), []byte(existing), 0o644); err != nil {
		t.Fatal(err)
	}

	if _, err := ApplyCodexConfig("http://example/tingly/codex", []string{"my-rule"}, DefaultCodexPrefs(), true); err != nil {
		t.Fatalf("ApplyCodexConfig: %v", err)
	}

	cfg := loadCodexConfigForTest(t, filepath.Join(codexDir, "config.toml"))
	if cfg["approval_policy"] != "untrusted" {
		t.Errorf("approval_policy lost: %v", cfg["approval_policy"])
	}
	if cfg["disable_response_storage"] != true {
		t.Errorf("disable_response_storage lost: %v", cfg["disable_response_storage"])
	}
	if cfg["hide_agent_reasoning"] != false {
		t.Errorf("hide_agent_reasoning lost: %v", cfg["hide_agent_reasoning"])
	}
	shell, _ := cfg["shell_environment_policy"].(map[string]interface{})
	if shell["inherit"] != "all" {
		t.Errorf("shell_environment_policy.inherit lost: %v", shell)
	}
	// Managed field overwritten with our default (first model)
	if cfg["model"] != "my-rule" {
		t.Errorf("model = %v, want my-rule (tingly should overwrite the default)", cfg["model"])
	}
}

func TestApplyCodexConfig_PreservesOtherProvidersAndProfiles(t *testing.T) {
	tempDir := t.TempDir()
	t.Setenv("HOME", tempDir)

	codexDir := filepath.Join(tempDir, ".codex")
	if err := os.MkdirAll(codexDir, 0o755); err != nil {
		t.Fatal(err)
	}
	existing := `[model_providers.openai]
name = "OpenAI"
base_url = "https://api.openai.com/v1"
wire_api = "chat"

[model_providers.tingly-box]
name = "Old Tingly"
base_url = "http://old-host/tingly/codex"
wire_api = "responses"

[profiles.work]
model = "gpt-5"
model_provider = "openai"

[profiles.legacy-tingly]
model = "tingly-legacy"
model_provider = "tingly-box"
`
	if err := os.WriteFile(filepath.Join(codexDir, "config.toml"), []byte(existing), 0o644); err != nil {
		t.Fatal(err)
	}

	if _, err := ApplyCodexConfig("http://new-host/tingly/codex", []string{"tingly-codex"}, DefaultCodexPrefs(), true); err != nil {
		t.Fatalf("ApplyCodexConfig: %v", err)
	}

	cfg := loadCodexConfigForTest(t, filepath.Join(codexDir, "config.toml"))
	providers, _ := cfg["model_providers"].(map[string]interface{})

	// User's openai provider preserved
	openai, _ := providers["openai"].(map[string]interface{})
	if openai["base_url"] != "https://api.openai.com/v1" {
		t.Errorf("openai provider not preserved: %#v", openai)
	}

	// Our tingly-box provider overwritten with new base_url
	tb, _ := providers["tingly-box"].(map[string]interface{})
	if tb["base_url"] != "http://new-host/tingly/codex" {
		t.Errorf("tingly-box.base_url = %v, want http://new-host/tingly/codex", tb["base_url"])
	}

	// User's [profiles.work] preserved
	profiles, _ := cfg["profiles"].(map[string]interface{})
	work, _ := profiles["work"].(map[string]interface{})
	if work["model"] != "gpt-5" || work["model_provider"] != "openai" {
		t.Errorf("profiles.work not preserved: %#v", work)
	}

	// User's [profiles.legacy-tingly] preserved (we don't garbage-collect
	// orphaned tingly profiles from previous applies — the user may want
	// to keep them).
	if _, ok := profiles["legacy-tingly"]; !ok {
		t.Errorf("profiles.legacy-tingly removed; expected preservation")
	}

	// Our new profile present
	tingly, _ := profiles["tingly-codex"].(map[string]interface{})
	if tingly["model"] != "tingly-codex" {
		t.Errorf("profiles.tingly-codex not written: %#v", profiles)
	}
}

func TestApplyCodexConfig_Idempotent(t *testing.T) {
	tempDir := t.TempDir()
	t.Setenv("HOME", tempDir)

	if _, err := ApplyCodexConfig("http://h/tingly/codex", []string{"a", "b"}, DefaultCodexPrefs(), true); err != nil {
		t.Fatal(err)
	}
	first, err := os.ReadFile(filepath.Join(tempDir, ".codex", "config.toml"))
	if err != nil {
		t.Fatal(err)
	}
	if _, err := ApplyCodexConfig("http://h/tingly/codex", []string{"a", "b"}, DefaultCodexPrefs(), true); err != nil {
		t.Fatal(err)
	}
	cfg := loadCodexConfigForTest(t, filepath.Join(tempDir, ".codex", "config.toml"))
	profiles, _ := cfg["profiles"].(map[string]interface{})
	if len(profiles) != 2 {
		t.Errorf("idempotent apply added profiles: got %d, want 2 (%#v)", len(profiles), profiles)
	}
	// Sanity: at least the second run produced an updated file (backup exists)
	// but profile set shouldn't grow.
	_ = first
}

// When the sanitized profile key already exists, we overwrite. Backups
// (restored via `agent restore codex`) are the safety net — extra collision
// logic isn't worth the complexity for users who have explicitly opted in to
// tingly-box managing their codex config.
func TestApplyCodexConfig_OverwritesCollidingProfile(t *testing.T) {
	tempDir := t.TempDir()
	t.Setenv("HOME", tempDir)

	codexDir := filepath.Join(tempDir, ".codex")
	if err := os.MkdirAll(codexDir, 0o755); err != nil {
		t.Fatal(err)
	}
	existing := `[profiles.tingly-codex]
model = "stale-value"
model_provider = "openai"
`
	if err := os.WriteFile(filepath.Join(codexDir, "config.toml"), []byte(existing), 0o644); err != nil {
		t.Fatal(err)
	}

	if _, err := ApplyCodexConfig("http://h/tingly/codex", []string{"tingly-codex"}, DefaultCodexPrefs(), true); err != nil {
		t.Fatal(err)
	}

	cfg := loadCodexConfigForTest(t, filepath.Join(codexDir, "config.toml"))
	profiles, _ := cfg["profiles"].(map[string]interface{})
	ours, _ := profiles["tingly-codex"].(map[string]interface{})
	if ours["model"] != "tingly-codex" || ours["model_provider"] != "tingly-box" {
		t.Errorf("expected colliding profile overwritten with tingly values, got %#v", ours)
	}
	if _, suffixed := profiles["tingly-codex-1"]; suffixed {
		t.Errorf("did not expect suffixed key; profiles=%#v", profiles)
	}
}

func TestApplyCodexConfig_WritesCatalogAndPointsConfigAtIt(t *testing.T) {
	tempDir := t.TempDir()
	t.Setenv("HOME", tempDir)

	if _, err := ApplyCodexConfig("http://h/tingly/codex", []string{"tingly-codex", "tingly-gpt5"}, DefaultCodexPrefs(), true); err != nil {
		t.Fatalf("ApplyCodexConfig: %v", err)
	}

	codexDir := filepath.Join(tempDir, ".codex")
	cfg := loadCodexConfigForTest(t, filepath.Join(codexDir, "config.toml"))

	wantCatalog := filepath.Join(codexDir, "tingly-model-catalog.json")
	if cfg["model_catalog_json"] != wantCatalog {
		t.Errorf("model_catalog_json = %v, want %v", cfg["model_catalog_json"], wantCatalog)
	}

	data, err := os.ReadFile(wantCatalog)
	if err != nil {
		t.Fatalf("read catalog: %v", err)
	}
	var catalog struct {
		Schema string                   `json:"$schema"`
		Models []map[string]interface{} `json:"models"`
	}
	if err := json.Unmarshal(data, &catalog); err != nil {
		t.Fatalf("unmarshal catalog: %v\n%s", err, data)
	}
	if catalog.Schema != codexModelCatalogSchema {
		t.Errorf("$schema = %q, want %q", catalog.Schema, codexModelCatalogSchema)
	}
	if len(catalog.Models) != 2 {
		t.Fatalf("len(models) = %d, want 2", len(catalog.Models))
	}
	gotSlugs := map[string]bool{}
	for _, m := range catalog.Models {
		gotSlugs[m["slug"].(string)] = true
		// Spot-check a few required ModelInfo fields so a future refactor
		// can't silently drop them and start tripping Codex's deserializer.
		for _, key := range []string{"display_name", "supported_reasoning_levels", "shell_type", "visibility", "truncation_policy", "input_modalities", "context_window", "max_context_window", "auto_compact_token_limit", "effective_context_window_percent"} {
			if _, ok := m[key]; !ok {
				t.Errorf("catalog entry %v missing required key %q", m["slug"], key)
			}
		}
	}
	if !gotSlugs["tingly-codex"] || !gotSlugs["tingly-gpt5"] {
		t.Errorf("catalog slugs = %v, want tingly-codex+tingly-gpt5", gotSlugs)
	}
}

func TestApplyCodexConfig_CatalogContextMetadataIsExplicit(t *testing.T) {
	tempDir := t.TempDir()
	t.Setenv("HOME", tempDir)

	if _, err := ApplyCodexConfig("http://h/tingly/codex", []string{"tingly-codex", "tingly-gpt5"}, DefaultCodexPrefs(), true); err != nil {
		t.Fatalf("ApplyCodexConfig: %v", err)
	}

	data, err := os.ReadFile(filepath.Join(tempDir, ".codex", "tingly-model-catalog.json"))
	if err != nil {
		t.Fatalf("read catalog: %v", err)
	}
	var catalog struct {
		Models []struct {
			Slug                          string `json:"slug"`
			ContextWindow                 int    `json:"context_window"`
			MaxContextWindow              int    `json:"max_context_window"`
			AutoCompactTokenLimit         int    `json:"auto_compact_token_limit"`
			EffectiveContextWindowPercent int    `json:"effective_context_window_percent"`
		} `json:"models"`
	}
	if err := json.Unmarshal(data, &catalog); err != nil {
		t.Fatalf("unmarshal: %v\n%s", err, data)
	}
	if len(catalog.Models) != 2 {
		t.Fatalf("models len = %d, want 2", len(catalog.Models))
	}

	bySlug := map[string]struct {
		Slug                          string `json:"slug"`
		ContextWindow                 int    `json:"context_window"`
		MaxContextWindow              int    `json:"max_context_window"`
		AutoCompactTokenLimit         int    `json:"auto_compact_token_limit"`
		EffectiveContextWindowPercent int    `json:"effective_context_window_percent"`
	}{}
	for _, model := range catalog.Models {
		bySlug[model.Slug] = model
		if model.ContextWindow != codexDefaultContextWindow {
			t.Errorf("%s context_window = %d, want %d", model.Slug, model.ContextWindow, codexDefaultContextWindow)
		}
		if model.AutoCompactTokenLimit != codexAutoCompactTokenLimit(model.ContextWindow) {
			t.Errorf("%s auto_compact_token_limit = %d, want configured percentage of context_window", model.Slug, model.AutoCompactTokenLimit)
		}
		if model.EffectiveContextWindowPercent != codexEffectiveContextWindowPercent {
			t.Errorf("%s effective_context_window_percent = %d, want %d", model.Slug, model.EffectiveContextWindowPercent, codexEffectiveContextWindowPercent)
		}
	}
	if bySlug["tingly-codex"].MaxContextWindow != codexDefaultMaxContextWindow {
		t.Errorf("tingly-codex max_context_window = %d, want %d", bySlug["tingly-codex"].MaxContextWindow, codexDefaultMaxContextWindow)
	}
	if bySlug["tingly-gpt5"].MaxContextWindow != codexDefaultMaxContextWindow {
		t.Errorf("tingly-gpt5 max_context_window = %d, want %d", bySlug["tingly-gpt5"].MaxContextWindow, codexDefaultMaxContextWindow)
	}
}

// supported_reasoning_levels deserializes into Vec<ReasoningEffortPreset>
// upstream — a list of {effort, description} objects. Regression test for a
// bug where we emitted bare strings and Codex rejected the catalog at startup
// with "invalid type: string ..., expected struct ReasoningEffortPreset".
func TestApplyCodexConfig_CatalogReasoningPresetsAreObjects(t *testing.T) {
	tempDir := t.TempDir()
	t.Setenv("HOME", tempDir)

	if _, err := ApplyCodexConfig("http://h/tingly/codex", []string{"tingly-codex"}, DefaultCodexPrefs(), true); err != nil {
		t.Fatalf("ApplyCodexConfig: %v", err)
	}

	data, err := os.ReadFile(filepath.Join(tempDir, ".codex", "tingly-model-catalog.json"))
	if err != nil {
		t.Fatalf("read catalog: %v", err)
	}
	var catalog struct {
		Models []struct {
			SupportedReasoningLevels []struct {
				Effort      string `json:"effort"`
				Description string `json:"description"`
			} `json:"supported_reasoning_levels"`
		} `json:"models"`
	}
	if err := json.Unmarshal(data, &catalog); err != nil {
		t.Fatalf("unmarshal: %v\n%s", err, data)
	}
	if len(catalog.Models) != 1 {
		t.Fatalf("models len = %d, want 1", len(catalog.Models))
	}
	levels := catalog.Models[0].SupportedReasoningLevels
	if len(levels) == 0 {
		t.Fatal("supported_reasoning_levels empty")
	}
	for i, lvl := range levels {
		if lvl.Effort == "" {
			t.Errorf("levels[%d].effort empty (likely emitted as bare string)", i)
		}
		if lvl.Description == "" {
			t.Errorf("levels[%d].description empty", i)
		}
	}
}

func TestApplyCodexConfig_NoModels_SkipsCatalog(t *testing.T) {
	tempDir := t.TempDir()
	t.Setenv("HOME", tempDir)

	if _, err := ApplyCodexConfig("http://h/tingly/codex", nil, DefaultCodexPrefs(), true); err != nil {
		t.Fatalf("ApplyCodexConfig: %v", err)
	}

	cfg := loadCodexConfigForTest(t, filepath.Join(tempDir, ".codex", "config.toml"))
	if _, ok := cfg["model_catalog_json"]; ok {
		t.Errorf("model_catalog_json should not be set when no models: %v", cfg["model_catalog_json"])
	}
	if _, err := os.Stat(filepath.Join(tempDir, ".codex", "tingly-model-catalog.json")); !os.IsNotExist(err) {
		t.Errorf("catalog file should not exist when no models, err=%v", err)
	}
}

func TestApplyCodexConfig_WriteCatalogFalse_SkipsCatalog(t *testing.T) {
	tempDir := t.TempDir()
	t.Setenv("HOME", tempDir)

	result, err := ApplyCodexConfig("http://h/tingly/codex", []string{"tingly-codex"}, DefaultCodexPrefs(), false)
	if err != nil {
		t.Fatalf("ApplyCodexConfig: %v", err)
	}
	if !result.Success {
		t.Fatalf("expected success: %s", result.Message)
	}

	codexDir := filepath.Join(tempDir, ".codex")
	cfg := loadCodexConfigForTest(t, filepath.Join(codexDir, "config.toml"))
	if _, ok := cfg["model_catalog_json"]; ok {
		t.Errorf("model_catalog_json should be absent when writeCatalog=false, got %v", cfg["model_catalog_json"])
	}
	if _, err := os.Stat(filepath.Join(codexDir, "tingly-model-catalog.json")); !os.IsNotExist(err) {
		t.Errorf("catalog file should not exist when writeCatalog=false")
	}
}

func TestApplyCodexConfig_BacksUpExistingCatalog(t *testing.T) {
	tempDir := t.TempDir()
	t.Setenv("HOME", tempDir)

	codexDir := filepath.Join(tempDir, ".codex")
	if err := os.MkdirAll(codexDir, 0o755); err != nil {
		t.Fatal(err)
	}
	catalogPath := filepath.Join(codexDir, "tingly-model-catalog.json")
	stale := []byte(`{"models":[{"slug":"old"}]}`)
	if err := os.WriteFile(catalogPath, stale, 0o644); err != nil {
		t.Fatal(err)
	}

	if _, err := ApplyCodexConfig("http://h/tingly/codex", []string{"new-model"}, DefaultCodexPrefs(), true); err != nil {
		t.Fatalf("ApplyCodexConfig: %v", err)
	}

	backupDir := filepath.Join(codexDir, "backup")
	matches, err := filepath.Glob(filepath.Join(backupDir, "tingly-model-catalog.json.bak-*.json"))
	if err != nil {
		t.Fatal(err)
	}
	if len(matches) == 0 {
		t.Errorf("expected backup of existing catalog in %s, none found", backupDir)
	}
}

func TestApplyCodexConfig_NoModels_OnlyTouchesManagedFields(t *testing.T) {
	tempDir := t.TempDir()
	t.Setenv("HOME", tempDir)

	codexDir := filepath.Join(tempDir, ".codex")
	if err := os.MkdirAll(codexDir, 0o755); err != nil {
		t.Fatal(err)
	}
	existing := `model = "user-custom"
some_user_flag = true
`
	if err := os.WriteFile(filepath.Join(codexDir, "config.toml"), []byte(existing), 0o644); err != nil {
		t.Fatal(err)
	}

	if _, err := ApplyCodexConfig("http://h/tingly/codex", nil, DefaultCodexPrefs(), true); err != nil {
		t.Fatal(err)
	}

	cfg := loadCodexConfigForTest(t, filepath.Join(codexDir, "config.toml"))
	// model untouched because we have nothing to put there
	if cfg["model"] != "user-custom" {
		t.Errorf("model should be untouched when no models given, got %v", cfg["model"])
	}
	if cfg["some_user_flag"] != true {
		t.Errorf("some_user_flag lost: %v", cfg["some_user_flag"])
	}
	// Provider still installed so codex can talk to tingly-box
	providers, _ := cfg["model_providers"].(map[string]interface{})
	if _, ok := providers["tingly-box"]; !ok {
		t.Errorf("tingly-box provider should still be installed: %#v", providers)
	}
}

func TestApplyCodexConfig_PrefsAppliedTopLevelAndPerProfile(t *testing.T) {
	tempDir := t.TempDir()
	t.Setenv("HOME", tempDir)

	prefs := &CodexPrefs{
		ModelReasoningEffort:            "high",
		ModelReasoningSummary:           "detailed",
		ModelVerbosity:                  "low",
		ModelSupportsReasoningSummaries: "true",
	}
	if _, err := ApplyCodexConfig("http://h/tingly/codex", []string{"tingly-codex"}, prefs, true); err != nil {
		t.Fatalf("ApplyCodexConfig: %v", err)
	}

	cfg := loadCodexConfigForTest(t, filepath.Join(tempDir, ".codex", "config.toml"))
	// Top-level
	if cfg["model_reasoning_effort"] != "high" {
		t.Errorf("top model_reasoning_effort = %v, want high", cfg["model_reasoning_effort"])
	}
	if cfg["model_reasoning_summary"] != "detailed" {
		t.Errorf("top model_reasoning_summary = %v, want detailed", cfg["model_reasoning_summary"])
	}
	if cfg["model_verbosity"] != "low" {
		t.Errorf("top model_verbosity = %v, want low", cfg["model_verbosity"])
	}
	if cfg["model_supports_reasoning_summaries"] != true {
		t.Errorf("top model_supports_reasoning_summaries = %v, want true", cfg["model_supports_reasoning_summaries"])
	}
	// Per-profile (self-contained)
	profiles, _ := cfg["profiles"].(map[string]interface{})
	p, _ := profiles["tingly-codex"].(map[string]interface{})
	if p["model_reasoning_effort"] != "high" {
		t.Errorf("profile model_reasoning_effort = %v, want high", p["model_reasoning_effort"])
	}
	if p["model_verbosity"] != "low" {
		t.Errorf("profile model_verbosity = %v, want low", p["model_verbosity"])
	}
}

func TestApplyCodexConfig_PrefsRejectInvalidEnumAndCannotClobberManaged(t *testing.T) {
	tempDir := t.TempDir()
	t.Setenv("HOME", tempDir)

	prefs := &CodexPrefs{
		ModelReasoningEffort:            "bogus", // invalid enum -> dropped
		ModelSupportsReasoningSummaries: "yes",   // not "true" -> dropped
	}
	if _, err := ApplyCodexConfig("http://h/tingly/codex", []string{"m1"}, prefs, true); err != nil {
		t.Fatalf("ApplyCodexConfig: %v", err)
	}

	cfg := loadCodexConfigForTest(t, filepath.Join(tempDir, ".codex", "config.toml"))
	if _, ok := cfg["model_reasoning_effort"]; ok {
		t.Errorf("invalid enum should be dropped, got %v", cfg["model_reasoning_effort"])
	}
	if _, ok := cfg["model_supports_reasoning_summaries"]; ok {
		t.Errorf("non-true bool should be dropped, got %v", cfg["model_supports_reasoning_summaries"])
	}
	// Managed fields remain controlled by tingly-box.
	if cfg["model_provider"] != "tingly-box" {
		t.Errorf("model_provider = %v, want tingly-box", cfg["model_provider"])
	}
	if cfg["model"] != "m1" {
		t.Errorf("model = %v, want m1", cfg["model"])
	}
}
