package agent

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"
)

func claudeSettingsPath(t *testing.T) string {
	t.Helper()
	home, err := os.UserHomeDir()
	if err != nil {
		t.Fatalf("UserHomeDir: %v", err)
	}
	return filepath.Join(home, ".claude", "settings.json")
}

func claudeOnboardingPath(t *testing.T) string {
	t.Helper()
	home, err := os.UserHomeDir()
	if err != nil {
		t.Fatalf("UserHomeDir: %v", err)
	}
	return filepath.Join(home, ".claude.json")
}

func TestClaudeCodeConfig_Apply_Creates(t *testing.T) {
	home := t.TempDir()
	t.Setenv("HOME", home)
	t.Setenv("USERPROFILE", home)

	cfg := &ClaudeCodeConfig{}
	result, err := cfg.Apply(&ClaudeCodeParams{
		BaseURL: "https://tingly.local/tingly/claude_code",
		APIKey:  "sk-test",
		ModelConfig: ClaudeCodeModelConfig{
			Default: "tingly/cc",
		},
	})
	if err != nil {
		t.Fatalf("Apply returned error: %v", err)
	}
	if !result.Success {
		t.Fatalf("expected success, got %+v", result)
	}
	if result.AgentType != AgentTypeClaudeCode {
		t.Errorf("expected AgentType=claude-code, got %v", result.AgentType)
	}
	if len(result.ConfigFiles) != 2 {
		t.Errorf("expected 2 config files (settings.json + .claude.json), got %v", result.ConfigFiles)
	}

	settingsPath := claudeSettingsPath(t)
	data, err := os.ReadFile(settingsPath)
	if err != nil {
		t.Fatalf("expected settings.json to be written: %v", err)
	}
	if !strings.Contains(string(data), "sk-test") {
		t.Errorf("expected settings.json to contain the API key, got %s", data)
	}
	if !strings.Contains(string(data), "tingly/cc") {
		t.Errorf("expected settings.json to contain the model, got %s", data)
	}

	onboardingPath := claudeOnboardingPath(t)
	if _, err := os.Stat(onboardingPath); err != nil {
		t.Fatalf("expected .claude.json to be written: %v", err)
	}
}

func TestClaudeCodeConfig_Apply_UpdatesAndBackups(t *testing.T) {
	home := t.TempDir()
	t.Setenv("HOME", home)
	t.Setenv("USERPROFILE", home)

	cfg := &ClaudeCodeConfig{}
	if _, err := cfg.Apply(&ClaudeCodeParams{BaseURL: "https://first", APIKey: "key-1"}); err != nil {
		t.Fatalf("first Apply returned error: %v", err)
	}

	result, err := cfg.Apply(&ClaudeCodeParams{BaseURL: "https://second", APIKey: "key-2"})
	if err != nil {
		t.Fatalf("second Apply returned error: %v", err)
	}
	if !result.Success {
		t.Fatalf("expected success, got %+v", result)
	}
	if len(result.BackupPaths) == 0 {
		t.Error("expected a backup path to be recorded on update")
	}

	data, err := os.ReadFile(claudeSettingsPath(t))
	if err != nil {
		t.Fatalf("ReadFile: %v", err)
	}
	if !strings.Contains(string(data), "key-2") {
		t.Errorf("expected settings.json to contain the updated key, got %s", data)
	}
	if strings.Contains(string(data), "key-1") {
		t.Errorf("expected settings.json to no longer contain the old key, got %s", data)
	}
}

func TestClaudeCodeConfig_Apply_InstallStatusLine(t *testing.T) {
	home := t.TempDir()
	t.Setenv("HOME", home)
	t.Setenv("USERPROFILE", home)

	cfg := &ClaudeCodeConfig{}
	result, err := cfg.Apply(&ClaudeCodeParams{
		BaseURL:           "https://tingly.local",
		APIKey:            "sk-test",
		InstallStatusLine: true,
		StatusLineScript:  []byte("#!/bin/bash\necho stub-statusline\n"),
	})
	if err != nil {
		t.Fatalf("Apply returned error: %v", err)
	}
	if !result.Success {
		t.Fatalf("expected success, got %+v", result)
	}

	scriptPath := filepath.Join(home, ".claude", "tingly-statusline.sh")
	installed, err := os.ReadFile(scriptPath)
	if err != nil {
		t.Fatalf("expected status line script to be installed: %v", err)
	}
	if !strings.Contains(string(installed), "stub-statusline") {
		t.Errorf("expected installed script to contain the caller-supplied content, got %s", installed)
	}

	data, err := os.ReadFile(claudeSettingsPath(t))
	if err != nil {
		t.Fatalf("ReadFile: %v", err)
	}
	if !strings.Contains(string(data), "statusLine") {
		t.Errorf("expected settings.json to contain statusLine entry, got %s", data)
	}
}

