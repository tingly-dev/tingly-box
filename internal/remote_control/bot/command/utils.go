package command

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"
)

// ExpandPath expands a path to include user home directory if needed
func ExpandPath(path string) (string, error) {
	if strings.HasPrefix(path, "~/") || path == "~" {
		home, err := os.UserHomeDir()
		if err != nil {
			return "", fmt.Errorf("failed to get home directory: %w", err)
		}
		if path == "~" {
			return home, nil
		}
		return filepath.Join(home, path[2:]), nil
	}
	return path, nil
}

// ValidateProjectPath validates that a project path exists and is accessible
func ValidateProjectPath(path string) error {
	// Check if path exists
	info, err := os.Stat(path)
	if err != nil {
		if os.IsNotExist(err) {
			// Try to create the directory
			if err := os.MkdirAll(path, 0755); err != nil {
				return fmt.Errorf("path does not exist and could not be created: %w", err)
			}
			return nil
		}
		return fmt.Errorf("failed to access path: %w", err)
	}

	// Check if it's a directory
	if !info.IsDir() {
		return fmt.Errorf("path is not a directory")
	}

	return nil
}

// ShortenPath returns a shortened version of a path for display
func ShortenPath(path string) string {
	home, err := os.UserHomeDir()
	if err == nil && strings.HasPrefix(path, home) {
		return "~" + path[len(home):]
	}

	// Show last 2 components
	parts := strings.Split(filepath.Clean(path), string(filepath.Separator))
	if len(parts) > 2 {
		return filepath.Join("...", parts[len(parts)-2], parts[len(parts)-1])
	}

	return path
}

// AddSessionInfoLastActivity adds LastActivity to SessionInfo
// This is a helper for converting from bot.SessionInfo to command.SessionInfo
func AddSessionInfoLastActivity(info *SessionInfo, lastActivity interface{}) *SessionInfo {
	// The LastActivity field is already in SessionInfo
	// This is just a placeholder for any additional processing needed
	return info
}
