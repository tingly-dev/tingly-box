# Scenario Overview

This chapter is the map of the shell: the icon rail, the **Agent** sidebar and the **Power-ups** menu. There is no `/agent` overview page any more — `/agent` opens whichever agent page you were last on (the Agent rail item does the same), and a brand-new install lands there with the sidebar expanded, where each agent's Quick Start takes over.

---

## The Rail

![Agent sidebar](../images/scenario-overview.png)

The narrow strip on the far left. Top to bottom:

| Item | Opens | Shown |
|------|-------|-------|
| **Dashboard** | Usage, Team usage, Quota history — [Usage Dashboard](./11-dashboard.md) | always |
| **Agent** | the agent pages (this chapter) | always |
| **Team** | Teams and Team Keys — [Team](./07-team.md) | default; switch in Power-ups |
| **Image** | Playground, profiles, Library, Image API — [Image](./07-image.md) | default; switch in Power-ups |
| **Projects** | Claude Code in a folder or a project — [Projects](./22-projects.md); an orange **dot** means a thread is waiting on you | Projects power-up on |
| **IM** | Bots, Remote Control, IM Notify — [Remote](./12-remote-control.md) | Projects power-up on |
| **Power-ups** | the pages of the opt-in features that are on: Guardrails, Prompt (User Request, Skills), Bench | at least one of them on |
| **Tools** | MCP and Servertool — [MCP & Tools](./16-mcp-tools.md) | MCP power-up on |
| **Credential** | Credentials, and VModel — [Credentials](./08-credentials.md) | always |
| **Help** (lightbulb) | [Tips & Help](./13-help.md) | always |
| **Star** | opens the project on GitHub (the one place the app asks for a star) | always |
| **gear** (bottom) | the *Preferences* popover — language, theme, **Power-ups**, **All settings**, **Feedback** — [System Settings](./17-system-settings.md) | always |

The logo at the top opens the GitHub repository; its tooltip shows the version. **System** has no rail icon of its own: **gear → All settings** opens it. When the server can't be reached, a red **Offline** marker appears just above the gear.

The Agent sidebar, like every sidebar, ends with a **version row** (the version plus a badge: green check = up to date, orange arrow = update available — click for the Update dialog) and the slogan *Your Intelligence, Orchestrated.*

---

## The Agent Sidebar

Opening **Agent** lists every agent, in groups:

1. **Claude Code** (captioned `default`), then each of its **profiles** — named by their own name, captioned `Claude Code · <id>` — and **New profile**
2. Coding agents: **Claude Desktop**, **Codex**, **OpenCode**, **Pi**, **WorkBuddy**, **DeepSeek**, **Xcode**, **VS Code**, **Cursor**, **Custom**
3. SDK endpoints: **OpenAI SDK**, **Anthropic SDK**, **Embedding**, **Decisions**

