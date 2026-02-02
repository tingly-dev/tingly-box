package data

import (
	"crypto/sha256"
	_ "embed"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"sync"
	"time"

	"github.com/bmatcuk/doublestar/v4"
	"github.com/google/uuid"
	"github.com/tingly-dev/tingly-box/internal/typ"
)

//go:embed skill_client.json
var defaultSkillClientConfig []byte

const (
	// SkillLocationsFile is the filename for skill locations storage
	SkillLocationsFile = "skill_locations.json"
	// SkillClientConfigFile is the filename for IDE client scanning configuration
	SkillClientConfigFile = "skill_client.json"
)

// DefaultIDEAdapters returns the list of supported IDE adapters from embedded config
func DefaultIDEAdapters() ([]typ.IDEAdapter, error) {
	var config struct {
		Version  string           `json:"version"`
		Adapters []typ.IDEAdapter `json:"adapters"`
	}
	if err := json.Unmarshal(defaultSkillClientConfig, &config); err != nil {
		return nil, fmt.Errorf("failed to parse embedded config: %w", err)
	}
	return config.Adapters, nil
}

// SkillManager manages skill locations
type SkillManager struct {
	configDir string
	filePath  string
	locations map[string]*typ.SkillLocation
	mu        sync.RWMutex
}

// NewSkillManager creates a new skill manager
func NewSkillManager(configDir string) (*SkillManager, error) {
	if err := os.MkdirAll(configDir, 0755); err != nil {
		return nil, fmt.Errorf("failed to create config directory: %w", err)
	}

	sm := &SkillManager{
		configDir: configDir,
		filePath:  filepath.Join(configDir, SkillLocationsFile),
		locations: make(map[string]*typ.SkillLocation),
	}

	if err := sm.load(); err != nil {
		// If file doesn't exist, it's okay - start with empty locations
		if !os.IsNotExist(err) {
			return nil, fmt.Errorf("failed to load skill locations: %w", err)
		}
	}

	return sm, nil
}

// load loads skill locations from disk
func (sm *SkillManager) load() error {
	sm.mu.Lock()
	defer sm.mu.Unlock()

	data, err := os.ReadFile(sm.filePath)
	if err != nil {
		return err
	}

	var locations []typ.SkillLocation
	if err := json.Unmarshal(data, &locations); err != nil {
		return err
	}

	sm.locations = make(map[string]*typ.SkillLocation)
	for i := range locations {
		sm.locations[locations[i].ID] = &locations[i]
	}

	return nil
}

// save saves skill locations to disk
func (sm *SkillManager) save() error {
	data, err := json.MarshalIndent(sm.locationsList(), "", "  ")
	if err != nil {
		return err
	}

	return os.WriteFile(sm.filePath, data, 0644)
}

// locationsList returns locations as a slice
func (sm *SkillManager) locationsList() []typ.SkillLocation {
	locations := make([]typ.SkillLocation, 0, len(sm.locations))
	for _, loc := range sm.locations {
		locations = append(locations, *loc)
	}
	return locations
}

// AddLocation adds a new skill location
func (sm *SkillManager) AddLocation(name, path string, ideSource typ.IDESource) (*typ.SkillLocation, error) {
	sm.mu.Lock()
	defer sm.mu.Unlock()

	// Check for duplicate paths
	for _, loc := range sm.locations {
		if loc.Path == path {
			return nil, fmt.Errorf("location with path '%s' already exists", path)
		}
	}

	id := uuid.New().String()
	location := &typ.SkillLocation{
		ID:         id,
		Name:       name,
		Path:       path,
		IDESource:  ideSource,
		SkillCount: 0,
	}

	// Get icon and grouping strategy from IDE adapters
	adapters, err := DefaultIDEAdapters()
	if err != nil {
		// Continue without icon if config fails to load
		adapters = []typ.IDEAdapter{}
	}
	for _, adapter := range adapters {
		if adapter.Key == ideSource {
			location.Icon = adapter.Icon
			location.GroupingStrategy = adapter.GroupingStrategy
			break
		}
	}

	sm.locations[id] = location

	if err := sm.save(); err != nil {
		delete(sm.locations, id)
		return nil, err
	}

	return location, nil
}

// RemoveLocation removes a skill location by ID
func (sm *SkillManager) RemoveLocation(id string) error {
	sm.mu.Lock()
	defer sm.mu.Unlock()

	if _, exists := sm.locations[id]; !exists {
		return fmt.Errorf("location with ID '%s' not found", id)
	}

	delete(sm.locations, id)
	return sm.save()
}

