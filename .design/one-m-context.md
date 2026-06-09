# 1M Context Window — 4 Scenario Cases

> Audience: tingly-box contributors. How 1M context window is handled for
> each agent scenario.

## Problem

Models support 1M context, but agent configs (CC, Claude Desktop, Codex)
are one-time: no way to independently toggle 1M or propagate it to profiles.

## Approach: `Rule.Context1M` flag + scenario-specific materialisation

`rule.request_model` stays **clean** (e.g. `"tingly/cc-sonnet"`). A separate
`Rule.Context1M bool` flag is the single source of truth. Each scenario
reads this flag at config-generation time and materialises it in the
format that scenario's client expects.

### Case 1 — Claude Code

`resolveCCModelSlots` reads `Context1M` and appends `[1m]` to the model
name in the env vars written to `settings.json`:

```
rule: { request_model: "tingly/cc-sonnet", context_1m: true }
→ env: ANTHROPIC_DEFAULT_SONNET_MODEL=tingly/cc-sonnet[1m]
→ CC perceives [1m], sends context-1m-2025-08-07 beta header
```

User must re-apply config and restart CC for the change to take effect.
Profiles also work: `resolveCCModelSlots` resolves profile rules the same way.

### Case 2 — Claude Desktop

`buildInferenceModelsJson` appends `[1m]` to model names in the generated
`inferenceModels` config when `context_1m` is set on the rule. User must
re-apply the config snippet to `claude_desktop_config.json`.

### Case 3 — Codex

`RenderCodexModelCatalog` sets `context_window: 1000000` (instead of 200k)
for models with `Context1M=true`. `CollectCodexContext1M` reads the flag
from codex-scenario rules. User must re-apply the Codex config.

### Case 4 — Others (OpenAI, Anthropic, etc.)

The flag is stored on the rule. No client config materialisation needed —
the server can use the flag to inform `/models` endpoint or future features.

## Server-side routing

When CC sends `tingly/cc-sonnet[1m]`, `MatchRuleByModelAndScenario` strips
`[1m]` and matches the clean rule:

```
priority: exact match > strip-[1m] match > wildcard match
```

## Beta header forwarding

Upstream #1157 (`mergeBetaFlags` + `claudeCodeAllowedUpstreamBetas` whitelist)
allows `context-1m-2025-08-07` to pass through to Anthropic.

## Frontend

One toggle: the 1M switch on the rule card's model header
(`ModelRequestHeader.oneM`), visible for CC / Claude Desktop / Codex
scenarios. Writes `context_1m` via the existing autosave path.

Quick Config derives env strings from `request_model + context_1m`
(read-only — no inline 1M toggle in the modal).
