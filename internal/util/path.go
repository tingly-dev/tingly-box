package util

import (
	"fmt"
	"os"
	"path/filepath"
	"runtime"
	"strings"
)

// ExpandConfigDir expands ~ to user home directory and returns absolute path
func ExpandConfigDir(path string) (string, error) {
	if path == "" {
		return path, nil
	}

	// Expand ~ to user home directory
	if strings.HasPrefix(path, "~/") || path == "~" {
		homeDir, err := GetUserPath()
		if err != nil {
			return "", fmt.Errorf("failed to get user home directory: %w", err)
		}
		if path == "~" {
			path = homeDir
		} else {
			path = filepath.Join(homeDir, path[2:])
		}
	}

	// Convert to absolute path
	return filepath.Abs(path)
}

// GetUserPath returns the user's home directory path across all platforms
// It provides a consistent way to get the user directory regardless of the operating system
func GetUserPath() (string, error) {
	// Use the standard library function which works across all platforms
	homeDir, err := os.UserHomeDir()
	if err != nil {
		return "", err
	}

	// Ensure the path is cleaned and absolute
	return filepath.Clean(homeDir), nil
}

// GetUserConfigPath returns the platform-specific user config directory
// This follows the conventions of each operating system:
// - Linux/Unix: ~/.config
// - macOS: ~/Library/Application Support
// - Windows: %APPDATA%
func GetUserConfigPath(appName string) (string, error) {
	homeDir, err := GetUserPath()
	if err != nil {
		return "", err
	}

	var configDir string
	switch runtime.GOOS {
	case "windows":
		// On Windows, use %APPDATA% directory
		appData := os.Getenv("APPDATA")
		if appData != "" {
			configDir = filepath.Join(appData, appName)
		} else {
			// Fallback to home directory
			configDir = filepath.Join(homeDir, "AppData", "Roaming", appName)
		}
	case "darwin":
		// On macOS, use ~/Library/Application Support
		configDir = filepath.Join(homeDir, "Library", "Application Support", appName)
	default:
		// On Linux/Unix, use ~/.config
		configDir = filepath.Join(homeDir, ".config", appName)
	}

	return configDir, nil
}

// GetUserCachePath returns the platform-specific user cache directory
// This follows the conventions of each operating system:
// - Linux/Unix: ~/.cache
// - macOS: ~/Library/Caches
// - Windows: %LOCALAPPDATA%
func GetUserCachePath(appName string) (string, error) {
	homeDir, err := GetUserPath()
	if err != nil {
		return "", err
	}

	var cacheDir string
	switch runtime.GOOS {
	case "windows":
		// On Windows, use %LOCALAPPDATA% directory
		localAppData := os.Getenv("LOCALAPPDATA")
		if localAppData != "" {
			cacheDir = filepath.Join(localAppData, appName, "Cache")
		} else {
			// Fallback to home directory
			cacheDir = filepath.Join(homeDir, "AppData", "Local", appName, "Cache")
		}
	case "darwin":
		// On macOS, use ~/Library/Caches
		cacheDir = filepath.Join(homeDir, "Library", "Caches", appName)
	default:
		// On Linux/Unix, use ~/.cache
		cacheDir = filepath.Join(homeDir, ".cache", appName)
	}

	return cacheDir, nil
}

// EnsureDir creates a directory if it doesn't exist
func EnsureDir(path string) error {
	return os.MkdirAll(path, 0755)
}
