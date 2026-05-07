package mcp

import "github.com/tingly-dev/tingly-box/internal/mcp/runtime"

// IsVirtualTool reports whether the normalized MCP tool should execute server-side.
func IsVirtualTool(normalizedName string, registry *runtime.VirtualToolRegistry) bool {
	sourceID, toolName, ok := runtime.ParseNormalizedToolName(normalizedName)
	if !ok {
		return false
	}
	// Advisor is always treated as a virtual tool.
	if sourceID == "advisor" || (sourceID == "builtin" && toolName == "advisor") {
		return true
	}
	if registry == nil {
		return false
	}
	_, ok = registry.Get(toolName)
	return ok
}

func IsVirtualToolName(name string, registry *runtime.VirtualToolRegistry) bool {
	return IsVirtualTool(name, registry)
}
