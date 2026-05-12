package agent

import aiagent "github.com/tingly-dev/tingly-box/ai/agent"

// Re-export types from ai/agent for backward compatibility

// Type aliases for backward compatibility
type (
	// AgentType represents the type of AI agent to configure
	AgentType = aiagent.AgentType

	// ApplyAgentRequest represents a request to apply agent configuration
	ApplyAgentRequest = aiagent.ApplyAgentRequest

	// ApplyAgentResult represents the result of applying agent configuration
	ApplyAgentResult = aiagent.ApplyAgentResult

	// RestoreAgentRequest represents a request to restore agent configuration
	RestoreAgentRequest = aiagent.RestoreAgentRequest

	// RestoreAgentResult represents the result of restoring agent configuration
	RestoreAgentResult = aiagent.RestoreAgentResult

	// AgentInfo provides information about an agent type
	AgentInfo = aiagent.AgentInfo
)

// Re-export constants for backward compatibility
const (
	// AgentTypeClaudeCode represents Claude Code agent
	AgentTypeClaudeCode = aiagent.AgentTypeClaudeCode

	// AgentTypeOpenCode represents OpenCode IDE extension
	AgentTypeOpenCode = aiagent.AgentTypeOpenCode

	// AgentTypeCodex represents the OpenAI Codex CLI
	AgentTypeCodex = aiagent.AgentTypeCodex
)

// Re-export functions for backward compatibility
var (
	// ParseAgentType parses an agent type string, supporting aliases
	ParseAgentType = aiagent.ParseAgentType

	// ListAgentInfo returns information about all supported agent types
	ListAgentInfo = aiagent.ListAgentInfo

	// GetAgentInfo returns information about a specific agent type
	GetAgentInfo = aiagent.GetAgentInfo
)
