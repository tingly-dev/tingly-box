# Rule UUID Conventions

Rule UUIDs in tingly-box are not all random identifiers — built-in rules use
deterministic, human-readable UUIDs so that every consumer (frontend quick
config, `tbclient`, TUI quickstart, config migrations) can address them by a
stable constant:

| Kind | Format | Example |
|---|---|---|
| Legacy built-ins | hyphenated string | `built-in-cc`, `built-in-cc-haiku`, `built-in-codex` |
| Modern built-ins | `builtin:<scenario>:<model>` | `builtin:claude_desktop:claude-haiku-4-5` |
| SmartGuide internal | `_internal_smart_guide_<botUUID>` | — |
| User-created rules | random v4 UUID | — |

The constants live in `internal/server/config/migration.go`
(`RuleUUIDBuiltin*`), alongside `BuiltinRuleUUID(scenario, model)` which
builds the modern form. Anything that is system-seeded must have a
deterministic UUID; randomness is reserved for rules the user creates.

**Direction:** `builtin:<scenario>:<model>` is the target convention. The
legacy `built-in-*` UUIDs will eventually be migrated onto it (a larger
change touching every consumer that hardcodes them); new system-seeded
rules must use the modern form from day one.

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

Profile rules are therefore already on the target convention — when the
legacy `built-in-cc-*` main-scenario rules are unified later, profiles
need no further migration.

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
  `RuleStateStore.RenameRuleUUID` (load-balancer position) and
  `UsageStore.RenameRuleUUID` (historical usage attribution). Daily/monthly
  usage aggregates do not carry `rule_uuid` and need no migration.

### Profile deletion

Because profile IDs are recycled, `DeleteProfile` purges the deleted
rules' rows from `rule_service_index` (`RuleStateStore.DeleteRules`).
Otherwise a future profile reusing the same ID would inherit the old
profile's service pinning. Usage records are intentionally kept — they are
historical facts about the old profile.
