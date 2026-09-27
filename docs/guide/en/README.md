# Tingly-Box User Guide

Tingly-Box is an AI agent orchestration platform providing an LLM gateway, remote control, and safety guardrails. This guide documents the complete Web UI organized by feature area.

---

## Table of Contents

### I. Getting Started
- [Initialization & Provider Setup](./01-getting-started.md)

### II. Agent Scenarios

Agent scenarios are the core of Tingly-Box — they proxy API requests from AI coding tools to your configured providers.

- [Scenario Overview](./02-scenario-overview.md) — Navigation hub and visibility management
- [Claude Code](./03-scenario-claude-code.md) — Primary scenario with Profile support, unified/separate model modes, and forwarding rules
- [Cursor](./04-scenario-cursor.md) — Cursor proxy with cloud-reachability guidance (hidden by default)
- [Codex](./04-scenario-codex.md) — OpenAI Codex CLI proxy with auto-config support
- [Other Coding Agents](./04-scenario-coding-agents.md) — OpenCode, Pi, DeepSeek Harness, VS Code, Xcode, Claude Desktop
- [OpenAI / Anthropic SDK Proxy](./05-scenario-sdk-proxy.md) — OpenAI-compatible and Anthropic native interfaces
- [Custom / Embed](./06-scenario-special.md) — Custom catch-all scenario (formerly OpenClaw) and the Embedding API proxy

### III. Team & Image

Team and Image are no longer scenarios nested under Agent — each is its own top-level Activity Bar entry, shown in the sidebar by default.

- [Team](./07-team.md) — Isolated multi-team workspaces: their own routing rules, Sharing Keys, and the cross-team Team Keys overview page
- [Image](./07-image.md) — Image Playground (generate, edit, mask, sketch, multi-image references, paginated overview) and the Image API configuration page

### IV. Configuration Chain

Provider and credential management is a prerequisite for all scenarios.

- [Credentials](./08-credentials.md) — API Keys, OAuth, provider configuration
- [Virtual Models](./09-virtual-models.md) — Built-in synthetic models for demos and dry-runs
- [API Tokens](./10-api-tokens.md) — Manage access tokens for external clients

### V. Other Main Entry Points

- [Usage Dashboard](./11-dashboard.md) — Request stats, token usage, response performance, reasoning tokens, activity heatmap, Team usage
- [Remote](./12-remote-control.md) — Control Claude Code via IM platforms: Bots, Remote Control routing, IM Notify (WeChat, Telegram, Feishu, etc.)
- [Tips & Help](./13-help.md) — Desktop shortcut, provider catalog, and routing/tier guides in one accordion page
- [Prompt Management](./14-prompt-management.md) — User recordings, Skills, Commands (Full Edition)
- [Guardrails](./15-guardrails.md) — Policy import/export, rule management, protected credentials, audit history
- [MCP & Tools](./16-mcp-tools.md) — MCP server registration and local mode

### VI. System Settings

- [System Settings](./17-system-settings.md) — Proxy, language, theme, version info, logs
- [Access Control](./18-access-control.md) — User token and model token management

### VII. Experimental Features

- [Experimental Features](./19-experimental.md) — Skills IDE, Guardrails, MCP toggles

### VIII. Advanced Topics

- [Routing Rules & Plugins](./20-routing-rules.md) — Direct routing (tiers/circuit breaker), Smart routing (SmartOp conditions), rule plugin flags, the Troubleshoot probe panel
- [Model Select](./21-model-select.md) — Assign providers and models to forwarding rules

---

## Edition Notes

Some features are available in **Full Edition** only:
- Prompt Management (user recordings, Skills)
- Remote (IM bots, routing, notifications)

Some features must be manually enabled on the Experimental Features page before appearing in the sidebar:
- Guardrails
- MCP Tools
