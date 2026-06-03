# Scenario Overview

Path: `/agent`

---

![Scenario Overview](../images/scenario-overview.png)

## Page Function

The **Scenarios** page is Tingly-Box's agent navigation hub, displaying all available scenarios as a card grid.

### Scenario Cards

Each card contains:
- **Icon**: The logo of the tool/platform the scenario represents
- **Name**: Scenario name (e.g. Claude Code, Codex, OpenCode)
- **Description**: A two-line truncated summary
- **Hidden badge**: Gray `Hidden` badge shown on hidden scenarios

### Visibility Management

A **toggle switch** at the bottom of each card controls whether the scenario appears in the left sidebar (Activity Bar).

- Toggle off → scenario is hidden from the sidebar but still directly accessible via the overview page
- Toggle on → scenario reappears in the sidebar

> Only certain scenarios support hiding; Claude Code always appears in the sidebar.

---

## Full Scenario List

| Scenario | Path | Description |
|----------|------|-------------|
| Claude Code | `/agent/claude_code` | Primary CLI coding assistant with multi-profile support |
| Claude Desktop | `/agent/claude_desktop` | Claude desktop app proxy |
| Codex | `/agent/codex` | OpenAI Codex CLI proxy |
| OpenCode | `/agent/opencode` | OpenCode CLI proxy |
| Xcode | `/agent/xcode` | Apple Xcode AI feature proxy |
| VS Code | `/agent/vscode` | VS Code AI extension proxy |
| OpenAI | `/agent/openai` | OpenAI SDK-compatible interface |
| Anthropic | `/agent/anthropic` | Anthropic SDK native interface |
| Claw (Agent) | `/agent/agent` | OpenClaw universal agent interface |
| Embed | `/agent/embed` | Embedding API proxy |
| ImageGen | `/agent/imagegen` | Image generation API proxy |
| Playground | `/agent/playground` | Interactive image generation test bench |

---

## Navigation Structure

The left Activity Bar icon corresponds to the **Scenarios** group. Clicking it displays all visible scenario navigation items in the secondary sidebar.

- Each scenario nav item supports direct-click navigation to the configuration page
- Claude Code supports multiple Profiles; each Profile appears as a separate sub-item

---

## Related Pages

- [Claude Code Scenario](./03-scenario-claude-code.md)
- [Other Coding Agents](./04-scenario-coding-agents.md)
- [OpenAI / Anthropic SDK Proxy](./05-scenario-sdk-proxy.md)
- [Claw / Embed / ImageGen](./06-scenario-special.md)
- [Playground](./07-scenario-playground.md)
