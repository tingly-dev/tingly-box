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

func TestBuildOpenCodeModels_DeclaresAttachmentAndModalities(t *testing.T) {
	models := BuildOpenCodeModels([]string{"tingly-opencode-a", "tingly-opencode-b"})
	if len(models) != 2 {
		t.Fatalf("expected 2 models, got %d", len(models))
	}

	for name, raw := range models {
		entry, ok := raw.(map[string]interface{})
		if !ok {
			t.Fatalf("model %q entry is not a map: %v", name, raw)
		}
		if entry["name"] != name {
			t.Errorf("model %q: name = %v, want %q", name, entry["name"], name)
		}
		if entry["attachment"] != true {
			t.Errorf("model %q: attachment = %v, want true", name, entry["attachment"])
		}
		modalities, ok := entry["modalities"].(map[string]interface{})
		if !ok {
			t.Fatalf("model %q: modalities is not a map: %v", name, entry["modalities"])
		}
		input, ok := modalities["input"].([]string)
		if !ok || len(input) != 2 || input[0] != "text" || input[1] != "image" {
			t.Errorf("model %q: modalities.input = %v, want [text image]", name, modalities["input"])
		}
		output, ok := modalities["output"].([]string)
		if !ok || len(output) != 1 || output[0] != "text" {
			t.Errorf("model %q: modalities.output = %v, want [text]", name, modalities["output"])
		}
	}
}

func TestBuildOpenCodeModels_EmptyFallsBackToDefault(t *testing.T) {
	models := BuildOpenCodeModels(nil)
	if len(models) != 1 {
		t.Fatalf("expected 1 default model, got %d", len(models))
	}
	entry, ok := models["tingly-opencode"].(map[string]interface{})
	if !ok {
		t.Fatalf("missing default model 'tingly-opencode': %v", models)
	}
	if entry["attachment"] != true {
		t.Errorf("default model attachment = %v, want true", entry["attachment"])
	}
}
