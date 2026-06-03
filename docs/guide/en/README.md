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
- [Other Coding Agents](./04-scenario-coding-agents.md) — Codex, OpenCode, VS Code, Xcode, Claude Desktop
- [OpenAI / Anthropic SDK Proxy](./05-scenario-sdk-proxy.md) — OpenAI-compatible and Anthropic native interfaces
- [Claw Agent / Embed / ImageGen](./06-scenario-special.md) — OpenClaw, Embedding API, Image Generation
- [Playground](./07-scenario-playground.md) — Interactive image generation test bench

### III. Configuration Chain

Provider and credential management is a prerequisite for all scenarios.

- [Credentials](./08-credentials.md) — API Keys, OAuth, import/export, provider configuration
- [Virtual Models](./09-virtual-models.md) — Built-in synthetic models for demos and dry-runs
- [API Tokens](./10-api-tokens.md) — Manage access tokens for external clients

### IV. Other Main Entry Points

- [Usage Dashboard](./11-dashboard.md) — Request stats, token usage, cache hit rate
- [Remote Control](./12-remote-control.md) — Control Claude Code via IM platforms (WeChat, Telegram, Feishu, etc.)
- [Remote Coder](./13-remote-coder.md) — Web chat interface and session management
- [Prompt Management](./14-prompt-management.md) — User recordings, Skills, Commands (Full Edition)
- [Guardrails](./15-guardrails.md) — Policy import/export, rule management, audit history
- [MCP & Tools](./16-mcp-tools.md) — MCP server registration and local mode

### V. System Settings

- [System Settings](./17-system-settings.md) — Proxy, language, theme, version info, logs
- [Access Control](./18-access-control.md) — User token and model token management

### VI. Experimental Features

- [Experimental Features](./19-experimental.md) — Skills IDE, Guardrails, MCP toggles

### VII. Advanced Topics

- [Routing Rules & Extensions](./20-routing-rules.md) — Direct routing (tiers/circuit breaker), Smart routing (SmartOp conditions), rule extension flags

---

## Edition Notes

Some features are available in **Full Edition** only:
- Prompt Management (user recordings, Skills)
- Remote Control (IM bots)
- Remote Coder

Some features must be manually enabled on the Experimental Features page before appearing in the sidebar:
- Guardrails
- MCP Tools
