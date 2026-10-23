# Routing Rules & Plugins

Path: the **Model Rules** section of every agent page (`/agent/*`, `/image/api`)

Routing rules are the core mechanism by which Tingly-Box dispatches requests. Each rule is bound to a request model (`request_model`) and determines how requests are distributed across one or more upstream services (Credential/Provider).

---

## First-Run Guide

![Routing Guide](../images/routing-guide.png)

The first time an agent page shows a routing graph, a **Direct Routing Guide** opens **automatically, once**, walking you through how routing is built up from scratch: **Connect AI to add a provider → ＋ Add model for your first model → change/remove a model → load balancing within a tier → tier-based failover**.

- It auto-opens **only once per user**, then never nags again
- The left side is the step navigator; the right side shows the matching routing diagram plus an explanation. Steps that reference a toolbar button display a **mock toolbar** (Test All · Connect AI · New Model) with the target button highlighted so you know exactly where to click
- To see it again, click the **?** (*How routing works*) button beside the **Model Rules** title — or in the Quick Start card, or on [Tips & Help](./13-help.md) — at any time
- Use **Previous / Next** at the bottom to page through; the last step's button is **Got it!** to close

> Smart routing has its own Smart Routing Guide — switch to Smart mode and open it via the same **?** button.

---

## Routing Graph Overview

![Direct Routing Graph](../images/routing-graph-direct.png)

Each rule card embeds a routing graph that visualizes the request flow path. The graph supports two modes, switchable via the toggle button inside the rule card: **Direct** and **Smart**.

---

## The Rule Header

Each rule card opens with its **request model name** (e.g. `cc`; click the pencil to rename it — it is what incoming API calls are matched against, and may use wildcards such as `*`), followed by a row of chips:

