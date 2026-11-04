# Claude Code: switch the default to a profile's config

Status: **simplified design proposed, not implemented.** The branch still has v1
(copy + persisted snapshot + "restore"); this document replaces it.

## Problem

People run the default Claude Code routing (`claude_code`) and want, for a while,
exactly what one of their profiles routes to. Editing the default rule by hand,
slot by slot, is slow and error-prone.

## Design: an explicit overwrite, nothing automatic

Two separate, explicit user actions; the product does nothing behind their back.

- **Copy** — keep the default's current config as a profile. This is the
  "Duplicate as profile" action of the profile-copy work
  (`.design/profile-duplicate.md` on `tp/reimplement-profile-copy-from-ol-958f2e`);
  this feature adds nothing to it.
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
Switch to config   [ ds ]  [ sonnet ]  [ codex ]
```

One chip per profile; hidden when there are no profiles. Clicking a chip opens
one confirm:

```
Switch to "ds"
The default config will be overwritten with a copy of "ds". "ds" itself is not changed.
The current default config is not saved — to keep it, copy it as a profile first.

                                   [Cancel]  [Switch]
```

Success toast: `Switched to "ds"`. No lit chip: the row is an action, not a status
(a derived "which profile equals the default" indicator can come later if wanted).

## Backend

```
POST /api/v1/scenario/claude_code/default/switch   {profile: "p1"}   → 200
```

`Config.UseProfileAsDefault(profile)`: validate (the profile exists and has
routing — otherwise the default would end up unrouted; nothing is written on a
refusal), then overwrite the default's `cc` and slot rules with deep copies of the
profile's (services, LB tactic, smart routing, rule flags; a slot the profile has
not split is switched off so it follows the default again), save, and undo in
memory if the save fails. Only the main scenario is accepted; profile scopes and
other scenarios are rejected.

Not copied: the profile's scenario flags and Claude Code overrides (`env`,
`unset_env`, `default_mode`) — those are gateway-wide / per-launch settings that
would rewrite `~/.claude/settings.json`, not routing.

## Relation to the profile-copy branch

No code dependency any more: the switch does not call `DuplicateProfile`. The
branches only touch the same files (`routes.go`, `types.go`, `openapi.json` —
regenerate, not merge —, the `agent.ts` i18n blocks, `mocks/handlers.ts`), so
either can land first. The dialog text mentions copying; wording stays correct
whether or not the copy button has landed.

## Wording

| | en | zh |
|---|---|---|
| row label | Switch to config | 切换到配置 |
| chip tooltip | Make the default config a copy of this one; this one is not changed | 把默认配置换成此配置的副本；此配置本身不会改动 |
| dialog title | Switch to "{{name}}" | 切换到“{{name}}” |
| dialog body | The default config will be overwritten with a copy of "{{name}}". "{{name}}" itself is not changed. | 默认配置将被“{{name}}”的副本覆盖；“{{name}}”本身不会改动。 |
| dialog warning | The current default config is not saved — to keep it, copy it as a profile first. | 当前默认配置不会被保存；如需保留，请先把它复制为配置。 |
| confirm | Switch | 切换 |
| toast | Switched to "{{name}}" | 已切换到“{{name}}” |
| failure | Failed to switch the default config | 切换默认配置失败 |

(ru follows the same keys.) zh says 配置 here while the rest of the zh UI says
方案 for profiles — one concept, two words; settle which word wins separately.

## What happens to v1 (unmerged, nothing to migrate)

- **Removed:** `Config.ClaudeCodeDefaultSwitch` (and the `claude_code_default_switch`
  key), `RestoreClaudeCodeDefault`, `GET`/`DELETE …/default-switch`, the status
  struct, the "(edited)" / "(deleted)" chip states and their i18n keys, the
  routing-equality helper.
- **Kept:** the rule-overwrite loop (`copyCCRoutingDeep`, slot split/merge) becomes
  `UseProfileAsDefault`; the chip row, `useSlotRouting.reload`, API wrapper and mock
  handler shrink to the one POST.
- **Tests:** replace snapshot/restore/persistence tests with: overwrite copies
  routing and slot split/merge, source profile untouched and unaliased, a
  rejected switch changes nothing, non-default scopes refused.
