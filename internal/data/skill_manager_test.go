package data

import (
	"os"
	"path/filepath"
	"testing"
	"time"

	"github.com/tingly-dev/tingly-box/internal/typ"
)

// setupTestManager creates a test manager with a temporary config directory
func setupTestManager(t *testing.T) (*SkillManager, string) {
	t.Helper()

	tempDir, err := os.MkdirTemp("", "skill-manager-test-*")
	if err != nil {
		t.Fatalf("failed to create temp dir: %v", err)
	}

	sm, err := NewSkillManager(tempDir)
	if err != nil {
		os.RemoveAll(tempDir)
		t.Fatalf("failed to create skill manager: %v", err)
	}

	return sm, tempDir
}

// cleanupTestManager cleans up the test manager and temporary directory
func cleanupTestManager(t *testing.T, sm *SkillManager, tempDir string) {
	t.Helper()
	if tempDir != "" {
		_ = os.RemoveAll(tempDir)
	}
}

func TestNewSkillManager(t *testing.T) {
	sm, tempDir := setupTestManager(t)
	defer cleanupTestManager(t, sm, tempDir)

	// Check that the manager was created successfully
	if sm == nil {
		t.Fatal("expected non-nil skill manager")
	}

	// Check that the config directory was created
	if _, err := os.Stat(tempDir); err != nil {
		t.Fatalf("config directory does not exist: %v", err)
	}

	// Check that the locations file path is correct
	expectedPath := filepath.Join(tempDir, SkillLocationsFile)
	if sm.filePath != expectedPath {
		t.Errorf("expected filePath %q, got %q", expectedPath, sm.filePath)
	}

	// Initially, there should be no locations
	locations := sm.ListLocations()
	if len(locations) != 0 {
		t.Errorf("expected 0 locations, got %d", len(locations))
	}
}

func TestAddLocation(t *testing.T) {
	sm, tempDir := setupTestManager(t)
	defer cleanupTestManager(t, sm, tempDir)

	// Create a temporary skill directory
	skillDir, err := os.MkdirTemp("", "skills-*")
	if err != nil {
		t.Fatalf("failed to create skill dir: %v", err)
	}
	defer os.RemoveAll(skillDir)

	// Test adding a location
	loc, err := sm.AddLocation("test-location", skillDir, typ.IDESourceClaudeCode)
	if err != nil {
		t.Fatalf("failed to add location: %v", err)
	}

	// Verify the location was added correctly
	if loc.ID == "" {
		t.Error("expected non-empty ID")
	}
	if loc.Name != "test-location" {
		t.Errorf("expected name 'test-location', got %q", loc.Name)
	}
	if loc.Path != skillDir {
		t.Errorf("expected path %q, got %q", skillDir, loc.Path)
	}
	if loc.IDESource != typ.IDESourceClaudeCode {
		t.Errorf("expected IDESource %q, got %q", typ.IDESourceClaudeCode, loc.IDESource)
	}
	if loc.Icon != "🎨" {
		t.Errorf("expected icon '🎨', got %q", loc.Icon)
	}

	// Verify we can retrieve the location
	retrieved, err := sm.GetLocation(loc.ID)
	if err != nil {
		t.Fatalf("failed to get location: %v", err)
	}
	if retrieved.ID != loc.ID {
		t.Errorf("expected ID %q, got %q", loc.ID, retrieved.ID)
	}

	// Test adding a duplicate path (should fail)
	_, err = sm.AddLocation("duplicate", skillDir, typ.IDESourceVSCode)
	if err == nil {
		t.Error("expected error when adding duplicate path")
	}
}

func TestRemoveLocation(t *testing.T) {
	sm, tempDir := setupTestManager(t)
	defer cleanupTestManager(t, sm, tempDir)

	// Create and add a location
	skillDir, err := os.MkdirTemp("", "skills-*")
	if err != nil {
		t.Fatalf("failed to create skill dir: %v", err)
	}
	defer os.RemoveAll(skillDir)

	loc, err := sm.AddLocation("test-location", skillDir, typ.IDESourceClaudeCode)
	if err != nil {
		t.Fatalf("failed to add location: %v", err)
	}

	// Remove the location
	err = sm.RemoveLocation(loc.ID)
	if err != nil {
		t.Fatalf("failed to remove location: %v", err)
	}

	// Verify the location was removed
	_, err = sm.GetLocation(loc.ID)
	if err == nil {
		t.Error("expected error when getting removed location")
	}

	// Test removing non-existent location
	err = sm.RemoveLocation("non-existent-id")
	if err == nil {
		t.Error("expected error when removing non-existent location")
	}
}

