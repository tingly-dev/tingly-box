# Scenario Overview

There is no longer a dedicated `/agent` overview page. `/agent` now redirects straight to whichever agent scenario you were last on (`AgentLanding`) — opening the whole card grid before every edit was one more click than necessary once this guide was written. Hiding/showing scenarios moved into the **Agent sidebar itself**, in a lightweight edit mode.

---

![Agent sidebar in edit mode](../images/scenario-overview.png)

## Managing Visibility: Sidebar Edit Mode

Click the **eye icon** in the Agent sidebar's header (next to the collapse icon, visible only while the **Agents** rail item is active) to enter edit mode:

- The icon turns into a **checkmark** while editing; click it again (or elsewhere) to exit
- Every hideable row grows its own small eye-icon toggle on the right — click to hide/show that scenario
- Hidden scenarios stay in the list (dimmed, at ~50% opacity) **below a divider**, after the still-visible rows, instead of disappearing or being interleaved — so the visible part of the list, while editing, already reads as the sidebar you will get once you click Done
- Outside edit mode, hidden rows are simply not shown at all
- A one-time coach mark points at the eye icon the first time you land on a scenario page with more than one sidebar row, so the mechanism is discoverable without a tour; dismissing it (or using the eye icon) doesn't show it again

> Only certain scenarios support hiding; Claude Code always appears in the sidebar (it anchors its Profiles).

Each scenario row also has a **tooltip** showing its description, so you don't need to open a page to recall what it's for.

---

## Full Scenario List

Sidebar order (also the hide-eligibility order):

| Scenario | Path | Description |
|----------|------|-------------|
| Claude Code | `/agent/claude_code` | Route Claude Code with custom profiles and per-task models |
| Claude Desktop | `/agent/claude_desktop` | Connect Claude Desktop as an MCP client through Tingly Box |
| Codex | `/agent/codex` | Configure Codex CLI through your provider keys |
| OpenCode | `/agent/opencode` | Open-source coding agent powered by your provider |
| Pi | `/agent/pi` | Route the Pi coding agent through your provider |
| DeepSeek | `/agent/dsh` | Route DeepSeek Harness (dsh) through your provider — self-hosted Web UI |
| Xcode | `/agent/xcode` | Bring your model into Xcode's coding intelligence |
| VS Code | `/agent/vscode` | Power VS Code Copilot Chat through Tingly Box |
| Cursor | `/agent/cursor` | Bring your model into Cursor, with Cursor compatibility handling on by default (hidden by default) |
| Custom | `/agent/custom` | Bring your own request model name — generic catch-all scenario (hidden by default) |
| OpenAI SDK | `/agent/openai` | Drop-in OpenAI-compatible SDK endpoint |
| Anthropic SDK | `/agent/anthropic` | Drop-in Anthropic-compatible SDK endpoint |
| Embedding | `/agent/embed` | Route embedding requests to your provider |

> "Custom" was previously labeled "OpenClaw" / "Claw Agent" in the sidebar and docs; the path also moved from `/agent/agent` to `/agent/custom`.
> Cursor calls its configured Base URL from **Cursor's own cloud backend**, not from the local Cursor app, so a `localhost` address won't work unless this server is publicly reachable over HTTPS — see [Cursor Scenario](./04-scenario-cursor.md).

By default, only **Custom**, **Pi**, and **Cursor** are hidden; everything else (including Image and Team, below) is visible out of the box.

---

## Power-ups: Team, Image, Remote, and Other Optional Rail Items

**Team** and **Image** are not configuration pages nested under Agent at all — each is its own top-level Activity Bar entry with its own pages (see [Team](./07-team.md) and [Image](./07-image.md)). Whether their rail item shows is still controlled by the same hidden-scenario mechanism as the Agent sidebar above, but the switch for them (and for **Remote**, Bench, MCP Tools, Guardrails, and the Prompt Management sub-features) now lives in one place instead of being spread across pages: the **Power-ups** menu.

Open it from the user-preferences menu at the bottom of the rail (the gear/app icon) → hover or click **Power-ups** to open its submenu beside it:

![Power-ups menu](../images/power-ups-menu.png)

- One row per power-up: icon, name (with an **Experimental** or **Beta** tag where relevant), a one-line description, and a switch
- Clicking a row's label/description (when it's on) navigates straight to that feature's page and closes the menu
- Team, Image and Remote toggle the same hidden-scenario set the Agent sidebar's edit mode uses — turning one off only hides its rail item; nothing it manages (routing rules, running bots, …) stops
- Flag-backed power-ups (Bench, Desk, MCP Tools, Guardrails, and — Full Edition only — the Prompt Management sub-features) use the same `_global` feature flags as [Experimental Features](./19-experimental.md); turning Desk on also shows a warning about who gains access once it's live

---

## Navigation Structure

The left Activity Bar icon for scenarios is labeled **Agent** (previous name: "Scenarios"). Clicking it displays all visible scenario navigation items in the secondary sidebar.

- Each scenario nav item supports direct-click navigation to its configuration page
- Claude Code supports multiple Profiles; each Profile appears as a separate sub-item, grouped right after Claude Code
- The secondary sidebar header has two icons: the **eye** (toggles edit mode, described above) and a **collapse** icon that shrinks the secondary sidebar down to a thin strip (just an expand arrow) for more screen space — click the arrow to expand it back, or hover the collapsed rail to see the sidebar as a temporary flyout without leaving it collapsed. This only affects the secondary panel; the primary Activity Bar icons on the far left always stay visible.
- In a narrow window the sidebar starts collapsed automatically, and clicking a rail item with multiple pages opens its sidebar as a flyout over the content instead of pushing it — so pages stay one click away without permanently eating screen width.

---

## Related Pages

- [Claude Code Scenario](./03-scenario-claude-code.md)
- [Cursor Scenario](./04-scenario-cursor.md)
- [Other Coding Agents](./04-scenario-coding-agents.md)
- [OpenAI / Anthropic SDK Proxy](./05-scenario-sdk-proxy.md)
- [Custom / Embed](./06-scenario-special.md)
- [Team](./07-team.md)
- [Image](./07-image.md)
