package agenttask

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/google/uuid"
)

func TestCreateWorkspace_IsStableAndPrivate(t *testing.T) {
	configDir := t.TempDir()
	taskID := uuid.NewString()

	first, err := CreateWorkspace(configDir, taskID)
	if err != nil {
		t.Fatal(err)
	}
	second, err := CreateWorkspace(configDir, taskID)
	if err != nil {
		t.Fatal(err)
	}
	if first != second || !filepath.IsAbs(first) {
		t.Fatalf("workspace not stable: first=%q second=%q", first, second)
	}
	info, err := os.Stat(first)
	if err != nil {
		t.Fatal(err)
	}
	if !info.IsDir() || info.Mode().Perm() != 0o700 {
		t.Fatalf("workspace mode = %v", info.Mode())
	}
	if err := validateWorkspace(first); err != nil {
		t.Fatalf("validateWorkspace: %v", err)
	}
}

func TestCreateWorkspace_RejectsPathLikeTaskID(t *testing.T) {
	if _, err := CreateWorkspace(t.TempDir(), "../escape"); err == nil {
		t.Fatal("expected invalid task ID error")
	}
}

func TestCreateWorkspace_RequiresConfigDir(t *testing.T) {
	if _, err := CreateWorkspace("", uuid.NewString()); err == nil {
		t.Fatal("expected config dir error")
	}
}

func TestValidateWorkspace_RequiresAbsolutePath(t *testing.T) {
	if err := validateWorkspace("relative/path"); err == nil {
		t.Fatal("expected absolute path error")
	}
}
