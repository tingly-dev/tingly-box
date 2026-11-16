package runtime

import (
	mcptools "github.com/tingly-dev/tingly-box/internal/mcp/tools"
	"github.com/tingly-dev/tingly-box/internal/typ"
)

// SourceUsage reports where a source's tools may run. Client availability is not
// a setting: ordinary MCP tools can be offered through a profile, and profiles
// plus switches decide which. Server Tools are always gateway-only.
func SourceUsage(source typ.MCPSourceConfig) typ.MCPToolUsage {
	usage := typ.MCPToolUsage{Gateway: SourceVisibility(source) == typ.ToolVisibilityServer}
	if source.Usage != nil {
		usage.Gateway = source.Usage.Gateway
	}
	// A Server Tool exists only to be run by the gateway; its switch is the
	// tool's own enablement, not a separate usage flag.
	server := IsServerToolSource(source)
	usage.Client = !server
	if server {
		usage.Gateway = true
	}
	return usage
}

// EffectiveToolPolicy applies source enablement, its allow list, and a per-tool override.
// A tool policy cannot grant a tool excluded by the source allow list.
func EffectiveToolPolicy(source typ.MCPSourceConfig, name string) (bool, typ.MCPToolUsage) {
	all, allowed := buildAllowList(source.Tools)
	enabled := typ.IsMCPSourceEnabled(source) && (all || allowed[name])
	usage := SourceUsage(source)
	if source.Preset == mcptools.GitHubPreset && !mcptools.GitHubToolAllowed(name) {
		enabled = false
	}
	if policy, ok := source.ToolPolicies[name]; ok {
		if policy.Enabled != nil && !*policy.Enabled {
			enabled = false
		}
		if policy.Usage != nil && !IsServerToolSource(source) {
			usage.Gateway = policy.Usage.Gateway
		}
	}
	return enabled, usage
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

// profileByID returns the profile for an endpoint ID. Default always exists,
// even before the user has saved anything.
func profileByID(cfg *typ.MCPRuntimeConfig, id string) (typ.MCPProfile, bool) {
	for _, p := range cfg.Profiles {
		if p.ID == id {
			return p, true
		}
	}
	if id == typ.MCPDefaultProfileID {
		return typ.MCPProfile{ID: id, Name: "Default"}, true
	}
	return typ.MCPProfile{}, false
}

// ProfileAllows reports whether a profile exposes a tool: the profile is on,
// the tool is in it, and the tool is on in it. Global enablement is checked by
// the caller against the effective tool policy.
func (r *Runtime) ProfileAllows(profileID, normalizedName string) bool {
	if r == nil {
		return false
	}
	cfg := r.GetConfig()
	if cfg == nil {
		return false
	}
	profile, ok := profileByID(cfg, profileID)
	if !ok || profile.Enabled != nil && !*profile.Enabled {
		return false
	}
	return profileToolOn(profile, normalizedName)
}

func profileToolOn(profile typ.MCPProfile, normalizedName string) bool {
	for _, tool := range profile.Tools {
		if tool.Name == normalizedName {
			return tool.Enabled == nil || *tool.Enabled
		}
	}
	// Default holds every tool; only the ones switched off are listed.
	return profile.ID == typ.MCPDefaultProfileID
}

func (r *Runtime) ProfileExists(profileID string) bool {
	cfg := r.GetConfig()
	if cfg == nil {
		return false
	}
	_, ok := profileByID(cfg, profileID)
	return ok
}

// Profiles returns the saved profiles with Default always first.
func Profiles(cfg *typ.MCPRuntimeConfig) []typ.MCPProfile {
	out := []typ.MCPProfile{}
	def, _ := profileByID(cfg, typ.MCPDefaultProfileID)
	out = append(out, def)
	for _, p := range cfg.Profiles {
		if p.ID != typ.MCPDefaultProfileID {
			out = append(out, p)
		}
	}
	return out
}
