package agent

import (
	"fmt"
	"strings"
)

// ParseAgentType parses an agent type string, supporting aliases.
// Returns the normalized AgentType or an error if invalid.
// Supported aliases:
//   - "cc", "claude", "claude-code" -> AgentTypeClaudeCode
//   - "oc", "opencode" -> AgentTypeOpenCode
//   - "codex" -> AgentTypeCodex (no shorthand — unlike @cc/@oc, there's no
//     established "@codex"-style abbreviation for it)
//   - "dsh" -> AgentTypeDsh
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
	case "codex":
		return AgentTypeCodex, nil
	case "dsh":
		return AgentTypeDsh, nil
	default:
		return "", fmt.Errorf("unknown agent type: %s (supported: cc/claude-code, oc/opencode, codex, dsh)", input)
	}
}
