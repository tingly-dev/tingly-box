package info

import (
	"net/http"
	"strings"
	"testing"
	"time"
)

// TestCompareVersions tests version comparison logic.
func TestCompareVersions(t *testing.T) {
	tests := []struct {
		name     string
		v1       string
		v2       string
		expected int
	}{
		{name: "Equal versions", v1: "0.1.0", v2: "0.1.0", expected: 0},
		{name: "v1 greater than v2", v1: "0.2.0", v2: "0.1.0", expected: 1},
		{name: "v1 less than v2", v1: "0.1.0", v2: "0.2.0", expected: -1},
		{name: "With v prefix", v1: "v0.2.0", v2: "v0.1.0", expected: 1},
		{name: "Date format versions", v1: "2024.12.07", v2: "2024.11.01", expected: 1},
		{name: "Timestamp format", v1: "260130.1200", v2: "260124.1430", expected: 1},
		{name: "Mixed format", v1: "0.2.0+build.456", v2: "0.1.0+build.123", expected: 1},
		{name: "Different length numbers", v1: "260130.1200", v2: "260130.900", expected: 1},
		{name: "Same major, different minor", v1: "1.2.0", v2: "1.1.9", expected: 1},
		{name: "Complex version string", v1: "v20241207.1430", v2: "v20241207.1200", expected: 1},
		{name: "Equal rc versions", v1: "0.260709.0-rc1", v2: "v0.260709.0-rc1", expected: 0},
		{name: "Release greater than rc", v1: "0.260709.0", v2: "0.260709.0-rc1", expected: 1},
		{name: "RC less than release", v1: "0.260709.0-rc1", v2: "0.260709.0", expected: -1},
		{name: "RC number ordering", v1: "0.260709.0-rc2", v2: "0.260709.0-rc1", expected: 1},
		{name: "Build metadata ignored", v1: "0.260709.0+build.2", v2: "0.260709.0+build.1", expected: 0},
	}

	t.Logf("Running version comparison tests with %d test cases", len(tests))

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := CompareVersions(tt.v1, tt.v2)
			t.Logf("Comparing: v1=%s, v2=%s, result=%d, expected=%d", tt.v1, tt.v2, result, tt.expected)
			if result != tt.expected {
				t.Errorf("CompareVersions(%q, %q) = %d, expected %d", tt.v1, tt.v2, result, tt.expected)
			}
		})
	}
}

// TestNew tests the Checker initialization.
func TestNew(t *testing.T) {
	t.Log("Testing Checker initialization")

	vc := New()

	if vc == nil {
		t.Fatal("New() returned nil")
	}
	t.Log("✓ Checker created successfully")

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

// TestCheckLatestVersion_Cache tests the caching mechanism.
func TestCheckLatestVersion_Cache(t *testing.T) {
	t.Log("Testing version check caching mechanism")

	vc := &Checker{
		httpClient: &http.Client{Timeout: 10 * time.Second},
		cache:      &cache{ttl: 2 * time.Hour},
	}
	t.Log("✓ Test Checker created")

	cachedVersion := "0.1.0"
	cachedURL := GithubReleases

	vc.cache.latestVersion = cachedVersion
	vc.cache.releaseURL = cachedURL
	vc.cache.checkTime = time.Now()

	t.Logf("Cached version set to: %s", cachedVersion)

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

// TestCheckLatestVersion_Integration tests the actual npm registry.
// Requires network access; skip with -short.
func TestCheckLatestVersion_Integration(t *testing.T) {
	if testing.Short() {
		t.Skip("Skipping integration test in short mode")
	}

	t.Log("=== Integration Test: npm Registry + GitHub Releases ===")

	vc := New()
	version, url, err := vc.CheckLatestVersion()

	t.Log("----------------------------------------")
	t.Logf("| Version: %s", version)
	t.Logf("| Release URL: %s", url)
	t.Logf("| Error: %v", err)
	t.Log("----------------------------------------")

	if err != nil {
		t.Logf("⚠️  Warning: Failed to check latest version (may be network issue): %v", err)
		return
	}

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

	if !strings.Contains(url, "github.com/tingly-dev/tingly-box/releases") {
		t.Errorf("❌ Expected release URL to point to GitHub releases, got: %s", url)
	} else {
		t.Logf("✓ Release URL correctly points to GitHub releases")
	}
}

// TestPrereleaseUpdateDetection tests that prerelease versions only update when
// the registry version is actually newer.
func TestPrereleaseUpdateDetection(t *testing.T) {
	tests := []struct {
		name           string
		currentVersion string
		latestVersion  string
		expectUpdate   bool
	}{
		{"Dev version behind release", "0.260604.2+dev", "0.260604.3", true},
		{"Alpha version behind release", "0.260604.2-alpha", "0.260604.2", true},
		{"Beta version behind release", "0.260604.2-beta", "0.260604.2", true},
		{"RC version behind release", "0.260604.2-rc1", "0.260604.2", true},
		{"Same release version", "0.260604.2", "0.260604.2", false},
		{"Same RC version", "v0.260709.0-rc1", "0.260709.0-rc1", false},
		{"Older version", "0.260600.0", "0.260604.2", true},
	}

	t.Log("Testing prerelease version update detection logic")

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			hasUpdate := CompareVersions(tt.latestVersion, tt.currentVersion) > 0

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
