package config

import (
	"os"
	"path/filepath"
	"testing"
	"time"
)

func TestSaveDoesNotRewriteUnchangedConfig(t *testing.T) {
	t.Parallel()

	dir := t.TempDir()
	configFile := filepath.Join(dir, "config.json")

	cfg := &Config{
		ConfigFile:        configFile,
		UserToken:         "user-token",
		ModelToken:        "model-token",
		VirtualModelToken: "virtual-token",
		JWTSecret:         "jwt-secret",
		Rules:             nil,
		Scenarios:         nil,
	}

	if err := cfg.Save(); err != nil {
		t.Fatalf("first save failed: %v", err)
	}

	info1, err := os.Stat(configFile)
	if err != nil {
		t.Fatalf("stat after first save failed: %v", err)
	}

	time.Sleep(1100 * time.Millisecond)

	if err := cfg.Save(); err != nil {
		t.Fatalf("second save failed: %v", err)
	}

	info2, err := os.Stat(configFile)
	if err != nil {
		t.Fatalf("stat after second save failed: %v", err)
	}

	if !info2.ModTime().Equal(info1.ModTime()) {
		t.Fatalf("expected unchanged config save to keep modtime, got %v -> %v", info1.ModTime(), info2.ModTime())
	}
}
