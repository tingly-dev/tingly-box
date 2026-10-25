package typ

// MCPToolUsage separates downstream MCP exposure from model tool injection.
// A nil usage preserves the legacy visibility default.
type MCPToolUsage struct {
	Client  bool `json:"client"`
	Gateway bool `json:"gateway"`
}

type MCPToolPolicy struct {
	Enabled *bool         `json:"enabled,omitempty"`
	Usage   *MCPToolUsage `json:"usage,omitempty"`
}

// MCPDefaultProfileID is the one profile that always exists. It contains every
// tool; Tools holds only the entries switched off in it.
const MCPDefaultProfileID = "default"

// MCPProfileTool is one tool in a profile, addressed by normalized name.
// A nil Enabled means on; off keeps the tool listed.
type MCPProfileTool struct {
	Name    string `json:"name"`
	Enabled *bool  `json:"enabled,omitempty"`
}

// MCPProfile is one entry point to the gateway. A custom profile exposes only
// the tools it lists; Default exposes all tools except those switched off.
type MCPProfile struct {
	ID      string           `json:"id"`
	Name    string           `json:"name"`
	Enabled *bool            `json:"enabled,omitempty"`
	Tools   []MCPProfileTool `json:"tools"`
}
