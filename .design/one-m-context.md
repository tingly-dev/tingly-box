# 1M Context Window — Flag-First Design

> Audience: tingly-box contributors. How the 1M context window is handled
> per agent scenario.

## Problem

Models support 1M context, but agent configs (Claude Code, Claude Desktop,
Codex) are one-time: no way to independently toggle 1M per model or propagate
it to profiles.

## Approach

**One source of truth: `Rule.Context1M` (`context_1m`).** The rule's
`request_model` always stays clean — toggling 1M never renames the rule.
The `[1m]` suffix is a *client* convention (Claude Code / Claude Desktop
read the model name, perceive `[1m]`, and send the `context-1m-2025-08-07`
beta header), not a tingly-box concept, so it is appended only when
**rendering the client-facing config** from the flag.

The 1M switch sits on the rule card's model node (`ModelRequestHeader.oneM`);
`RuleCard` always reads/writes `context_1m`. The three `Use*` pages that use
1M opt in via `TemplatePage`'s `showOneM` prop; everywhere else the switch is
hidden.

### Render sites (flag → client config)

- **Claude Code** (`derivePrefsFromRules`, frontend; `resolveClaudeCodeModels`
  in `internal/tbclient`, backend launch path): when the rule has
  `context_1m`, the env value gets the suffix (`ANTHROPIC_*_MODEL=ds[1m]`).
- **Claude Desktop** (`buildInferenceModelsJson`): suffixed name is emitted
  into the `inferenceModels` list.
- **Codex** (`CollectCodexContext1M` → `RenderCodexModelCatalog`): no wire
  suffix at all — the flag sets the catalog `context_window: 1000000`.
- **Others**: switch hidden, nothing materialised.

User re-applies the config (and restarts the client) to take effect.

A manually `[1m]`-named rule (hand-edited config) still flows through
verbatim; the suffix helpers are idempotent (`WithContextWindow1M` strips
before appending), so flag + suffixed name never double-tags.

## Routing — `MatchRuleByModelAndScenario`

The client requests the suffixed name (`ds[1m]`) while the rule stores the
clean name (`ds`), so matching is **`[1m]`-tolerant**: after an exact match
fails, both the incoming model and each rule's `request_model` are compared
with `[1m]` stripped.

```
priority: exact match > [1m]-tolerant (stripped) match > wildcard
```

## Beta header forwarding

Upstream #1157 (`mergeBetaFlags` + `claudeCodeAllowedUpstreamBetas` whitelist)
forwards `context-1m-2025-08-07` through to Anthropic.

## Code map

- `internal/typ/type.go` — `Rule.Context1M` (single source of truth).
- `internal/typ/model_tag.go` — `[1m]` helpers (`StripContextWindow1M`,
  `WithContextWindow1M`). Mirrored in
  `frontend/src/components/rule-card/utils.ts` (`stripOneM` / `withOneM`).
- `internal/server/config/config.go::MatchRuleByModelAndScenario` —
  `[1m]`-tolerant match.
- `internal/server/config/apply_config.go` — `CollectCodexContext1M`,
  `RenderCodexModelCatalog(models, context1M)`, `ApplyCodexConfig(..., context1M)`.
- `internal/tbclient/tb_client.go::resolveClaudeCodeModels` — appends `[1m]`
  for flagged rules in the `tb`-launched Claude Code env.
- Frontend: `RuleCard` flag toggle (shown via `TemplatePage.showOneM`);
  `derivePrefsFromRules` / `buildInferenceModelsJson` append `[1m]` at render
  time; `context_1m` round-trips through `ruleToConfigRecord` /
  `buildRuleUpdatePayload` so the flag survives autosaves of other fields.
