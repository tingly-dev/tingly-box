package tools

// The actual tool implementations live in internal/mcp/builtin_server, which
// the runtime spawns as a stdio MCP server via the `mcp-builtin` command.
// This file only holds the shared source/tool identifiers; the live source
// config is built in internal/mcp/runtime/builtin_registry.go.

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
