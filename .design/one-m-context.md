# 1M Context Window — 4 Scenario Cases

> Audience: tingly-box contributors. How the 1M context window is handled
> per agent scenario.

## Problem

Models support 1M context, but agent configs (Claude Code, Claude Desktop,
Codex) are one-time: no way to independently toggle 1M per model or propagate
it to profiles.

## Approach

1M is only meaningful for three scenarios, and they differ in *how* the
client perceives it — so each is handled where it lives, not through generic
infrastructure. The 1M switch sits on the rule card's model node
(`ModelRequestHeader.oneM`); its handler in `RuleCard` branches by scenario.

### Case 1 — Claude Code  &  Case 2 — Claude Desktop

`[1m]` is a **client convention baked into the model name**. Toggling 1M
**renames the rule's `request_model`** (e.g. `tingly/cc-sonnet` →
`tingly/cc-sonnet[1m]`). That renamed name is both the tingly-box rule name
and the model name the client is told to use:

- Claude Code: `derivePrefsFromRules` reads `request_model` verbatim into the
  env (`ANTHROPIC_*_MODEL`), so the CC client perceives `[1m]` and sends the
  `context-1m-2025-08-07` beta header.
- Claude Desktop: `buildInferenceModelsJson` emits `request_model` verbatim
  into the `inferenceModels` list.

User re-applies the config and restarts the client to take effect.

### Case 3 — Codex

Codex has **no wire suffix** — 1M is purely a catalog `context_window`
budget. Toggling 1M flips the `context_1m` flag on the rule (no rename).
`CollectCodexContext1M` reads the flag and `RenderCodexModelCatalog` sets
`context_window: 1000000` for those models. User re-applies the Codex config.

### Case 4 — Others (OpenAI, Anthropic, …)

No 1M handling — the switch is hidden. Nothing to materialise.

## Routing — `MatchRuleByModelAndScenario`

Because the rule name carries `[1m]` for CC/CD, an incoming `ds[1m]` matches
the renamed rule exactly. To stay robust when the suffix appears on only one
side (e.g. a launch path that emits the clean name, or hand-edited configs),
matching is **`[1m]`-tolerant**: after an exact match fails, both the incoming
model and each rule's `request_model` are compared with `[1m]` stripped.

```
priority: exact match > [1m]-tolerant (stripped) match > wildcard
```

Rules are updated in place by UUID (`UpdateRule`), so a rename doesn't orphan
rule state, and the uniqueness guard prevents a colliding name.

## Beta header forwarding

Upstream #1157 (`mergeBetaFlags` + `claudeCodeAllowedUpstreamBetas` whitelist)
forwards `context-1m-2025-08-07` through to Anthropic.

## Code map

- `internal/typ/model_tag.go` — `[1m]` helpers (`HasContextWindow1M`,
  `StripContextWindow1M`, `WithContextWindow1M`). Mirrored in
  `frontend/src/components/rule-card/utils.ts`.
- `internal/server/config/config.go::MatchRuleByModelAndScenario` —
  `[1m]`-tolerant match.
- `internal/server/config/apply_config.go` — `CollectCodexContext1M`,
  `RenderCodexModelCatalog(models, context1M)`, `ApplyCodexConfig(..., context1M)`.
- `internal/typ/type.go` — `Rule.Context1M` (Codex only).
- Frontend: `RuleCard` scenario-aware toggle handler; `context_1m` round-trips
  through `ruleToConfigRecord` / `buildRuleUpdatePayload` so the Codex flag
  survives autosaves of other fields.
