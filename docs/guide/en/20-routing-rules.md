# Routing Rules & Extensions

Path: `/scenario/*` (Rule cards within each scenario page)

Routing rules are the core mechanism by which Tingly-Box dispatches requests. Each rule is bound to a request model (`request_model`) and determines how requests are distributed across one or more upstream services (Credential/Provider).

---

## Routing Graph Overview

![Routing Graph Overview](../images/routing-graph-direct.png)

Each rule card embeds a routing graph that visualizes the request flow path. The graph supports two modes, switchable via the toggle button in the top-right corner: **Direct** and **Smart**.

---

## Direct Routing (Tier Mode)

Direct routing is the default mode (`lbTactic: "tier"`). Service nodes are arranged in priority tiers:

```
Request Entry
  │
  ├── T0 (highest priority): multiple services share load
  ├── T1: fallback when T0 circuit is fully open
  └── T2: final fallback when T1 is also open
```

### Tier Behavior

| Concept | Description |
|---------|-------------|
| Same-tier services | Round-robin or weighted load sharing |
| Cross-tier fallback | When all services in the current tier have open circuits, requests automatically route to the next tier |
| Tier number (T0/T1…) | Lower number = higher priority; drag service nodes to adjust tier |

### Circuit Breaker

Each service node has an independent circuit breaker with the following states:

```
Closed (normal) ──── 3 consecutive failures ──→ Open (tripped)
                                                    │
                                               30s cooldown
                                                    │
                                               HalfOpen (probe)
                                                    │
                              ┌─── success ───→ Closed (recovered)
                              └─── failure ───→ Open (re-tripped)
```

| State | Meaning |
|-------|---------|
| **Closed** | Normal — accepts requests |
| **Open** | Tripped — rejects requests, waiting for cooldown (default 30s) |
| **HalfOpen** | Sends a probe request; success → Closed, failure → Open again |

### Mid-request Failover

Via the `firstChunkGate` buffer mechanism (v2), if an upstream fails before the first response chunk is received, the request silently switches to another service in the same tier or the next tier — transparent to the client.

---

## Smart Routing

When smart routing is enabled (`smartEnabled: true`), each sub-rule (SmartOp) in the rule chain can carry conditions. Requests are matched in order; the **first sub-rule where all conditions pass** wins.

```
Request Entry
  │
  ├── SmartOp 1 (condition A AND condition B) ── matches ──→ route to service group A
  ├── SmartOp 2 (condition C)                ── matches ──→ route to service group B
  └── SmartOp N (no conditions — catch-all)   ──────────────→ route to default service group
```

### SmartOp Condition Catalog

| Condition Key | Type | Values | Description |
|--------------|------|--------|-------------|
| `agent.claude_code` | enum | `main` / `subagent` / `compact` | Claude Code request type |
| `token` | threshold | `ge:<N>` / `le:<N>` | Input token count (greater-than-or-equal / less-than-or-equal) |
| `thinking` | bool | `on` / `off` | Whether the client has enabled extended thinking |
| `service_ttft` | performance | `fastest` / `fast` / `slow` / `slowest` | Upstream TTFT (time-to-first-token) performance tier |
| `service_capacity` | status | `available` / `degraded` / `unavailable` | Upstream service current capacity state |
| `context_system` | presence | `exists` / `missing` | Whether the request carries a system prompt |
| `latest_user` | content type | `text` / `image` / `file` / `rich` | Content type of the most recent user message |

### Design Tips

- Multiple conditions in one SmartOp use **AND logic** (all must pass)
- The last sub-rule should be **unconditional** (ops=[]) as the default catch-all fallback
- Use `agent.claude_code=compact` to route compact-mode requests to cheaper models
- Use `token ge:100000` to route very long contexts to services with large context windows

---

## Rule Extensions (Flags)

The **Extensions** card on the right of each rule card provides pre-built flags that tune request/response behavior at the rule level — without touching service configuration.

Click the Extensions card or its **+** button to open the **Flag Catalog** (category sidebar + detail panel).

### App

| Flag | Key | Description |
|------|-----|-------------|
| Cursor compatibility | `cursor_compat` | Normalize rich content, gate tools, and strip stream usage for Cursor clients |
| Auto-detect Cursor | `cursor_compat_auto` | Automatically detect Cursor via request headers and apply compatibility processing |
| Claude Code compatibility | `claude_code_compat` | Rewrite `system` role entries in the messages array to `user` before forwarding, for third-party Anthropic-compatible providers that reject the non-standard role |

### Request (OpenAI)

| Flag | Key | Type | Description |
|------|-----|------|-------------|
| Custom User-Agent | `custom_user_agent` | String | Override the outbound User-Agent header (applies to generic OpenAI/Anthropic clients; vendor-specific clients like Claude Code OAuth keep their own UA) |
| OpenAI endpoint override | `openai_endpoint_override` | Enum | Force Chat Completions or Responses API, overriding the provider default (OpenAI providers only) |
| Use max_completion_tokens | `use_max_completion_tokens` | Toggle | Rewrite `max_tokens` → `max_completion_tokens`; required by o1/o3/gpt-5 model families |
| Use max_tokens (legacy) | `use_max_tokens` | Toggle | Rewrite `max_completion_tokens` → `max_tokens`; for older OpenAI-compatible providers |
| Block tools | `block_tools` | String | Comma-separated tool names to strip from requests before forwarding (works across OpenAI Chat/Responses, Anthropic, and Google) |

### Response

| Flag | Key | Type | Description |
|------|-----|------|-------------|
| Skip usage in response | `skip_usage` | Toggle | Strip the `usage` block from responses (both SSE deltas and final body) |

### Reasoning

| Flag | Key | Type | Description |
|------|-----|------|-------------|
| Thinking | `thinking_effort` | Enum | Unified extended-thinking control: `By Client` (pass-through) / `Off` (force disabled) / `Low` (~1K tokens) / `Medium` (~5K) / `High` (~20K) / `Max` (~32K). Mapped to `budget_tokens` for Anthropic targets and `reasoning_effort` for OpenAI targets |

### Vision

| Flag | Key | Type | Description |
|------|-----|------|-------------|
| Vision Proxy | `vision_proxy_service` | Service ref | Describe images via a vision-capable model so text-only downstream models can process image-bearing requests. Takes precedence over the scenario-level Vision Proxy when both are configured |

### Routing

| Flag | Key | Type | Description |
|------|-----|------|-------------|
| Session affinity | `session_affinity` | Integer (seconds) | TTL for session-to-service pinning: follow-up requests in the same session keep hitting the same service until TTL expires. 0 disables. Session identity resolved from `metadata.user_id`, `X-Tingly-Session-ID` header, or client IP |

---

## Related Pages

- [Scenario Overview](./02-scenario-overview.md)
- [Claude Code Scenario](./03-scenario-claude-code.md)
- [Credentials](./08-credentials.md)
- [Experimental Features](./19-experimental.md)
