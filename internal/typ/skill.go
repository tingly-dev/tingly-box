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
	RelativeSkillsDir string    `json:"relative_skills_dir"`
	RelativeDetectDir string    `json:"relative_detect_dir"`
	Icon              string    `json:"icon"`
	SupportsSymlink   bool      `json:"supports_symlink"`
	// ScanPatterns defines glob patterns for finding skill files (e.g., ["*.md", "skill/*.md"])
	// If empty, defaults to ["*.md"]
	ScanPatterns []string `json:"scan_patterns,omitempty"`
}

// DefaultIDEAdapters returns the list of supported IDE adapters
func DefaultIDEAdapters() []IDEAdapter {
	return []IDEAdapter{
		{
			Key:               IDESourceClaudeCode,
			DisplayName:       "Claude Code",
			RelativeDetectDir: ".claude",
			Icon:              "🎨",
			SupportsSymlink:   true,
			ScanPatterns:      []string{"skills/**/*.md", "**/SKILL.md"},
		},
		{
			Key:               IDESourceOpenCode,
			DisplayName:       "OpenCode",
			RelativeDetectDir: ".config/opencode",
			Icon:              "💻",
			SupportsSymlink:   true,
			ScanPatterns:      []string{"**/*.md"},
		},
		{
			Key:               IDESourceVSCode,
			DisplayName:       "VS Code",
			RelativeDetectDir: ".vscode",
			Icon:              "💡",
			SupportsSymlink:   true,
			ScanPatterns:      []string{"**/*.md"},
		},
		{
			Key:               IDESourceCursor,
			DisplayName:       "Cursor",
			RelativeDetectDir: ".cursor",
			Icon:              "🎯",
			SupportsSymlink:   false, // Cursor doesn't support symlinks
			ScanPatterns:      []string{"**/*.md"},
		},
		{
			Key:               IDESourceCodex,
			DisplayName:       "Codex",
			RelativeDetectDir: ".codex",
			Icon:              "📜",
			SupportsSymlink:   true,
			ScanPatterns:      []string{"**/*.md"},
		},
		{
			Key:               IDESourceAntigravity,
			DisplayName:       "Antigravity",
			RelativeDetectDir: ".gemini/antigravity",
			Icon:              "🔄",
			SupportsSymlink:   true,
			ScanPatterns:      []string{"**/*.md"},
		},
		{
			Key:               IDESourceAmp,
			DisplayName:       "Amp",
			RelativeDetectDir: ".config/agents",
			Icon:              "⚡",
			SupportsSymlink:   true,
			ScanPatterns:      []string{"**/*.md"},
		},
		{
			Key:               IDESourceKiloCode,
			DisplayName:       "Kilo Code",
			RelativeDetectDir: ".kilocode",
			Icon:              "🪜",
			SupportsSymlink:   true,
			ScanPatterns:      []string{"**/*.md"},
		},
		{
			Key:               IDESourceRooCode,
			DisplayName:       "Roo Code",
			RelativeDetectDir: ".roo",
			Icon:              "🦘",
			SupportsSymlink:   true,
			ScanPatterns:      []string{"**/*.md"},
		},
		{
			Key:               IDESourceGoose,
			DisplayName:       "Goose",
			RelativeDetectDir: ".config/goose",
			Icon:              "🪿",
			SupportsSymlink:   true,
			ScanPatterns:      []string{"**/*.md"},
		},
		{
			Key:               IDESourceGeminiCLI,
			DisplayName:       "Gemini CLI",
			RelativeDetectDir: ".gemini",
			Icon:              "💎",
			SupportsSymlink:   true,
			ScanPatterns:      []string{"**/*.md"},
		},
		{
			Key:               IDESourceGitHubCopilot,
			DisplayName:       "GitHub Copilot",
			RelativeDetectDir: ".copilot",
			Icon:              "🐙",
			SupportsSymlink:   true,
			ScanPatterns:      []string{"**/*.md"},
		},
		{
			Key:               IDESourceClawdbot,
			DisplayName:       "Clawdbot",
			RelativeDetectDir: ".clawdbot",
			Icon:              "🦞",
			SupportsSymlink:   true,
			ScanPatterns:      []string{"**/*.md"},
		},
		{
			Key:               IDESourceDroid,
			DisplayName:       "Droid",
			RelativeDetectDir: ".factory",
			Icon:              "🤖",
			SupportsSymlink:   true,
			ScanPatterns:      []string{"**/*.md"},
		},
		{
			Key:               IDESourceWindsurf,
			DisplayName:       "Windsurf",
			RelativeDetectDir: ".codeium/windsurf",
			Icon:              "🌊",
			SupportsSymlink:   true,
			ScanPatterns:      []string{"**/*.md"},
		},
		{
			Key:               IDESourceCustom,
			DisplayName:       "Custom",
			RelativeDetectDir: "",
			Icon:              "📂",
			SupportsSymlink:   true,
			ScanPatterns:      []string{"**/*.md"},
		},
	}
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
