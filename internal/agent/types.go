package agent

import (
	"fmt"
	"strings"
)

// AgentType represents the type of AI agent to configure
type AgentType string

const (
	// AgentTypeClaudeCode represents Claude Code agent
	AgentTypeClaudeCode AgentType = "claude-code"

	// AgentTypeOpenCode represents OpenCode IDE extension
	AgentTypeOpenCode AgentType = "opencode"
)

// String returns the string representation of AgentType
func (at AgentType) String() string {
	return string(at)
}

// IsValid checks if the AgentType is valid
func (at AgentType) IsValid() bool {
	switch at {
	case AgentTypeClaudeCode, AgentTypeOpenCode:
		return true
	default:
		return false
	}
}

// ParseAgentType parses an agent type string, supporting aliases.
// Returns the normalized AgentType or an error if invalid.
// Supported aliases:
//   - "cc", "claude", "claude-code" -> AgentTypeClaudeCode
//   - "oc", "opencode" -> AgentTypeOpenCode
func ParseAgentType(input string) (AgentType, error) {
	if input == "" {
		return "", fmt.Errorf("agent type cannot be empty")
	}

	normalized := strings.ToLower(strings.TrimSpace(input))

	switch normalized {
	case "cc", "claude", "claude-code", "claudecode":
		return AgentTypeClaudeCode, nil
	case "oc", "opencode", "open-code":
		return AgentTypeOpenCode, nil
	default:
		return "", fmt.Errorf("unknown agent type: %s (supported: cc/claude-code, oc/opencode)", input)
	}
}

// ApplyAgentRequest represents a request to apply agent configuration
type ApplyAgentRequest struct {
	// AgentType is the type of agent to configure (required)
	AgentType AgentType

	// Provider is the provider UUID to use (optional, prompts if empty)
	Provider string

	// Model is the model name to use (optional, prompts if empty)
	Model string

	// Unified specifies unified mode for claude-code (single config for all models)
	// Only applicable for AgentTypeClaudeCode
	Unified bool

	// Force skips confirmation prompts
	Force bool

	// Preview shows what would be applied without actually applying
	Preview bool

	// InstallStatusLine installs the status line script for Claude Code
	// Only applicable for AgentTypeClaudeCode
	InstallStatusLine bool
}

// ApplyAgentResult represents the result of applying agent configuration
type ApplyAgentResult struct {
	// Success indicates whether the operation completed successfully
	Success bool

	// AgentType is the type of agent that was configured
	AgentType AgentType

	// ProviderName is the name of the provider that was selected
	ProviderName string

	// ProviderUUID is the UUID of the provider that was selected
	ProviderUUID string

	// Model is the model name that was selected
	Model string

	// ConfigFiles lists the files that were created or updated
	ConfigFiles []string

	// BackupPaths lists the paths to backup files created
	BackupPaths []string

	// RulesCreated indicates how many new routing rules were created
	RulesCreated int

	// RulesUpdated indicates how many existing routing rules were updated
	RulesUpdated int

	// Message contains a human-readable result message
	Message string
}

// RestoreAgentRequest represents a request to restore agent configuration
// from the most recent on-disk backup.
type RestoreAgentRequest struct {
	// AgentType is the type of agent to restore (required)
	AgentType AgentType

	// Force skips confirmation prompts (CLI use)
	Force bool
}

// RestoreAgentResult represents the result of restoring agent configuration.
type RestoreAgentResult struct {
	// Success is true only when every relevant config file was restored
	// without error.
	Success bool

	// AgentType is the type of agent that was restored.
	AgentType AgentType

	// RestoredFiles lists "<original> <- <backup>" entries for files that
	// were successfully restored.
	RestoredFiles []string

	// PreRestoreBackups lists the safety snapshots taken of each live file
	// before the restore overwrote it.
	PreRestoreBackups []string

	// Failures lists per-file error messages, e.g. "no backup found".
	// Non-empty Failures with empty RestoredFiles means Success == false.
	Failures []string

	// Message is a human-readable summary suitable for CLI output.
	Message string
}

// AgentInfo provides information about an agent type
type AgentInfo struct {
	// Type is the agent type
	Type AgentType

	// Name is the display name
	Name string

	// Description is a brief description
	Description string

	// ConfigFiles lists the configuration files this agent uses
	ConfigFiles []string

	// Scenario is the corresponding routing rule scenario
	Scenario string
}

// ListAgentInfo returns information about all supported agent types
func ListAgentInfo() []AgentInfo {
	return []AgentInfo{
		{
			Type:        AgentTypeClaudeCode,
			Name:        "Claude Code",
			Description: "Claude Code CLI agent (@cc)",
			ConfigFiles: []string{
				"~/.claude/settings.json",
				"~/.claude.json",
			},
			Scenario: "claude_code",
		},
		{
			Type:        AgentTypeOpenCode,
			Name:        "OpenCode",
			Description: "OpenCode IDE extension",
			ConfigFiles: []string{
				"~/.config/opencode/opencode.json",
			},
			Scenario: "opencode",
		},
	}
}

// GetAgentInfo returns information about a specific agent type
func GetAgentInfo(agentType AgentType) (AgentInfo, bool) {
	for _, info := range ListAgentInfo() {
		if info.Type == agentType {
			return info, true
		}
	}
	return AgentInfo{}, false
}