func TestClaudeCodeConfig_Apply_ExtraConfig(t *testing.T) {
	home := t.TempDir()
	t.Setenv("HOME", home)
	t.Setenv("USERPROFILE", home)

	cfg := &ClaudeCodeConfig{}
	result, err := cfg.Apply(&ClaudeCodeParams{
		BaseURL:     "https://tingly.local",
		APIKey:      "sk-test",
		ExtraConfig: map[string]interface{}{"customKey": "customValue"},
	})
	if err != nil {
		t.Fatalf("Apply returned error: %v", err)
	}
	if !result.Success {
		t.Fatalf("expected success, got %+v", result)
	}

	data, err := os.ReadFile(claudeSettingsPath(t))
	if err != nil {
		t.Fatalf("ReadFile: %v", err)
	}
	if !strings.Contains(string(data), "customValue") {
		t.Errorf("expected settings.json to contain the extra config value, got %s", data)
	}
}

func TestClaudeCodeConfig_Apply_InvalidParams(t *testing.T) {
	cfg := &ClaudeCodeConfig{}
	if _, err := cfg.Apply("not-the-right-type"); err == nil {
		t.Fatal("expected error for invalid params type")
	}
}

func TestClaudeCodeConfig_Restore_NoBackupFails(t *testing.T) {
	home := t.TempDir()
	t.Setenv("HOME", home)
	t.Setenv("USERPROFILE", home)

	cfg := &ClaudeCodeConfig{}
	result, err := cfg.Restore()
	if err != nil {
		t.Fatalf("Restore returned error: %v", err)
	}
	if result.Success {
		t.Fatalf("expected restore to fail with no backups, got %+v", result)
	}
	if len(result.Failures) == 0 {
		t.Error("expected failures to be recorded when no backup exists")
	}
}

func TestClaudeCodeConfig_Restore_AfterApplyTwice(t *testing.T) {
	home := t.TempDir()
	t.Setenv("HOME", home)
	t.Setenv("USERPROFILE", home)

	cfg := &ClaudeCodeConfig{}
	if _, err := cfg.Apply(&ClaudeCodeParams{BaseURL: "https://first", APIKey: "key-1"}); err != nil {
		t.Fatalf("first Apply returned error: %v", err)
	}
	if _, err := cfg.Apply(&ClaudeCodeParams{BaseURL: "https://second", APIKey: "key-2"}); err != nil {
		t.Fatalf("second Apply returned error: %v", err)
	}

	// Restore itself creates a pre-restore backup of the live (key-2) file.
	// Backup filenames carry a second-granularity timestamp
	// (backupTimestampLayout = "20060102-150405"), so a restore that lands in
	// the same wall-clock second as the second Apply's backup would collide
	// on the file path. This is a pre-existing property of the backup
	// scheme (not something this test should paper over), so give it a
	// moment to cross a timestamp boundary rather than asserting success
	// unconditionally.
	time.Sleep(1100 * time.Millisecond)

	result, err := cfg.Restore()
	if err != nil {
		t.Fatalf("Restore returned error: %v", err)
	}
	if !result.Success {
		t.Fatalf("expected restore success, got %+v", result)
	}
	if len(result.RestoredFiles) == 0 {
		t.Error("expected at least one restored file")
	}

	data, err := os.ReadFile(claudeSettingsPath(t))
	if err != nil {
		t.Fatalf("ReadFile: %v", err)
	}
	if !strings.Contains(string(data), "key-1") {
		t.Errorf("expected settings.json to be restored to the first apply's key, got %s", data)
	}
}