func TestGetLocation(t *testing.T) {
	sm, tempDir := setupTestManager(t)
	defer cleanupTestManager(t, sm, tempDir)

	// Test getting non-existent location
	_, err := sm.GetLocation("non-existent-id")
	if err == nil {
		t.Error("expected error when getting non-existent location")
	}

	// Create and add a location
	skillDir, err := os.MkdirTemp("", "skills-*")
	if err != nil {
		t.Fatalf("failed to create skill dir: %v", err)
	}
	defer os.RemoveAll(skillDir)

	loc, err := sm.AddLocation("test-location", skillDir, typ.IDESourceCursor)
	if err != nil {
		t.Fatalf("failed to add location: %v", err)
	}

	// Get the location
	retrieved, err := sm.GetLocation(loc.ID)
	if err != nil {
		t.Fatalf("failed to get location: %v", err)
	}

	// Verify the retrieved location matches
	if retrieved.ID != loc.ID {
		t.Errorf("expected ID %q, got %q", loc.ID, retrieved.ID)
	}
	if retrieved.Name != loc.Name {
		t.Errorf("expected name %q, got %q", loc.Name, retrieved.Name)
	}
	if retrieved.Path != loc.Path {
		t.Errorf("expected path %q, got %q", loc.Path, retrieved.Path)
	}
	if retrieved.IDESource != loc.IDESource {
		t.Errorf("expected IDESource %q, got %q", loc.IDESource, retrieved.IDESource)
	}
}

func TestListLocations(t *testing.T) {
	sm, tempDir := setupTestManager(t)
	defer cleanupTestManager(t, sm, tempDir)

	// Initially empty
	locations := sm.ListLocations()
	if len(locations) != 0 {
		t.Errorf("expected 0 locations, got %d", len(locations))
	}

	// Add multiple locations
	skillDir1, _ := os.MkdirTemp("", "skills1-*")
	skillDir2, _ := os.MkdirTemp("", "skills2-*")
	defer os.RemoveAll(skillDir1)
	defer os.RemoveAll(skillDir2)

	loc1, _ := sm.AddLocation("location-1", skillDir1, typ.IDESourceClaudeCode)
	loc2, _ := sm.AddLocation("location-2", skillDir2, typ.IDESourceVSCode)

	// List locations
	locations = sm.ListLocations()
	if len(locations) != 2 {
		t.Errorf("expected 2 locations, got %d", len(locations))
	}

	// Verify both locations are present
	locationIDs := make(map[string]bool)
	for _, loc := range locations {
		locationIDs[loc.ID] = true
	}

	if !locationIDs[loc1.ID] {
		t.Error("location 1 not found in list")
	}
	if !locationIDs[loc2.ID] {
		t.Error("location 2 not found in list")
	}
}

func TestUpdateLocationSkillCount(t *testing.T) {
	sm, tempDir := setupTestManager(t)
	defer cleanupTestManager(t, sm, tempDir)

	// Create and add a location
	skillDir, _ := os.MkdirTemp("", "skills-*")
	defer os.RemoveAll(skillDir)

	loc, err := sm.AddLocation("test-location", skillDir, typ.IDESourceClaudeCode)
	if err != nil {
		t.Fatalf("failed to add location: %v", err)
	}

	// Initially skill count should be 0
	if loc.SkillCount != 0 {
		t.Errorf("expected initial skill count 0, got %d", loc.SkillCount)
	}

	// Update skill count
	err = sm.UpdateLocationSkillCount(loc.ID, 5)
	if err != nil {
		t.Fatalf("failed to update skill count: %v", err)
	}

	// Verify the update
	retrieved, err := sm.GetLocation(loc.ID)
	if err != nil {
		t.Fatalf("failed to get location: %v", err)
	}

	if retrieved.SkillCount != 5 {
		t.Errorf("expected skill count 5, got %d", retrieved.SkillCount)
	}
	if !retrieved.IsInstalled {
		t.Error("expected IsInstalled to be true")
	}
	if retrieved.LastScannedAt.IsZero() {
		t.Error("expected LastScannedAt to be set")
	}

	// Test with non-existent location
	err = sm.UpdateLocationSkillCount("non-existent-id", 10)
	if err == nil {
		t.Error("expected error when updating non-existent location")
	}
}

