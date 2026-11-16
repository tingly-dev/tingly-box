package runtime

import (
	"strings"

	mcptools "github.com/tingly-dev/tingly-box/internal/mcp/tools"
	coretool "github.com/tingly-dev/tingly-box/internal/tool"
	"github.com/tingly-dev/tingly-box/internal/typ"
)

func SourceVisibility(source typ.MCPSourceConfig) typ.ToolVisibility {
	if source.ID == mcptools.ServerWebtoolsSourceID {
		return typ.ToolVisibilityServer
	}
	if source.Visibility == typ.ToolVisibilityServer {
		return typ.ToolVisibilityServer
	}
	if source.Visibility == "" && (source.ID == mcptools.BuiltinAdvisorSourceID || source.Transport == "advisor" || source.Advisor != nil) {
		return typ.ToolVisibilityServer
	}
	return typ.ToolVisibilityClient
}

func SourceImplementation(source typ.MCPSourceConfig) typ.ToolImplementation {
	if source.Transport == "advisor" || source.Advisor != nil {
		return typ.ToolImplementationVirtual
	}
	return typ.ToolImplementationMCP
}

func SourceProvider(source typ.MCPSourceConfig) typ.ToolProvider {
	if source.Origin != "" {
		if source.Origin == "builtin" {
			return typ.ToolProviderBuiltin
		}
		return typ.ToolProviderCustom
	}
	if source.ID == mcptools.ServerWebtoolsSourceID || source.ID == mcptools.BuiltinWebtoolsSourceID || source.ID == mcptools.BuiltinAdvisorSourceID {
		return typ.ToolProviderBuiltin
	}
	return typ.ToolProviderCustom
}

func SourceToolDescriptor(source typ.MCPSourceConfig, tool SourceTool) typ.ToolDescriptor {
	return typ.ToolDescriptor{
		Name:           tool.Name,
		SourceID:       source.ID,
		Visibility:     SourceVisibility(source),
		Implementation: SourceImplementation(source),
		Provider:       SourceProvider(source),
		Description:    tool.Description,
	}
}

func VirtualToolDescriptor(sourceID string, tool coretool.VirtualTool) typ.ToolDescriptor {
	visibility := tool.Visibility
	if visibility == "" {
		visibility = typ.ToolVisibilityServer
	}
	return typ.ToolDescriptor{
		Name:           tool.Name,
		SourceID:       sourceID,
		Visibility:     visibility,
		Implementation: typ.ToolImplementationVirtual,
		Provider:       SourceProvider(typ.MCPSourceConfig{ID: sourceID}),
		Description:    tool.Description,
	}
}

func IsServerVisibleSource(source typ.MCPSourceConfig) bool {
	return SourceUsage(source).Gateway
}

func IsClientVisibleSource(source typ.MCPSourceConfig) bool {
	return SourceUsage(source).Client
}

func IsServerVisibleVirtualTool(tool coretool.VirtualTool) bool {
	return tool.Visibility == "" || tool.Visibility == typ.ToolVisibilityServer
}

func IsClientVisibleVirtualTool(tool coretool.VirtualTool) bool {
	return tool.Visibility == typ.ToolVisibilityClient
}

// IsServerToolSource is independent of implementation: built-in web tools use
// stdio MCP internally but have gateway-only Server Tool semantics.
func IsServerToolSource(source typ.MCPSourceConfig) bool {
	return source.ID == mcptools.ServerWebtoolsSourceID || SourceImplementation(source) == typ.ToolImplementationVirtual
}

func IsBuiltinSourceID(sourceID string) bool {
	sourceID = strings.TrimSpace(sourceID)
	return sourceID == mcptools.ServerWebtoolsSourceID || sourceID == mcptools.BuiltinWebtoolsSourceID || sourceID == mcptools.BuiltinAdvisorSourceID || sourceID == "builtin"
}
