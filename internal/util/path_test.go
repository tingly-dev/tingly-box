package util

import (
	"runtime"
	"testing"
)

func TestGetUserPath(t *testing.T) {
	path, err := GetUserPath()
	if err != nil {
		t.Fatalf("GetUserPath failed: %v", err)
	}

	if path == "" {
		t.Fatal("GetUserPath returned empty path")
	}

	// Verify it's an absolute path
	if path[0] != '/' && (len(path) > 1 && path[1] != ':') {
		t.Errorf("GetUserPath returned non-absolute path: %s", path)
	}
}

func TestGetUserConfigPath(t *testing.T) {
	appName := "tingly-box-test"

	configPath, err := GetUserConfigPath(appName)
	if err != nil {
		t.Fatalf("GetUserConfigPath failed: %v", err)
	}

	if configPath == "" {
		t.Fatal("GetUserConfigPath returned empty path")
	}

	// Verify the app name is in the path
	if !contains(configPath, appName) {
		t.Errorf("App name '%s' not found in config path: %s", appName, configPath)
	}

	// Platform-specific checks
	switch runtime.GOOS {
	case "windows":
		if !contains(configPath, "AppData") && !contains(configPath, "APPDATA") {
			t.Errorf("Windows config path should contain AppData: %s", configPath)
		}
	case "darwin":
		if !contains(configPath, "Library") {
			t.Errorf("macOS config path should contain Library: %s", configPath)
		}
	default:
		if !contains(configPath, ".config") {
			t.Errorf("Linux/Unix config path should contain .config: %s", configPath)
		}
	}
}

func TestGetUserCachePath(t *testing.T) {
	appName := "tingly-box-test"

	cachePath, err := GetUserCachePath(appName)
	if err != nil {
		t.Fatalf("GetUserCachePath failed: %v", err)
	}

	if cachePath == "" {
		t.Fatal("GetUserCachePath returned empty path")
	}

	// Verify the app name is in the path
	if !contains(cachePath, appName) {
		t.Errorf("App name '%s' not found in cache path: %s", appName, cachePath)
	}

	// Platform-specific checks
	switch runtime.GOOS {
	case "windows":
		if !contains(cachePath, "AppData") && !contains(cachePath, "LOCALAPPDATA") {
			t.Errorf("Windows cache path should contain AppData: %s", cachePath)
		}
		if !contains(cachePath, "Cache") {
			t.Errorf("Windows cache path should contain Cache: %s", cachePath)
		}
	case "darwin":
		if !contains(cachePath, "Library") {
			t.Errorf("macOS cache path should contain Library: %s", cachePath)
		}
		if !contains(cachePath, "Caches") {
			t.Errorf("macOS cache path should contain Caches: %s", cachePath)
		}
	default:
		if !contains(cachePath, ".cache") {
			t.Errorf("Linux/Unix cache path should contain .cache: %s", cachePath)
		}
	}
}

func TestEnsureDir(t *testing.T) {
	// Test with a temporary directory
	testDir := "/tmp/tingly-box-test-dir"

	// First call should create the directory
	err := EnsureDir(testDir)
	if err != nil {
		t.Fatalf("EnsureDir failed: %v", err)
	}

	// Second call should not fail (directory already exists)
	err = EnsureDir(testDir)
	if err != nil {
		t.Fatalf("EnsureDir failed on existing directory: %v", err)
	}
}

// Helper function to check if a string contains a substring (case-insensitive)
func contains(s, substr string) bool {
	return len(s) >= len(substr) && (s == substr || len(s) > len(substr) &&
		(s[:len(substr)] == substr || s[len(s)-len(substr):] == substr ||
			func() bool {
				for i := 0; i <= len(s)-len(substr); i++ {
					if s[i:i+len(substr)] == substr {
						return true
					}
				}
				return false
			}()))
}
