# Other Coding Agent Scenarios

This chapter covers coding tool proxy scenarios beyond Claude Code: Codex, OpenCode, VS Code, Xcode, and Claude Desktop. Their configuration structure is similar to Claude Code.

---

## Codex

Path: `/agent/codex`

Proxies OpenAI Codex CLI API requests to your configured providers.

### Page Structure

1. **Codex Configuration Card**: Shows proxy Base URL and API Key
2. **Agent Setup Card**:
   - Installation command (one-click copy)
   - **Auto Config** button: Automatically writes proxy configuration to the Codex config file
3. **Models and Forwarding Rules** (collapsible): Manage routing rules for the Codex scenario

### Configuration Flow

1. Install Codex CLI (see the install command)
2. Click **Config** to get the proxy address
3. Click **Auto Config** to write the config automatically, or manually set `OPENAI_BASE_URL` and `OPENAI_API_KEY`
4. Use Codex CLI in your terminal

---

## OpenCode

Path: `/agent/opencode`

Proxies OpenCode CLI requests. The page structure is identical to Codex:

- Config card + proxy address/key
- Agent setup + install guide
- Forwarding rules management

---

## VS Code

Path: `/agent/vscode`

Proxies API requests from VS Code AI extensions (e.g. GitHub Copilot Chat, Continue).

### Setup

VS Code extensions typically specify the API endpoint via a `baseURL` environment variable or extension settings. Point it to the proxy address provided by Tingly-Box.

---

## Xcode

Path: `/agent/xcode`

Proxies Apple Xcode AI feature (Xcode Intelligence) API requests. Configuration is similar to VS Code — point the API endpoint to the Tingly-Box proxy address.

---

## Claude Desktop

Path: `/agent/claude_desktop`

Proxies Claude Desktop app API requests.

### Page Structure

1. **Claude Desktop Configuration Card**: Shows proxy address and API Key
2. **Config Modal**: Provides the complete `claude_desktop_config.json` snippet — copy and paste into Claude Desktop's configuration file
3. **Models and Forwarding Rules** (collapsible)

### Configuration Flow

1. Click **Config** to open the configuration modal
2. Copy the JSON snippet
3. Open Claude Desktop settings file and paste the configuration
4. Restart Claude Desktop

---

## Zen Mode

All the above scenarios support Zen Mode (fullscreen immersive view):

| Scenario | Zen Path |
|----------|---------|
| Codex | `/zen/codex` |
| OpenCode | `/zen/opencode` |
| VS Code | `/zen/vscode` |
| Xcode | `/zen/xcode` |

---

## Scenario Visibility

On the [Scenario Overview](./02-scenario-overview.md) page, use the toggle at the bottom of each card to hide infrequently used scenarios from the sidebar.

---

## Related Pages

- [Claude Code Scenario](./03-scenario-claude-code.md)
- [Scenario Overview](./02-scenario-overview.md)
- [Credentials](./08-credentials.md)
