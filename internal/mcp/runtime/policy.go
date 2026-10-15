package runtime

import (
	"slices"

	"github.com/tingly-dev/tingly-box/internal/typ"
)

func SourceUsage(source typ.MCPSourceConfig) typ.MCPToolUsage {
	if source.Usage != nil {
		usage := *source.Usage
		if SourceImplementation(source) == typ.ToolImplementationVirtual {
			usage.Client = false
		}
		return usage
	}
	server := SourceVisibility(source) == typ.ToolVisibilityServer
	return typ.MCPToolUsage{Client: !server && SourceImplementation(source) != typ.ToolImplementationVirtual, Gateway: server}
}

// EffectiveToolPolicy applies source enablement, its allow list, and a per-tool override.
// A tool policy cannot grant a tool excluded by the source allow list.
func EffectiveToolPolicy(source typ.MCPSourceConfig, name string) (bool, typ.MCPToolUsage) {
	all, allowed := buildAllowList(source.Tools)
	enabled := typ.IsMCPSourceEnabled(source) && (all || allowed[name])
	usage := SourceUsage(source)
	if policy, ok := source.ToolPolicies[name]; ok {
		if policy.Enabled != nil && !*policy.Enabled {
			enabled = false
		}
		if policy.Usage != nil {
			usage = *policy.Usage
		}
	}
	// In-process Advisor needs model-loop context and cannot be called by an MCP client.
	if SourceImplementation(source) == typ.ToolImplementationVirtual {
		usage.Client = false
	}
	return enabled, usage
}

func grantAllows(grants []string, value string) bool {
	return slices.Contains(grants, "*") || slices.Contains(grants, value)
}

func (r *Runtime) IsGatewayToolName(name string) bool {
	id, tool, ok := ParseNormalizedToolName(name)
	if !ok {
		return false
	}
	cfg := r.GetConfig()
	if cfg == nil {
		return false
	}
	for _, source := range cfg.Sources {
		if source.ID == id {
			_, usage := EffectiveToolPolicy(source, tool)
			return usage.Gateway
		}
	}
	return false
}

func (r *Runtime) ClientAllows(clientID, sourceID, normalizedName string) bool {
	if r == nil {
		return false
	}
	cfg := r.GetConfig()
	if cfg == nil {
		return false
	}
	for _, profile := range cfg.ClientProfiles {
		if profile.ID == clientID {
			return (profile.Enabled == nil || *profile.Enabled) && grantAllows(profile.Sources, sourceID) && grantAllows(profile.Tools, normalizedName)
		}
	}
	// Preserve the existing aggregate endpoint and source-scoped endpoints only
	// while no explicit profiles have been configured. Unknown names fail closed.
	if cfg.ClientProfilesConfigured || len(cfg.ClientProfiles) != 0 {
		return false
	}
	if clientID == "tb" || clientID == "all" {
		return true
	}
	return clientID == sourceID
}

func (r *Runtime) ClientExists(clientID string) bool {
	cfg := r.GetConfig()
	if cfg == nil {
		return false
	}
	for _, p := range cfg.ClientProfiles {
		if p.ID == clientID {
			return true
		}
	}
	if cfg.ClientProfilesConfigured || len(cfg.ClientProfiles) != 0 {
		return false
	}
	if clientID == "tb" || clientID == "all" {
		return true
	}
	for _, s := range cfg.Sources {
		if s.ID == clientID {
			return true
		}
	}
	return false
}
