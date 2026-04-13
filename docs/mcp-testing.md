# MCP Gateway Testing Guide

This guide tests tingly-box MCP local mode, where tingly-box acts as an MCP server (replacing Bifrost).

## Prerequisites

- tingly-box server running on port 12580
- User token from `~/.tingly-box/config.json`

## Step 1: Verify MCP Mode

Ensure tingly-box is in **clienttool** mode (external clients connect to tingly-box):

```bash
TOKEN=$(cat ~/.tingly-box/config.json | jq -r '.user_token')
curl -s http://localhost:12580/api/v1/mcp/mode \
  -H "Authorization: Bearer $TOKEN" | jq .
```

Expected: `{"success":true,"mode":"clienttool"}`

If mode is "servertool", switch to clienttool:

```bash
curl -s -X PUT http://localhost:12580/api/v1/mcp/mode \
  -H "Authorization: Bearer $TOKEN" \
  -H "Content-Type: application/json" \
  -d '{"mode": "clienttool"}' | jq .
```

## Step 2: Register an MCP Source

Register a filesystem MCP server (or any MCP server from [mcp.so](https://mcp.so)):

```bash
TOKEN=$(cat ~/.tingly-box/config.json | jq -r '.user_token')

curl -s -X PUT http://localhost:12580/api/v1/mcp/config \
  -H "Authorization: Bearer $TOKEN" \
  -H "Content-Type: application/json" \
  -d '{
    "sources": [
      {
        "id": "filesystem",
        "name": "filesystem",
        "transport": "stdio",
        "command": "npx",
        "args": ["-y", "@modelcontextprotocol/server-filesystem", "/Users/your_username/projects"],
        "tools": ["*"],
        "enabled": true
      }
    ]
  }' | jq .
```

Or use the Web UI: http://localhost:12580 → MCP -> Sources → Registered Servers

## Step 3: Verify Tools Discovery

Each endpoint exposes a different set of tools based on the source ID:

```bash
TOKEN=$(cat ~/.tingly-box/config.json | jq -r '.user_token')

# Expose only webtools tools
curl -s -X POST "http://localhost:12580/api/v1/mcp/webtools" \
  -H "Authorization: Bearer $TOKEN" \
  -H "Content-Type: application/json" \
  -d '{
    "jsonrpc": "2.0",
    "id": 1,
    "method": "tools/list",
    "params": {}
  }' | jq '.result.tools | map(.name)'

# Expose only filesystem tools
curl -s -X POST "http://localhost:12580/api/v1/mcp/filesystem" \
  -H "Authorization: Bearer $TOKEN" \
  -H "Content-Type: application/json" \
  -d '{
    "jsonrpc": "2.0",
    "id": 1,
    "method": "tools/list",
    "params": {}
  }' | jq '.result.tools | map(.name)'

# Expose all tools (aggregation)
curl -s -X POST "http://localhost:12580/api/v1/mcp/all" \
  -H "Authorization: Bearer $TOKEN" \
  -H "Content-Type: application/json" \
  -d '{
    "jsonrpc": "2.0",
    "id": 1,
    "method": "tools/list",
    "params": {}
  }' | jq '.result.tools | map(.name)'
```

Expected:
- `/mcp/webtools` returns 2 tools (mcp_web_fetch, mcp_web_search)
- `/mcp/filesystem` returns ~14 filesystem tools
- `/mcp/all` returns the aggregated set of all sources

## Step 4: Connect Claude Code CLI

Register tingly-box as an MCP server in Claude Code CLI using the `tb` endpoint (exposes all tools):

```bash
TOKEN=$(cat ~/.tingly-box/config.json | jq -r '.user_token')
claude mcp add --transport http tb \
  http://localhost:12580/api/v1/mcp/tb \
  --header "Authorization: Bearer $TOKEN"
```

Alternatively, connect only a specific source (e.g. filesystem):

```bash
claude mcp add --transport http filesystem \
  http://localhost:12580/api/v1/mcp/filesystem \
  --header "Authorization: Bearer $TOKEN"
```

Verify the connection:

```bash
# List available tools
claude mcp list

# Or test a tool directly in Claude Code
claude
# Then try: Use tingly_box_mcp__filesystem__read_text_file to read a file
```

## Troubleshooting

### "User authorization header required"

Ensure the `Authorization: Bearer <token>` header is included in every request.

### Tools not appearing

1. Check MCP mode: `GET /api/v1/mcp/mode` should return `"clienttool"`
2. Verify source is registered: `GET /api/v1/mcp/config`
3. Check if source is enabled: Look for `"enabled": true` in the source config

### Connection refused

Ensure tingly-box server is running:

```bash
lsof -i :12580 | grep LISTEN
```

## Architecture

In **clienttool** mode (default):
- tingly-box acts as an MCP server (SSE/HTTP transport)
- External clients (Claude Code CLI, OpenCode, etc.) connect to tingly-box
- tingly-box forwards tool calls to registered MCP sources (stdio processes or HTTP endpoints)
- Each endpoint (`/mcp/<source_id>`) exposes only that source's tools
- The `/mcp/all` endpoint and unknown client names expose all tools as an aggregation

In **servertool** mode:
- tingly-box connects to external MCP servers as a client
- AI model calls are intercepted and tools are injected into the request
- The AI model is unaware of MCP - tools appear as native capabilities
