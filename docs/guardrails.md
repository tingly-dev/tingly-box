# Guardrails

This module provides a flexible, standalone guardrails engine that can be configured via JSON/YAML and unit-tested independently. It supports rule-based filters (e.g., text/regex) and model-judge rules for more flexible safety checks.

## Content-aware filtering

Content is split into three parts, and rules can target each part independently:

- `text`: the single model response message
- `messages`: full message history
- `command`: function-calling payloads

Two knobs control targeting:

- `scope.content_types`: gate whether a rule should run for an input
- `params.targets`: define which content parts a rule should evaluate

If `targets` is omitted, rules evaluate all available content by default.

## Example configuration

See `docs/examples/guardrails.yaml` for a complete sample. A minimal rule-based filter looks like:

```yaml
rules:
  - id: "dangerous-command"
    name: "Block Dangerous Command"
    type: "text_match"
    enabled: true
    scope:
      content_types: ["command"]
    params:
      patterns: ["rm -rf"]
      targets: ["command"]
      verdict: "block"
```

For Claude Code integrations, you can combine a **pre-execution** command block
with a **post-execution** tool_result filter:

```yaml
  - id: "block-ssh-access"
    name: "Block SSH directory access"
    type: "text_match"
    enabled: true
    scope:
      scenarios: ["anthropic"]
      directions: ["response"]
      content_types: ["command"]
    params:
      patterns: ["~/.ssh", "/.ssh", "id_rsa", "authorized_keys"]
      targets: ["command"]
      verdict: "block"
      reason: "ssh access command blocked"

  - id: "filter-ssh-output"
    name: "Filter SSH output"
    type: "text_match"
    enabled: true
    scope:
      scenarios: ["anthropic"]
      directions: ["request"]
      content_types: ["text", "messages"]
    params:
      patterns: ["~/.ssh", "id_rsa", "known_hosts", "authorized_keys"]
      targets: ["text", "messages"]
      verdict: "block"
      reason: "ssh output filtered"
```

## Rule types

### text_match

Matches patterns against content. Supports literal or regex matching, case sensitivity, and minimum match counts.

Common params:

- `patterns` (required)
- `targets` (optional): `text`, `messages`, `command`
- `use_regex` (optional)
- `case_sensitive` (optional)
- `verdict` (optional)
- `reason` (optional)

### model_judge

Delegates to an external judge model/service. The judge implementation is injected and can use the filtered content.

Common params:

- `model` (optional)
- `prompt` (optional)
- `targets` (optional)
- `verdict_on_error` (optional)
- `verdict_on_refuse` (optional)

## Loading config

You can load YAML/JSON from disk with `guardrails.LoadConfig`, then build an engine:

```go
cfg, err := guardrails.LoadConfig("guardrails.yaml")
if err != nil {
    // handle error
}
engine, err := guardrails.BuildEngine(cfg, guardrails.Dependencies{})
```

## Enabling guardrails in the server

Guardrails are loaded at startup when the scenario flag is enabled and a config
file exists in the config directory. The server will auto-detect:

- `guardrails.yaml`
- `guardrails.yml`
- `guardrails.json`

To enable guardrails for Claude Code, set the scenario extension in your config:

```yaml
scenarios:
  claude_code:
    extensions:
      guardrails: true
```

## Technical route (integration plan)

This section documents how Guardrails is integrated end-to-end, with a focus on Claude Code (Anthropic) streaming.

### Goals

- Support both **pre-execution blocking** (tool_use) and **post-execution filtering** (tool_result).
- Keep Guardrails standalone and testable, with a clean integration surface in the proxy.
- Allow flexible targeting by content type (`command`, `text`, `messages`).

### Integration layers

1) **Engine layer** (`internal/guardrails`)
   - Rules are evaluated against a normalized `guardrails.Input`.
   - `Content` contains `Command`, `Text`, and `Messages`.
   - Rules can be scoped by scenario/model/direction/content types.

2) **Streaming response hook (tool_use blocking)**
   - Implemented in `internal/server/guardrails_hooks.go`.
   - On each streaming event, the accumulator extracts tool_use data.
   - If Guardrails returns `VerdictBlock`, we register a block in stream state.
   - The stream layer buffers `tool_use` blocks and suppresses them when blocked.
   - Instead of forwarding the tool call, a guardrails text warning is emitted.

3) **Request-time hook (tool_result filtering)**
   - Implemented in `internal/server/guardrails_request.go`.
   - When client sends `tool_result` back, we evaluate Guardrails on that content.
   - If blocked, the tool_result content is replaced with a guardrails warning
     and then forwarded to the model (so the model sees the warning, not the data).

### End-to-end flow (Claude Code)

```
Client → Proxy → Model
      ← Proxy ← Model

1) Model emits tool_use
2) Proxy Guardrails (command) decides:
   - allow → forward tool_use to client
   - block → suppress tool_use, emit guardrails warning text

3) Client executes tool and sends tool_result
4) Proxy Guardrails (tool_result) decides:
   - allow → forward tool_result to model
   - block → replace tool_result content with warning, forward to model
```

### What this achieves

- **Dangerous commands never execute** if blocked at tool_use stage.
- **Sensitive outputs are not exposed to the model** if blocked at tool_result stage.
- Client may still display local tool output (by design); the model does not see it.

### Configuration guidance

- Use `content_types: ["command"]` for pre-execution blocking rules.
- Use `content_types: ["text"]` or `["messages"]` for post-execution filtering rules.
- Combine both for high-risk scenarios (e.g., `~/.ssh`, credentials, secrets).

### Limitations

- If a client executes tools locally and displays results, the proxy cannot prevent
  the local UI from showing data; it can only prevent the model from receiving it.
- For stronger client-side privacy, an on-device guardrail hook is required.
