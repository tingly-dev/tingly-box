package agent

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func opencodeConfigPath(t *testing.T) string {
	t.Helper()
	home, err := os.UserHomeDir()
	if err != nil {
		t.Fatalf("UserHomeDir: %v", err)
	}
	return filepath.Join(home, ".config", "opencode", "opencode.json")
}

func TestOpenCodeConfig_Apply_Creates(t *testing.T) {
	home := t.TempDir()
	t.Setenv("HOME", home)
	t.Setenv("USERPROFILE", home)

	cfg := &OpenCodeConfig{}
	result, err := cfg.Apply(&OpenCodeParams{
		Config: map[string]interface{}{
			"provider": map[string]interface{}{
				"tingly": map[string]interface{}{
					"apiKey": "sk-test",
				},
			},
		},
	})
	if err != nil {
		t.Fatalf("Apply returned error: %v", err)
	}
	if !result.Success {
		t.Fatalf("expected success, got %+v", result)
	}
	if len(result.ConfigFiles) != 1 || !strings.Contains(result.ConfigFiles[0], "created") {
		t.Errorf("expected one created config file entry, got %v", result.ConfigFiles)
	}

	data, err := os.ReadFile(opencodeConfigPath(t))
	if err != nil {
		t.Fatalf("expected opencode.json to be written: %v", err)
	}
	if !strings.Contains(string(data), "sk-test") {
		t.Errorf("expected opencode.json to contain the API key, got %s", data)
	}
}

func TestOpenCodeConfig_Apply_MergesExistingProviders(t *testing.T) {
	home := t.TempDir()
	t.Setenv("HOME", home)
	t.Setenv("USERPROFILE", home)

	cfg := &OpenCodeConfig{}
	if _, err := cfg.Apply(&OpenCodeParams{
		Config: map[string]interface{}{
			"provider": map[string]interface{}{
				"tingly": map[string]interface{}{"apiKey": "key-1"},
			},
		},
	}); err != nil {
		t.Fatalf("first Apply returned error: %v", err)
	}

	result, err := cfg.Apply(&OpenCodeParams{
		Config: map[string]interface{}{
			"provider": map[string]interface{}{
				"other": map[string]interface{}{"apiKey": "key-2"},
			},
		},
	})
	if err != nil {
		t.Fatalf("second Apply returned error: %v", err)
	}
	if !result.Success {
		t.Fatalf("expected success, got %+v", result)
	}
	if len(result.BackupPaths) == 0 {
		t.Error("expected a backup path to be recorded on update")
	}

	data, err := os.ReadFile(opencodeConfigPath(t))
	if err != nil {
		t.Fatalf("ReadFile: %v", err)
	}
	// Both providers should be present — merge preserves the first provider
	// while adding the second.
	if !strings.Contains(string(data), "key-1") {
		t.Errorf("expected opencode.json to retain the first provider, got %s", data)
	}
	if !strings.Contains(string(data), "key-2") {
		t.Errorf("expected opencode.json to contain the new provider, got %s", data)
	}
}

func TestOpenCodeConfig_Apply_InvalidParams(t *testing.T) {
	cfg := &OpenCodeConfig{}
	if _, err := cfg.Apply("not-the-right-type"); err == nil {
		t.Fatal("expected error for invalid params type")
	}
}

func TestOpenCodeConfig_Restore_NoBackupFails(t *testing.T) {
	home := t.TempDir()
	t.Setenv("HOME", home)
	t.Setenv("USERPROFILE", home)

	cfg := &OpenCodeConfig{}
	result, err := cfg.Restore()
	if err != nil {
		t.Fatalf("Restore returned error: %v", err)
	}
	if result.Success {
		t.Fatalf("expected restore to fail with no backups, got %+v", result)
	}
}
