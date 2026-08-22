# Other Coding Agent Scenarios

This chapter covers coding tool proxy scenarios beyond Claude Code and Codex: OpenCode, Pi, DeepSeek Harness, VS Code, Xcode, and Claude Desktop. Their configuration structure is similar to Claude Code.

---

## OpenCode

Path: `/agent/opencode`

Proxies OpenCode CLI requests. The page structure is identical to Codex:

- Config card + proxy address/key
- Agent setup + install guide
- Forwarding rules management

---

## Pi

Path: `/agent/pi`

![Pi Scenario](../images/pi-scenario.png)

Proxies the Pi coding agent's requests. Same structure as OpenCode/Codex — Quick Start step 3 is **Install Pi**, step 4 **Configure Pi**.

- **Config** (top-right) opens a modal explaining how to point Pi's provider base URL and API key at the values shown on the page — see Pi's own repo for its exact config file format
- **View on GitHub** link in the install step

---

## DeepSeek Harness (dsh)

Path: `/agent/dsh`

![DeepSeek Harness Scenario](../images/dsh-scenario.png)

Proxies requests for [DeepSeek Harness](https://github.com/deepseek-ai/dsh), an agent harness with its own **self-hosted Web UI** (unlike the other CLI-based scenarios).

- **Open Web UI** button (top-right, next to Config) jumps to the locally-running dsh Web UI — by default `http://127.0.0.1:3080`. Tingly-Box does **not** launch dsh for you; start it yourself first with `npx @deepseek-ai/dsh web` (requires Node.js), then use this button once it's running
- Quick Start step 3 is **Install DeepSeek Harness**, step 4 **Configure DSH** — the config step points dsh's model-provider plugin at this page's Base URL and API Key
- `dsh` is in developer preview; its configuration surface may change

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

## Scenario Visibility

On the [Scenario Overview](./02-scenario-overview.md) page, use the hover eye icon on each card to hide infrequently used scenarios from the sidebar.

---

## Related Pages

- [Claude Code Scenario](./03-scenario-claude-code.md)
- [Codex Scenario](./04-scenario-codex.md)
- [Scenario Overview](./02-scenario-overview.md)
- [Credentials](./08-credentials.md)
