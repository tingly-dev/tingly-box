package agent

import (
	"fmt"
	"os"
	"path/filepath"
)

// expandUser resolves a leading "~" or "~/" in p against the current user's
// home directory. Other paths are returned unchanged.
func expandUser(p string) (string, error) {
	if p == "~" || hasPrefix(p, "~/") {
		home, err := os.UserHomeDir()
		if err != nil {
			return "", fmt.Errorf("failed to get home directory: %w", err)
		}
		if p == "~" {
			return home, nil
		}
		return filepath.Join(home, p[2:]), nil
	}
	return p, nil
}

// hasPrefix checks if a string starts with a prefix without using strings package
func hasPrefix(s, prefix string) bool {
	return len(s) >= len(prefix) && s[:len(prefix)] == prefix
}
