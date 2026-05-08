package servertool

import (
	"github.com/tingly-dev/tingly-box/internal/client"
	mcpruntime "github.com/tingly-dev/tingly-box/internal/mcp/runtime"
	"github.com/tingly-dev/tingly-box/internal/typ"
)

// AdvisorProvider implements ToolProvider for the built-in advisor virtual tool.
type AdvisorProvider struct {
	cfg   typ.AdvisorConfig
	cp    *client.ClientPool
	store *mcpruntime.SessionStore
}

// NewAdvisorProvider constructs an AdvisorProvider.
// cp and store may be nil in tests.
func NewAdvisorProvider(cfg typ.AdvisorConfig, cp *client.ClientPool, store *mcpruntime.SessionStore) AdvisorProvider {
	return AdvisorProvider{cfg: cfg, cp: cp, store: store}
}

// Descriptor returns the VirtualTool definition for the advisor.
func (p AdvisorProvider) Descriptor() mcpruntime.VirtualTool {
	return mcpruntime.NewAdvisorVirtualTool(p.cfg, p.cp, p.store)
}

// Hook returns the AdvisorHook that injects AdvisorContext before each call.
func (p AdvisorProvider) Hook() Hook {
	return AdvisorHook{}
}
