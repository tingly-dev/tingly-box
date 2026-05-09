package servertool

import coretool "github.com/tingly-dev/tingly-box/internal/tool"

// IsMCPToolName reports whether name looks like a normalized MCP tool name
// (i.e. it parses successfully via ParseNormalizedToolName).
func IsMCPToolName(name string) bool {
	return coretool.IsMCPToolName(name)
}

// RemapLegacyAdvisorToolName normalises the legacy "advisor" source ID to "builtin".
// All other tool names pass through unchanged.
func RemapLegacyAdvisorToolName(toolName string) string {
	sourceID, toolNameOnly, ok := coretool.ParseNormalizedToolName(toolName)
	if !ok {
		return toolName
	}
	if sourceID == "advisor" && toolNameOnly == "advisor" {
		return coretool.NormalizeToolName("builtin", "advisor")
	}
	return toolName
}
