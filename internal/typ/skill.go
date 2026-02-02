package typ

import (
	"time"
)

// IDESource represents the type of IDE/source for skills
type IDESource string

const (
	IDESourceClaudeCode    IDESource = "claude-code"
	IDESourceOpenCode      IDESource = "opencode"
	IDESourceVSCode        IDESource = "vscode"
	IDESourceCursor        IDESource = "cursor"
	IDESourceCodex         IDESource = "codex"
	IDESourceAntigravity   IDESource = "antigravity"
	IDESourceAmp           IDESource = "amp"
	IDESourceKiloCode      IDESource = "kilo-code"
	IDESourceRooCode       IDESource = "roo-code"
	IDESourceGoose         IDESource = "goose"
	IDESourceGeminiCLI     IDESource = "gemini-cli"
	IDESourceGitHubCopilot IDESource = "github-copilot"
	IDESourceClawdbot      IDESource = "clawdbot"
	IDESourceDroid         IDESource = "droid"
	IDESourceWindsurf      IDESource = "windsurf"
	IDESourceCustom        IDESource = "custom"
)

// IDEAdapter defines the configuration for an IDE adapter
type IDEAdapter struct {
	Key               IDESource `json:"key"`
	DisplayName       string    `json:"display_name"`
	RelativeDetectDir string    `json:"relative_detect_dir"`
	Icon              string    `json:"icon"`
	SupportsSymlink   bool      `json:"supports_symlink"`
	// ScanPatterns defines glob patterns for finding skill files (e.g., ["*.md", "skill/*.md"])
	// Patterns are relative to the detected IDE base directory (relative_detect_dir).
	// If empty, defaults to ["**/*.md"]
	ScanPatterns []string `json:"scan_patterns,omitempty"`
	// GroupingStrategy defines how skills should be grouped in the UI
	GroupingStrategy *GroupingStrategy `json:"grouping_strategy,omitempty"`
}

// GroupingStrategy defines how skills should be grouped in the UI
type GroupingStrategy struct {
	// Mode: "flat" (no grouping), "auto" (automatic based on file count), "pattern" (by pattern)
	Mode string `json:"mode"`
	// GroupPattern: pattern for grouping when mode="pattern", e.g., "skills" groups by skills directory
	// The pattern is searched in the file path, and everything up to (and including) the match becomes the group key
	GroupPattern string `json:"group_pattern,omitempty"`
	// MinFilesForSplit: minimum files before splitting a group (only for auto mode)
	MinFilesForSplit int `json:"min_files_for_split,omitempty"`
}

// SkillLocation represents a skill location (directory)
type SkillLocation struct {
	ID               string    `json:"id"`
	Name             string    `json:"name"`
	Path             string    `json:"path"`
	IDESource        IDESource `json:"ide_source"`
	SkillCount       int       `json:"skill_count"`
	Icon             string    `json:"icon,omitempty"`
	IsAutoDiscovered bool      `json:"is_auto_discovered,omitempty"`
	IsInstalled      bool      `json:"is_installed,omitempty"`
	LastScannedAt    time.Time `json:"last_scanned_at,omitempty"`
	// GroupingStrategy: optional override for this specific location
	GroupingStrategy *GroupingStrategy `json:"grouping_strategy,omitempty"`
}

// Skill represents a single skill file
type Skill struct {
	ID          string    `json:"id"`
	Name        string    `json:"name"`
	Filename    string    `json:"filename"`
	Path        string    `json:"path"`
	LocationID  string    `json:"location_id"`
	FileType    string    `json:"file_type"`
	Description string    `json:"description,omitempty"`
	ContentHash string    `json:"content_hash,omitempty"`
	Size        int64     `json:"size,omitempty"`
	ModifiedAt  time.Time `json:"modified_at,omitempty"`
	Content     string    `json:"content,omitempty"`
}

// DiscoveryResult represents the result of IDE discovery
type DiscoveryResult struct {
	TotalIdesScanned int             `json:"total_ides_scanned"`
	IdesFound        []IDESource     `json:"ides_found"`
	SkillsFound      int             `json:"skills_found"`
	Locations        []SkillLocation `json:"locations"`
}

// ScanResult represents the result of scanning a location
type ScanResult struct {
	LocationID string  `json:"location_id"`
	Skills     []Skill `json:"skills"`
	Error      string  `json:"error,omitempty"`
}
