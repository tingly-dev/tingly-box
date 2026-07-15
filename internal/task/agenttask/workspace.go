package agenttask

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"

	"github.com/google/uuid"
)

// CreateWorkspace creates the stable, service-owned workspace for a Task.
// Repeated calls are idempotent and return the same canonical absolute path.
func CreateWorkspace(configDir, taskID string) (string, error) {
	if strings.TrimSpace(configDir) == "" {
		return "", fmt.Errorf("config dir is required")
	}
	if _, err := uuid.Parse(taskID); err != nil {
		return "", fmt.Errorf("invalid task ID: %w", err)
	}
	root, err := filepath.Abs(configDir)
	if err != nil {
		return "", fmt.Errorf("resolve config dir: %w", err)
	}
	workspace := filepath.Join(root, "tasks", taskID, "workspace")
	if err := os.MkdirAll(workspace, 0o700); err != nil {
		return "", fmt.Errorf("create workspace: %w", err)
	}
	canonical, err := filepath.EvalSymlinks(workspace)
	if err != nil {
		return "", fmt.Errorf("canonicalize workspace: %w", err)
	}
	return canonical, nil
}

func validateWorkspace(path string) error {
	if !filepath.IsAbs(path) {
		return fmt.Errorf("workspace path must be absolute")
	}
	canonical, err := filepath.EvalSymlinks(path)
	if err != nil {
		return fmt.Errorf("resolve workspace: %w", err)
	}
	if canonical != filepath.Clean(path) {
		return fmt.Errorf("workspace path is not canonical")
	}
	info, err := os.Stat(canonical)
	if err != nil {
		return fmt.Errorf("stat workspace: %w", err)
	}
	if !info.IsDir() {
		return fmt.Errorf("workspace path is not a directory")
	}
	return nil
}