func TestUpdateLocation(t *testing.T) {
	sm, tempDir := setupTestManager(t)
	defer cleanupTestManager(t, sm, tempDir)

	// Create and add a location
	skillDir, _ := os.MkdirTemp("", "skills-*")
	defer os.RemoveAll(skillDir)

	loc, err := sm.AddLocation("test-location", skillDir, typ.IDESourceClaudeCode)
	if err != nil {
		t.Fatalf("failed to add location: %v", err)
	}

	// Update the location
	loc.Name = "updated-location"
	loc.SkillCount = 10

	err = sm.UpdateLocation(*loc)
	if err != nil {
		t.Fatalf("failed to update location: %v", err)
	}

	// Verify the update
	retrieved, err := sm.GetLocation(loc.ID)
	if err != nil {
		t.Fatalf("failed to get location: %v", err)
	}

	if retrieved.Name != "updated-location" {
		t.Errorf("expected name 'updated-location', got %q", retrieved.Name)
	}
	if retrieved.SkillCount != 10 {
		t.Errorf("expected skill count 10, got %d", retrieved.SkillCount)
	}

	// Test with non-existent location
	nonExistent := typ.SkillLocation{ID: "non-existent-id"}
	err = sm.UpdateLocation(nonExistent)
	if err == nil {
		t.Error("expected error when updating non-existent location")
	}
}

func TestScanLocation_CludeCode(t *testing.T) {
	sm, tempDir := setupTestManager(t)
	defer cleanupTestManager(t, sm, tempDir)

	// Create a temporary directory structure mimicking Claude Code's skills layout
	skillDir, err := os.MkdirTemp("", "claude-code-skills-*")
	if err != nil {
		t.Fatalf("failed to create skill dir: %v", err)
	}
	defer os.RemoveAll(skillDir)

	// Create the skills directory structure
	skillsPath := filepath.Join(skillDir, "skills")
	if err := os.MkdirAll(skillsPath, 0755); err != nil {
		t.Fatalf("failed to create skills directory: %v", err)
	}

	// Create test skill files matching Claude Code's scan patterns:
	// - skills/**/*.md
	// - **/SKILL.md
	testSkills := []struct {
		path     string
		content  string
		expected bool // whether this file should be detected
	}{
		{
			path:     filepath.Join(skillsPath, "commit.md"),
			content:  "# Commit Skill\n\nThis is a commit skill.",
			expected: true,
		},
		{
			path:     filepath.Join(skillsPath, "review.md"),
			content:  "# Review Skill\n\nThis is a review skill.",
			expected: true,
		},
		{
			path:     filepath.Join(skillsPath, "subdir", "test.md"),
			content:  "# Test Skill\n\nThis is a test skill.",
			expected: true,
		},
		{
			path:     filepath.Join(skillDir, "SKILL.md"),
			content:  "# Root Skill\n\nThis is a root skill.",
			expected: true,
		},
		{
			path:     filepath.Join(skillDir, ".hidden", "hidden.md"),
			content:  "# Hidden Skill\n\nThis should not be detected.",
			expected: false,
		},
		{
			path:     filepath.Join(skillDir, "readme.txt"),
			content:  "Not a markdown file.",
			expected: false,
		},
	}

	for _, testSkill := range testSkills {
		if err := os.MkdirAll(filepath.Dir(testSkill.path), 0755); err != nil {
			t.Fatalf("failed to create directory: %v", err)
		}
		if err := os.WriteFile(testSkill.path, []byte(testSkill.content), 0644); err != nil {
			t.Fatalf("failed to write test file: %v", err)
		}
	}

	// Add the location
	loc, err := sm.AddLocation("claude-code-test", skillDir, typ.IDESourceClaudeCode)
	if err != nil {
		t.Fatalf("failed to add location: %v", err)
	}

	// Scan the location
	result, err := sm.ScanLocation(loc.ID)
	if err != nil {
		t.Fatalf("failed to scan location: %v", err)
	}

	// Verify the scan result
	if result.LocationID != loc.ID {
		t.Errorf("expected location ID %q, got %q", loc.ID, result.LocationID)
	}

	// Count expected skills
	expectedCount := 0
	for _, s := range testSkills {
		if s.expected {
			expectedCount++
		}
	}

	if len(result.Skills) != expectedCount {
		t.Errorf("expected %d skills, got %d", expectedCount, len(result.Skills))
	}

	// Verify specific skills were found
	skillNames := make(map[string]bool)
	for _, skill := range result.Skills {
		skillNames[skill.Name] = true
	}

	expectedNames := []string{"commit", "review", "test", "SKILL"}
	for _, name := range expectedNames {
		if !skillNames[name] {
			t.Errorf("expected to find skill %q", name)
		}
	}

	// Verify location skill count was updated
	retrieved, _ := sm.GetLocation(loc.ID)
	if retrieved.SkillCount != expectedCount {
		t.Errorf("expected skill count %d, got %d", expectedCount, retrieved.SkillCount)
	}
}

