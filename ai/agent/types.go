package agent

// AgentType represents the type of AI agent to configure
type AgentType string

const (
	// AgentTypeClaudeCode represents Claude Code agent
	AgentTypeClaudeCode AgentType = "claude-code"

	// AgentTypeOpenCode represents OpenCode IDE extension
	AgentTypeOpenCode AgentType = "opencode"

	// AgentTypeCodex represents the OpenAI Codex CLI
	AgentTypeCodex AgentType = "codex"
)

// String returns the string representation of AgentType
func (at AgentType) String() string {
	return string(at)
}

// IsValid checks if the AgentType is valid
func (at AgentType) IsValid() bool {
	switch at {
	case AgentTypeClaudeCode, AgentTypeOpenCode, AgentTypeCodex:
		return true
	default:
		return false
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

	// Warnings collects non-fatal messages emitted during apply, e.g. when
	// no routing service is configured yet so rule sync is skipped.
	Warnings []string

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
