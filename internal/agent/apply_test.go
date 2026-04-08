package agent

import (
	"encoding/json"
	"testing"
)

func TestGenerateClaudeCodeEnv_Unified(t *testing.T) {
	env := GenerateClaudeCodeEnv("http://localhost:12580", "test-token", true)

	// Required base keys
	cases := map[string]string{
		"DISABLE_TELEMETRY":                        "1",
		"DISABLE_ERROR_REPORTING":                  "1",
		"CLAUDE_CODE_DISABLE_NONESSENTIAL_TRAFFIC": "1",
		"CLAUDE_CODE_MAX_OUTPUT_TOKENS":            "32000",
		"API_TIMEOUT_MS":                           "3000000",
		"ANTHROPIC_BASE_URL":                       "http://localhost:12580/tingly/claude_code",
		"ANTHROPIC_AUTH_TOKEN":                     "test-token",
		// Unified: all slots point to tingly/cc
		"ANTHROPIC_MODEL":                "tingly/cc",
		"ANTHROPIC_DEFAULT_HAIKU_MODEL":  "tingly/cc",
		"ANTHROPIC_DEFAULT_OPUS_MODEL":   "tingly/cc",
		"ANTHROPIC_DEFAULT_SONNET_MODEL": "tingly/cc",
		"CLAUDE_CODE_SUBAGENT_MODEL":     "tingly/cc",
	}

	for k, want := range cases {
		if got := env[k]; got != want {
			t.Errorf("env[%s] = %q, want %q", k, got, want)
		}
	}
}

func TestGenerateClaudeCodeEnv_Separate(t *testing.T) {
	env := GenerateClaudeCodeEnv("http://localhost:12580", "test-token", false)

	cases := map[string]string{
		"ANTHROPIC_MODEL":                "tingly/cc-default",
		"ANTHROPIC_DEFAULT_HAIKU_MODEL":  "tingly/cc-haiku",
		"ANTHROPIC_DEFAULT_OPUS_MODEL":   "tingly/cc-opus",
		"ANTHROPIC_DEFAULT_SONNET_MODEL": "tingly/cc-sonnet",
		"CLAUDE_CODE_SUBAGENT_MODEL":     "tingly/cc-subagent",
		"CLAUDE_CODE_MAX_OUTPUT_TOKENS":  "32000",
	}

	for k, want := range cases {
		if got := env[k]; got != want {
			t.Errorf("env[%s] = %q, want %q", k, got, want)
		}
	}
}

// TestGenerateClaudeCodeEnv_SettingsJSON verifies the env map produces a valid
// settings.json structure (the same shape written to ~/.claude/settings.json).
func TestGenerateClaudeCodeEnv_SettingsJSON(t *testing.T) {
	env := GenerateClaudeCodeEnv("http://127.0.0.1:12580", "tok", true)

	// Simulate what ApplyClaudeSettingsFromEnv writes: {"env": <env map>}
	payload := map[string]interface{}{"env": env}
	data, err := json.MarshalIndent(payload, "", "  ")
	if err != nil {
		t.Fatalf("failed to marshal settings JSON: %v", err)
	}

	var parsed map[string]interface{}
	if err := json.Unmarshal(data, &parsed); err != nil {
		t.Fatalf("settings JSON is not valid: %v", err)
	}

	envSection, ok := parsed["env"].(map[string]interface{})
	if !ok {
		t.Fatal("settings JSON missing 'env' section")
	}

	if envSection["CLAUDE_CODE_MAX_OUTPUT_TOKENS"] != "32000" {
		t.Errorf("CLAUDE_CODE_MAX_OUTPUT_TOKENS missing or wrong in settings JSON")
	}
	if envSection["ANTHROPIC_BASE_URL"] != "http://127.0.0.1:12580/tingly/claude_code" {
		t.Errorf("ANTHROPIC_BASE_URL missing or wrong in settings JSON")
	}
}

func TestGenerateOpenCodePayload_DefaultModels(t *testing.T) {
	payload := GenerateOpenCodePayload("http://localhost:12580/tingly/opencode", "tok", nil)

	data, err := json.MarshalIndent(payload, "", "  ")
	if err != nil {
		t.Fatalf("failed to marshal payload: %v", err)
	}

	var parsed map[string]interface{}
	if err := json.Unmarshal(data, &parsed); err != nil {
		t.Fatalf("payload JSON is not valid: %v", err)
	}

	if parsed["$schema"] != "https://opencode.ai/config.json" {
		t.Errorf("$schema missing or wrong: %v", parsed["$schema"])
	}

	providers, ok := parsed["provider"].(map[string]interface{})
	if !ok {
		t.Fatal("missing 'provider' section")
	}

	tb, ok := providers["tingly-box"].(map[string]interface{})
	if !ok {
		t.Fatal("missing 'tingly-box' provider")
	}

	if tb["npm"] != "@ai-sdk/anthropic" {
		t.Errorf("npm = %v, want @ai-sdk/anthropic", tb["npm"])
	}

	opts, ok := tb["options"].(map[string]interface{})
	if !ok {
		t.Fatal("missing options in tingly-box provider")
	}
	if opts["baseURL"] != "http://localhost:12580/tingly/opencode" {
		t.Errorf("baseURL = %v", opts["baseURL"])
	}
	if opts["apiKey"] != "tok" {
		t.Errorf("apiKey = %v", opts["apiKey"])
	}

	// Default models should contain tingly-opencode
	models, ok := tb["models"].(map[string]interface{})
	if !ok {
		t.Fatal("missing models in tingly-box provider")
	}
	if _, exists := models["tingly-opencode"]; !exists {
		t.Errorf("default model 'tingly-opencode' not found in models: %v", models)
	}
}

func TestGenerateOpenCodePayload_CustomModels(t *testing.T) {
	customModels := map[string]interface{}{
		"tingly/cc-default": map[string]interface{}{"name": "tingly/cc-default"},
		"tingly/cc-haiku":   map[string]interface{}{"name": "tingly/cc-haiku"},
	}

	payload := GenerateOpenCodePayload("http://localhost:12580/tingly/opencode", "tok", customModels)

	data, _ := json.Marshal(payload)
	var parsed map[string]interface{}
	json.Unmarshal(data, &parsed)

	providers := parsed["provider"].(map[string]interface{})
	tb := providers["tingly-box"].(map[string]interface{})
	models := tb["models"].(map[string]interface{})

	if len(models) != 2 {
		t.Errorf("expected 2 models, got %d", len(models))
	}
	if _, ok := models["tingly/cc-default"]; !ok {
		t.Error("tingly/cc-default not found")
	}
	if _, ok := models["tingly/cc-haiku"]; !ok {
		t.Error("tingly/cc-haiku not found")
	}
}
