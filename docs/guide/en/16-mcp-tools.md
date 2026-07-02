# MCP & Tools

Paths: `/mcp/sources`, `/mcp/local-mode`, `/tools/servertool`

![MCP & Tools](../images/mcp.png)

MCP (Model Context Protocol) tool support allows registering external tool servers for Claude Code and other scenarios, including built-in web tools and custom MCP servers.

> **Note**: The MCP feature must be enabled on the [Experimental Features](./19-experimental.md) page before the sidebar entry appears.

---

## MCP Registered Servers (`/mcp/sources`)

### Two-Step Setup

**Step 1: Install Agent**

The top of the page shows agent installation instructions for configuring Tingly-Box as an MCP proxy, including a one-click-copy CLI install command.

**Step 2: Configure Tools**

Two sections:

#### Built-in Web Tools

| Tool | Description |
|------|-------------|
| **mcp_web_search** | Web search tool — requires Serper API Key configuration |
| **mcp_web_fetch** | Web content fetching (Jina Reader integration) |

Each tool has an independent toggle (enable/disable) and required configuration fields (e.g. API Key input).

#### Custom MCP Servers

**Toolbar:**
- **Add Server**: Add a new custom MCP server
- Status filter: All / Active / Disabled

**Server list:**

| Column | Description |
|--------|-------------|
| Server ID | Unique server identifier |
| Connection | Connection info (command/URL) |
| Transport | Transport type badge: STDIO / HTTP / SSE |
| Visibility | Client-side or Server-side |
| Status | Enable/disable toggle |
| Actions | Edit, delete |

**Custom server configuration:**
- Server ID
- Transport type (STDIO / HTTP / SSE)
- Connection parameters (command or URL)
- Visibility setting

---

## MCP Local Mode (`/mcp/local-mode`)

![MCP Local Mode](../images/mcp-local-mode.png)

Configure Claude Code to use Tingly-Box as an MCP server.

The top of the page shows the current status:
- **Active** badge (green): MCP service is running and external clients can connect
- Info banner: `Tingly-Box is running in Client Tool mode. Register MCP sources in the Sources page, then connect your MCP client using the instructions below.`

### Connection Information

Displays the **MCP Endpoint URL** — the complete endpoint (including auth) that Claude Code needs to connect.

### Connect Claude Code

**Method 1: Claude CLI**

```bash
claude mcp add --transport http tb "<mcp-endpoint-url>" \
  --header "Authorization: Bearer $(cat ~/.tingly-box/config.json | jq -r '.user_token')"
```

The command auto-reads the User Token from `~/.tingly-box/config.json` as the Bearer Token — no manual token entry needed.

**Method 2: Manual Configuration File**

Add the following to Claude Desktop's configuration file (includes the Authorization header):

```json
{
  "mcpServers": {
    "tb": {
      "url": "<mcp-endpoint-url>",
      "headers": { "Authorization": "Bearer <your-token>" }
    }
  }
}
```

The page notes the default configuration file path for each OS.

---

## Server Tool (`/tools/servertool`)

Path: `/tools/servertool`

View and test the MCP tools currently available on the Tingly-Box server side.

---

## Related Pages

- [Experimental Features](./19-experimental.md)
- [Guardrails](./15-guardrails.md)
- [Claude Code Scenario](./03-scenario-claude-code.md)
