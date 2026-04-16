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

func TestLoadConfigWithImports(t *testing.T) {
	dir := t.TempDir()
	rootPath := filepath.Join(dir, "guardrails.yaml")
	importPath := filepath.Join(dir, "builtin", "secrets.yaml")

	if err := os.MkdirAll(filepath.Dir(importPath), 0700); err != nil {
		t.Fatalf("mkdir: %v", err)
	}

	rootData := []byte(`imports:
  - builtin/secrets.yaml
groups:
  - id: "default"
    name: "Default"
policies:
  - id: "local"
    name: "Local"
    kind: "content"
    groups: ["default"]
    enabled: true
    match:
      patterns: ["local"]
`)
	importData := []byte(`policies:
  - id: "imported"
    name: "Imported"
    kind: "content"
    groups: ["default"]
    enabled: false
    match:
      patterns: ["imported"]
`)

	if err := os.WriteFile(rootPath, rootData, 0600); err != nil {
		t.Fatalf("write root file: %v", err)
	}
	if err := os.WriteFile(importPath, importData, 0600); err != nil {
		t.Fatalf("write import file: %v", err)
	}

	cfg, err := LoadConfig(rootPath)
	if err != nil {
		t.Fatalf("load config with imports: %v", err)
	}
	if len(cfg.Imports) != 1 {
		t.Fatalf("expected 1 import, got %d", len(cfg.Imports))
	}
	if len(cfg.Policies) != 2 {
		t.Fatalf("expected 2 policies, got %d", len(cfg.Policies))
	}
}

func TestLoadConfigRejectsImportedGroups(t *testing.T) {
	dir := t.TempDir()
	rootPath := filepath.Join(dir, "guardrails.yaml")
	importPath := filepath.Join(dir, "custom", "bad.yaml")

	if err := os.MkdirAll(filepath.Dir(importPath), 0700); err != nil {
		t.Fatalf("mkdir: %v", err)
	}

	rootData := []byte(`imports:
  - custom/bad.yaml
groups:
  - id: "default"
    name: "Default"
`)
	importData := []byte(`groups:
  - id: "extra"
    name: "Extra"
policies:
  - id: "imported"
    kind: "content"
    match:
      patterns: ["imported"]
`)

	if err := os.WriteFile(rootPath, rootData, 0600); err != nil {
		t.Fatalf("write root file: %v", err)
	}
	if err := os.WriteFile(importPath, importData, 0600); err != nil {
		t.Fatalf("write import file: %v", err)
	}

	if _, err := LoadConfig(rootPath); err == nil {
		t.Fatal("expected imported groups to fail")
	}
}