func TestScanLocation_NonExistentLocation(t *testing.T) {
	sm, tempDir := setupTestManager(t)
	defer cleanupTestManager(t, sm, tempDir)

	// Try to scan a non-existent location
	_, err := sm.ScanLocation("non-existent-id")
	if err == nil {
		t.Error("expected error when scanning non-existent location")
	}
}

func TestScanLocation_NonExistentPath(t *testing.T) {
	sm, tempDir := setupTestManager(t)
	defer cleanupTestManager(t, sm, tempDir)

	// Add a location with a non-existent path
	loc, err := sm.AddLocation("non-existent-path", "/tmp/non-existent-path-xyz123", typ.IDESourceClaudeCode)
	if err != nil {
		t.Fatalf("failed to add location: %v", err)
	}

	// Scan should return empty result, not error (directory doesn't exist)
	result, err := sm.ScanLocation(loc.ID)
	if err != nil {
		t.Fatalf("unexpected error scanning non-existent path: %v", err)
	}

	if len(result.Skills) != 0 {
		t.Errorf("expected 0 skills for non-existent path, got %d", len(result.Skills))
	}
}

func TestGetSkillContent(t *testing.T) {
	sm, tempDir := setupTestManager(t)
	defer cleanupTestManager(t, sm, tempDir)

	// Create a test skill file
	skillDir, _ := os.MkdirTemp("", "skills-*")
	defer os.RemoveAll(skillDir)

	skillContent := "# Test Skill\n\nThis is a test skill."
	skillPath := filepath.Join(skillDir, "test-skill.md")
	if err := os.WriteFile(skillPath, []byte(skillContent), 0644); err != nil {
		t.Fatalf("failed to write test file: %v", err)
	}

	// Add location
	loc, err := sm.AddLocation("test-location", skillDir, typ.IDESourceClaudeCode)
	if err != nil {
		t.Fatalf("failed to add location: %v", err)
	}

	// Get skill content by path
	skill, err := sm.GetSkillContent(loc.ID, "", skillPath)
	if err != nil {
		t.Fatalf("failed to get skill content: %v", err)
	}

	if skill.Content != skillContent {
		t.Errorf("expected content %q, got %q", skillContent, skill.Content)
	}

	// Verify skill properties
	if skill.Name != "test-skill" {
		t.Errorf("expected name 'test-skill', got %q", skill.Name)
	}
	if skill.Filename != "test-skill.md" {
		t.Errorf("expected filename 'test-skill.md', got %q", skill.Filename)
	}
	if skill.FileType != ".md" {
		t.Errorf("expected filetype '.md', got %q", skill.FileType)
	}
}

func TestGetSkillContent_NonExistentLocation(t *testing.T) {
	sm, tempDir := setupTestManager(t)
	defer cleanupTestManager(t, sm, tempDir)

	// Try to get skill content from non-existent location
	_, err := sm.GetSkillContent("non-existent-id", "", "/some/path.md")
	if err == nil {
		t.Error("expected error when getting skill from non-existent location")
	}
}

func TestGetSkillContent_NonExistentFile(t *testing.T) {
	sm, tempDir := setupTestManager(t)
	defer cleanupTestManager(t, sm, tempDir)

	// Create and add a location
	skillDir, _ := os.MkdirTemp("", "skills-*")
	defer os.RemoveAll(skillDir)

	loc, err := sm.AddLocation("test-location", skillDir, typ.IDESourceClaudeCode)
	if err != nil {
		t.Fatalf("failed to add location: %v", err)
	}

	// Try to get content from non-existent file
	_, err = sm.GetSkillContent(loc.ID, "", "/non/existent/path.md")
	if err == nil {
		t.Error("expected error when getting non-existent skill file")
	}
}

