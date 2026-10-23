# Experimental Features

Path: `/system/experimental` (**System → Experimental**)

![Experimental Features](../images/experimental.png)

The page lists the capabilities that are still in early iteration, each with an **On / Off** chip. Turning one on makes its rail item and pages appear; turning it off hides them but **keeps your data and configuration**. The same switches also live in the rail gear's **Preferences → Power-ups** menu (which adds Team, Image and Virtual Models, and tags each row *Exp.* / *Beta*) — the two are the same global flags, so use whichever is closer. See [Scenario Overview § Power-ups](./02-scenario-overview.md#power-ups-the-menu-behind-the-gear).

> There is no longer an edition split: the desktop app shows exactly the same features as the browser and CLI (the old *lite* edition is gone), so nothing here is "Full Edition only".

If you open a page whose feature is off — a bookmark to `/guardrails`, say — you are sent here with the message *`<feature> is off. Turn it on below to continue.`*; switching it on takes you straight back to that page.

---

## Features (in page order)

### User prompts and Skills

**Flags**: `skill_user`, `skill_ide`

- **User prompts** — *Manage reusable instructions for user requests*: the **User Request** page (`/prompt/user`)
- **Skills** — *Manage code snippets and skills from your IDE*: the **Skills** page (`/prompt/skill`)

Both appear under the **Power-ups** rail item. See [Prompt Management](./14-prompt-management.md).

### Guardrails

**Flag**: `guardrails` — *Block risky tool calls and filter sensitive outputs*

Adds the **Guardrails** group (Overview, Policy Groups, Policies, Secrets, History) under **Power-ups**. When on, an info alert reminds you that the Guardrails page is available. See [Guardrails](./15-guardrails.md).

### MCP Tools

**Flag**: `mcp` — *MCP (Model Context Protocol) tools such as web search and web fetch*

Adds the **Tools** rail item: **MCP** (registered servers, local mode) and **Servertool**. See [MCP & Tools](./16-mcp-tools.md).

### Bench

**Flag**: `bench` (tagged *Beta* in Power-ups) — *The probe test workbench — raw client requests, flag overlays and payload editing*

Adds **Bench** (`/bench`) under **Power-ups** — *one probe request with every knob resident*. Three columns: **Compose** (*what do I send?*) → **Request** (*what the client sends*) → **Payload** (*what actually goes out*), with the **Result** (*what happened?*) under the request. **Run** (⌘/Ctrl + Enter) fires exactly that request.

- **Target** — pick a provider and model; **Protocol** and **Content** (a one-message *Message*, *Blank*, *Multi-turn*, *Tool round-trip*, *Image*, *Mid-conversation system*, or your own *Custom* body in the protocol's wire format)
- **Scope** — *Through TB* (full routing and middleware) or *Direct* (bypass Tingly-Box, to tell upstream failures from gateway ones)
- **Parameters** — *Nonstream / Stream*, a **Thinking** slider, and **Tool** / **Vision** presets
- **Plugins** — overlay rule flags for this request only (nothing is saved; flags are TB middleware, so they don't apply in *Direct* scope)

It is the roomier sibling of the per-rule [probe panel](./20-routing-rules.md#probe-panel): same engine, but you leave the rule to experiment freely.

![Bench](../images/bench.png)

### Projects

**Flag**: `projects` (tagged *Beta*) — *Use Claude Code from the web and your IM chats: run it in a folder you choose, or hand a project its work and a coordinator runs each task as a thread in its own git worktree.*

Adds the **Projects** and **IM** rail items. It is **off by default** because it lets Claude Code run on this machine: while on, a warning reads *Anyone who can sign in to this tingly-box, or who may use Remote Control through one of its bots, can now run Claude Code on this machine, in the folders and repositories they choose, and approve the tool calls it makes.* Installs that were already chatting with bots through Remote get the switch turned on automatically once, so those chats keep working. See [Projects](./22-projects.md) and [Remote](./12-remote-control.md).

---

## How to Enable

1. Open the **gear → All settings → Experimental** (or the **Power-ups** submenu)
2. Click the **Off** chip of the feature to switch it **On**
3. The rail updates immediately; if you were sent here from a gated page, you return to it

To disable, switch the chip **Off**.

---

## Related Pages

- [Guardrails](./15-guardrails.md)
- [MCP & Tools](./16-mcp-tools.md)
- [Prompt Management](./14-prompt-management.md)
- [Projects](./22-projects.md)
- [System Settings](./17-system-settings.md)
