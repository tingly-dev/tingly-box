package main

import (
	"strings"
	"testing"

	tomlpkg "github.com/pelletier/go-toml/v2"
)

// A custom codex provider only sends the gateway key from auth.json when it
// declares requires_openai_auth; without it codex sends no Authorization header
// and the gateway answers 401. This mirrors what `tb` itself writes for codex
// (internal/config/apply_config_codex.go).
func TestCodexConfigTOML(t *testing.T) {
	raw := codexConfigTOML("http://127.0.0.1:1234/tingly/codex", "tingly-codex")

	var cfg map[string]any
	if err := tomlpkg.Unmarshal([]byte(raw), &cfg); err != nil {
		t.Fatalf("config.toml does not parse: %v\n%s", err, raw)
	}
	if cfg["model"] != "tingly-codex" {
		t.Errorf("model = %v", cfg["model"])
	}
	name, _ := cfg["model_provider"].(string)
	providers, _ := cfg["model_providers"].(map[string]any)
	provider, _ := providers[name].(map[string]any)
	if name == "" || provider == nil {
		t.Fatalf("model_provider %q has no [model_providers] stanza:\n%s", name, raw)
	}
	if provider["base_url"] != "http://127.0.0.1:1234/tingly/codex" || provider["wire_api"] != "responses" {
		t.Errorf("provider stanza = %v", provider)
	}
	if provider["requires_openai_auth"] != true {
		t.Errorf("provider must set requires_openai_auth so codex sends the key from auth.json: %v", provider)
	}
	if strings.Contains(raw, "disable_response_storage") {
		t.Errorf("config sets disable_response_storage, which codex ignores as unknown:\n%s", raw)
	}
}
