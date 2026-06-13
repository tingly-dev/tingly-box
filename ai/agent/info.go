package agent

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

	// NPMPackage is the canonical npm package name used by `ci install`.
	// Empty for agents that aren't distributed via npm.
	NPMPackage string
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
			Scenario:   "claude_code",
			NPMPackage: "@anthropic-ai/claude-code",
		},
		{
			Type:        AgentTypeOpenCode,
			Name:        "OpenCode",
			Description: "OpenCode IDE extension",
			ConfigFiles: []string{
				"~/.config/opencode/opencode.json",
			},
			Scenario:   "opencode",
			NPMPackage: "opencode-ai",
		},
		{
			Type:        AgentTypeCodex,
			Name:        "Codex",
			Description: "OpenAI Codex CLI (@codex)",
			ConfigFiles: []string{
				"~/.codex/config.toml",
				"~/.codex/auth.json",
			},
			Scenario:   "codex",
			NPMPackage: "@openai/codex",
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