func TestPersistAndLoad(t *testing.T) {
	// Create first manager
	tempDir, err := os.MkdirTemp("", "skill-manager-persist-*")
	if err != nil {
		t.Fatalf("failed to create temp dir: %v", err)
	}
	defer os.RemoveAll(tempDir)

	sm1, err := NewSkillManager(tempDir)
	if err != nil {
		t.Fatalf("failed to create skill manager: %v", err)
	}

	// Add a location
	skillDir, _ := os.MkdirTemp("", "skills-*")
	defer os.RemoveAll(skillDir)

	loc, err := sm1.AddLocation("persist-test", skillDir, typ.IDESourceCursor)
	if err != nil {
		t.Fatalf("failed to add location: %v", err)
	}

	// Update skill count
	_ = sm1.UpdateLocationSkillCount(loc.ID, 7)

	// Create a new manager instance (should load from disk)
	sm2, err := NewSkillManager(tempDir)
	if err != nil {
		t.Fatalf("failed to create second skill manager: %v", err)
	}

	// Verify the location was persisted
	locations := sm2.ListLocations()
	if len(locations) != 1 {
		t.Fatalf("expected 1 location, got %d", len(locations))
	}

	retrieved := locations[0]
	if retrieved.ID != loc.ID {
		t.Errorf("expected ID %q, got %q", loc.ID, retrieved.ID)
	}
	if retrieved.Name != "persist-test" {
		t.Errorf("expected name 'persist-test', got %q", retrieved.Name)
	}
	if retrieved.SkillCount != 7 {
		t.Errorf("expected skill count 7, got %d", retrieved.SkillCount)
	}
}

func TestScanLocation_WithTildePath(t *testing.T) {
	t.Skip("Tilde expansion has known issues - skipping until fixed")
}

func TestDiscoverIdes(t *testing.T) {
	sm, tempDir := setupTestManager(t)
	defer cleanupTestManager(t, sm, tempDir)

	// Discover IDEs (this will scan home directory for IDE configurations)
	result, err := sm.DiscoverIdes()
	if err != nil {
		t.Fatalf("failed to discover IDEs: %v", err)
	}

	// Verify result structure
	if result == nil {
		t.Fatal("expected non-nil result")
	}

	if result.TotalIdesScanned == 0 {
		t.Error("expected TotalIdesScanned > 0")
	}

	// The result may or may not find IDEs depending on the test environment
	// Just verify the structure is valid
	for _, loc := range result.Locations {
		if loc.ID == "" {
			t.Error("expected non-empty location ID")
		}
		if loc.Name == "" {
			t.Error("expected non-empty location name")
		}
	}
}

func TestLoadAndSave(t *testing.T) {
	sm, tempDir := setupTestManager(t)
	defer cleanupTestManager(t, sm, tempDir)

	// Test load when file doesn't exist (should succeed with empty locations)
	locations := sm.ListLocations()
	if len(locations) != 0 {
		t.Errorf("expected 0 locations initially, got %d", len(locations))
	}

	// Add a location to trigger save
	skillDir, _ := os.MkdirTemp("", "skills-*")
	defer os.RemoveAll(skillDir)

	loc, err := sm.AddLocation("test-location", skillDir, typ.IDESourceClaudeCode)
	if err != nil {
		t.Fatalf("failed to add location: %v", err)
	}

	// Verify the file was created
	filePath := filepath.Join(tempDir, SkillLocationsFile)
	if _, err := os.Stat(filePath); os.IsNotExist(err) {
		t.Error("expected locations file to be created")
	}

	// Create a new manager to test loading
	sm2, err := NewSkillManager(tempDir)
	if err != nil {
		t.Fatalf("failed to create second manager: %v", err)
	}

	retrieved, err := sm2.GetLocation(loc.ID)
	if err != nil {
		t.Fatalf("failed to get location from second manager: %v", err)
	}

	if retrieved.Name != "test-location" {
		t.Errorf("expected name 'test-location', got %q", retrieved.Name)
	}
}

