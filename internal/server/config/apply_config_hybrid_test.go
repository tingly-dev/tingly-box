package config

import (
	"encoding/json"
	"os"
	"path/filepath"
	"testing"

	"github.com/pelletier/go-toml/v2"
)

// Hybrid mode keeps requests on the tingly-box gateway while leaving
// ~/.codex/auth.json free to hold a native ChatGPT login. The gateway token
// therefore rides in config.toml's provider stanza as experimental_bearer_token
// (with requires_openai_auth=true) rather than in auth.json.

func TestRenderCodexConfigTOML_HybridEmbedsBearerToken(t *testing.T) {
	tomlBytes, err := RenderCodexConfigTOML("http://h/tingly/codex", []string{"tingly-codex"}, DefaultCodexPrefs(), false, "tingly-box-secret")
	if err != nil {
		t.Fatalf("RenderCodexConfigTOML: %v", err)
	}
	cfg := map[string]interface{}{}
	if err := toml.Unmarshal(tomlBytes, &cfg); err != nil {
		t.Fatalf("unmarshal: %v\n%s", err, tomlBytes)
	}
	providers, _ := cfg["model_providers"].(map[string]interface{})
	tb, _ := providers["tingly-box"].(map[string]interface{})
	if tb["experimental_bearer_token"] != "tingly-box-secret" {
		t.Errorf("experimental_bearer_token = %v, want tingly-box-secret", tb["experimental_bearer_token"])
	}
	if tb["requires_openai_auth"] != true {
		t.Errorf("requires_openai_auth = %v, want true", tb["requires_openai_auth"])
	}
	// Managed routing fields are still present so requests go through tingly.
	if cfg["model_provider"] != "tingly-box" {
		t.Errorf("model_provider = %v, want tingly-box", cfg["model_provider"])
	}
}

func TestRenderCodexConfigTOML_GatewayProviderShape(t *testing.T) {
	// Classic gateway path (bearerToken == ""): no hybrid bearer token, the key
	// is sourced from auth.json (requires_openai_auth=true), and the removed
	// `preferred_auth_method` field must not reappear (config-schema.json is
	// additionalProperties:false and rejects it).
	tomlBytes, err := RenderCodexConfigTOML("http://h/tingly/codex", []string{"tingly-codex"}, DefaultCodexPrefs(), false, "")
	if err != nil {
		t.Fatalf("RenderCodexConfigTOML: %v", err)
	}
	cfg := map[string]interface{}{}
	if err := toml.Unmarshal(tomlBytes, &cfg); err != nil {
		t.Fatalf("unmarshal: %v", err)
	}
	providers, _ := cfg["model_providers"].(map[string]interface{})
	tb, _ := providers["tingly-box"].(map[string]interface{})
	if _, ok := tb["experimental_bearer_token"]; ok {
		t.Errorf("gateway config unexpectedly carries experimental_bearer_token: %#v", tb)
	}
	if tb["requires_openai_auth"] != true {
		t.Errorf("requires_openai_auth = %v, want true (gateway sources key from auth.json)", tb["requires_openai_auth"])
	}
	if _, ok := tb["preferred_auth_method"]; ok {
		t.Errorf("provider carries preferred_auth_method, which is not a valid Codex config-schema field: %#v", tb)
	}
}

func TestApplyCodexConfigWithContextWindows_HybridWritesBearerToken(t *testing.T) {
	tempDir := t.TempDir()
	t.Setenv("HOME", tempDir)

	result, err := ApplyCodexConfigWithContextWindows("http://h/tingly/codex", []string{"tingly-codex"}, DefaultCodexPrefs(), true, nil, "tok-123")
	if err != nil {
		t.Fatalf("ApplyCodexConfigWithContextWindows: %v", err)
	}
	if !result.Success {
		t.Fatalf("expected success, got %+v", result)
	}
	cfg := loadCodexConfigForTest(t, filepath.Join(tempDir, ".codex", "config.toml"))
	providers, _ := cfg["model_providers"].(map[string]interface{})
	tb, _ := providers["tingly-box"].(map[string]interface{})
	if tb["experimental_bearer_token"] != "tok-123" {
		t.Errorf("experimental_bearer_token = %v, want tok-123", tb["experimental_bearer_token"])
	}
}

func TestApplyCodexAuth_HybridWithoutTokens_LeavesAuthJSONUntouched(t *testing.T) {
	tempDir := t.TempDir()
	t.Setenv("HOME", tempDir)

	codexDir := filepath.Join(tempDir, ".codex")
	if err := os.MkdirAll(codexDir, 0o755); err != nil {
		t.Fatal(err)
	}
	authPath := filepath.Join(codexDir, "auth.json")
	original := `{
  "auth_mode": "chatgpt",
  "tokens": {
    "access_token": "existing-access",
    "refresh_token": "existing-refresh"
  }
}`
	if err := os.WriteFile(authPath, []byte(original), 0o600); err != nil {
		t.Fatal(err)
	}

	result, err := ApplyCodexAuth(CodexAuthHybrid, "should-be-ignored", nil)
	if err != nil {
		t.Fatalf("ApplyCodexAuth: %v", err)
	}
	if !result.Success {
		t.Fatalf("expected success, got %+v", result)
	}

	got, err := os.ReadFile(authPath)
	if err != nil {
		t.Fatalf("read auth.json: %v", err)
	}
	if string(got) != original {
		t.Errorf("auth.json was modified.\n--- got ---\n%s\n--- want ---\n%s", got, original)
	}
}

func TestApplyCodexAuth_HybridWithTokens_WritesChatGPTLogin(t *testing.T) {
	tempDir := t.TempDir()
	t.Setenv("HOME", tempDir)

	tokens := &CodexChatGPTTokens{
		AccessToken:  "acc",
		RefreshToken: "ref",
		IDToken:      "idt",
		AccountID:    "acct-1",
	}
	result, err := ApplyCodexAuth(CodexAuthHybrid, "gateway-key", tokens)
	if err != nil {
		t.Fatalf("ApplyCodexAuth: %v", err)
	}
	if !result.Success {
		t.Fatalf("expected success, got %+v", result)
	}

	data, err := os.ReadFile(filepath.Join(tempDir, ".codex", "auth.json"))
	if err != nil {
		t.Fatalf("read auth.json: %v", err)
	}
	payload := map[string]interface{}{}
	if err := json.Unmarshal(data, &payload); err != nil {
		t.Fatalf("unmarshal auth.json: %v", err)
	}
	if payload["auth_mode"] != "chatgpt" {
		t.Errorf("auth_mode = %v, want chatgpt", payload["auth_mode"])
	}
	// The gateway key must NOT leak into auth.json in hybrid mode.
	if _, ok := payload["OPENAI_API_KEY"]; ok {
		t.Errorf("auth.json unexpectedly carries OPENAI_API_KEY: %#v", payload)
	}
	tok, _ := payload["tokens"].(map[string]interface{})
	if tok["access_token"] != "acc" || tok["refresh_token"] != "ref" {
		t.Errorf("tokens block wrong: %#v", tok)
	}
}
