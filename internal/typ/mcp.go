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

// MCPClientProfile is a downstream grant, independent of upstream connections.
// Empty grants expose no tools; ["*"] explicitly grants all eligible tools.
type MCPClientProfile struct {
	ID      string   `json:"id"`
	Name    string   `json:"name"`
	Enabled *bool    `json:"enabled,omitempty"`
	Sources []string `json:"sources"`
	Tools   []string `json:"tools"` // normalized tool names
}
