package tools

// The actual tool implementations live in internal/mcp/builtin_server, which
// the runtime spawns as a stdio MCP server via the `mcp-builtin` command.
// This file only holds the source configuration shared with the rest of the
// codebase (registry, transforms, readiness checks).

const (
	BuiltinWebtoolsSourceID   = "webtools"
	BuiltinWebtoolsSourceName = "Built-in Web Tools"
	BuiltinWebSearchToolName  = "mcp_web_search"
	BuiltinWebFetchToolName   = "mcp_web_fetch"
)

var builtinWebtoolDefaultNames = []string{
	BuiltinWebSearchToolName,
	BuiltinWebFetchToolName,
}

// DefaultBuiltinWebtoolNames returns a copy of default builtin webtools names.
func DefaultBuiltinWebtoolNames() []string {
	out := make([]string, len(builtinWebtoolDefaultNames))
	copy(out, builtinWebtoolDefaultNames)
	return out
}

// BuiltinWebtoolsSource defines the built-in webtools source configuration
var BuiltinWebtoolsSource = map[string]interface{}{
	"id":         BuiltinWebtoolsSourceID,
	"name":       BuiltinWebtoolsSourceName,
	"transport":  "builtin", // Special transport for built-in tools
	"enabled":    true,
	"visibility": "client",
	"tools":      DefaultBuiltinWebtoolNames(),
	"env": map[string]string{
		"SERPER_API_KEY": "${SERPER_API_KEY}", // User provides via UI
	},
}
