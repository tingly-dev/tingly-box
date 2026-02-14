package guardrails

import (
	"os"
	"path/filepath"
	"testing"
)

func TestLoadConfigYAML(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "guardrails.yaml")

	data := []byte(`version: "v1"
rules:
  - id: "test"
    name: "Test"
    type: "text_match"
    enabled: true
    params:
      patterns: ["rm -rf"]
`)

	if err := os.WriteFile(path, data, 0600); err != nil {
		t.Fatalf("write file: %v", err)
	}

	cfg, err := LoadConfig(path)
	if err != nil {
		t.Fatalf("load config: %v", err)
	}
	if cfg.Version != "v1" {
		t.Fatalf("expected version v1, got %q", cfg.Version)
	}
	if len(cfg.Rules) != 1 {
		t.Fatalf("expected 1 rule, got %d", len(cfg.Rules))
	}
}

func TestLoadConfigJSON(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "guardrails.json")

	data := []byte(`{"version":"v1","rules":[{"id":"test","name":"Test","type":"text_match","enabled":true,"params":{"patterns":["rm -rf"]}}]}`)

	if err := os.WriteFile(path, data, 0600); err != nil {
		t.Fatalf("write file: %v", err)
	}

	cfg, err := LoadConfig(path)
	if err != nil {
		t.Fatalf("load config: %v", err)
	}
	if cfg.Version != "v1" {
		t.Fatalf("expected version v1, got %q", cfg.Version)
	}
	if len(cfg.Rules) != 1 {
		t.Fatalf("expected 1 rule, got %d", len(cfg.Rules))
	}
}
