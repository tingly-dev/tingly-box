package bot

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"
)

// Helper functions

// ExpandPath expands ~ and environment variables in a path
func ExpandPath(path string) (string, error) {
	return ExpandPathFrom(path, "")
}

// ExpandPathFrom expands a user-provided path with the given baseDir as the
// reference for relative paths. ~/ and absolute paths are handled as in
// ExpandPath. When path is relative and baseDir is non-empty, it is joined
// with baseDir; otherwise it falls back to filepath.Abs (process cwd).
func ExpandPathFrom(path, baseDir string) (string, error) {
	if path == "~" {
		return os.UserHomeDir()
	}
	if strings.HasPrefix(path, "~/") {
		home, err := os.UserHomeDir()
		if err != nil {
			return "", err
		}
		return filepath.Join(home, path[2:]), nil
	}
	if filepath.IsAbs(path) {
		return filepath.Clean(path), nil
	}
	if baseDir != "" {
		return filepath.Clean(filepath.Join(baseDir, path)), nil
	}
	return filepath.Abs(path)
}

// ValidateProjectPath validates that a path is a valid project directory
func ValidateProjectPath(path string) error {
	info, err := os.Stat(path)
	if err != nil {
		return err
	}
	if !info.IsDir() {
		return fmt.Errorf("not a directory: %s", path)
	}
	return nil
}

// ShortenPath shortens a path for display
func ShortenPath(path string) string {
	home, err := os.UserHomeDir()
	if err == nil && strings.HasPrefix(path, home) {
		return "~" + path[len(home):]
	}
	if len(path) > 40 {
		return "..." + path[len(path)-37:]
	}
	return path
}
