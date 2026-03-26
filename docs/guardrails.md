# Guardrails

Guardrails adds rule-based safety checks around model output, tool calls, tool results, and protected credentials.

## What Guardrails manages

Guardrails is organized into three user-facing areas:

- **Policies**: Define concrete rules.
- **Policy Groups**: Organize policies and provide shared defaults.
- **Protected Credentials**: Store secrets that should be masked before model access.

Built-in policies are shown directly inside the Policies page under the matching category. They start disabled and can be enabled manually.

## Configuration location

Guardrails configuration is stored under the app config directory:

- `guardrails/guardrails.yaml`
- `guardrails/history.json`
- `guardrails/db/guardrails.db`

The server still checks legacy flat config paths such as `guardrails.yaml` in the root config directory as a fallback, but the dedicated `guardrails/` directory is the current layout.

## Config structure

Guardrails config is policy-first:

```yaml
strategy: most_severe
error_strategy: review

groups:
  - id: default
    name: Default
    enabled: true
    severity: high
    default_verdict: block
    default_scope:
      scenarios: [claude_code, anthropic, openai]

policies:
  - id: block-ssh-read
    name: Block SSH directory reads
    group: default
    kind: resource_access
    enabled: true
    scope:
      scenarios: [claude_code]
    match:
      actions:
        include: [read]
      resources:
        type: path
        mode: prefix
        values: ["~/.ssh", "/etc/ssh"]
    verdict: block
    reason: Reading SSH directories is blocked.
```

### Top-level fields

- `strategy`: How multiple policy results are merged.
- `error_strategy`: Fallback behavior if a policy evaluation fails.
- `groups`: Shared defaults and organization.
- `policies`: Concrete guardrail rules.

## Policy kinds

Guardrails supports three policy kinds.

### `resource_access`
Protects files, directories, URLs, or similar resources.

Typical use cases:
- Block reads from `~/.ssh`
- Block writes to `.env`
- Restrict network access to specific endpoints

Common match fields:
- `actions.include`
- `resources.type`
- `resources.mode`
- `resources.values`

### `command_execution`
Matches command content and execution intent.

Typical use cases:
- Block `rm -rf`
- Review download-and-execute patterns
- Restrict dangerous shell commands

Common match fields:
- `terms`
- optional `resources`

### `content`
Used for privacy-oriented content filtering. In the UI this is presented as **Privacy Policy**.

Typical use cases:
- Block secrets in tool output
- Review sensitive model output
- Match private key or token patterns

Common match fields:
- `patterns`
- `pattern_mode`
- `case_sensitive`
- `match_mode`
- `min_matches`

## Groups

Policy Groups are used to manage a set of policies and define shared defaults.

A group can provide:
- enabled / disabled state
- severity
- default verdict
- default scenario scope

The UI also includes a non-deletable **Default** group, which acts as the fallback group for policies that are not assigned elsewhere.

## Supported verdicts

User-configurable policies use these verdicts:

- `allow`
- `review`
- `block`

Protected credential masking is handled separately by the Protected Credentials feature, not by a policy verdict.

## Scope

Scope is scenario-based.

Example scenarios include:
- `claude_code`
- `anthropic`
- `openai`

A policy can narrow itself to specific scenarios with:

```yaml
scope:
  scenarios: [claude_code, anthropic]
```

If omitted, the policy can inherit default scope from its group.

## Protected Credentials

Protected Credentials are stored separately from policy definitions.

When a protected credential is enabled:
- the real secret is replaced with an alias before content is sent to the model
- the real value is restored locally when needed for tool execution or local handling

This means credential masking is driven directly by the Protected Credentials page, not by creating a dedicated mask policy.

## Built-in policies

Built-in policies are shipped as templates and appear directly inside the Policies page.

Behavior:
- built-ins start disabled
- users can enable them manually
- built-ins are not deletable from the UI

## Example file

See the full example config here:

- `/Users/seviezhou/github/tingly-box/docs/examples/guardrails.yaml`
