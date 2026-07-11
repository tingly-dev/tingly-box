# Rule UUID Conventions

Rule UUIDs in tingly-box are not all random identifiers — built-in rules use
deterministic, human-readable UUIDs so that every consumer (frontend quick
config, `tbclient`, TUI quickstart, config migrations) can address them by a
stable constant:

| Kind | Format | Example |
|---|---|---|
| Modern built-ins | `builtin:<scenario>:<tier>` | `builtin:claude_code:haiku`, `builtin:openai:default`, `builtin:claude_desktop:claude-haiku-4-5` |
| Legacy built-ins | hyphenated string | `built-in-openai`, `built-in-codex`, `built-in-opencode` |
| SmartGuide internal | `_internal_smart_guide_<botUUID>` | — |
| User-created rules | random v4 UUID | — |

The constants live in `internal/server/config/migration.go`
(`RuleUUIDCC*` / `RuleUUIDBuiltin*`), alongside
`BuiltinRuleUUID(scenario, model)` which builds the modern form. Anything
that is system-seeded must have a deterministic UUID; randomness is
reserved for rules the user creates.

**Direction:** `builtin:<scenario>:<model>` is the target convention.
All system-seeded built-ins now use this form. New system-seeded rules
must use the modern form from day one.

## Claude Code main scenario

The six Claude Code built-ins migrated from legacy hyphenated UUIDs to
the modern form (`migrate20260611`, renamed by exact legacy-UUID match so
user-customized request models are irrelevant):

| Legacy | Modern |
|---|---|
| `built-in-cc` | `builtin:claude_code:cc` |
| `built-in-cc-default` | `builtin:claude_code:default` |
| `built-in-cc-haiku` | `builtin:claude_code:haiku` |
| `built-in-cc-sonnet` | `builtin:claude_code:sonnet` |
| `built-in-cc-opus` | `builtin:claude_code:opus` |
| `built-in-cc-subagent` | `builtin:claude_code:subagent` |

The legacy constants (`RuleUUIDBuiltinCC*`) are kept for two reasons:
older migrations in the `Migrate` chain run *before* the rename and must
still address pre-rename configs, and runtime consumers (`generateCCEnv`,
`tbclient.resolveClaudeCodeModels`) keep a legacy-UUID fallback for
configs loaded without migration. `defaultRuleByUUID` resolves legacy
aliases to the modern templates so old migrations keep finding them in
`DefaultRules`.

## Profile rules

Currently the only profiled surface is Claude Code (`claude_code:p1`, …),
so this is the only section at this stage; new profiled scenarios should
follow the same scheme.

### Problem

Profile rules (the per-profile copies created for `claude_code:p1`,
`claude_code:p2`, …) were created with **random v4 UUIDs**
(`uuid.New().String()` in `newCCProfileRules`). That broke the convention
and produced linkage side effects:

- profile rules could not be addressed deterministically — any consumer
  had to fall back to matching by `request_model` or loading the whole
  scenario rule list;
- rule identity was not reproducible across installs/exports, so SQLite
  state keyed by `rule_uuid` (load-balancer `rule_service_index`, usage
  records) could never be correlated with "the haiku rule of profile p1";
- delete + recreate of a profile produced brand-new identities even though
  profile IDs themselves are recycled (`p1` is reused after deletion).

### Convention

A profile rule's UUID applies the modern built-in form directly, with the
profiled scenario name as the scenario segment:

```
builtin:<base>:<profileID>:<tier>     // BuiltinRuleUUID(profiledScenario, tier)
```

| Tier (request_model) | Profile p1 |
|----------------------|------------------------------|
| `cc` (unified)       | `builtin:claude_code:p1:cc`  |
| `default`            | `builtin:claude_code:p1:default` |
| `haiku`              | `builtin:claude_code:p1:haiku`   |
| `sonnet`             | `builtin:claude_code:p1:sonnet`  |
| `opus`               | `builtin:claude_code:p1:opus`    |
| `subagent`           | `builtin:claude_code:p1:subagent` |

Profile and main-scenario UUIDs share one builder: the profile form is
just `BuiltinRuleUUID` applied to the profiled scenario name, so
`builtin:claude_code:haiku` (main) and `builtin:claude_code:p1:haiku`
(profile) come from the same rule.

`ccProfileTiers` (migration.go) is the set of system-seeded tier names;
`newCCProfileRules` (config.go) assigns canonical UUIDs at creation time
via `BuiltinRuleUUID(profiledScenario, requestModel)`.

### Migration (`migrate20260611`)

Normalizes existing configs:

- every `claude_code:<pN>` rule whose `request_model` is a known tier gets
  renamed to the canonical UUID;
- custom request models (no built-in counterpart) are left untouched;
- a rename is skipped (with a warning) if the canonical UUID is already
  taken, so duplicate tier rules can't steal an identity;
- the pass is idempotent and **not marker-gated** — it self-heals configs
  written by older builds on every start;
- SQLite state keyed by `rule_uuid` is re-keyed along with the rule:
  `UsageStore.RenameRuleUUID` (historical usage attribution). Daily/monthly
  usage aggregates do not carry `rule_uuid` and need no migration.
  (`RuleStateStore.RenameRuleUUID` used to re-key the load-balancer
  position too; that store was removed with the phantom CurrentServiceID
  pointer — the orphaned `rule_service_index` table is left in old DBs.)

### Usage path (`tingly-box cc --profile`)

`generateCCEnv` (internal/command/cc_command.go) resolves the per-tier
`ANTHROPIC_*_MODEL` env vars from the rules the request will actually hit,
looked up by canonical UUID:

- profile mode: `BuiltinRuleUUID(profiledScenario, tier)` →
  `builtin:claude_code:p1:haiku`, falling back to the seeded short tier
  name (`haiku`) when the rule is missing/inactive;
- main scenario: the legacy `built-in-cc-*` constants, falling back to the
  canonical `tingly/cc-*` names (same scheme as
  `tbclient.resolveClaudeCodeModels`).

Before normalization this was impossible for profiles — the env hardcoded
the seeded short names and silently broke if a user renamed a profile
rule's `request_model`. Request routing itself matches by
scenario + request_model and needs no UUID.

1M context interacts with this path: a rule with the `context_1m` flag is
advertised to Claude Code with a `[1m]` model-name suffix (the client
strips it back off and sends the `context-1m` beta header). Both
`generateCCEnv` and `tbclient.resolveClaudeCodeModels` append the suffix
when the canonical rule carries the flag, so toggling 1M on a profile
rule takes effect on the next `cc --profile` launch without any modal.
The 1M auto-detection (`context_1m_integration.go`) matches on the base
scenario, so profiled scenarios are covered.

Claude Desktop is special: it has no env channel — its model picker comes
verbatim from `/v1/models`, which lists rule request models. Toggling
`context_1m` on a claude_desktop rule therefore renames the rule itself
(`UpdateRule` keeps the `[1m]` suffix in sync with the flag), so the
picker shows the 1M variant and the picked name routes back exactly.

Because the suffix can legitimately exist on either side independently
(renamed rule vs. stale client config, suffixed env vs. bare rule),
`MatchRuleByModelAndScenario` normalizes `[1m]` on **both** the incoming
model and the rule name before comparing — for claude_code and
claude_desktop base scenarios only (exact match still wins first; other
scenarios keep strict matching).

### Profile deletion

Usage records are intentionally kept across profile deletion — they are
historical facts about the old profile. (`RuleStateStore.DeleteRules` used
to also purge load-balancer service pinning here; that store was removed
with the phantom CurrentServiceID pointer, so there is nothing left to
purge.)
