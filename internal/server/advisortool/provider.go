package advisortool

import (
	"github.com/tingly-dev/tingly-box/internal/client"
	mcpruntime "github.com/tingly-dev/tingly-box/internal/mcp/runtime"
	"github.com/tingly-dev/tingly-box/internal/server/servertool"
	coretool "github.com/tingly-dev/tingly-box/internal/tool"
	"github.com/tingly-dev/tingly-box/internal/typ"
)

// Provider implements servertool.ToolProvider for the built-in advisor virtual tool.
type Provider struct {
	cfg   typ.AdvisorConfig
	cp    *client.ClientPool
	store *mcpruntime.SessionStore
}

// NewProvider constructs an advisor servertool provider.
// cp and store may be nil in tests.
func NewProvider(cfg typ.AdvisorConfig, cp *client.ClientPool, store *mcpruntime.SessionStore) Provider {
	return Provider{cfg: cfg, cp: cp, store: store}
}

// Descriptor returns the VirtualTool definition for the advisor.
func (p Provider) Descriptor() coretool.VirtualTool {
	return mcpruntime.NewAdvisorVirtualTool(p.cfg, p.cp, p.store)
}

// Hook returns the AdvisorHook that injects AdvisorContext before each call.
func (p Provider) Hook() servertool.Hook {
	return servertool.AdvisorHook{}
}