| Chip | Meaning |
|------|---------|
| **`1M: Off ⌄`** | Whether the rule adds the 1M context window. *Off* only means the rule doesn't add it; a client asking for 1M itself is still honoured. Present on agents that support it (Claude Code, Claude Desktop, Codex) |
| **`Endpoint: Auto ⌄`** | Which upstream API style the rule uses: **Auto** (the model Catalog, then the provider's setting, default Chat; when both are supported follow the client), **Chat** (always Chat Completions), **Responses** (always Responses; support is checked before saving) or **Anthropic** (*Force Anthropic style* — always Anthropic Messages upstream, converting OpenAI clients; providers without an Anthropic endpoint fall back to Auto). Only meaningful for OpenAI-compatible upstreams |
| **`Compact: Auto ⌄`** | Codex only: this model's auto-compact trigger — see [Codex](./04-scenario-codex.md#per-model-auto-compact) |
| **`Plugins N`** | Appears when the rule has plugins on — click for the list and **Manage plugins** (below) |

On the right of the header: a **bolt** (quick test of this rule — see [Probe Panel](#probe-panel)), the ⋮ **Rule actions** menu — **Plugins**, **Test Probe**, **Copy Base64** / **Copy JSONL** (export), **Activate / Deactivate Rule**, **Edit flag**, **Delete Rule** (where the page allows them) — and a fold chevron. The page toolbar above the rules has **sort by name**, **collapse/expand all**, **?** and, where allowed, **Test All**, **Connect AI** and **New Model**.

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
| Tier number (T0/T1…) | Lower number = higher priority; click a model's tier chip to set it, or use the ↑/↓ actions on the model card to move it between tiers (hover the card to see them). Models in the same tier are load balanced |

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

![Smart Routing Graph](../images/routing-graph-smart.png)

When smart routing is enabled (`smartEnabled: true`), each sub-rule (SmartOp) in the rule chain can carry conditions. Requests are matched in order; the **first sub-rule where all conditions pass** wins.

```
Request Entry
  │
  ├── SmartOp 1 (condition A AND condition B) ── matches ──→ route to service group A
  ├── SmartOp 2 (condition C)                ── matches ──→ route to service group B
  └── SmartOp N (no conditions — catch-all)   ──────────────→ route to default service group
```

### SmartOp Condition Catalog

Grouped by category, as shown in the condition picker:

| Condition | Category | Operators | Description |
|-----------|----------|-----------|-------------|
| Agent: Claude Code (`agent.claude_code`) | Agent | Equals: `main` / `subagent` / `compact` | Claude Code request kind |
| System Prompt (`context_system`) | Context | Contains: `<text>` | Whether the request's system prompt contains the given text |
| Latest User Message (`latest_user`) | Context | Contains: `<text>` | Whether the most recent user message contains the given text |
| Time range (`time`) | Time | A daily start–end window in a chosen timezone | Match requests inside or outside a recurring daily time range |
| Thinking (`thinking`) | Request | `Enabled` / `Disabled` | Whether the client has enabled extended thinking |
| Token Count (`token`) | Request | `≥ N` / `≤ N` | Input token count |
| Service TTFT (`service_ttft`) | Service | `Avg ≤/≥ N ms`, `P99 ≤/≥ N ms` | Time-to-first-token across the rule's services (best-service average or P99) |
| Service Capacity (`service_capacity`) | Service | `Util ≤/≥/</> N%` | Seat/concurrency utilization across the rule's services |
| Service Quota (`service_quota`) | Service | `Quota ≤/≥/</> N%` | Tightest (max) upstream quota usage across the rule's services' providers — the hottest one, not the average. Absent quota data lets the op **pass** rather than block, so quota-blind rules and cold data never break routing |

### Design Tips

- Multiple conditions in one SmartOp use **AND logic** (all must pass)
- The last sub-rule should be **unconditional** (ops=[]) as the default catch-all fallback
- Use `agent.claude_code` = `compact` to route compact-mode requests to cheaper models
- Use `token ≥ 100000` to route very long contexts to services with large context windows
- Use `service_quota` `Quota ≥ 90%` to fail a rule over to a backup service group before a provider's quota is actually exhausted

---

## Rule Plugins (Flags)

Plugins are pre-built flags that tune request/response behaviour **per rule** — without touching service configuration. The rule header's **`Plugins N`** chip opens a popover listing the plugins that are on (name and value, **×** to remove one; click a row to jump to it); **Manage plugins** — or ⋮ → **Plugins**, the only way to add the first one — opens the **Flag Catalog**: a category sidebar (**App**, **Request**, **Request (O)** for OpenAI, **Request (A)** for Anthropic, **Response**, **Reasoning**, **Vision**, **Observe**, **Routing**) next to a detail panel where each flag is toggled and configured.

![Rule Plugins popover](../images/rule-plugins-popover.png)

![Rule Plugins Catalog](../images/rule-extensions.png)

Scenario-level plugins (the **Plugins** card on the agent page — Thinking, Vision Proxy, Record) apply to every rule; a rule's own value overrides them. The catalog is rendered from the backend's flag registry, so it can gain flags without a UI release — the tables below list what ships today.

### App

| Flag | Key | Description |
|------|-----|-------------|
| Cursor compatibility | `cursor_compat` | Normalize rich content, gate tools, and strip stream usage for Cursor clients |
| Auto-detect Cursor | `cursor_compat_auto` | Automatically detect Cursor via request headers and apply compatibility processing |
| Claude Code compatibility | `claude_code_compat` | Fold Claude Code's mid-conversation `system`-role messages into a neighbouring user turn before forwarding, for third-party Anthropic-compatible providers that reject that non-standard role. On by default for the built-in Claude Code rules; turn off for native Anthropic fidelity |
| Clean Header | `clean_header` | Strip `x-anthropic-billing-header` blocks from system messages and neutralise the geolocation markers Claude Code embeds in the system prompt date, so they don't leak to third-party providers. On by default for the built-in Claude Code rules; suppressed automatically when the rule routes to a Claude OAuth provider |

### Request (protocol-agnostic)

| Flag | Key | Type | Description |
|------|-----|------|-------------|
| Custom Headers | `extra_headers` | Header list | Append custom HTTP headers to the outbound upstream request for requests matched by this rule. Applies to API-key providers only — OAuth/vendor providers (Claude Code, Codex, Kimi, Gemini, Antigravity) keep their handshake headers and ignore this. Headers are sent as configured, including ones the gateway also sets (`Authorization`, `User-Agent`, …) — overriding those is your call and your responsibility |

### Request (OpenAI)

| Flag | Key | Type | Description |
|------|-----|------|-------------|
| Custom User-Agent | `custom_user_agent` | String | Override the outbound User-Agent header (applies to generic OpenAI/Anthropic clients; vendor-specific clients like Claude Code OAuth keep their own UA) |
| OpenAI endpoint override | `openai_endpoint_override` | Enum | `Auto` (model Catalog, then provider default, then Chat) / `Force Chat Completions` / `Force Responses API`. Shown in the header as the **Endpoint** chip. Anthropic/Google providers ignore it |
| Use max_completion_tokens | `use_max_completion_tokens` | Toggle | Rewrite `max_tokens` → `max_completion_tokens`; required by o1/o3/gpt-5 model families |
| Use max_tokens (legacy) | `use_max_tokens` | Toggle | Rewrite `max_completion_tokens` → `max_tokens`; for older OpenAI-compatible providers |
| Block tools | `block_tools` | String | Comma-separated tool names to strip from requests before forwarding (works across OpenAI Chat/Responses, Anthropic, and Google) |

### Request (Anthropic)

| Flag | Key | Type | Description |
|------|-----|------|-------------|
| Anthropic endpoint override | `anthropic_endpoint_override` | Enum | `Auto (follow inbound client)` / `Force Anthropic style`: always send the request in Anthropic form — an OpenAI client's request is converted, and a dual-URL provider serves its Anthropic URL. Only providers that support Anthropic style can be forced; others fall back to Auto with a warning. The **Endpoint → Anthropic** choice in the rule header sets this |
| Claude organization ID | `claude_org_id` | String | Controls the `anthropic-organization-id` header for Claude OAuth providers. Empty (default) sends none; **Login organization (from OAuth)** attributes requests to the organization captured at login (needed for org-bound entitlements); or enter an organization UUID |
| Claude Code version | `claude_code_version` | Enum | Which Claude Code release the Claude OAuth chain impersonates upstream: `Default` (latest accepted, currently 2.1.280 — the native client) / `2.1.280` / `2.1.86` (legacy emulation). Claude OAuth providers only |
| 1M Context Window | `context_1m` | Toggle | Always request Anthropic's 1M-token context for supported models by adding the `context-1m-2025-08-07` beta flag upstream; unset leaves requests unchanged (clients that ask for 1M themselves are still honoured). Set from the header's **1M** chip |

### Response

| Flag | Key | Type | Description |
|------|-----|------|-------------|
| Skip usage in response | `skip_usage` | Toggle | Strip the `usage` block from responses (both SSE deltas and final body) |

### Reasoning

| Flag | Key | Type | Description |
|------|-----|------|-------------|
| Thinking | `thinking_effort` | Enum | Unified extended-thinking control: `By Client` (pass-through) / `Off` (force disabled) / `Low` (~4K tokens) / `Medium` (~10K) / `High` (~20K) / `Max` (~32K). Sent natively as `reasoning_effort` for OpenAI targets and `output_config.effort` for Anthropic targets, with a `budget_tokens` fallback for budget-based models |

### Vision

| Flag | Key | Type | Description |
|------|-----|------|-------------|
| Vision Proxy | `vision_proxy_service` | Service ref | Describe images via a vision-capable model so text-only downstream models can process image-bearing requests. Takes precedence over the scenario-level Vision Proxy when both are configured |

### Observe

| Flag | Key | Type | Description |
|------|-----|------|-------------|
| Recording | `recording` | Multi-select | Record this rule's traffic at the chosen capture points for debugging and replay: **Client request (inbound)** — exactly as the client sent it — and **Upstream request (outbound)** — the final request after all transforms. A rule-level selection overrides the scenario's Record plugin. Records are written under the config directory's `record/` folder, grouped by scenario. Response-side capture points aren't offered yet |

### Routing

| Flag | Key | Type | Description |
|------|-----|------|-------------|
| Session affinity | `session_affinity` | Integer (seconds) | **Rule-level** TTL for session-to-service pinning: follow-up requests in the same session keep hitting the same service until TTL expires. 0 disables. Built-in Claude Code, Claude Desktop, and Codex rules default to 1800 s. Session identity resolved from `metadata.user_id`, `X-Tingly-Session-ID` header, or client IP |

---

## Probe Panel

A diagnostic panel for firing a real test request through a rule (or directly at a provider) and inspecting exactly what happened, without leaving the routing graph. (It used to be reached from a **Troubleshoot** toolbar button; that button is gone — the agent pages' **Log** button now opens the request log instead, see [System Settings § Logs](./17-system-settings.md#logs-systemlogs).)

### Entry points

- **Quick test** — the **bolt** icon in each rule header runs a one-click streaming test against just that rule; the result renders as a compact coloured pill (`2.6s`, or `Success` / `Failed`) beside the icon. Click the pill to open the full panel pre-loaded with that result.
- **Test All** (toolbar, top right of Model Rules) fires the same quick test against every *active* rule at once — inactive rules are skipped since they can't match real traffic anyway.
- **⋮ → Test Probe** on a rule opens the full panel directly, without running anything first.
- The same panel, aimed at a single model, opens from a model card's bolt in [Model Select](./21-model-select.md).

### Panel layout

![Probe Panel](../images/probe-panel.png)

A left **control rail** (what will be sent) next to a right **results column** (what came back), so the two never have to be reconciled by memory. The title bar names the target (`cc · cc`) and carries the last verdict, **Copy cURL**, **Copy response**, **Re-run** and the filled **Run Test**.

**Request Config (left rail)** — orthogonal axes that compose freely:

| Axis | Options | Notes |
|------|---------|-------|
| Protocol | `O Chat` / `O Resp.` / `A` | The client-side wire protocol the loopback speaks (OpenAI Chat, OpenAI Responses, Anthropic). Locked to the rule's own scenario protocol for rule targets; reduced to what the provider can actually speak for provider targets; unavailable for Google providers (their own SDK) |
| Request | `Nonstream` / `Stream` | Stream is the default — closest to production traffic |
| Thinking | `None` / `Low` / `Medium` / `High` / `Max` (stepped slider) | Extended-thinking effort; maps to the provider's native knob (Anthropic `budget_tokens`, OpenAI `reasoning_effort`, Gemini `thinking_budget`) |
| Scope | `Through TB` / `Direct` | Direct skips Tingly-Box's routing & middleware entirely, to tell whether a failure is upstream or inside TB. Locked to "Through TB" for rule targets — that's what a rule test is for |

An **Advanced** accordion (collapsed) holds the **Content** group:

| Axis | Options | Notes |
|------|---------|-------|
| Tool | `Off` / `On` | Attaches a tool definition so the probe exercises tool calling; composes with both stream and nonstream |
| Vision | `Off` / `User` / `Tool` | Attaches a red test image — in the user message, or returned from a synthetic tool round (the shape agent tools use for screenshots). A vision-capable route answers "red"; anything else means the image was dropped or corrupted along the path |
| Message | free text | Override the default probe message (defaults differ for tool vs. plain probes) |

**Results (right column)**, appearing after **Run Test**:
- A **Success**/**Failed** status bar with **latency** and **token** chips (or the error message on failure)
- **Request Journey** (collapsible): rule + applied flags, routing source (session affinity / smart routing / load balancer / pinned), resolved provider → model, resolved upstream API style, upstream URL, and the loopback request URL — each field renders as a greyed placeholder until the backend actually reports it
- **Response** (collapsible): the extracted assistant text, with its own copy button
- **Raw JSON** (collapsible): the full raw response body
- **cURL** (bottom, full panel width): a live-regenerated `curl` command matching the current Request Config — nothing is executed, it's pure construction — with a hint to swap in your own key before running it yourself

The dialog is resizable (drag the corner) for when the journey or a long streamed response needs more room. For a free-form workbench — your own request body, flag overlays — see [Bench](./19-experimental.md#bench).

---

## Related Pages

- [Scenario Overview](./02-scenario-overview.md)
- [Claude Code Scenario](./03-scenario-claude-code.md)
- [Credentials](./08-credentials.md)
- [Model Select](./21-model-select.md)
- [Experimental Features](./19-experimental.md)
