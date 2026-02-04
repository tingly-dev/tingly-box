package server

import (
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"strings"
	"time"
)

const (
	npmRegistryAPI = "https://registry.npmjs.org/%s"
	npmmirrorAPI   = "https://registry.npmmirror.com/%s"
	tinglyBoxNPM   = "tingly-box"
)

// NpmPackage represents an npm registry package response
type NpmPackage struct {
	Name     string `json:"name"`
	DistTags struct {
		Latest string `json:"latest"`
	} `json:"dist-tags"`
}

// VersionChecker handles version-related operations
type VersionChecker struct {
	httpClient *http.Client
	cache      *versionCache
}

// versionCache caches version check results
type versionCache struct {
	latestVersion string
	releaseURL    string
	checkTime     time.Time
	ttl           time.Duration
}

// newVersionChecker creates a new VersionChecker
func newVersionChecker() *VersionChecker {
	return &VersionChecker{
		httpClient: &http.Client{
			Timeout: 10 * time.Second,
		},
		cache: &versionCache{
			ttl: 24 * time.Hour, // Cache for 24 hours
		},
	}
}

// CheckLatestVersion checks for the latest version with fallback:
// 1. npm registry (primary)
// 2. npmmirror (China mirror, fallback)
func (vc *VersionChecker) CheckLatestVersion() (version, releaseURL string, err error) {
	// Check cache first
	if vc.cache.latestVersion != "" && time.Since(vc.cache.checkTime) < vc.cache.ttl {
		return vc.cache.latestVersion, vc.cache.releaseURL, nil
	}

	// Try npm registry
	version, releaseURL, err = vc.checkFromNpm()
	if err == nil {
		// Update cache and return
		vc.cache.latestVersion = version
		vc.cache.releaseURL = releaseURL
		vc.cache.checkTime = time.Now()
		return version, releaseURL, nil
	}

	// npm failed, try npmmirror (China mirror)
	version, releaseURL, err = vc.checkFromNpmMirror()
	if err == nil {
		// Update cache and return
		vc.cache.latestVersion = version
		vc.cache.releaseURL = releaseURL
		vc.cache.checkTime = time.Now()
		return version, releaseURL, nil
	}

	// All failed, return the last error
	return "", "", err
}

// checkFromNpm fetches version from npm registry API
func (vc *VersionChecker) checkFromNpm() (version, releaseURL string, err error) {
	resp, err := vc.httpClient.Get(fmt.Sprintf(npmRegistryAPI, tinglyBoxNPM))
	if err != nil {
		return "", "", fmt.Errorf("failed to fetch npm package: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		return "", "", fmt.Errorf("npm API returned status %d", resp.StatusCode)
	}

	body, err := io.ReadAll(resp.Body)
	if err != nil {
		return "", "", fmt.Errorf("failed to read npm response: %w", err)
	}

	var pkg NpmPackage
	if err := json.Unmarshal(body, &pkg); err != nil {
		return "", "", fmt.Errorf("failed to parse npm package: %w", err)
	}

	version = pkg.DistTags.Latest
	if version == "" {
		return "", "", fmt.Errorf("npm package has no latest version")
	}

	// Use npm package page as release URL
	releaseURL = fmt.Sprintf("https://www.npmjs.com/package/%s", tinglyBoxNPM)

	return version, releaseURL, nil
}

// checkFromNpmMirror fetches version from npmmirror (China mirror)
func (vc *VersionChecker) checkFromNpmMirror() (version, releaseURL string, err error) {
	resp, err := vc.httpClient.Get(fmt.Sprintf(npmmirrorAPI, tinglyBoxNPM))
	if err != nil {
		return "", "", fmt.Errorf("failed to fetch npmmirror package: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		return "", "", fmt.Errorf("npmmirror API returned status %d", resp.StatusCode)
	}

	body, err := io.ReadAll(resp.Body)
	if err != nil {
		return "", "", fmt.Errorf("failed to read npmmirror response: %w", err)
	}

	var pkg NpmPackage
	if err := json.Unmarshal(body, &pkg); err != nil {
		return "", "", fmt.Errorf("failed to parse npmmirror package: %w", err)
	}

	version = pkg.DistTags.Latest
	if version == "" {
		return "", "", fmt.Errorf("npmmirror package has no latest version")
	}

	// Use npmmirror package page as release URL
	releaseURL = fmt.Sprintf("https://npmmirror.com/package/%s", tinglyBoxNPM)

	return version, releaseURL, nil
}

// compareVersions compares two version strings
// Returns: 1 if v1 > v2, -1 if v1 < v2, 0 if equal
func compareVersions(v1, v2 string) int {
	// Remove 'v' prefix if present
	v1 = strings.TrimPrefix(v1, "v")
	v2 = strings.TrimPrefix(v2, "v")

	// Handle custom format: vYYMMDD.HHMM or similar
	// Split by common separators
	parts1 := strings.FieldsFunc(v1, func(r rune) bool {
		return r == '.' || r == '-' || r == '_'
	})
	parts2 := strings.FieldsFunc(v2, func(r rune) bool {
		return r == '.' || r == '-' || r == '_'
	})

	// Compare each part
	maxLen := len(parts1)
	if len(parts2) > maxLen {
		maxLen = len(parts2)
	}

	for i := 0; i < maxLen; i++ {
		var p1, p2 string

		if i < len(parts1) {
			p1 = parts1[i]
		}
		if i < len(parts2) {
			p2 = parts2[i]
		}

		// Compare numerically if both are numeric
		if isNumeric(p1) && isNumeric(p2) {
			// Normalize to same length for string comparison
			if len(p1) < len(p2) {
				p1 = strings.Repeat("0", len(p2)-len(p1)) + p1
			} else if len(p2) < len(p1) {
				p2 = strings.Repeat("0", len(p1)-len(p2)) + p2
			}
			if p1 < p2 {
				return -1
			} else if p1 > p2 {
				return 1
			}
		} else {
			// String comparison
			if p1 < p2 {
				return -1
			} else if p1 > p2 {
				return 1
			}
		}
	}

	return 0
}

// isNumeric checks if a string is numeric
func isNumeric(s string) bool {
	if s == "" {
		return false
	}
	for _, c := range s {
		if c < '0' || c > '9' {
			return false
		}
	}
	return true
}
