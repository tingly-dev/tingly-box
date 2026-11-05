# MCP & Tools

Paths: `/mcp/tools` (**MCP Tools**) · `/mcp/profile/:profileId` (**MCP profiles**; `/mcp/profile/default` is the main one) · `/mcp/server-tools` (**Server Tools**)

MCP (Model Context Protocol) lets your coding agents use extra tools — a GitHub connection, a knowledge base, a local script. Tingly-Box connects to those tool services once, then offers them to each agent through an MCP endpoint of its own. A few tools run *inside* the gateway instead (web search/fetch, Advisor) and need no MCP at all.

> **Where it lives.** The pages sit under the **Power-ups** rail item and are always there. The *MCP Tools* switch in the [Power-ups menu](./02-scenario-overview.md#power-ups-the-menu-behind-the-gear) (or [Experimental Features](./19-experimental.md)) turns MCP *execution* on; while it is off the MCP pages say *MCP execution is off. Your configuration is saved; enable execution to use it.* and offer an **Enable MCP** button.

## Quick Start

Give your coding agents extra tools — connect an MCP server once, then hand its tools to Claude Code, Codex or OpenCode.

1. Click **Power-ups** in the rail → **MCP Tools** (`/mcp/tools`). If a banner says *MCP execution is off*, click **Enable MCP**.
2. Click **Connect tools**. Under **Any MCP** give it a **Connection name** plus a service URL (**Remote service**) or a start command (**Local command**) — or pick **GitHub** and paste a personal access token. Click **Connect and inspect tools**; the tools it discovers join the Default profile at once.
3. Click **Power-ups** → **MCP Integration** (the Default MCP profile). In **Add to client** pick **Claude Code**, **Codex** or **OpenCode**, copy the command (it already contains your endpoint and token) and run it — for Claude Code that is `claude mcp add --transport http tb "<Base URL>/api/v1/mcp/default" --header "Authorization: Bearer …"`.
4. Click **Verify this profile**.

**Verify:** a green message reports *Actual gateway initialization and tool discovery passed* with the tool count, and the agent now lists the `tb` server (Claude Code: `/mcp`). Web search/fetch and Advisor are not MCP — they run inside the gateway and live on [Server Tools](#server-tools-mcpserver-tools).

---

## How the pieces fit

| Piece | What it is | Page |
|-------|------------|------|
| **MCP Tools** | the connections to tool services and the tools they offer; each tool has one global on/off switch | `/mcp/tools` |
| **MCP profile** | one entry point (an endpoint URL) for an agent, holding the set of tools that agent may use | `/mcp/profile/:profileId` |
| **Server Tools** | tools the gateway itself runs during a model request — no MCP connection, no profile | `/mcp/server-tools` |

A tool switched off on **MCP Tools** is off in every profile. The **Power-ups** sidebar lists, in order: **Server Tools**, **MCP Tools**, one row per profile (the default is **MCP Integration**, captioned *Default MCP profile*) and **New MCP Profile**.

---

## MCP Tools (`/mcp/tools`)

*Connect any MCP and manage its tools. A tool switched off here is off everywhere.*

![MCP Tools: a connection card per MCP with its tools, switches and a Connect tools button](../images/mcp-tools.png)

**Connect tools** (top right) opens the dialog of the same name. Pick **Any MCP** or **GitHub**:

- **Any MCP** — a **Connection name**, then **Remote service** (an HTTP **Endpoint URL**), **Local command** (a STDIO server: command, environment variables, working directory) or **SSE** (an SSE URL); authentication, proxy and an allow list (`*` discovers every tool) sit under *Authentication and advanced connection settings*
- **GitHub** — a fixed, curated subset of the official GitHub MCP server (read repositories, issues, pull requests and Actions logs, plus comments, issues and pull requests). All it needs is a **GitHub personal access token**; use a fine-grained token limited to the repositories you want

![The Connect tools dialog: Any MCP with Remote service, Local command and SSE, or GitHub](../images/mcp-connect-tools.png)

**Connect and inspect tools** discovers the tools and adds them to the Default profile right away.

Each connection is a card: its name and origin (built-in or external), a status — **Connected**, **Checking…**, **Off**, **Needs attention** or **Failed** — and a **Settings** button (**Fix connection** when it failed), then its address or command with small chips (auth headers, env vars, proxy; amber *Needs …* chips for anything missing). Under it the connection's tools are compact rows:

- a switch per tool (`3 of 12 tools on`), a **Filter tools…** box and bulk **Turn on** / **Turn off** for all or the matching tools
- the tool's parameters as a copyable JSON schema, and **Test tool** — enter **Arguments (JSON)** and **Run test**
- a tool excluded by the connection's allow list says so and points you to the connection settings

![A tool row expanded to show its parameters as a copyable JSON schema, with Test tool](../images/mcp-tool-schema.png)

**Settings** opens the connection's drawer: enable or disable the whole connection (off turns all its tools off in every profile), edit the address or command and authentication, or remove the connection. Below the cards, **Choose which tools each profile exposes** jumps to the Default profile.

---

## MCP profiles (`/mcp/profile/:profileId`)

*An MCP profile is one entry point to your tools. Default has every tool; create others to give an agent only what it needs.*

![The Default MCP profile (MCP Integration): endpoint URL, Add to client, and every tool with its switch](../images/mcp-profile.png)

- **Header** — the profile's name (the default is **MCP Integration**), rename and remove icons for custom profiles, a **Profile on** switch, a refresh button and **Verify this profile**; custom profiles also get **Add tools**
- **Connection** — the **Endpoint URL** (`<Base URL>/api/v1/mcp/<profile id>`, with a copy button) and **Add to client**: choose **Claude Code**, **Codex** or **OpenCode** and copy the registration with your endpoint and token already written in:
  - Claude Code: `claude mcp add --transport http tb "<endpoint>" --header "Authorization: Bearer <token>"` (a custom profile uses the server name `tb-<profile-name>`)
  - Codex: a `[mcp_servers.tb]` block for `~/.codex/config.toml`
  - OpenCode: an `mcp` entry for `~/.config/opencode/opencode.json`
- **Tools in this MCP profile** — every tool with its own switch (`N of M tools on`). **Default always contains every tool, including ones you connect later**; switching one off hides it from this profile only. A **custom** profile holds only the tools you add with **Add tools** — new tools are never added automatically. A tool that is off on the **MCP Tools** page shows *Off on the MCP Tools page, so it is unavailable here*
- A GitHub connection can be limited to **GitHub repositories** (`owner/repo`) per profile; without a list, its tools reach every repository the token can

**Verify this profile** runs a real initialization and tool discovery against the gateway and reports the number of tools. **New MCP Profile** (sidebar) asks for a name. Removing a profile cuts off the agents connected to it; the tools and other profiles are untouched.

---

## Server Tools (`/mcp/server-tools`)

*Provided and run by Tingly Box on the server. Configure each service and enable its tools; no MCP connection or profile is needed.* They are injected into model requests and are never offered through MCP profiles.

![Server Tools: Web tools (Search, Fetch) and Advisor](../images/mcp-server-tools.png)

- **Web tools** — **Search** uses Serper and needs a **Serper API key**; **Fetch** uses Jina Reader and needs none. Open **Web tool settings**, enter the key, **Save web tool settings**, then enable Search and Fetch separately. Ready tools replace native web tools on supported model routes. Search queries go to Serper and fetched URLs to Jina Reader, so enable only what you want the gateway to run
- **Advisor** (*Experimental*) — the gateway consults a second model and returns its advice to the current model. **Configure Advisor model** → **Choose consultation model**; until one is set the card says *Needs a consultation model*

---

## Old addresses

These still work and redirect: `/mcp/sources` → `/mcp/tools`; `/tools/servertool` → `/mcp/server-tools`; `/mcp`, `/mcp/local-mode`, `/mcp/clients`, `/mcp/routes` and `/mcp/profiles` → `/mcp/profile/default`.

---

## Related Pages

- [Experimental Features](./19-experimental.md)
- [Guardrails](./15-guardrails.md)
- [Claude Code Scenario](./03-scenario-claude-code.md)