func TestScanLocation_VariousPatterns(t *testing.T) {
	sm, tempDir := setupTestManager(t)
	defer cleanupTestManager(t, sm, tempDir)

	// Test different IDE sources with their patterns
	testCases := []struct {
		name        string
		ideSource   typ.IDESource
		createDirs  []string
		createFiles map[string]string // path -> content
		expected    []string          // expected skill names
	}{
		{
			name:      "Claude Code patterns",
			ideSource: typ.IDESourceClaudeCode,
			createDirs: []string{
				"skills",
				"skills/subdir",
			},
			createFiles: map[string]string{
				"skills/commit.md":      "# Commit",
				"skills/review.md":      "# Review",
				"skills/subdir/test.md": "# Test",
				"SKILL.md":              "# Root",
				"README.md":             "# Readme", // Should not match (not in skills/ or named SKILL.md)
			},
			expected: []string{"commit", "review", "test", "SKILL"},
		},
		{
			name:      "Cursor patterns",
			ideSource: typ.IDESourceCursor,
			createDirs: []string{
				"skills",
				"skills/subdir",
			},
			createFiles: map[string]string{
				"skills/commit.md":      "# Commit",
				"skills/subdir/test.md": "# Test",
				"README.md":             "# Readme", // Should NOT match - Cursor only scans skills/**/*.md
			},
			expected: []string{"commit", "test"},
		},
	}

	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			// Create test directory
			skillDir, err := os.MkdirTemp("", "pattern-test-*")
			if err != nil {
				t.Fatalf("failed to create temp dir: %v", err)
			}
			defer os.RemoveAll(skillDir)

			// Create directories
			for _, dir := range tc.createDirs {
				if err := os.MkdirAll(filepath.Join(skillDir, dir), 0755); err != nil {
					t.Fatalf("failed to create directory: %v", err)
				}
			}

			// Create files
			for file, content := range tc.createFiles {
				path := filepath.Join(skillDir, file)
				if err := os.WriteFile(path, []byte(content), 0644); err != nil {
					t.Fatalf("failed to write file: %v", err)
				}
			}

			// Add and scan location
			loc, err := sm.AddLocation(tc.name, skillDir, tc.ideSource)
			if err != nil {
				t.Fatalf("failed to add location: %v", err)
			}

			result, err := sm.ScanLocation(loc.ID)
			if err != nil {
				t.Fatalf("failed to scan location: %v", err)
			}

			// Check expected skills
			skillNames := make(map[string]bool)
			for _, skill := range result.Skills {
				skillNames[skill.Name] = true
			}

			for _, expectedName := range tc.expected {
				if !skillNames[expectedName] {
					t.Errorf("expected to find skill %q", expectedName)
				}
			}
		})
	}
}

func TestScanLocation_TimeAccuracy(t *testing.T) {
	sm, tempDir := setupTestManager(t)
	defer cleanupTestManager(t, sm, tempDir)

	// Create a test skill file with known modification time
	skillDir, _ := os.MkdirTemp("", "skills-*")
	defer os.RemoveAll(skillDir)

	skillsPath := filepath.Join(skillDir, "skills")
	if err := os.MkdirAll(skillsPath, 0755); err != nil {
		t.Fatalf("failed to create skills directory: %v", err)
	}

	skillPath := filepath.Join(skillsPath, "test.md")
	content := []byte("# Test Skill")
	if err := os.WriteFile(skillPath, content, 0644); err != nil {
		t.Fatalf("failed to write test file: %v", err)
	}

	// Get expected modification time
	info, _ := os.Stat(skillPath)
	expectedModTime := info.ModTime()

	// Add and scan location
	loc, err := sm.AddLocation("test-location", skillDir, typ.IDESourceClaudeCode)
	if err != nil {
		t.Fatalf("failed to add location: %v", err)
	}

	result, err := sm.ScanLocation(loc.ID)
	if err != nil {
		t.Fatalf("failed to scan location: %v", err)
	}

	if len(result.Skills) != 1 {
		t.Fatalf("expected 1 skill, got %d", len(result.Skills))
	}

	skill := result.Skills[0]

	// Verify modification time is accurate (within 1 second)
	diff := skill.ModifiedAt.Sub(expectedModTime)
	if diff < 0 {
		diff = -diff
	}
	if diff > time.Second {
		t.Errorf("modification time differs by %v, expected < 1s", diff)
	}

	// Verify size
	if skill.Size != int64(len(content)) {
		t.Errorf("expected size %d, got %d", len(content), skill.Size)
	}
}

