// Package versioncheck provides version lookup against the npm registry
// (with npmmirror as a China-mirror fallback) and semver-style comparison.
// It is intentionally stateless beyond a short-lived in-memory cache so that
// it can be wired into any HTTP handler without server-level dependencies.
package info

import (
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"strconv"
	"strings"
	"time"
)

const (
	npmRegistryAPI = "https://registry.npmjs.org/%s"
	npmmirrorAPI   = "https://registry.npmmirror.com/%s"
	tinglyBoxNPM   = "tingly-box"
	GithubReleases = "https://github.com/tingly-dev/tingly-box/releases"
)

// NpmPackage represents an npm registry package response.
type NpmPackage struct {
	Name     string `json:"name"`
	DistTags struct {
		Latest string `json:"latest"`
	} `json:"dist-tags"`
}

// cache holds a single version-check result with a TTL.
type cache struct {
	latestVersion string
	releaseURL    string
	checkTime     time.Time
	ttl           time.Duration
}

// Checker handles version-related operations.
type Checker struct {
	httpClient *http.Client
	cache      *cache
}

// New creates a new Checker with default settings (10 s HTTP timeout, 2 h cache TTL).
func New() *Checker {
	return &Checker{
		httpClient: &http.Client{
			Timeout: 10 * time.Second,
		},
		cache: &cache{
			ttl: 2 * time.Hour,
		},
	}
}

// CheckLatestVersion returns the latest published version with a fallback strategy:
//  1. npm registry (primary)
//  2. npmmirror (China mirror)
//
// Results are cached for 2 hours to avoid hammering the registry.
func (c *Checker) CheckLatestVersion() (version, releaseURL string, err error) {
	if c.cache.latestVersion != "" && time.Since(c.cache.checkTime) < c.cache.ttl {
		return c.cache.latestVersion, c.cache.releaseURL, nil
	}

	version, releaseURL, err = c.checkFromNpm()
	if err == nil {
		c.updateCache(version, releaseURL)
		return version, releaseURL, nil
	}

	version, releaseURL, err = c.checkFromNpmMirror()
	if err == nil {
		c.updateCache(version, releaseURL)
		return version, releaseURL, nil
	}

	return "", "", err
}

func (c *Checker) updateCache(version, releaseURL string) {
	c.cache.latestVersion = version
	c.cache.releaseURL = releaseURL
	c.cache.checkTime = time.Now()
}

func (c *Checker) checkFromNpm() (version, releaseURL string, err error) {
	return c.fetchFromRegistry(fmt.Sprintf(npmRegistryAPI, tinglyBoxNPM), "npm")
}

func (c *Checker) checkFromNpmMirror() (version, releaseURL string, err error) {
	return c.fetchFromRegistry(fmt.Sprintf(npmmirrorAPI, tinglyBoxNPM), "npmmirror")
}

func (c *Checker) fetchFromRegistry(url, registryName string) (version, releaseURL string, err error) {
	resp, err := c.httpClient.Get(url)
	if err != nil {
		return "", "", fmt.Errorf("failed to fetch %s package: %w", registryName, err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		return "", "", fmt.Errorf("%s API returned status %d", registryName, resp.StatusCode)
	}

	body, err := io.ReadAll(resp.Body)
	if err != nil {
		return "", "", fmt.Errorf("failed to read %s response: %w", registryName, err)
	}

	var pkg NpmPackage
	if err := json.Unmarshal(body, &pkg); err != nil {
		return "", "", fmt.Errorf("failed to parse %s package: %w", registryName, err)
	}

	if pkg.DistTags.Latest == "" {
		return "", "", fmt.Errorf("%s package has no latest version", registryName)
	}

	return pkg.DistTags.Latest, GithubReleases, nil
}

// CompareVersions compares two version strings.
// Returns 1 if v1 > v2, -1 if v1 < v2, 0 if equal.
// Handles semver prerelease precedence, date-based (YYMMDD.HHMM), and mixed formats.
func CompareVersions(v1, v2 string) int {
	core1, prerelease1 := splitVersion(v1)
	core2, prerelease2 := splitVersion(v2)

	if result := compareVersionParts(core1, core2); result != 0 {
		return result
	}

	return comparePrerelease(prerelease1, prerelease2)
}

func splitVersion(v string) (core []string, prerelease []string) {
	v = strings.TrimPrefix(v, "v")
	if buildIndex := strings.IndexByte(v, '+'); buildIndex >= 0 {
		v = v[:buildIndex]
	}

	coreText := v
	prereleaseText := ""
	if prereleaseIndex := strings.IndexAny(v, "-_"); prereleaseIndex >= 0 {
		coreText = v[:prereleaseIndex]
		prereleaseText = v[prereleaseIndex+1:]
	}

	core = strings.FieldsFunc(coreText, func(r rune) bool { return r == '.' })
	if prereleaseText != "" {
		prerelease = strings.FieldsFunc(prereleaseText, func(r rune) bool { return r == '.' || r == '-' || r == '_' })
	}
	return core, prerelease
}

func compareVersionParts(parts1, parts2 []string) int {
	maxLen := len(parts1)
	if len(parts2) > maxLen {
		maxLen = len(parts2)
	}

	for i := 0; i < maxLen; i++ {
		p1 := "0"
		p2 := "0"
		if i < len(parts1) {
			p1 = parts1[i]
		}
		if i < len(parts2) {
			p2 = parts2[i]
		}

		if result := compareVersionPart(p1, p2); result != 0 {
			return result
		}
	}

	return 0
}

func compareVersionPart(p1, p2 string) int {
	// Numeric identifiers compare as integers (avoid string allocation from zero-padding).
	if n1, err := strconv.Atoi(p1); err == nil {
		if n2, err := strconv.Atoi(p2); err == nil {
			if n1 < n2 {
				return -1
			} else if n1 > n2 {
				return 1
			}
			return 0
		}
	}
	// Mixed or non-numeric — ASCII ordering happens to match semver rules
	// (numeric identifiers have lower precedence than non-numeric ones).
	if p1 < p2 {
		return -1
	} else if p1 > p2 {
		return 1
	}
	return 0
}

func comparePrerelease(parts1, parts2 []string) int {
	if len(parts1) == 0 && len(parts2) == 0 {
		return 0
	}
	if len(parts1) == 0 {
		return 1
	}
	if len(parts2) == 0 {
		return -1
	}

	maxLen := len(parts1)
	if len(parts2) > maxLen {
		maxLen = len(parts2)
	}

	for i := 0; i < maxLen; i++ {
		if i >= len(parts1) {
			return -1
		}
		if i >= len(parts2) {
			return 1
		}
		if result := compareVersionPart(parts1[i], parts2[i]); result != 0 {
			return result
		}
	}

	return 0
}
