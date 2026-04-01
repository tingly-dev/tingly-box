package guardrails

import (
	"os"
	"path/filepath"
	"testing"
)

func TestLoadConfigYAML(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "guardrails.yaml")

	data := []byte(`groups:
  - id: "default"
    name: "Default"
policies:
  - id: "test"
    name: "Test"
    kind: "content"
    groups: ["default"]
    enabled: true
    match:
      patterns: ["rm -rf"]
`)

	if err := os.WriteFile(path, data, 0600); err != nil {
		t.Fatalf("write file: %v", err)
	}

	cfg, err := LoadConfig(path)
	if err != nil {
		t.Fatalf("load config: %v", err)
	}
	if len(cfg.Policies) != 1 {
		t.Fatalf("expected 1 policy, got %d", len(cfg.Policies))
	}
}

func TestLoadConfigJSON(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "guardrails.json")

	data := []byte(`{"groups":[{"id":"default","name":"Default"}],"policies":[{"id":"test","name":"Test","kind":"content","groups":["default"],"enabled":true,"match":{"patterns":["rm -rf"]}}]}`)

	if err := os.WriteFile(path, data, 0600); err != nil {
		t.Fatalf("write file: %v", err)
	}

	cfg, err := LoadConfig(path)
	if err != nil {
		t.Fatalf("load config: %v", err)
	}
	if len(cfg.Policies) != 1 {
		t.Fatalf("expected 1 policy, got %d", len(cfg.Policies))
	}
}
