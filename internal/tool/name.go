package tool

import "strings"

const normalizedPrefix = "tingly_box_mcp__"

const (
	BuiltinAdvisorSourceID = "advisor"
	BuiltinAdvisorToolName = "advisor"
)

// IsNormalizedToolName checks whether a tool name is a normalized server tool name.
func IsNormalizedToolName(name string) bool {
	return strings.HasPrefix(name, normalizedPrefix) && strings.Count(name, "__") >= 2
}

// IsMCPToolName checks whether a tool name is a normalized MCP tool.
// Deprecated: use IsNormalizedToolName for protocol-neutral code.
func IsMCPToolName(name string) bool {
	return IsNormalizedToolName(name)
}

// NormalizeToolName converts source/tool pair to normalized tool name.
func NormalizeToolName(sourceID, toolName string) string {
	return normalizedPrefix + sourceID + "__" + toolName
}

// ParseNormalizedToolName parses normalized name and returns sourceID/toolName.
func ParseNormalizedToolName(name string) (string, string, bool) {
	if !strings.HasPrefix(name, normalizedPrefix) {
		return "", "", false
	}
	rest := strings.TrimPrefix(name, normalizedPrefix)
	parts := strings.SplitN(rest, "__", 2)
	if len(parts) != 2 || parts[0] == "" || parts[1] == "" {
		return "", "", false
	}
	return parts[0], parts[1], true
}
