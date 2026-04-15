# MCP Tools Architecture

**Last Updated**: 2026-04-15
**Cache Level**: Project
**Expires**: 2026-05-15 (~30 days)
**Branch**: main
**Hash**: 4f39fcc2

## Overview

Tingly-Box implements a Model Context Protocol (MCP) runtime that enables external tool integration for AI models. The system supports two modes: **servertool** (default, intercepts tool calls) and **clienttool** (exposes tools to external MCP clients).

## Components

| Component | Location | Purpose |
|-----------|----------|---------|
| **MCP Runtime** | `internal/mcpruntime/runtime.go` | Core MCP tool discovery, normalization, and execution |
| **MCP Integration** | `internal/server/mcp_integration.go` | Tool injection into AI requests (OpenAI/Anthropic) |
| **MCP Anthropic Loop** | `internal/server/mcp_anthropic_loop.go` | Iterative tool execution for Anthropic APIs |
| **MCP Session** | `internal/mcpruntime/session.go` | Persistent session management per source |
| **Local Mode Handler** | `internal/mcp/local/handler.go` | HTTP API for clienttool mode management |
| **Tool Config Store** | `internal/data/db/tool_config_store.go` | SQLite persistence for MCP configs |
| **MCP Module Handler** | `internal/server/module/mcp/handler.go` | CRUD API for MCP runtime configuration |

## Data Structures

**MCPRuntimeConfig** (`internal/typ/type.go:243-248`):
```go
type MCPRuntimeConfig struct {
    Mode           MCPMode           // "servertool" or "clienttool"
    Sources        []MCPSourceConfig // Multiple MCP server connections
    RequestTimeout int               // Seconds, default: 30
}
```

**MCPSourceConfig** (`internal/typ/type.go:250-275`):
```go
type MCPSourceConfig struct {
    ID        string            // Unique identifier for normalized tool names
    Name      string            // Display name
    Enabled   *bool             // nil = enabled (backwards compatible)
    Transport string            // "http", "stdio", or "sse"
    Endpoint  string            // URL for HTTP/SSE
    Headers   map[string]string // Static headers
    Tools     []string          // Allow-list (empty = all tools)
    Command   string            // Command for stdio
    Args      []string          // Command arguments
    Cwd       string            // Working directory
    Env       map[string]string // Environment variables
    ProxyURL  string            // HTTP proxy URL
    // ... local mode specific fields
}
```

## Current Websearch/Fetch Implementation

**Python Script** (`scripts/mcp_web_tools.py`):
- Standalone MCP stdio server
- Exposes `mcp_web_search` (Serper API) and `mcp_web_fetch` (Jina Reader)
- Requires `SERPER_API_KEY` environment variable
- Implements JSON-RPC 2.0 with MCP specification

**Frontend** (`frontend/src/pages/mcp/MCPBuiltin.tsx`):
- Builtin ID: `webtools`
- Checkboxes for individual tool enablement
- Default config: `python3 mcp_web_tools.py`
- Env passthrough: `HTTP_PROXY`, `HTTPS_PROXY`, `NO_PROXY`

## Dependencies

- **go-mcp-sdk**: MCP protocol implementation for Go
- **Anthropic SDK**: Tool format conversion
- **OpenAI SDK**: Tool format conversion
- **SQLite**: Configuration persistence via `tool_config_store`

## Data Flow

### Servertool Mode (Default)
1. AI request received (`handleOpenAIChatRequest`/`handleAnthropic*Request`)
2. `injectMCPToolsInto*Request()` called
3. For each enabled source:
   - Connect via stdio/HTTP
   - List tools from MCP server
   - Normalize tool names to `tingly_box_mcp__<source_id>__<tool_name>`
4. Merge tools into request (deduplicated by name)
5. AI model returns tool calls
6. `mcp_anthropic_loop.go` executes calls iteratively (up to 6 rounds)
7. Results fed back to AI model

### Clienttool Mode
1. External MCP clients register via HTTP API
2. Clients stored in in-memory registry (`internal/mcp/local/registry.go`)
3. Tingly-box tools exposed to external clients via HTTP endpoint
4. Tool execution flows from external client → tingly-box → upstream AI

## Key Patterns

- **Tool Name Normalization**: All MCP tools prefixed with `tingly_box_mcp__` to prevent naming conflicts
- **Session Caching**: Persistent sessions per source to avoid reconnection overhead
- **Allow-list Filtering**: Tools configured via string array in `Tools` field
- **Deduplication**: Merge functions prevent duplicate tools by name
- **Mode-based Architecture**: Single global switch determines tool flow direction

## Integration Points

- **Request Handlers**: `internal/server/openai_chat.go`, `internal/server/anthropic*.go`
- **Transport Layer**: `internal/mcpruntime/transport_stdio.go`, `internal/mcpruntime/transport_http.go`
- **Frontend API**: `frontend/src/services/api.ts` → `/api/mcp/config`
- **Configuration**: `internal/server/config/config.go` → `GetToolConfig()`/`SetToolConfig()`

## Current Limitations

1. **Global Mode Switch**: `Mode` applies to all tools (servertool OR clienttool, not per-tool)
2. **Python Dependency**: Built-in webtools require Python runtime
3. **SERPER_API_KEY**: Must be set in shell environment, no UI configuration
4. **Tool Registration**: Simple text input for tool names, no granular control
5. **Build/Deploy**: Python script not bundled with binary

## Related Areas

- **Virtual Model Registry**: `internal/virtualmodel/registry.go` - Tool registration for virtual models
- **Protocol Transform**: `internal/protocol/transform/` - Tool call format conversion
- **Scenario Flags**: `internal/server/config/config.go` - `GetScenarioFlag()` for MCP enablement
