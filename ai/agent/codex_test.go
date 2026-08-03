package agent

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func codexConfigPath(t *testing.T) string {
	t.Helper()
	home, err := os.UserHomeDir()
	if err != nil {
		t.Fatalf("UserHomeDir: %v", err)
	}
	return filepath.Join(home, ".codex", "config.toml")
}

func codexAuthPath(t *testing.T) string {
	t.Helper()
	home, err := os.UserHomeDir()
	if err != nil {
		t.Fatalf("UserHomeDir: %v", err)
	}
	return filepath.Join(home, ".codex", "auth.json")
}

func TestCodexConfig_Apply_APIKeyMode(t *testing.T) {
	home := t.TempDir()
	t.Setenv("HOME", home)
	t.Setenv("USERPROFILE", home)

	cfg := &CodexConfig{}
	result, err := cfg.Apply(&CodexParams{
		CodexBaseURL: "https://tingly.local/tingly/codex",
		APIKey:       "sk-test",
		Models:       []string{"tingly/codex"},
		Prefs:        DefaultCodexPrefs(),
		WriteCatalog: true,
		AuthMode:     CodexAuthAPIKey,
	})
	if err != nil {
		t.Fatalf("Apply returned error: %v", err)
	}
	if !result.Success {
		t.Fatalf("expected success, got %+v", result)
	}
	if len(result.ConfigFiles) != 2 {
		t.Errorf("expected config.toml + auth.json to both be written, got %v", result.ConfigFiles)
	}

	configData, err := os.ReadFile(codexConfigPath(t))
	if err != nil {
		t.Fatalf("expected config.toml to be written: %v", err)
	}
	if !strings.Contains(string(configData), "tingly.local") {
		t.Errorf("expected config.toml to reference the base URL, got %s", configData)
	}

	authData, err := os.ReadFile(codexAuthPath(t))
	if err != nil {
		t.Fatalf("expected auth.json to be written: %v", err)
	}
	if !strings.Contains(string(authData), "sk-test") {
		t.Errorf("expected auth.json to contain the API key, got %s", authData)
	}
}

func TestCodexConfig_Apply_ChatGPTMode_SkipsConfigToml(t *testing.T) {
	home := t.TempDir()
	t.Setenv("HOME", home)
	t.Setenv("USERPROFILE", home)

	cfg := &CodexConfig{}
	result, err := cfg.Apply(&CodexParams{
		AuthMode: CodexAuthChatGPT,
		ChatGPTTokens: &CodexChatGPTTokens{
			IDToken:      "id-token",
			AccessToken:  "access-token",
			RefreshToken: "refresh-token",
			AccountID:    "acct-1",
		},
	})
	if err != nil {
		t.Fatalf("Apply returned error: %v", err)
	}
	if !result.Success {
		t.Fatalf("expected success, got %+v", result)
	}

	// ChatGPT mode leaves config.toml untouched (native OAuth talks directly
	// to OpenAI), so it should not appear in the ConfigFiles list.
	for _, f := range result.ConfigFiles {
		if strings.Contains(f, "config.toml") {
			t.Errorf("expected config.toml NOT to be touched in ChatGPT mode, got %v", result.ConfigFiles)
		}
	}

	if _, err := os.Stat(codexConfigPath(t)); !os.IsNotExist(err) {
		t.Errorf("expected config.toml to not exist, stat err=%v", err)
	}

	authData, err := os.ReadFile(codexAuthPath(t))
	if err != nil {
		t.Fatalf("expected auth.json to be written: %v", err)
	}
	if !strings.Contains(string(authData), "chatgpt") {
		t.Errorf("expected auth.json to record chatgpt auth mode, got %s", authData)
	}
}

func TestCodexConfig_Apply_HybridMode(t *testing.T) {
	home := t.TempDir()
	t.Setenv("HOME", home)
	t.Setenv("USERPROFILE", home)

	cfg := &CodexConfig{}
	result, err := cfg.Apply(&CodexParams{
		CodexBaseURL: "https://tingly.local/tingly/codex",
		APIKey:       "sk-test",
		Models:       []string{"tingly/codex"},
		Prefs:        DefaultCodexPrefs(),
		WriteCatalog: true,
		AuthMode:     CodexAuthHybrid,
		ChatGPTTokens: &CodexChatGPTTokens{
			AccessToken:  "access-token",
			RefreshToken: "refresh-token",
		},
	})
	if err != nil {
		t.Fatalf("Apply returned error: %v", err)
	}
	if !result.Success {
		t.Fatalf("expected success, got %+v", result)
	}

	// Hybrid mode writes both config.toml (gateway) and auth.json (native login).
	configData, err := os.ReadFile(codexConfigPath(t))
	if err != nil {
		t.Fatalf("expected config.toml to be written: %v", err)
	}
	if !strings.Contains(string(configData), "tingly.local") {
		t.Errorf("expected config.toml to reference the base URL, got %s", configData)
	}
	if _, err := os.Stat(codexAuthPath(t)); err != nil {
		t.Fatalf("expected auth.json to be written: %v", err)
	}
}

func TestCodexConfig_Apply_InvalidParams(t *testing.T) {
	cfg := &CodexConfig{}
	if _, err := cfg.Apply("not-the-right-type"); err == nil {
		t.Fatal("expected error for invalid params type")
	}
}

func TestCodexConfig_Restore_NoBackupFails(t *testing.T) {
	home := t.TempDir()
	t.Setenv("HOME", home)
	t.Setenv("USERPROFILE", home)

	cfg := &CodexConfig{}
	result, err := cfg.Restore()
	if err != nil {
		t.Fatalf("Restore returned error: %v", err)
	}
	if result.Success {
		t.Fatalf("expected restore to fail with no backups, got %+v", result)
	}
}
