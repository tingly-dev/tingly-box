package servertool

import (
	mcpruntime "github.com/tingly-dev/tingly-box/internal/mcp/runtime"
)

// IsMCPToolName reports whether name looks like a normalized MCP tool name
// (i.e. it parses successfully via ParseNormalizedToolName).
func IsMCPToolName(name string) bool {
	return mcpruntime.IsMCPToolName(name)
}

// RemapLegacyAdvisorToolName normalises the legacy "advisor" source ID to "builtin".
// All other tool names pass through unchanged.
func RemapLegacyAdvisorToolName(toolName string) string {
	sourceID, toolNameOnly, ok := mcpruntime.ParseNormalizedToolName(toolName)
	if !ok {
		return toolName
	}
	if sourceID == "advisor" && toolNameOnly == "advisor" {
		return mcpruntime.NormalizeToolName("builtin", "advisor")
	}
	return toolName
}

