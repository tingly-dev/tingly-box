# Getting Started

This chapter takes you from a fresh install to a working agent: connect a provider, point a coding tool at the gateway, and see the first request arrive.

## Quick Start

From zero to a coding agent talking through Tingly-Box, in about five minutes. This is the canonical first run; every other page's Quick Start assumes you have done steps 1–3.

1. **Install** — `npm install -g tingly-box@latest` (needs Node.js). No global install? `npx -y tingly-box@latest` does the whole thing in one command. (Prefer the desktop app? Install it instead and skip to step 3 — see [Desktop App](./23-desktop-app.md#quick-start).)
2. **Start** — run `tb start`. It keeps running in the background, opens the Web UI at `http://localhost:12580` and signs you in. If the login page ever appears, run `tb token view auth --reveal` and paste the token into **Authentication Token**.
3. **Connect a provider** — a fresh install opens the **Claude Code** page. In its **Quick Start** card click **Connect AI**, pick a provider (search by name; *Custom endpoint* takes any base URL), paste the API key and click **Connect**. Rather sign in with a subscription? Pick an *OAuth sign-in* provider instead — no key needed.
4. **Pick a model** — click **Choose Model** and select the provider and model Claude Code should use.
5. **Install the agent and run Auto Config** — copy the install command from the card (`npm install -g @anthropic-ai/claude-code`), click **I've installed it**, then **Auto Config**. One click and Tingly-Box writes the agent's config (for Claude Code, also its status line); **Config** lets you review it first. Using another tool? Pick it in the **Agent** sidebar: [Codex](./04-scenario-codex.md#quick-start), [OpenCode, Pi, VS Code and others](./04-scenario-coding-agents.md#quick-start), or your own code through the [SDK endpoints](./05-scenario-sdk-proxy.md#quick-start).
6. **Verify** — start the agent (`claude`) and send a message. The status line under the page title changes from `No requests yet · finish the setup below` to `Connected`, and [Dashboard](./11-dashboard.md#quick-start) in the rail shows the request.

**Then, optionally:** share with a group via [Team](./07-team.md#quick-start), generate pictures in [Image](./07-image.md#quick-start), drive Claude Code from chat apps with [Remote](./12-remote-control.md#quick-start) or from the browser with [Projects](./22-projects.md#quick-start). The rest of this page explains what you just saw in detail.

---

## First Launch

The Quick Start above is what the first-launch card walks you through. There is no separate onboarding wizard. A brand-new install lands on an **agent page** — the one you were last on, which for a first run is **Claude Code** — with the sidebar **expanded** so the other agents are visible. The page's **Quick Start** card does the guiding:

![Quick Start on a fresh install](../images/claude-code-quickstart.png)

1. **Connect AI Provider** — click **Connect AI**. The provider picker opens *in place* (no navigation away); when the provider is added the card moves straight on
2. **Select a Model** — **Choose Model** scrolls to **Model Rules** and spotlights **＋ Add model**; pick a provider and model in the [Model Select](./21-model-select.md) dialog (or **Skip**)
3. **Install** the tool — copy the install command (Claude Code: `npm install -g @anthropic-ai/claude-code`), or **I've installed it**
4. **Auto Config** — one click writes the tool's config so it talks to Tingly-Box (for Claude Code it also installs the status line); **Config** lets you review first

The status line under the page title changes from `No requests yet · finish the setup below` to `Connected` once the tool sends its first request. The same card exists on Codex, OpenCode, Pi, WorkBuddy, DeepSeek and VS Code — see [Scenario Overview](./02-scenario-overview.md) and [Claude Code](./03-scenario-claude-code.md#4-quick-start-card).

> The old behaviour — sending provider-less installs to the **Tips & Help** page — is gone; `/onboarding` still redirects to Help, where the provider catalog stays reachable.

---

## Connecting Your First Provider

Whether from Quick Start, a Model Rules toolbar (**Connect AI**), the [Credentials](./08-credentials.md) page, or the **Provider Catalog** card on [Tips & Help](./13-help.md), it is the same picker:

![Connect AI Picker](../images/connect-ai.png)

A search box sits at the top, followed by:

- **Custom** — **Custom endpoint** (any OpenAI/Anthropic-compatible base URL), **Import** (from a file or the clipboard) and **Paste & detect** (paste a `.env`, curl command or JSON snippet — URL and credentials are extracted for you)
- **OAuth sign-in** — providers you sign in to with a subscription (Claude Code, Google Gemini CLI, Antigravity, Codex, Kimi Code, xAI Grok, ZCode, WorkBuddy, …). Clicking one launches the OAuth flow directly — no API key — and the token is saved when you authorize
- **Self-hosted**, **Cloud** and **API key** providers grouped by region and protocol (OpenAI · Anthropic)

Picking any non-OAuth provider opens the config form — see [Credentials · Adding a Provider](./08-credentials.md#adding-a-provider-the-connect-ai-flow) for every field.

![Provider config form](../images/connect-ai-form.png)

---

## Next Steps

- [Scenario Overview](./02-scenario-overview.md) — the rail, the Agent sidebar and Power-ups
- [Claude Code](./03-scenario-claude-code.md) — the primary scenario, in full
- [Credentials](./08-credentials.md) — manage providers and see their quota
