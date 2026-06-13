# Codex Scenario

Path: `/agent/codex`

The Codex scenario proxies OpenAI Codex CLI API requests to your configured providers, with support for automatic configuration and flexible forwarding rules.

---

![Codex Scenario](../images/codex.png)

## Page Structure

The page is organized top to bottom as follows:

### 1. Codex Configuration Card

Shows connection information for the current scenario:
- **Base URL**: The proxy address Codex CLI should use (with copy button)
- **API Key**: Token for CLI use (with copy/reveal button)

### 2. Agent Setup Card

- **Installation command**: Provides the Codex CLI install command with one-click copy
- **Auto Config** button: Automatically writes the proxy configuration to the Codex config file (sets `OPENAI_BASE_URL` and `OPENAI_API_KEY`)

### 3. Models and Forwarding Rules (collapsible)

Manage routing rules for the Codex scenario — add, edit, and delete rules.

---

## Configuration Flow

1. Add at least one provider in [Credentials](./08-credentials.md)
2. Open the Codex scenario page and confirm the Base URL and API Key
3. Install Codex CLI (see the install command)
4. Click **Auto Config** to write the proxy configuration automatically, or set manually:
   - `OPENAI_BASE_URL`: Set to the Base URL value
   - `OPENAI_API_KEY`: Set to the API Key value
5. Use Codex CLI in your terminal

---

## Related Pages

- [Claude Code Scenario](./03-scenario-claude-code.md)
- [Other Coding Agents](./05-scenario-coding-agents.md)
- [Credentials](./08-credentials.md)
