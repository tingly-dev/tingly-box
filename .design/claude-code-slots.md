# Claude Code: per-slot rules (no modes)

Status: implemented (TEP #2197; #2195, #2196, #2198, #2199).

## Problem

Claude Code used to have two modes: **unified** (every model slot requests the `cc` rule) and **separate** (every slot has its own rule). The common need sits in between — "everything on one model, but the subagent on another" — and meant switching to separate and keeping five rules identical by hand. Two modes also meant two rule sets, two env resolvers, two UIs and a mode change that rewrote rules.

## Design

There are no modes. The state is the rules themselves; there is no slot list and no mode flag:

- The **default slot** (`ANTHROPIC_MODEL`) is the rule `builtin:<scenario>:cc` (request model `cc`) and is always on.
- Each other slot — `haiku` `sonnet` `opus` `fable` `subagent` — has a rule `builtin:<scenario>:<slot>`. **A slot whose rule is active has its own rule**; an inactive rule means the slot follows the default slot.
- "Separate" is just every slot split; "unified" is none split. The legacy `unified` / `separate` scenario flags survive only as shortcuts (`separate=true` splits all, `unified=true` merges all) and are read back as derived values, so old clients keep working.

UI: a **Slots** row under Base URL / API Key, for the main scenario and for profiles alike. `default` is always lit; clicking another slot gives it its own rule (the rules card then shows it) or hands it back. "Split all" / "Merge all" flip every slot at once. There is no mode switch and no mode dialog; creating a profile does not ask for a mode.

## Invariants

- Turning a slot on reuses its built-in rule. A rule without routing yet — or a missing rule — starts as a copy of the default rule's routing, so nothing changes until the user edits it. Turning it off only switches the rule off, so turning it on again restores it.
- Only slot toggles touch tier rules. A whole-record scenario write never does (`SetScenarioConfig` just stores).
- `agent apply` switches only the default rule on. Tier rules that are on (split slots) are left untouched; the others get the provider but stay off.
- Rejected: unknown profiles and the default slot. Everything else, main scenario and profiles, is accepted.
- `agent.ClaudeCodeSlotModels` is the one resolver behind the settings file and the tbclient env: a slot with an active rule of its own requests it, otherwise the default slot's request model. `derivePrefsFromRules({ rules, slots })` mirrors it in the frontend.
- TUI quickstart asks only for the subagent model: another model splits the subagent slot (routed to exactly that model — no inherited smart routing or 1M), "same as the default" undoes an earlier split. It never switches tier rules on.
- `agent apply --unified` is accepted and ignored.

- `ANTHROPIC_MODEL` is written only when the sonnet slot is split (`agent.OmitFollowedDefaultModel`, also in `ai/agent` `BuildEnv`). With `ANTHROPIC_MODEL=cc` and `ANTHROPIC_DEFAULT_SONNET_MODEL=cc`, Claude Code's `/model` picker (checked on 2.1.309) ticks **Sonnet**, the first tier resolving to `cc`, never **Default**; with no `ANTHROPIC_MODEL` it ticks Default, and Default *is* the sonnet tier, i.e. the same model. A split sonnet slot keeps `ANTHROPIC_MODEL=cc` (Default must stay the main rule) and the picker shows it as the custom model "cc". `ClaudeCodeTiersFromEnv` falls back to the sonnet tier when the key is absent; the internal resolver (`ClaudeCodeSlotModels`) still returns the default slot. Files written earlier show as outdated in the config status until applied again; profile settings drop a stale `ANTHROPIC_MODEL` inherited from the main file.

## Migration

Both are one-time and idempotent through `MigrationsCompleted` markers.

- `20261022-claude-code-unified-slot-rules`: old `agent apply` switched every tier rule on regardless of mode, which would now read as "all slots split". For unified configs the tier rules are switched off. Separate configs are left alone.
- `20261023-claude-code-fold-separate`: for a separate scenario or profile, the `default` rule's routing is copied into `cc` (which becomes active) and the `default` rule is deleted; its slot rules stay on as they were, so routing is unchanged. `Flags.Unified/Separate` are cleared and profiles are marked unified. Unified configs just lose their (unused) `default` rule.
- `20261024-claude-code-short-request-models`: the main scenario's rules took the names profiles always had — `tingly/cc` → `cc`, `tingly/cc-sonnet` → `sonnet`, … Only a rule still carrying its seeded name is renamed, and never onto a name another rule already answers to.
- Settings files written earlier still name `tingly/cc`, `tingly/cc-<slot>`, `tingly/cc-default` (profile: `default`). `MatchRuleByModelAndScenario` aliases all of them to the short-named slot rule (`legacyCCRequestAlias`) **permanently**; no "separate" label is kept for migrated profiles.

## Left behind on purpose

`RuleUUIDCCDefault`, `RuleUUIDBuiltinCCDefault`, `ScenarioFlags.Unified/Separate`, `ProfileMeta.Unified` and `constant.FlagUnified/FlagSeparate` stay: the migrations above and the legacy flag shortcuts read them. Retire them by the policy in `config-migration.md`, not ad hoc.
