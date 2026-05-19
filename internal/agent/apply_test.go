package agent

import (
	"encoding/json"
	"testing"
)

func TestBuildOpenCodeConfig_DefaultModels(t *testing.T) {
	payload := BuildOpenCodeConfig("http://localhost:12580/tingly/opencode", "tok", nil)

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

func TestBuildOpenCodeConfig_CustomModels(t *testing.T) {
	customModels := map[string]interface{}{
		"tingly/cc-default": map[string]interface{}{"name": "tingly/cc-default"},
		"tingly/cc-haiku":   map[string]interface{}{"name": "tingly/cc-haiku"},
	}

	payload := BuildOpenCodeConfig("http://localhost:12580/tingly/opencode", "tok", customModels)

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
