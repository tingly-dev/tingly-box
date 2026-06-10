# Profile Rule UUID Convention

## Problem

Built-in rules use deterministic, human-readable UUIDs (`built-in-cc`,
`built-in-cc-haiku`, `builtin:claude_desktop:claude-haiku-4-5`), and every
consumer that needs to address a built-in rule relies on that stability:
the frontend quick config, `tbclient`, the TUI quickstart, and several
config migrations all look rules up by these constants.

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

## Convention

A profile rule's UUID is the main-scenario built-in UUID plus the profile
suffix, using the same `:` separator as profiled scenario names:

```
<builtin-uuid>:<profileID>
```

| Tier (request_model) | Built-in (main scenario) | Profile p1            |
|----------------------|--------------------------|-----------------------|
| `cc` (unified)       | `built-in-cc`            | `built-in-cc:p1`      |
| `default`            | `built-in-cc-default`    | `built-in-cc-default:p1` |
| `haiku`              | `built-in-cc-haiku`      | `built-in-cc-haiku:p1`   |
| `sonnet`             | `built-in-cc-sonnet`     | `built-in-cc-sonnet:p1`  |
| `opus`               | `built-in-cc-opus`       | `built-in-cc-opus:p1`    |
| `subagent`           | `built-in-cc-subagent`   | `built-in-cc-subagent:p1` |

Helpers live in `internal/server/config/migration.go`:

- `ProfileRuleUUID(builtinUUID, profileID)` — builds the canonical UUID;
- `ccProfileBuiltinByModel` — maps the short tier name a profile rule
  routes on to its built-in counterpart.

`newCCProfileRules` (config.go) derives the profile ID from the profiled
scenario and assigns canonical UUIDs at creation time.

## Migration (`migrate20260611`)

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

## Profile deletion

Because profile IDs are recycled, `DeleteProfile` now purges the deleted
rules' rows from `rule_service_index` (`RuleStateStore.DeleteRules`).
Otherwise a future profile reusing the same ID would inherit the old
profile's service pinning. Usage records are intentionally kept — they are
historical facts about the old profile.