// GetLocation retrieves a skill location by ID
func (sm *SkillManager) GetLocation(id string) (*typ.SkillLocation, error) {
	sm.mu.RLock()
	defer sm.mu.RUnlock()

	loc, exists := sm.locations[id]
	if !exists {
		return nil, fmt.Errorf("location with ID '%s' not found", id)
	}

	// Return a copy
	copy := *loc
	return &copy, nil
}

// ListLocations returns all skill locations
func (sm *SkillManager) ListLocations() []typ.SkillLocation {
	sm.mu.RLock()
	defer sm.mu.RUnlock()

	return sm.locationsList()
}

// UpdateLocationSkillCount updates the skill count for a location
func (sm *SkillManager) UpdateLocationSkillCount(id string, count int) error {
	sm.mu.Lock()
	defer sm.mu.Unlock()

	loc, exists := sm.locations[id]
	if !exists {
		return fmt.Errorf("location with ID '%s' not found", id)
	}

	loc.SkillCount = count
	loc.LastScannedAt = time.Now()
	loc.IsInstalled = true

	return sm.save()
}

// UpdateLocation updates a skill location
func (sm *SkillManager) UpdateLocation(location typ.SkillLocation) error {
	sm.mu.Lock()
	defer sm.mu.Unlock()

	if _, exists := sm.locations[location.ID]; !exists {
		return fmt.Errorf("location with ID '%s' not found", location.ID)
	}

	sm.locations[location.ID] = &location
	return sm.save()
}

// ScanLocation scans a location directory for skill files
func (sm *SkillManager) ScanLocation(locationID string) (*typ.ScanResult, error) {
	sm.mu.RLock()
	loc, exists := sm.locations[locationID]
	sm.mu.RUnlock()

	if !exists {
		return nil, fmt.Errorf("location with ID '%s' not found", locationID)
	}

	// Expand path if it starts with ~
	path := loc.Path
	if len(path) > 0 && path[0] == '~' {
		homeDir, err := os.UserHomeDir()
		if err != nil {
			return nil, fmt.Errorf("failed to get home directory: %w", err)
		}
		path = filepath.Join(homeDir, path[1:])
	}

	// Get scan patterns from IDE adapter
	adapters, err := sm.loadClientConfig()
	if err != nil {
		adapters, err = DefaultIDEAdapters()
		if err != nil {
			adapters = []typ.IDEAdapter{}
		}
	}

	var patterns []string
	for _, adapter := range adapters {
		if adapter.Key == loc.IDESource {
			patterns = adapter.ScanPatterns
			break
		}
	}

	skills, err := scanDirectoryForSkills(path, patterns)
	if err != nil {
		return &typ.ScanResult{
			LocationID: locationID,
			Skills:     nil,
			Error:      err.Error(),
		}, nil
	}

	// Set location ID for each skill
	for i := range skills {
		skills[i].LocationID = locationID
	}

	// Update skill count
	_ = sm.UpdateLocationSkillCount(locationID, len(skills))

	return &typ.ScanResult{
		LocationID: locationID,
		Skills:     skills,
	}, nil
}

// getDefaultScanPatterns returns default patterns if none specified
func getDefaultScanPatterns() []string {
	return []string{"**/*.md"}
}

// scanDirectoryForSkills scans a directory for skill files using glob patterns
func scanDirectoryForSkills(dirPath string, patterns []string) ([]typ.Skill, error) {
	var skills []typ.Skill

	// Use default patterns if none provided
	if len(patterns) == 0 {
		patterns = getDefaultScanPatterns()
	}

	// Check if directory exists
	if _, err := os.Stat(dirPath); err != nil {
		if os.IsNotExist(err) {
			return skills, nil
		}
		return nil, err
	}

	// Track files we've already added to avoid duplicates
	seenFiles := make(map[string]bool)

	// Use doublestar.FilepathGlob for each pattern (works with OS filesystem directly)
	for _, pattern := range patterns {
		// doublestar.FilepathGlob works with the OS filesystem
		// The pattern should include the base path
		globPattern := filepath.Join(dirPath, pattern)

		matches, err := doublestar.FilepathGlob(globPattern)
		if err != nil {
			// Invalid pattern, skip it
			continue
		}

		for _, fullPath := range matches {
			// Skip if we've already seen this file
			if seenFiles[fullPath] {
				continue
			}

			// Skip hidden files and files in hidden directories
			// Check all path components for dot directories
			relPath, err := filepath.Rel(dirPath, fullPath)
			if err == nil {
				// Split path and check each component
				components := filepath.SplitList(relPath)
				isHidden := false
				for _, comp := range components {
					if len(comp) > 0 && comp[0] == '.' {
						isHidden = true
						break
					}
				}
				if isHidden {
					continue
				}
			}

			// Get file info
			info, err := os.Stat(fullPath)
			if err != nil {
				// File no longer accessible, skip
				continue
			}

			// Skip directories
			if info.IsDir() {
				continue
			}

			seenFiles[fullPath] = true

			ext := filepath.Ext(info.Name())
			nameWithoutExt := info.Name()[:len(info.Name())-len(ext)]

			// Generate stable ID from file path (SHA256 hash, truncated to 16 chars for brevity)
			hash := sha256.Sum256([]byte(fullPath))
			stableID := hex.EncodeToString(hash[:])[:16]

			skill := typ.Skill{
				ID:         stableID,
				Name:       nameWithoutExt,
				Filename:   info.Name(),
				Path:       fullPath,
				LocationID: "", // Set by caller
				FileType:   ext,
				Size:       info.Size(),
				ModifiedAt: info.ModTime(),
			}

			skills = append(skills, skill)
		}
	}

	return skills, nil
}