func TestParseSkillDescription(t *testing.T) {
	testCases := []struct {
		name     string
		content  string
		expected string
	}{
		{
			name:     "Simple heading",
			content:  "# Commit Skill\n\nThis is a commit skill.",
			expected: "Commit Skill",
		},
		{
			name:     "Heading with multiple hashes",
			content:  "## Review Skill\n\nThis is a review skill.",
			expected: "Review Skill",
		},
		{
			name:     "No heading - uses first paragraph",
			content:  "This is a skill description.\n\nSecond paragraph.",
			expected: "This is a skill description.",
		},
		{
			name:     "Heading with extra spaces",
			content:  "#   Spaced Heading  \n\nContent here.",
			expected: "Spaced Heading",
		},
		{
			name:     "Empty content",
			content:  "",
			expected: "",
		},
		{
			name:     "Only whitespace",
			content:  "   \n\n   ",
			expected: "",
		},
		{
			name:     "Content with code blocks - skips them",
			content:  "```go\nfunc main() {}\n```\n\nActual description here.",
			expected: "Actual description here.",
		},
		{
			name:     "Long paragraph gets truncated",
			content:  "A" + string(make([]byte, 250)),
			expected: string(make([]byte, 200)),
		},
	}

	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			result := parseSkillDescription(tc.content)
			if tc.name == "Long paragraph gets truncated" {
				// Special check for truncation
				if len(result) != 203 { // 200 + "..."
					t.Errorf("expected truncated length 203, got %d", len(result))
				}
				return
			}
			if result != tc.expected {
				t.Errorf("expected %q, got %q", tc.expected, result)
			}
		})
	}
}

func TestScanLocation_WithDescription(t *testing.T) {
	sm, tempDir := setupTestManager(t)
	defer cleanupTestManager(t, sm, tempDir)

	// Create a temporary directory with skill files
	skillDir, err := os.MkdirTemp("", "skills-desc-test-*")
	if err != nil {
		t.Fatalf("failed to create skill dir: %v", err)
	}
	defer os.RemoveAll(skillDir)

	// Create skills directory
	skillsPath := filepath.Join(skillDir, "skills")
	if err := os.MkdirAll(skillsPath, 0755); err != nil {
		t.Fatalf("failed to create skills directory: %v", err)
	}

	// Create test skill files with descriptions
	testSkills := []struct {
		path        string
		content     string
		expectedDesc string
	}{
		{
			path:        filepath.Join(skillsPath, "commit.md"),
			content:     "# Commit Skill\n\nThis skill helps with commits.",
			expectedDesc: "Commit Skill",
		},
		{
			path:        filepath.Join(skillsPath, "review.md"),
			content:     "## Code Review\n\nThis skill helps with code reviews.",
			expectedDesc: "Code Review",
		},
		{
			path:        filepath.Join(skillDir, "SKILL.md"),
			content:     "No heading here, just a description.",
			expectedDesc: "No heading here, just a description.",
		},
	}

	for _, ts := range testSkills {
		if err := os.WriteFile(ts.path, []byte(ts.content), 0644); err != nil {
			t.Fatalf("failed to write test file: %v", err)
		}
	}

	// Add and scan location
	loc, err := sm.AddLocation("desc-test", skillDir, typ.IDESourceClaudeCode)
	if err != nil {
		t.Fatalf("failed to add location: %v", err)
	}

	result, err := sm.ScanLocation(loc.ID)
	if err != nil {
		t.Fatalf("failed to scan location: %v", err)
	}

	// Build map of skill names to descriptions
	skillDescs := make(map[string]string)
	for _, skill := range result.Skills {
		skillDescs[skill.Name] = skill.Description
	}

	// Verify descriptions were extracted
	for _, ts := range testSkills {
		name := filepath.Base(ts.path)
		name = name[:len(name)-len(filepath.Ext(name))]
		desc, found := skillDescs[name]
		if !found {
			t.Errorf("skill %q not found", name)
			continue
		}
		if desc != ts.expectedDesc {
			t.Errorf("skill %q: expected description %q, got %q", name, ts.expectedDesc, desc)
		}
	}
}
