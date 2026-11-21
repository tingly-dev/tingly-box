package main

import (
	"encoding/json"
	"strings"
	"testing"

	tomlpkg "github.com/pelletier/go-toml/v2"
)

// The codex CLI under test must be configured exactly as `tb`'s own apply flow
// configures it, or the harness exercises a setup no user has. In particular a
// custom provider only sends the gateway key from auth.json when it declares
// requires_openai_auth; without it codex sends no Authorization header and the
// gateway answers 401.
func TestCodexHomeFilesMatchProductConfig(t *testing.T) {
	configTOML, authJSON, err := codexHomeFiles("http://127.0.0.1:1234/tingly/codex", "tingly-codex", "tok-123")
	if err != nil {
		t.Fatal(err)
	}

	var cfg map[string]any
	if err := tomlpkg.Unmarshal(configTOML, &cfg); err != nil {
		t.Fatalf("config.toml does not parse: %v\n%s", err, configTOML)
	}
	if cfg["model"] != "tingly-codex" {
		t.Errorf("model = %v", cfg["model"])
	}
	name, _ := cfg["model_provider"].(string)
	providers, _ := cfg["model_providers"].(map[string]any)
	provider, _ := providers[name].(map[string]any)
	if name == "" || provider == nil {
		t.Fatalf("model_provider %q has no [model_providers] stanza:\n%s", name, configTOML)
	}
	if provider["base_url"] != "http://127.0.0.1:1234/tingly/codex" || provider["wire_api"] != "responses" {
		t.Errorf("provider stanza = %v", provider)
	}
	if provider["requires_openai_auth"] != true {
		t.Errorf("provider must set requires_openai_auth so codex sends the key from auth.json: %v", provider)
	}
	if strings.Contains(string(configTOML), "disable_response_storage") {
		t.Errorf("config sets disable_response_storage, which codex ignores as unknown:\n%s", configTOML)
	}

	var auth map[string]string
	if err := json.Unmarshal(authJSON, &auth); err != nil {
		t.Fatalf("auth.json does not parse: %v", err)
	}
	if auth["OPENAI_API_KEY"] != "tok-123" {
		t.Errorf("auth.json = %s", authJSON)
	}
}
