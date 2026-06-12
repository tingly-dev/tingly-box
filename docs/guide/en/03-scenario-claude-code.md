# Claude Code Scenario

Path: `/agent/claude_code`

Claude Code is Tingly-Box's primary scenario. It proxies Claude Code CLI API requests to your configured providers, with support for multiple profiles, unified/separate model modes, and fine-grained forwarding rules.

---

![Claude Code Scenario](../images/claude-code.png)

## Page Structure

The page is organized top to bottom as follows:

### 1. Provider Configuration Card (Claude Code Configuration)

Shows the connection information for the current Claude Code scenario:
- **Base URL**: The proxy address Claude Code CLI should use (with copy button)
- **API Key**: Token for CLI use (with copy/reveal button)

Three buttons in the top-right:
- **Unified Model / Separate Model**: Switch model configuration mode (see below)
- **Auto Config**: Quick-config button — opens the configuration wizard modal

#### Plugin Toggles

The **Plugin** row in the config card provides several scenario-level plugin dropdowns:

| Plugin | Description |
|--------|-------------|
| **Thinking** | Extended thinking budget level: `By Client` (pass through client setting) / `Off` / `Low` (~1K) / `Medium` (~5K) / `High` (~20K) / `Max` (~32K) |
| **Smart Compact** | Compresses conversation history to save tokens: `Off` / `On` |
| **Vision Proxy** | Proxies image URLs so providers that can't reach external images still work: `Off` / `On` |
| **Record** | Records sessions to Prompt Management: `Off` / `Request Only` / `Request + Response` / `Request + Transform + Response` |

---

### 2. Quick Start Stepper

A **4-step expandable card** shown on first use (progress persisted in browser local storage, auto-collapses when all steps are complete):

| Step | Status indicator | Details |
|------|-----------------|---------|
| 1. Connect AI Provider | Connected provider count | Expand to click **Connect** |
| 2. Select a Model | Configured / pending | Expand to click **Choose Model** |
| 3. Install Claude Code | Installed / pending | Expand to see npm official + mirror install commands (one-click copy) and **I've installed it** to mark manually |
| 4. Auto Config | Applied / pending | Expand to run **Quick Apply** (optionally check **Install Tingly-Box status line**); **Skip** button lets you bypass this step if you don't need auto-config |

- Completed steps show a ✓ icon and can be re-expanded by clicking
- Click **Reset** at the top to clear all step progress

---

### 3. Model Configuration Mode

Toggle **Unified Model** or **Separate Model** in the top-right:

| Mode | Description |
|------|-------------|
| **Unified Model** | All requests share one forwarding rule (`built-in-cc`) — simple, ideal for a single provider |
| **Separate Model** | Separate routing rules for default / haiku / sonnet / opus / subagent request types |

> **Important**: After switching modes, you must click **Auto Config** again to write the new configuration to Claude Code — the CLI won't pick it up automatically.

---

### 4. Auto Config Wizard

![Auto Config Modal](../images/claude-code-config-modal.png)

Click **Auto Config** to open the **Claude Code Configuration Guide** modal with two tabs:

**Auto Config Tab (recommended)**

- **Model routing**: 5 model slots matching Claude Code's internal use cases:
  - `ANTHROPIC_MODEL` (Default model)
  - `ANTHROPIC_DEFAULT_HAIKU_MODEL` (Haiku slot — lightweight tasks)
  - `ANTHROPIC_DEFAULT_SONNET_MODEL` (Sonnet slot — primary tasks)
  - `ANTHROPIC_DEFAULT_OPUS_MODEL` (Opus slot — complex reasoning)
  - `CLAUDE_CODE_SUBAGENT_MODEL` (Sub-agent model — sub-task delegation)

  Each slot auto-populates from current forwarding rules and can be overridden manually.

