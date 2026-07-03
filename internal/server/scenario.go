package server

import (
	"strings"

	"github.com/tingly-dev/tingly-box/internal/typ"
)

// IsValidRuleScenario checks if the given scenario is a valid RuleScenario
func IsValidRuleScenario(scenario typ.RuleScenario) bool {
	return typ.CanUseScenarioInPath(scenario)
}

// ExtractScenarioFromPath extracts the scenario segment from a request path,
// e.g. "/tingly/claude_code/v1/messages" -> "claude_code".
func ExtractScenarioFromPath(path string) string {
	if strings.Contains(path, "/openai/") {
		return "openai"
	}
	if strings.Contains(path, "/codex/") {
		return "codex"
	}
	if strings.Contains(path, "/anthropic/") {
		return "anthropic"
	}
	if strings.Contains(path, "/claude_code/") || strings.Contains(path, "/claude-code/") {
		return "claude_code"
	}
	if strings.Contains(path, "/tingly/") {
		// Extract scenario from tingly path
		parts := strings.Split(path, "/")
		for i, part := range parts {
			if part == "tingly" && i+1 < len(parts) {
				return parts[i+1]
			}
		}
	}
	return "unknown"
}
