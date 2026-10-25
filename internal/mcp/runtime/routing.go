package runtime

import (
	"context"
	"net/url"
	"path/filepath"

	"github.com/tingly-dev/tingly-box/internal/typ"
)

// RouteSource is a safe display projection, never a connection configuration.
// Credentials, environment values, URL query strings and command args are omitted.
type RouteSource struct {
	ID         string        `json:"id"`
	Name       string        `json:"name"`
	Origin     string        `json:"origin"`
	Transport  string        `json:"transport"`
	Address    string        `json:"address"`
	State      string        `json:"state"`
	Error      string        `json:"error,omitempty"`
	Processing string        `json:"processing"`
	Advisor    *RouteAdvisor `json:"advisor,omitempty"`
	Tools      []CatalogTool `json:"tools"`
}
type RouteAdvisor struct {
	ProviderUUID string `json:"provider_uuid"`
	ProviderName string `json:"provider_name,omitempty"`
	Model        string `json:"model"`
}
type ProfileRoute struct {
	ID       string        `json:"id"`
	Name     string        `json:"name"`
	Endpoint string        `json:"endpoint"`
	Enabled  bool          `json:"enabled"`
	Default  bool          `json:"default"`
	Sources  []RouteSource `json:"sources"`
}
type RoutingSnapshot struct {
	Sources     []RouteSource  `json:"sources"`
	Profiles    []ProfileRoute `json:"profiles"`
	ServerTools []RouteSource  `json:"server_tools"`
}

// Routing derives reachability from the same effective policies used by both
// the MCP bridge and model tool injection. Ordinary tools and server tools are
// independent uses; a source can appear in both without becoming two configs.
func (r *Runtime) Routing(ctx context.Context) RoutingSnapshot {
	out := RoutingSnapshot{Sources: []RouteSource{}, Profiles: []ProfileRoute{}, ServerTools: []RouteSource{}}
	catalog := r.Catalog(ctx, "")
	cfg := r.GetConfig()
	if cfg == nil {
		return out
	}
	statuses := map[string]SourceStatus{}
	for _, s := range catalog {
		statuses[s.SourceID] = s
	}
	for _, source := range cfg.Sources {
		status := statuses[source.ID]
		route := RouteSource{ID: source.ID, Name: source.Name, Origin: "external", Transport: source.Transport, Address: routeAddress(source), State: status.State, Error: routeError(source, status.Error), Processing: "standard", Tools: []CatalogTool{}}
		if route.Name == "" {
			route.Name = source.ID
		}
		if route.State == "" {
			route.State = "unchecked"
		}
		if SourceProvider(source) == typ.ToolProviderBuiltin {
			route.Origin = "builtin"
		}
		if SourceImplementation(source) == typ.ToolImplementationVirtual {
			route.Processing = "advisor"
			if source.Advisor != nil {
				route.Advisor = &RouteAdvisor{ProviderUUID: source.Advisor.ProviderUUID, Model: source.Advisor.Model}
			}
		}
		for _, tool := range status.Tools {
			tool.Enabled, tool.Usage = EffectiveToolPolicy(source, tool.Name)
			route.Tools = append(route.Tools, tool)
		}
		out.Sources = append(out.Sources, route)
		serverRoute := route
		serverRoute.Tools = []CatalogTool{}
		for _, tool := range route.Tools {
			if tool.Enabled && tool.Usage.Gateway {
				serverRoute.Tools = append(serverRoute.Tools, tool)
			}
		}
		// Keep failing or unconfigured sources visible when they are intended for
		// server use; never silently turn a failed branch into an empty success.
		if len(serverRoute.Tools) > 0 || route.State != "connected" && sourceIntendedFor(source, "gateway") {
			out.ServerTools = append(out.ServerTools, serverRoute)
		}
	}
	for _, profile := range Profiles(cfg) {
		route := ProfileRoute{ID: profile.ID, Name: profile.Name, Endpoint: "/api/v1/mcp/" + url.PathEscape(profile.ID), Enabled: profile.Enabled == nil || *profile.Enabled, Default: profile.ID == typ.MCPDefaultProfileID, Sources: []RouteSource{}}
		if route.Name == "" {
			route.Name = route.ID
		}
		for i, source := range cfg.Sources {
			sourceRoute := out.Sources[i]
			sourceRoute.Tools = []CatalogTool{}
			for _, tool := range out.Sources[i].Tools {
				if route.Enabled && tool.Enabled && tool.Usage.Client && r.ProfileAllows(profile.ID, tool.NormalizedName) {
					sourceRoute.Tools = append(sourceRoute.Tools, tool)
				}
			}
			intended := route.Enabled && profileListsSource(profile, source) && sourceIntendedFor(source, "client")
			if len(sourceRoute.Tools) > 0 || intended && sourceRoute.State != "connected" {
				route.Sources = append(route.Sources, sourceRoute)
			}
		}
		out.Profiles = append(out.Profiles, route)
	}
	return out
}

// profileListsSource reports whether a profile can reach any tool of the source,
// so a failing source stays visible instead of silently vanishing from the route.
func profileListsSource(profile typ.MCPProfile, source typ.MCPSourceConfig) bool {
	if profile.ID == typ.MCPDefaultProfileID {
		return true
	}
	for _, tool := range profile.Tools {
		if id, _, ok := ParseNormalizedToolName(tool.Name); ok && id == source.ID {
			return true
		}
	}
	return false
}
func sourceIntendedFor(source typ.MCPSourceConfig, usage string) bool {
	if !typ.IsMCPSourceEnabled(source) {
		return false
	}
	eligible := SourceUsage(source)
	for name := range source.ToolPolicies {
		enabled, toolUsage := EffectiveToolPolicy(source, name)
		if enabled {
			eligible.Client = eligible.Client || toolUsage.Client
			eligible.Gateway = eligible.Gateway || toolUsage.Gateway
		}
	}
	if usage == "client" {
		return eligible.Client
	}
	return eligible.Gateway
}
func routeAddress(source typ.MCPSourceConfig) string {
	if source.Endpoint != "" {
		parsed, err := url.Parse(source.Endpoint)
		if err == nil {
			parsed.User = nil
			parsed.RawQuery = ""
			parsed.Fragment = ""
			return parsed.String()
		}
		return ""
	}
	return filepath.Base(source.Command)
}

// Discovery errors can contain the complete endpoint or transport diagnostics.
// The route projection exposes a useful failure classification without secrets.
func routeError(source typ.MCPSourceConfig, detail string) string {
	if detail == "" {
		return ""
	}
	if source.Transport == "advisor" {
		return "Advisor configuration or connection failed"
	}
	return "Tool source discovery failed; check connection configuration"
}