- **Performance & limits**:
  - `API_TIMEOUT_MS`: API request timeout (ms)
  - `CLAUDE_CODE_MAX_OUTPUT_TOKENS`: Max output token count
  - `MAX_THINKING_TOKENS`: Thinking token budget (blank = model default)
  - `BASH_DEFAULT_TIMEOUT_MS`: Bash command default timeout
  - `BASH_MAX_TIMEOUT_MS`: Bash command max timeout
  - `CLAUDE_AUTOCOMPACT_PCT_OVERRIDE`: Auto-compact trigger threshold (%, default 85). When context usage reaches this %, history is automatically compacted. Set to 0 to disable.

- **Preview generated env**: Preview the env variable block that will be written to `~/.claude/settings.json`

- **Install Tingly-Box Claude Code status line** checkbox: When checked, also installs the status line script into `~/.claude/settings.json` — shows connection status in the Claude Code prompt.

Bottom button:
- **Quick Apply**: Writes the configuration (and optionally the status line) to Claude Code's config file; shows a result list of created/updated/backed-up files

**Manual Tab**

Shows and allows direct editing of the raw configuration scripts (JSON / PowerShell / Bash), for advanced users or manual deployments.

---

### 5. Models and Forwarding Rules

A collapsible node graph at the bottom showing the full routing chain for the current scenario:

![Model Select Dialog](../images/model-select.png)

Click a provider node in the routing graph, or use the "add" action on a forwarding rule, to open the **[Model Select](./21-model-select.md)** dialog. Models are grouped by provider, with search and quick-select support.

```
Entry node (Direct/Smart) → IF condition (e.g. agent.claude_code = subagent) → Provider
```

- Each rule can be expanded to see condition details
- Top-right: **Logs** (view routing logs), **New Key** (add a forwarding rule), **Import**
- Provider cards in the graph show model name and provider source

#### 1M Context Window Toggle

Each rule's model header shows a **1M** label with a toggle switch. Enabling it activates the `context_1m` flag for that rule — Tingly-Box appends `[1m]` to the model name in the generated env, signaling Claude Code to use 1M-token context windows. When you toggle this, the **Auto Config** modal opens automatically with a pending-change banner reminding you to re-apply the config and restart Claude Code.

---

## Profile Management

Claude Code supports multiple **Profiles** for projects or teams that need different providers or routing rules.

- All profiles are listed below Claude Code in the sidebar — click to switch
- Each profile has an independent path: `/agent/claude_code/profile/:profileId`
- Each profile has its own Base URL, API Key, and forwarding rules
- Profile pages additionally offer **npx** / **global** install mode:
  - `npx -y tingly-box@{version} cc --profile {profileId}`
  - `tingly-box cc --profile {profileId}`

### CLI: `tingly-box profile`

The `tingly-box profile` CLI command lets you inspect and launch saved profiles directly from the terminal — no web UI needed.

**Launch**

```bash
tingly-box profile               # Interactive: list profiles, prompt to select and launch
tingly-box profile p1            # Launch Claude Code with profile p1
tingly-box profile p1 --port 12580  # Launch against a remote Tingly-Box on port 12580
```

**Inspect**

```bash
tingly-box profile --list        # List all profiles (non-interactive, ID · name · mode)
tingly-box profile --show        # Interactive: pick a profile to inspect
tingly-box profile --show p1     # Show details for profile p1:
                                 #   Profile ID/name, Scenario path, Mode (unified/separate)
                                 #   Rules table: request_model → provider / model [active|inactive]
```

> `--list` and `--show` are mutually exclusive. If the profile name is not found, the command falls back to an interactive picker.

---

## Common Configuration Flow

1. Add at least one provider in [Credentials](./08-credentials.md)
2. Open the Claude Code page and confirm the Base URL and API Key
3. (Optional) Assign specific models to different request types in the forwarding rules
4. Click **Auto Config** → review settings → **Quick Apply** (optionally enable status line checkbox)
5. Start using Claude Code CLI

---

## Related Pages

- [Scenario Overview](./02-scenario-overview.md)
- [Credentials](./08-credentials.md)
- [Other Coding Agents](./04-scenario-coding-agents.md)