// resolvePath resolves a path relative to home directory if it's not absolute
func resolvePath(homeDir, path string) string {
	if filepath.IsAbs(path) {
		return path
	}
	return filepath.Join(homeDir, path)
}

// loadClientConfig loads IDE client scanning configuration
// First uses embedded default config, then merges user's custom config
func (sm *SkillManager) loadClientConfig() ([]typ.IDEAdapter, error) {
	// Parse embedded default config first
	var defaultConfig struct {
		Version  string           `json:"version"`
		Adapters []typ.IDEAdapter `json:"adapters"`
	}
	if err := json.Unmarshal(defaultSkillClientConfig, &defaultConfig); err != nil {
		return nil, fmt.Errorf("failed to parse embedded config: %w", err)
	}

	// Start with embedded adapters
	adapters := defaultConfig.Adapters

	// Try to load user's custom config (additional adapters)
	configPath := filepath.Join(sm.configDir, SkillClientConfigFile)
	data, err := os.ReadFile(configPath)
	if err != nil {
		if !os.IsNotExist(err) {
			return nil, fmt.Errorf("failed to read user config: %w", err)
		}
		// File doesn't exist, just use embedded config
		return adapters, nil
	}

	// Parse user config
	var userConfig struct {
		Version  string           `json:"version"`
		Adapters []typ.IDEAdapter `json:"adapters"`
	}
	if err := json.Unmarshal(data, &userConfig); err != nil {
		return nil, fmt.Errorf("failed to parse user config: %w", err)
	}

	// Merge: user can override existing adapters by using the same key
	userAdapterMap := make(map[string]typ.IDEAdapter)
	for _, adapter := range userConfig.Adapters {
		userAdapterMap[string(adapter.Key)] = adapter
	}

	// Build final list: defaults first, then user overrides/additions
	result := make([]typ.IDEAdapter, 0, len(adapters)+len(userConfig.Adapters))
	seenKeys := make(map[string]bool)

	// First, add default adapters (unless overridden by user)
	for _, adapter := range adapters {
		key := string(adapter.Key)
		if custom, exists := userAdapterMap[key]; exists {
			result = append(result, custom)
			seenKeys[key] = true
		} else {
			result = append(result, adapter)
			seenKeys[key] = true
		}
	}

	// Then, add any new adapters from user config
	for _, adapter := range userConfig.Adapters {
		key := string(adapter.Key)
		if !seenKeys[key] {
			result = append(result, adapter)
		}
	}

	return result, nil
}

// DiscoverIdes scans the home directory for installed IDEs with skills
func (sm *SkillManager) DiscoverIdes() (*typ.DiscoveryResult, error) {
	homeDir, err := os.UserHomeDir()
	if err != nil {
		return nil, fmt.Errorf("failed to get home directory: %w", err)
	}

	adapters, err := sm.loadClientConfig()
	if err != nil {
		return nil, fmt.Errorf("failed to load client config: %w", err)
	}

	result := &typ.DiscoveryResult{
		TotalIdesScanned: len(adapters),
		IdesFound:        []typ.IDESource{},
		SkillsFound:      0,
		Locations:        []typ.SkillLocation{},
	}

	for _, adapter := range adapters {
		// Skip empty paths
		if adapter.RelativeDetectDir == "" {
			continue
		}

		// Use relative_detect_dir as the base directory for scanning
		basePath := resolvePath(homeDir, adapter.RelativeDetectDir)

		// Check if IDE detect directory exists
		if _, err := os.Stat(basePath); os.IsNotExist(err) {
			continue
		}

		// Scan for skills using the base path and scan patterns
		skillsCount := 0
		if skills, err := scanDirectoryForSkills(basePath, adapter.ScanPatterns); err == nil {
			skillsCount = len(skills)
			result.SkillsFound += skillsCount
		}

		// Add to discovered locations
		result.IdesFound = append(result.IdesFound, adapter.Key)
		result.Locations = append(result.Locations, typ.SkillLocation{
			ID:               uuid.New().String(),
			Name:             adapter.DisplayName + " Skills",
			Path:             basePath,
			IDESource:        adapter.Key,
			SkillCount:       skillsCount,
			Icon:             adapter.Icon,
			GroupingStrategy: adapter.GroupingStrategy,
			IsAutoDiscovered: true,
			IsInstalled:      true,
			LastScannedAt:    time.Now(),
		})
	}

	return result, nil
}

