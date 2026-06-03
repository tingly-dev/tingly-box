# Claude Code Scenario

Path: `/agent/claude_code`

![Claude Code Scenario](../images/claude-code.png)

Claude Code is Tingly-Box's primary scenario. It proxies Claude Code CLI API requests to your configured providers, with support for multiple profiles, unified/separate model modes, and fine-grained forwarding rules.

---

## Page Structure

### 1. Provider Configuration Card (Claude Code Configuration)

Displays the provider information associated with the current Claude Code scenario:
- **Base URL**: The proxy address that Claude Code CLI should be configured to use (with copy button)
- **API Key**: The token for CLI use (with copy/reveal button)

The **Config** button in the top-right opens a configuration modal with full connection details and setup instructions.

### 2. Model Configuration Mode

Two modes are available, switchable via the toggle at the top of the page:

| Mode | Description |
|------|-------------|
| **Unified Model** | All requests use the same model routing rule — simple to configure |
| **Separate Model** | Configure separate routing rules for different request types (e.g. sonnet, haiku) |

> A confirmation dialog appears when switching modes.

### 3. Agent Setup Card

Provides installation and configuration guidance for Claude Code CLI:
- Installation command (one-click copy)
- **Apply** button: Automatically writes the current configuration to Claude Code's config file
- **Apply with Status Line**: Also configures the status bar display when applying

### 4. Models and Forwarding Rules

A collapsible section showing the routing rule graph for the current Claude Code scenario:
- View all configured forwarding rules
- Rules are displayed as a node graph: Entry → Routing Rule → Provider

---

## Profile Management

Claude Code supports multiple **Profiles** (configuration presets), useful when different projects or teams need different providers or rules.

### Accessing Profiles

- The secondary sidebar lists all created Profiles below Claude Code
- Each Profile has its own path: `/agent/claude_code/profile/:profileId`

### Profile Configuration Page

Same structure as the main Claude Code page, but settings apply only to that profile:
- Independent Base URL and API Key (different from the main profile)
- Independent model/forwarding rule configuration
- Profile name displayed in the page title area

### Creating a Profile

Profiles can be managed (create, rename, delete) within the Claude Code main page configuration modal.

---

## Zen Mode

Access `/zen/claude_code` or `/zen/claude_code/profile/:profileId` for a fullscreen immersive view that hides the sidebar, ideal for focused single-scenario configuration.

---

## Common Configuration Flow

1. Ensure at least one provider is added in [Credentials](./08-credentials.md)
2. Open the Claude Code page and click **Config** to view the proxy address and API Key
3. Run `claude config set baseURL <your-base-url>` in the terminal, or click **Apply** to auto-configure
4. (Optional) Adjust forwarding rules to route different model requests to different providers
5. Start using Claude Code CLI

---

## Related Pages

- [Scenario Overview](./02-scenario-overview.md)
- [Credentials](./08-credentials.md)
- [Other Coding Agents](./04-scenario-coding-agents.md)
