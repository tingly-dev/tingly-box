# Tingly-Box User Guide

Tingly-Box is an AI agent orchestration platform providing an LLM gateway, remote control, and safety guardrails. This guide documents the complete Web UI (and the desktop app, which shows the same product) organized by feature area.

---

## Table of Contents

### I. Getting Started
- [Initialization & Provider Setup](./01-getting-started.md) — first launch, Quick Start, connecting a provider

### II. Agent Scenarios

Agent scenarios are the core of Tingly-Box — they proxy API requests from AI coding tools to your configured providers. Every agent page shares one layout (status line, folding connection card, Plugins card, Quick Start, Model Rules), described once in the Claude Code chapter.

- [Scenario Overview](./02-scenario-overview.md) — the rail, the Agent sidebar (pin / hide agents) and the Power-ups menu
- [Claude Code](./03-scenario-claude-code.md) — the primary scenario: page structure, per-slot rules, profiles, Auto Config
- [Cursor](./04-scenario-cursor.md) — Cursor proxy with cloud-reachability guidance (hidden by default)
- [Codex](./04-scenario-codex.md) — OpenAI Codex CLI proxy: Auto Config, multi-model New Model, per-model auto-compact
- [Other Coding Agents](./04-scenario-coding-agents.md) — OpenCode, Pi, WorkBuddy, DeepSeek Harness, VS Code, Xcode, Claude Desktop
- [OpenAI / Anthropic SDK Proxy & Decisions](./05-scenario-sdk-proxy.md) — OpenAI-compatible and Anthropic native interfaces, and the Decisions API
- [Custom / Embed](./06-scenario-special.md) — Custom catch-all scenario (formerly OpenClaw) and the Embedding API proxy

### III. Team

Team is its own top-level rail item, shown by default.

- [Team](./07-team.md) — isolated multi-team workspaces: their own routing rules, Team Keys, and the cross-team Team Keys overview

### IV. Image

Image is likewise its own rail item, shown by default.

- [Image](./07-image.md) — Playground (generate, edit, mask, sketch, references, overview), profiles, the Library, and the Image API page

### V. Projects & Remote

- [Projects](./22-projects.md) — Claude Code from the web: working folders, projects with a coordinator and threads, answering questions, the IM page *(Beta power-up)*
- [Remote](./12-remote-control.md) — IM chats talk to Claude Code through Projects: Bots, Remote Control (access & profile), IM Notify

### VI. Configuration Chain

Provider and credential management is a prerequisite for all scenarios.

- [Credentials](./08-credentials.md) — API keys, OAuth, provider configuration, the Quota column
- [Virtual Models](./09-virtual-models.md) — built-in synthetic models for demos and dry-runs (a Power-up)

> The old "API Tokens" page (`/tingly-box-token`, labeled "Sharing" in the sidebar) is retired; its functionality moved into [Team · Team Keys](./07-team.md#team-keys-overview) (`/agent/team/keys`) — the old URL redirects there automatically.

### VII. Other Main Entry Points

- [Usage Dashboard](./11-dashboard.md) — one toolbar of filters, status cards, Summary / By Request / Activity, Team usage, Quota history
- [Tips & Help](./13-help.md) — desktop shortcut, provider catalog, and routing/tier/Team guides in one accordion page
- [Prompt Management](./14-prompt-management.md) — user recordings and Skills *(power-ups)*
- [Guardrails](./15-guardrails.md) — policy import/export, rule management, Secrets, audit history *(power-up)*
- [MCP & Tools](./16-mcp-tools.md) — MCP server registration and Servertool *(power-up)*
- [Desktop App & Tray Hub](./23-desktop-app.md) — the desktop window, the tray hub panel and the macOS menu bar

### VIII. System Settings

- [System Settings](./17-system-settings.md) — the gear and Preferences menu, language, theme, proxy, version/update, Develop, Logs
- [Access Control](./18-access-control.md) — User token and Model token management

### IX. Experimental Features

- [Experimental Features](./19-experimental.md) — the opt-in switches: Skills, Guardrails, MCP, Bench, Projects

### X. Advanced Topics

- [Routing Rules & Plugins](./20-routing-rules.md) — the rule header, Direct routing (tiers/circuit breaker), Smart routing (SmartOp conditions), rule plugin flags, the probe panel
- [Model Select](./21-model-select.md) — assign providers and models to forwarding rules

---

## Optional Features

Everything in this guide is available in every build, including the desktop app — there is no edition split any more. Some features are **opt-in** and appear only after you switch them on in the **Power-ups** menu (rail gear → Preferences → Power-ups) or on the Experimental Features page:

- Projects and IM (Beta), Bench (Beta)
- Guardrails, MCP Tools, User prompts, Skills (Experimental)
- Team, Image and Virtual Models are on by default and can be switched off there
