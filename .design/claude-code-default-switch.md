# Claude Code: use a profile as the default

Status: implemented. (An earlier snapshot-and-restore design was dropped before
merging: copy and switch are both explicit user actions, so nothing is stored.)

## Problem

People run the default Claude Code routing (`claude_code`) and want, for a while,
exactly what one of their profiles routes to. Editing the default rule by hand,
slot by slot, is slow and error-prone.

## Design: an explicit overwrite, nothing automatic

Two separate, explicit user actions; the product does nothing behind their back.

- **Copy** — keep the default's current config as a profile. This is the
  "Duplicate as profile" action of the profile-copy work
  (`.design/profile-duplicate.md`); this feature adds nothing to it.
- **Switch** — make the default a copy of profile X's config. X is never changed
  and stays independent afterwards. The default's previous config is
  **overwritten**; if the user wants it back, they copied it first.

No automatic backup, no snapshot, no restore, no "custom"/"edited"/"deleted"
states, nothing persisted for the switch. Switching back is switching to the
profile that holds the config you want.

The warning is the safety net: the confirm dialog says plainly that the default
will be overwritten and points at Copy.

## UI

A row on the default Claude Code page only (profiles never show it), above Slots:

```
Use profile   [ ds ]  [ sonnet ]  [ codex ]
```

One chip per profile; hidden when there are no profiles. Clicking a chip opens
one confirm:

```
Use profile "ds"
The default config will be overwritten with a copy of profile "ds". "ds" itself is not changed.
The current default config is not saved — to keep it, copy it as a profile first.

                                   [Cancel]  [Use profile]
```

Success toast: `Default now uses profile "ds"`. No lit chip: the row is an action, not a status
(a derived "which profile equals the default" indicator can come later if wanted).

## Backend

```
POST /api/v1/scenario/{scenario}/claude-code/use-profile   {profile: "p1"}   → 200
```

`Config.UseProfileAsDefault(profile)`: validate (the profile exists and has
routing — otherwise the default would end up unrouted; nothing is written on a
refusal), then overwrite the default's `cc` and slot rules with deep copies of the
profile's (services, LB tactic, smart routing, rule flags; a slot the profile has
not split is switched off so it follows the default again), save, and undo in
memory if the save fails. Only `scenario = claude_code` is accepted (profile
scopes and other scenarios: 400); an unknown profile is 404.

Not copied: the profile's scenario flags and Claude Code overrides (`env`,
`unset_env`, `default_mode`) — those are gateway-wide / per-launch settings that
would rewrite `~/.claude/settings.json`, not routing.

## Relation to the profile-copy work

No code dependency: `UseProfileAsDefault` does not call `DuplicateProfile`; it
only reuses `deepCopyJSON` from `scope_clone.go` and `ErrProfileNotFound`. The
confirm dialog points at the existing "Duplicate as profile" button.

The existing client-config status (`Config out of date · N differences`) reacts to
the changed rules as after any slot change; the user re-applies the Claude Code
settings file as usual. Switching does not rewrite `~/.claude/settings.json`.

## Wording

The user-facing verb is **use a profile** (the product's word for a named config
is "profile" / zh 方案), not "use config"; "switch" stays in code and the doc only.
No "restore" anywhere.

| | en | zh |
|---|---|---|
| row label | Use profile | 使用方案 |
| chip tooltip | Make the default config a copy of this profile; the profile is not changed | 把默认配置换成此方案的副本；此方案本身不会改动 |
| dialog title | Use profile "{{name}}" | 使用方案“{{name}}” |
| dialog body | The default config will be overwritten with a copy of profile "{{name}}". "{{name}}" itself is not changed. | 默认配置将被方案“{{name}}”的副本覆盖；“{{name}}”本身不会改动。 |
| dialog warning | The current default config is not saved — to keep it, copy it as a profile first. | 当前默认配置不会被保存；如需保留，请先把它复制为方案。 |
| confirm | Use profile | 使用方案 |
| toast | Default now uses profile "{{name}}" | 默认配置已改用方案“{{name}}” |
| failure | Failed to use the profile | 使用方案失败 |

(ru follows the same keys.)

## Code

`internal/config/cc_default_switch.go` (`UseProfileAsDefault`, tests in
`cc_default_switch_test.go`), `internal/server/module/scenario/handler_default_switch.go`,
`frontend/src/pages/scenario/components/ClaudeCodeUseProfile.tsx` (shown by
`AgentPage` when the descriptor sets `useProfileRow`, i.e. Claude Code's main
page only).
