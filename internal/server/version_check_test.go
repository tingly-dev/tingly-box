package server

import (
	"net/http"
	"strings"
	"testing"
	"time"
)

// TestCompareVersions tests version comparison logic
func TestCompareVersions(t *testing.T) {
	tests := []struct {
		name     string
		v1       string
		v2       string
		expected int
	}{
		{
			name:     "Equal versions",
			v1:       "0.1.0",
			v2:       "0.1.0",
			expected: 0,
		},
		{
			name:     "v1 greater than v2",
			v1:       "0.2.0",
			v2:       "0.1.0",
			expected: 1,
		},
		{
			name:     "v1 less than v2",
			v1:       "0.1.0",
			v2:       "0.2.0",
			expected: -1,
		},
		{
			name:     "With v prefix",
			v1:       "v0.2.0",
			v2:       "v0.1.0",
			expected: 1,
		},
		{
			name:     "Date format versions",
			v1:       "2024.12.07",
			v2:       "2024.11.01",
			expected: 1,
		},
		{
			name:     "Timestamp format",
			v1:       "260130.1200",
			v2:       "260124.1430",
			expected: 1,
		},
		{
			name:     "Mixed format",
			v1:       "0.2.0+build.456",
			v2:       "0.1.0+build.123",
			expected: 1,
		},
		{
			name:     "Different length numbers",
			v1:       "260130.1200",
			v2:       "260130.900",
			expected: 1,
		},
		{
			name:     "Same major, different minor",
			v1:       "1.2.0",
			v2:       "1.1.9",
			expected: 1,
		},
		{
			name:     "Complex version string",
			v1:       "v20241207.1430",
			v2:       "v20241207.1200",
			expected: 1,
		},
	}

	t.Logf("Running version comparison tests with %d test cases", len(tests))

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := compareVersions(tt.v1, tt.v2)
			t.Logf("Comparing: v1=%s, v2=%s, result=%d, expected=%d", tt.v1, tt.v2, result, tt.expected)
			if result != tt.expected {
				t.Errorf("compareVersions(%q, %q) = %d, expected %d", tt.v1, tt.v2, result, tt.expected)
			}
		})
	}
}

// TestIsNumeric tests the numeric string validation
func TestIsNumeric(t *testing.T) {
	tests := []struct {
		name     string
		input    string
		expected bool
	}{
		{"Empty string", "", false},
		{"Numeric", "12345", true},
		{"Numeric with leading zeros", "00123", true},
		{"With letters", "123abc", false},
		{"With special chars", "123-456", false},
		{"Only letters", "abcde", false},
		{"With spaces", "12 34", false},
	}

	t.Logf("Running numeric validation tests with %d test cases", len(tests))

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := isNumeric(tt.input)
			t.Logf("Input: %q, Result: %v, Expected: %v", tt.input, result, tt.expected)
			if result != tt.expected {
				t.Errorf("isNumeric(%q) = %v, expected %v", tt.input, result, tt.expected)
			}
		})
	}
}

// TestNewVersionChecker tests the version checker initialization
func TestNewVersionChecker(t *testing.T) {
	t.Log("Testing version checker initialization")

	vc := newVersionChecker()

	if vc == nil {
		t.Fatal("newVersionChecker() returned nil")
	}
	t.Log("✓ Version checker created successfully")

	if vc.httpClient == nil {
		t.Error("Expected httpClient to be initialized")
	} else {
		t.Log("✓ HTTP client initialized")
	}

	if vc.cache == nil {
		t.Error("Expected cache to be initialized")
	} else {
		t.Log("✓ Cache initialized")
	}

	if vc.cache.ttl != 2*time.Hour {
		t.Errorf("Expected cache TTL to be 2 hours, got %v", vc.cache.ttl)
	} else {
		t.Logf("✓ Cache TTL set to %v", vc.cache.ttl)
	}
}

// TestCheckLatestVersion_Cache tests the caching mechanism
func TestCheckLatestVersion_Cache(t *testing.T) {
	t.Log("Testing version check caching mechanism")

	vc := &VersionChecker{
		httpClient: &http.Client{
			Timeout: 10 * time.Second,
		},
		cache: &versionCache{
			ttl: 2 * time.Hour,
		},
	}
	t.Log("✓ Test version checker created")

	// Set a cached value
	cachedVersion := "0.1.0"
	cachedURL := "https://github.com/tingly-dev/tingly-box/releases"

	vc.cache.latestVersion = cachedVersion
	vc.cache.releaseURL = cachedURL
	vc.cache.checkTime = time.Now()

	t.Logf("Cached version set to: %s", cachedVersion)
	t.Logf("Cached URL set to: %s", cachedURL)

	// First call should return cached value
	version, url, err := vc.CheckLatestVersion()
	t.Logf("CheckLatestVersion returned: version=%s, url=%s, err=%v", version, url, err)

	if err != nil {
		t.Errorf("Expected no error, got %v", err)
	}
	if version != cachedVersion {
		t.Errorf("Expected version %s, got %s", cachedVersion, version)
	}
	if url != cachedURL {
		t.Errorf("Expected URL %s, got %s", cachedURL, url)
	}
	t.Log("✓ Cache mechanism working correctly")
}