Hover a row for a tooltip describing it. The header carries two icons: the **eye** (edit mode, below) and **collapse** (shrinks the sidebar to a thin strip; hover or click a rail item to see it as a flyout without expanding it for good — a narrow desktop window (900–1200 px) starts collapsed once you have used the toggle at least once — a first-time user always starts expanded — and expanding it by hand there lasts only for the session; on phone/tablet widths the whole navigation becomes a drawer behind the top bar's menu button).

### Pinning an agent

**Hover an agent row and click the pin** (*Pin to top*) to move it to the top of the list, above Claude Code, in the order you pinned them, separated from the rest by a divider. Click the filled pin (*Unpin*) to put it back. Pins are remembered in this browser.

### Managing Visibility: Sidebar Edit Mode

Click the **eye** in the sidebar header (visible while the **Agent** item is active) to enter edit mode:

- The icon becomes a **check** (*Done*); click it again to exit
- Every hideable row grows a small eye toggle on the right — *Hide from sidebar* / *Show in sidebar*
- Hidden agents stay in the list, **dimmed, below a divider**, so the visible part already reads as the sidebar you'll get
- Outside edit mode, hidden rows aren't shown at all
- A one-time coach mark (*Choose which agents show*) points at the eye the first time you land on an agent page

Everything is visible out of the box **except Cursor** (it needs a publicly reachable HTTPS address — see [Cursor](./04-scenario-cursor.md)). Claude Code and its profiles always show unless you hide Claude Code itself, which hides its profiles with it.

---

## Full Agent List

| Agent | Path | What it's for |
|-------|------|---------------|
| Claude Code | `/agent/claude_code` | Route Claude Code with custom profiles and per-task models |
| Claude Desktop | `/agent/claude_desktop` | Use your own providers as Claude Desktop's third-party inference gateway |
| Codex | `/agent/codex` | Configure Codex CLI through your provider keys |
| OpenCode | `/agent/opencode` | Open-source coding agent powered by your provider |
| Pi | `/agent/pi` | Route the Pi coding agent through your provider |
| WorkBuddy | `/agent/workbuddy` | Use your own models in Tencent WorkBuddy as a custom model |
| DeepSeek | `/agent/dsh` | Route the DeepSeek Harness (dsh) coding agent through your provider |
| Xcode | `/agent/xcode` | Bring your model into Xcode's coding intelligence |
| VS Code | `/agent/vscode` | Power VS Code Copilot Chat through Tingly Box |
| Cursor | `/agent/cursor` | Bring your model into Cursor, with compatibility handling built in (hidden by default) |
| Custom | `/agent/custom` | Bring your own request model name — a generic catch-all |
| OpenAI SDK | `/agent/openai` | Drop-in OpenAI-compatible SDK endpoint |
| Anthropic SDK | `/agent/anthropic` | Drop-in Anthropic-compatible SDK endpoint |
| Embedding | `/agent/embed` | Route embedding requests to your provider |
| Decisions | `/agent/decisions` | Route decision requests to your provider |

> "Custom" was previously labeled "OpenClaw" / "Claw Agent"; its path moved from `/agent/agent` to `/agent/custom`. Cursor calls its Base URL from **Cursor's own cloud**, so `localhost` won't work — see [Cursor Scenario](./04-scenario-cursor.md).

Every agent page shares one layout — see [Claude Code § Page Structure](./03-scenario-claude-code.md#page-structure) — and the pages for [Codex](./04-scenario-codex.md), [the other coding agents](./04-scenario-coding-agents.md), [the SDK endpoints](./05-scenario-sdk-proxy.md) and [Custom / Embed](./06-scenario-special.md) only describe what differs.

---

## Power-ups: the menu behind the gear

Optional capabilities live in one place. Click the **gear** at the foot of the rail, then hover (or click) **Power-ups** to open its submenu beside the popover:

![Power-ups menu](../images/power-ups-menu.png)

*Optional capabilities. Turn one on to add it to the navigation bar on the left; turn it off to hide it.* One row per power-up — icon, name (with an **Exp.** or **Beta** tag), a one-line description and a switch; clicking an enabled row's label opens its page:

| Power-up | Effect | Stage |
|----------|--------|-------|
| **Team** | the **Team** rail item | — |
| **Image** | the **Image** rail item (and its agent) | — |
| **Virtual Models** | the **VModel** row under Credential (the built-in synthetic providers keep working either way) | — |
| **Bench** | the **Bench** page | Beta |
| **Projects** | the **Projects** and **IM** rail items; shows `active n / m` bots; a warning explains who can then run Claude Code on this machine | Beta |
| **MCP Tools** | the **Tools** rail item | Exp. |
| **Guardrails** | the **Guardrails** pages | Exp. |
| **User prompts** / **Skills** | the **User Request** / **Skills** pages | Exp. |

- **Team**, **Image** and **Virtual Models** only show or hide an item — nothing they manage stops. The flag-backed ones (Bench, Projects, MCP, Guardrails, the Prompt pages) are the same `_global` flags as [Experimental Features](./19-experimental.md).
- A first-visit coach mark (*Power-ups moved here*) points at the gear.
- The former **Remote** and **Desk** switches are gone: IM chats now run on **Projects**, so one switch turns on both (see [Projects](./22-projects.md)).

---

## Related Pages

- [Claude Code Scenario](./03-scenario-claude-code.md)
- [Codex](./04-scenario-codex.md) · [Cursor](./04-scenario-cursor.md) · [Other Coding Agents](./04-scenario-coding-agents.md)
- [OpenAI / Anthropic SDK Proxy](./05-scenario-sdk-proxy.md) · [Custom / Embed](./06-scenario-special.md)
- [Team](./07-team.md) · [Image](./07-image.md) · [Projects](./22-projects.md)
