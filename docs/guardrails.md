# Guardrails

Guardrails adds rule-based safety checks around model output, tool calls, tool results, and protected credentials.

## What Guardrails manages

Guardrails is organized into three user-facing areas:

- **Policies**: Define concrete rules.
- **Policy Groups**: Organize policies and control which policy sets are active.
- **Protected Credentials**: Store secrets that should be masked before model access.

Built-in policies are shown directly inside the Policies page under the matching category. They start disabled and can be enabled manually.

## Configuration location

Guardrails configuration is stored under the app config directory:

- `guardrails/guardrails.yaml`
- `guardrails/builtin/*.yaml`
- `guardrails/custom/*.yaml`
- `guardrails/history.json`
- `guardrails/db/guardrails.db`

The server still checks legacy flat config paths such as `guardrails.yaml` in the root config directory as a fallback, but the dedicated `guardrails/` directory is the current layout.

## Config structure

Guardrails config is policy-first. The root file owns groups and global settings,
while imported child files own policy fragments:

```yaml
strategy: most_severe
error_strategy: review

imports:
  - builtin/filesystem_access.yaml
  - custom/team_rules.yaml

groups:
  - id: default
    name: Default
    enabled: true
    severity: high

policies:
```

### Top-level fields

- `strategy`: How multiple policy results are merged.
- `error_strategy`: Fallback behavior if a policy evaluation fails.
- `imports`: Additional policy fragment files, resolved relative to `guardrails.yaml`.
- `groups`: Policy collections and activation buckets.
- `policies`: Optional concrete guardrail rules defined directly in the root file.

### Imported policy files

Imported child files are intentionally narrow:

- they may declare `policies`
- they must not declare `groups`
- they must not declare `strategy`
- they must not declare `error_strategy`
- they must not declare nested `imports`

This keeps group ownership centralized in `guardrails.yaml` and makes file boundaries predictable.

Current limitation:

- the raw config editor can manage `imports`
- policy CRUD can now update imported policy fragments
- new policies created from the UI are written to `guardrails/custom/policies.yaml`, and the root config will add the import automatically when needed
- group CRUD still assumes root-file ownership and does not rewrite imported policy files

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

Policy Groups are used to manage sets of policies.

A group can provide:
- enabled / disabled state
- severity

A policy can belong to more than one group. A policy is active only when:
- the policy itself is enabled
- and at least one of its assigned groups is enabled

The UI also includes a non-deletable **Default** group.

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

If omitted, the policy applies to all supported scenarios.

## Protected Credentials

Protected Credentials are stored separately from policy definitions.

When a protected credential is enabled:
- the real secret is replaced with an alias before content is sent to the model
- the real value is restored locally when needed for tool execution or local handling

This means credential masking is driven directly by the Protected Credentials page, not by creating a dedicated mask policy.

## Built-in policies

Built-in policies are shipped as embedded examples using the same standard policy format as runtime config fragments.

Behavior:
- built-ins start disabled
- users can enable them manually
- built-ins are not deletable from the UI

At runtime, the intended layout is:

- embedded builtins act as the catalog/example source
- the first enable can materialize a policy file under `guardrails/builtin/`
- project-specific policies live under `guardrails/custom/`

## Example file

See the root example config here:

- `/Users/seviezhou/github/tingly-box/docs/examples/guardrails.yaml`

Related imported fragments:

- `/Users/seviezhou/github/tingly-box/docs/examples/builtin/filesystem_access.yaml`
- `/Users/seviezhou/github/tingly-box/docs/examples/custom/team_rules.yaml`
