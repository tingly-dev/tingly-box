# Guardrails

Guardrails is a standalone safety engine under `/Users/seviezhou/github/tingly-box/internal/guardrails`. It loads policy-based config from YAML/JSON, evaluates normalized input, and can be tested independently from the proxy.

## Core model

Guardrails evaluates a normalized `Input`:

- `command`: tool or function-calling payloads
- `text`: the current model output or tool result text
- `messages`: message history

Policies can scope themselves by:

- scenario
- direction
- content target

## Configuration model

Guardrails config is organized around:

- `groups`: shared defaults and risk grouping
- `policies`: individual safety policies

There are two main policy kinds:

### Operation policies

Use operation policies when you want to control tool behavior, commands, actions, or protected resources.

Typical examples:

- block reads of `~/.ssh`
- block writes to `.env`
- review delete operations on sensitive paths

Main match fields:

- `tool_names`
- `command_kinds`
- `actions.include`
- `resources.values`
- `resources.mode`

### Content policies

Use content policies when you want to filter model output or tool result text.

Typical examples:

- block private key material
- block secret-looking output
- review suspicious phrases in model responses

Main match fields:

- `patterns`
- `pattern_mode`
- `case_sensitive`
- `applies_to`

## Example configuration

See `/Users/seviezhou/github/tingly-box/docs/examples/guardrails.yaml` for a complete sample.

Minimal example:

```yaml
groups:
  - id: high-risk
    name: High Risk
    default_verdict: block

policies:
  - id: block-ssh-read
    name: Block SSH directory reads
    group: high-risk
    kind: operation
    enabled: true
    applies_to: [command]
    scope:
      scenarios: [claude_code]
      directions: [response]
    match:
      tool_names: [bash]
      command_kinds: [shell]
      actions:
        include: [read]
      resources:
        type: path
        mode: prefix
        values: ["~/.ssh"]
    reason: Reading SSH directory content is blocked.

  - id: block-secret-output
    name: Block secret output
    group: high-risk
    kind: content
    enabled: true
    applies_to: [tool_result, text]
    match:
      patterns:
        - BEGIN OPENSSH PRIVATE KEY
        - AKIA[0-9A-Z]+
      pattern_mode: regex
    reason: Secret-looking output is blocked.
```

## Loading config

```go
cfg, err := guardrails.LoadConfig("guardrails.yaml")
if err != nil {
    // handle error
}
engine, err := guardrails.BuildEngine(cfg, guardrails.Dependencies{})
```

## Enabling guardrails in the server

Guardrails are loaded when the feature flag is enabled and a config file exists in the config directory. The server will look for:

- `guardrails.yaml`
- `guardrails.yml`
- `guardrails.json`

Example scenario enablement:

```yaml
scenarios:
  claude_code:
    extensions:
      guardrails: true
```

## Integration path

Current integration is centered on Claude Code / Anthropic flows:

1. **Response-side command interception**
   - stream events are accumulated in `/Users/seviezhou/github/tingly-box/internal/server/guardrails_hooks.go`
   - tool calls are normalized into `command`
   - blocked commands are suppressed before reaching the client tool-execution path

2. **Request-side tool result filtering**
   - implemented in `/Users/seviezhou/github/tingly-box/internal/server/guardrails_request.go`
   - tool results are evaluated as `text`
   - blocked results are replaced before being forwarded back to the model

## Limitations

- If a client executes a tool locally and shows the result locally, the proxy cannot prevent local display.
- Guardrails can still prevent sensitive tool output from being forwarded to the model.
