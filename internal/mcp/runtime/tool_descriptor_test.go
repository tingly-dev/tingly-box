package runtime

import (
	"testing"

	mcptools "github.com/tingly-dev/tingly-box/internal/mcp/tools"
	"github.com/tingly-dev/tingly-box/internal/typ"
)

func TestToolDescriptorDimensions(t *testing.T) {
	webtools := typ.MCPSourceConfig{ID: mcptools.BuiltinWebtoolsSourceID}
	advisor := typ.MCPSourceConfig{ID: mcptools.BuiltinAdvisorSourceID, Advisor: &typ.AdvisorConfig{}}
	custom := typ.MCPSourceConfig{ID: "custom", Visibility: typ.ToolVisibilityServer}

	cases := []struct {
		name           string
		source         typ.MCPSourceConfig
		visibility     typ.ToolVisibility
		implementation typ.ToolImplementation
		provider       typ.ToolProvider
	}{
		{"builtin webtools", webtools, typ.ToolVisibilityClient, typ.ToolImplementationMCP, typ.ToolProviderBuiltin},
		{"builtin advisor", advisor, typ.ToolVisibilityServer, typ.ToolImplementationVirtual, typ.ToolProviderBuiltin},
		{"custom server mcp", custom, typ.ToolVisibilityServer, typ.ToolImplementationMCP, typ.ToolProviderCustom},
	}

	for _, tt := range cases {
		t.Run(tt.name, func(t *testing.T) {
			desc := SourceToolDescriptor(tt.source, SourceTool{Name: "tool", Description: "desc"})
			if desc.Visibility != tt.visibility {
				t.Fatalf("visibility = %s, want %s", desc.Visibility, tt.visibility)
			}
			if desc.Implementation != tt.implementation {
				t.Fatalf("implementation = %s, want %s", desc.Implementation, tt.implementation)
			}
			if desc.Provider != tt.provider {
				t.Fatalf("provider = %s, want %s", desc.Provider, tt.provider)
			}
		})
	}
}