// ScanIdes scans all IDE locations and returns discovered skills
// This is a comprehensive scan that checks all configured IDE locations
func (sm *SkillManager) ScanIdes() (*typ.DiscoveryResult, error) {
	homeDir, err := os.UserHomeDir()
	if err != nil {
		return nil, fmt.Errorf("failed to get home directory: %w", err)
	}

	adapters, err := sm.loadClientConfig()
	if err != nil {
		return nil, fmt.Errorf("failed to load client config: %w", err)
	}

	result := &typ.DiscoveryResult{
		TotalIdesScanned: len(adapters),
		IdesFound:        []typ.IDESource{},
		SkillsFound:      0,
		Locations:        []typ.SkillLocation{},
	}

	for _, adapter := range adapters {
		// Skip empty paths
		if adapter.RelativeDetectDir == "" {
			continue
		}

		// Use relative_detect_dir as the base directory for scanning
		basePath := resolvePath(homeDir, adapter.RelativeDetectDir)

		// Check if IDE detect directory exists
		if _, err := os.Stat(basePath); os.IsNotExist(err) {
			continue
		}

		// Scan for skills using the base path and scan patterns
		skills, _ := scanDirectoryForSkills(basePath, adapter.ScanPatterns)
		skillsCount := len(skills)
		result.SkillsFound += skillsCount

		// Add to discovered locations
		result.IdesFound = append(result.IdesFound, adapter.Key)
		result.Locations = append(result.Locations, typ.SkillLocation{
			ID:               uuid.New().String(),
			Name:             adapter.DisplayName + " Skills",
			Path:             basePath,
			IDESource:        adapter.Key,
			SkillCount:       skillsCount,
			Icon:             adapter.Icon,
			GroupingStrategy: adapter.GroupingStrategy,
			IsAutoDiscovered: true,
			IsInstalled:      true,
			LastScannedAt:    time.Now(),
		})
	}

	return result, nil
}

// GetSkillContent reads and returns the content of a skill file
func (sm *SkillManager) GetSkillContent(locationID, skillID, skillPath string) (*typ.Skill, error) {
	sm.mu.RLock()
	_, exists := sm.locations[locationID]
	sm.mu.RUnlock()

	if !exists {
		return nil, fmt.Errorf("location with ID '%s' not found", locationID)
	}

	// If skillPath is provided, use it directly
	if skillPath != "" {
		// Verify the file exists
		if _, err := os.Stat(skillPath); os.IsNotExist(err) {
			return nil, fmt.Errorf("skill file not found at path: %s", skillPath)
		}

		// Read file content
		content, err := os.ReadFile(skillPath)
		if err != nil {
			return nil, fmt.Errorf("failed to read skill file: %w", err)
		}

		// Get file info
		info, _ := os.Stat(skillPath)
		ext := filepath.Ext(skillPath)
		nameWithoutExt := filepath.Base(skillPath)
		if ext != "" {
			nameWithoutExt = nameWithoutExt[:len(nameWithoutExt)-len(ext)]
		}

		// Generate stable ID
		hash := sha256.Sum256([]byte(skillPath))
		stableID := hex.EncodeToString(hash[:])[:16]

		skill := &typ.Skill{
			ID:         stableID,
			Name:       nameWithoutExt,
			Filename:   filepath.Base(skillPath),
			Path:       skillPath,
			LocationID: locationID,
			FileType:   ext,
			Size:       info.Size(),
			ModifiedAt: info.ModTime(),
			Content:    string(content),
		}
		return skill, nil
	}

	// Otherwise, scan location to find by ID
	result, err := sm.ScanLocation(locationID)
	if err != nil {
		return nil, fmt.Errorf("failed to scan location: %w", err)
	}

	// Find the requested skill
	var targetSkill *typ.Skill
	for i := range result.Skills {
		if result.Skills[i].ID == skillID {
			targetSkill = &result.Skills[i]
			break
		}
	}

	if targetSkill == nil {
		return nil, fmt.Errorf("skill with ID '%s' not found", skillID)
	}

	// Read file content
	content, err := os.ReadFile(targetSkill.Path)
	if err != nil {
		return nil, fmt.Errorf("failed to read skill file: %w", err)
	}

	targetSkill.Content = string(content)
	return targetSkill, nil
}
