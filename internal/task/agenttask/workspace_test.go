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

func TestResolveExistingWorkspace_CanonicalizesSymlink(t *testing.T) {
	root := t.TempDir()
	workspace := filepath.Join(root, "workspace")
	if err := os.Mkdir(workspace, 0o700); err != nil {
		t.Fatal(err)
	}
	alias := filepath.Join(root, "workspace-alias")
	if err := os.Symlink(workspace, alias); err != nil {
		t.Fatal(err)
	}

	resolved, err := ResolveExistingWorkspace("  " + alias + string(filepath.Separator) + "  ")
	if err != nil {
		t.Fatal(err)
	}
	want, err := filepath.EvalSymlinks(workspace)
	if err != nil {
		t.Fatal(err)
	}
	if resolved != want {
		t.Fatalf("resolved workspace = %q, want %q", resolved, want)
	}
	if err := validateWorkspace(alias); err == nil {
		t.Fatal("expected stored symlink path to be rejected as non-canonical")
	}
}

func TestResolveExistingWorkspace_RejectsInvalidPaths(t *testing.T) {
	file, err := os.CreateTemp(t.TempDir(), "workspace-file")
	if err != nil {
		t.Fatal(err)
	}
	if err := file.Close(); err != nil {
		t.Fatal(err)
	}

	tests := []struct {
		name string
		path string
	}{
		{name: "blank", path: "  "},
		{name: "relative", path: "relative/path"},
		{name: "missing", path: filepath.Join(t.TempDir(), "missing")},
		{name: "file", path: file.Name()},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			if _, err := ResolveExistingWorkspace(test.path); err == nil {
				t.Fatalf("ResolveExistingWorkspace(%q) unexpectedly succeeded", test.path)
			}
		})
	}
}
