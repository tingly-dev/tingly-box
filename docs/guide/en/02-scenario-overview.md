# Scenario Overview

Path: `/agent`

---

![Scenario Overview](../images/scenario-overview.png)

## Page Function

The **Agents** page is Tingly-Box's agent navigation hub, displaying all available scenarios as a card grid. Page subtitle: *"Pick a scenario to configure. Hide the ones you don't use to keep the sidebar tidy."*

### Scenario Cards

Each card contains:
- **Icon**: The logo of the tool/platform the scenario represents
- **Name**: Scenario name (e.g. Claude Code, Codex, OpenCode)
- **Description**: A two-line truncated summary
- **Status line**: The card's live configuration state — a rule count (e.g. `3 rules`) if any routing rules exist, or `Not configured yet` if none do — so the page answers "what have I already set up?" at a glance
- **Hidden badge**: Gray `Hidden` badge shown on hidden scenarios

### Visibility Management

A small **eye icon** in the card's top-right corner controls whether the scenario appears in the left sidebar (Activity Bar). It's hidden by default and only appears on hover (or always-on for already-hidden cards), so it doesn't compete with the scenario name/description for attention.

- Click to hide → scenario is hidden from the sidebar but still directly accessible via the overview page (shown with a `Hidden` badge and a crossed-out eye icon)
- Click to unhide → scenario reappears in the sidebar

> Only certain scenarios support hiding; Claude Code always appears in the sidebar.

---

## Full Scenario List

Card grid order (also the sidebar order):

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
| Image Playground | `/agent/image` | Generate and edit images through Tingly Box, with an inline test bench |
| Team | `/agent/team` | Isolated multi-team workspaces with their own routing and sharing keys (hidden by default) |

> "Custom" was previously labeled "OpenClaw" / "Claw Agent" in the sidebar and docs; the path also moved from `/agent/agent` to `/agent/custom`.
> "Image Playground" absorbed the old standalone Playground page — `/agent/imagegen` and `/agent/playground` both redirect to `/agent/image` now.
> Cursor calls its configured Base URL from **Cursor's own cloud backend**, not from the local Cursor app, so a `localhost` address won't work unless this server is publicly reachable over HTTPS — see [Cursor Scenario](./04-scenario-cursor.md).

---

## Navigation Structure

The left Activity Bar icon corresponds to the **Scenarios** group. Clicking it displays all visible scenario navigation items in the secondary sidebar.

- Each scenario nav item supports direct-click navigation to the configuration page
- Claude Code supports multiple Profiles; each Profile appears as a separate sub-item
- The secondary sidebar header has two icons: a **pencil** (jumps back to this overview page to manage visibility) and a **collapse** icon that shrinks the secondary sidebar down to a thin strip (just an expand arrow) for more screen space — click the arrow to expand it back. This only collapses the secondary panel; the primary Activity Bar icons on the far left always stay visible.

---

## Related Pages

- [Claude Code Scenario](./03-scenario-claude-code.md)
- [Cursor Scenario](./04-scenario-cursor.md)
- [Other Coding Agents](./04-scenario-coding-agents.md)
- [OpenAI / Anthropic SDK Proxy](./05-scenario-sdk-proxy.md)
- [Custom / Embed / Image Playground / Team](./06-scenario-special.md)
