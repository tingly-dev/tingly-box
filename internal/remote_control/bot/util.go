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
	if strings.HasPrefix(path, "~/") {
		home, err := os.UserHomeDir()
		if err != nil {
			return "", err
		}
		return filepath.Join(home, path[2:]), nil
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