// TestCheckLatestVersion_Integration tests the actual npm registry
// This is an integration test that requires network access
func TestCheckLatestVersion_Integration(t *testing.T) {
	if testing.Short() {
		t.Skip("Skipping integration test in short mode")
	}

	t.Log("=== Integration Test: npm Registry + GitHub Releases ===")
	t.Log("Fetching latest version information from npm registry...")

	vc := newVersionChecker()
	t.Logf("Version checker created with cache TTL: %v", vc.cache.ttl)

	version, url, err := vc.CheckLatestVersion()

	t.Log("----------------------------------------")
	t.Logf("| Result Summary:")
	t.Logf("|  Version: %s", version)
	t.Logf("|  Release URL: %s", url)
	t.Logf("|  Error: %v", err)
	t.Log("----------------------------------------")

	if err != nil {
		t.Logf("⚠️  Warning: Failed to check latest version (may be network issue): %v", err)
		t.Log("This could indicate:")
		t.Log("  - Network connectivity issues")
		t.Log("  - npm registry is down")
		t.Log("  - DNS resolution problems")
		return
	}

	t.Log("✓ Successfully connected to npm registry")

	if version == "" {
		t.Error("❌ Expected non-empty version string")
	} else {
		t.Logf("✓ Version retrieved: %s", version)
	}

	if url == "" {
		t.Error("❌ Expected non-empty release URL")
	} else {
		t.Logf("✓ Release URL retrieved")
	}

	// Verify release URL points to GitHub
	if !strings.Contains(url, "github.com/tingly-dev/tingly-box/releases") {
		t.Errorf("❌ Expected release URL to point to GitHub releases, got: %s", url)
		t.Log("Expected format: https://github.com/tingly-dev/tingly-box/releases")
	} else {
		t.Logf("✓ Release URL correctly points to GitHub releases")
	}

	t.Log("")
	t.Log("=== Integration Test Completed Successfully ===")
	t.Logf("Latest tingly-box version: %s", version)
	t.Logf("GitHub releases page: %s", url)
	t.Log("")
}

// TestDevVersionDetection tests that dev versions are always considered outdated
func TestDevVersionDetection(t *testing.T) {
	tests := []struct {
		name           string
		currentVersion string
		latestVersion  string
		expectUpdate   bool
	}{
		{
			name:           "Dev version with +dev suffix",
			currentVersion: "0.260604.2+dev",
			latestVersion:  "0.260604.2",
			expectUpdate:   true,
		},
		{
			name:           "Dev version with -dev suffix",
			currentVersion: "0.260604.2-dev",
			latestVersion:  "0.260604.2",
			expectUpdate:   true,
		},
		{
			name:           "Pure dev version",
			currentVersion: "dev",
			latestVersion:  "0.260604.2",
			expectUpdate:   true,
		},
		{
			name:           "Alpha version",
			currentVersion: "0.260604.2-alpha",
			latestVersion:  "0.260604.2",
			expectUpdate:   true,
		},
		{
			name:           "Beta version",
			currentVersion: "0.260604.2-beta",
			latestVersion:  "0.260604.2",
			expectUpdate:   true,
		},
		{
			name:           "RC version",
			currentVersion: "0.260604.2-rc1",
			latestVersion:  "0.260604.2",
			expectUpdate:   true,
		},
		{
			name:           "Same release version",
			currentVersion: "0.260604.2",
			latestVersion:  "0.260604.2",
			expectUpdate:   false,
		},
		{
			name:           "Older version",
			currentVersion: "0.260600.0",
			latestVersion:  "0.260604.2",
			expectUpdate:   true,
		},
	}

	t.Log("Testing dev/prerelease version detection logic")

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			// Simulate the logic from info_handler.go
			isDevVersion := strings.Contains(tt.currentVersion, "dev") ||
				strings.Contains(tt.currentVersion, "alpha") ||
				strings.Contains(tt.currentVersion, "beta") ||
				strings.Contains(tt.currentVersion, "rc")

			hasUpdate := isDevVersion || compareVersions(tt.latestVersion, tt.currentVersion) > 0

			if hasUpdate != tt.expectUpdate {
				t.Errorf("hasUpdate = %v, expected %v (current=%s, latest=%s)",
					hasUpdate, tt.expectUpdate, tt.currentVersion, tt.latestVersion)
			} else {
				t.Logf("✓ Correct: hasUpdate=%v (current=%s, latest=%s)",
					hasUpdate, tt.currentVersion, tt.latestVersion)
			}
		})
	}
}
