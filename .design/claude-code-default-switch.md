# Claude Code: switch the default to a profile's config

Status: **redesign proposed, not implemented.** Branch `tp/switch-default-claude-config-to-47d51b`
still has v1 (copy + persisted snapshot + "restore", see the last section); this
document is the replacement, to be built on top of the profile-copy work
(`.design/profile-duplicate.md` on `tp/reimplement-profile-copy-from-ol-958f2e`).

## Problem

People run the default Claude Code routing (`claude_code`) and want, for a while,
exactly what one of their profiles routes to — then go back. Editing the default
rule by hand, slot by slot, is slow and error-prone.

## Model: copy, then switch — there is no "restore"

The default always **holds a copy** of some config; it never points at a profile
(requests resolve rules by scenario, and a pointer would put a second lookup on
the hot path and make the default's own rules ambiguous).

- **Switching to profile X** overwrites the default's rules with a copy of X's.
  X is never changed and stays independent of the default afterwards.
- **Keeping the default's current config** is just *copying it into a profile first*
  — the same action as "Duplicate as profile" on the default page. Switching
  back later is switching to that profile. So there is no snapshot, no stored
  "original", and no restore action: every config worth keeping is a profile.
- **What the row shows is derived, not stored.** The lit chip is the profile whose
  routing equals the default's right now (same split slots; same services, tactic,
  smart routing and rule flags in every active rule). If none matches, the
  default is **custom**: it has changes no profile holds. Nothing is persisted
  for the switch, so editing the default, deleting a profile, or running two
  tabs can never leave stale "(edited)" / "(deleted)" states.

## Flow

Row on the default page only (profiles never show it), above Slots:

```
Switch to config   [ ds ]  [ sonnet ]  [ codex ]  [ Custom ]      Save as profile…
```

- One chip per profile; the lit chip is the one equal to the default. If none is,
  a lit, non-clickable **Custom** chip stands for "the default's own,
  unsaved config". Hidden when there are no profiles (the title button
  "Duplicate as profile" already covers saving the default).
- Click a chip other than the lit one → one confirm dialog (this is the whole
  "two clicks"):

```
Switch to "ds"
The default will become a copy of "ds". "ds" itself is not changed.

[x] First save the current default as profile  [ default-copy ]
    (unchecked: the current default config is overwritten and cannot be recovered)

                                   [Cancel]  [Switch]
```

  The checkbox appears **only when the default is Custom** (no profile holds it)
  and is checked by default; when the default already equals some profile,
  switching away loses nothing, so the dialog is just a confirm. The name field
  reuses the copy branch's suggestion (`suggestProfileCopyName('default', …)` →
  `default-copy`, `-2`, …) and its validation errors, shown inline.
- Result: toast "Switched to "ds"", or "Switched to "ds"; the previous default
  was saved as "default-copy"". The new profile is now a chip, so going back is
  one click on it (and it lights up as soon as the default equals it).
- **Save as profile…** (shown while Custom) is the existing `DuplicateDialog`
  on the default config — saving without switching.

## Who copies: the backend, in one call

```
POST /api/v1/scenario/claude_code/default/switch   {profile: "p1", save_current_as?: "default-copy"}
  → {matches: "p1", saved_as?: ProfileMeta}
GET  /api/v1/scenario/claude_code/default/match    → {profile_id: "p1" | null}
```

`Config.UseProfileAsDefault(profile, saveCurrentAs)`:
1. validate first (profile exists and is routed; if `saveCurrentAs` is set, a free valid name) — nothing is written on a refusal;
2. if `saveCurrentAs != ""`: `DuplicateProfile(claude_code, "default", saveCurrentAs)` — the copy branch's
   existing function and endpoint semantics, unchanged;
3. overwrite the default's `cc` and slot rules with deep copies of the profile's
   (a slot the profile has not split is switched off so it follows the default
   again) and save; undo in memory if the save fails.

Order is the safety property: the backup is written **before** the overwrite, so a
failure leaves at worst an extra profile, never a lost config. The frontend does
not compose two calls, so there is no window between them and no partial state
to explain. `GET …/match` backs the lit chip (also returned by the POST).

## Reuse of the copy work

| need | reuse | add |
|---|---|---|
| save current default as a profile | `Config.DuplicateProfile(base, ProfileMainID, name)` | — |
| deep copy of rules | `deepCopyJSON`, `clonedRuleUUID` (`scope_clone.go`) | a replace-in-place variant: `cloneScopeLocked` requires an empty destination; here the destination is the default, whose built-in rules keep their UUIDs |
| name field, suggestion, errors | `DuplicateDialog`, `suggestProfileCopyName`, `profileApi.duplicateProfile` | a checkbox slot in the dialog (or a small `SwitchDialog` composing the same pieces) |
| "Duplicate as profile" entry | `DuplicateMainProfileButton` (title addon) | nothing — it stays; "Save as profile…" in the row opens the same dialog |

The copy branch is not modified by this work; the new overwrite helper lives in
its own file next to `scope_clone.go`.

**Branch order.** This depends on the copy branch (`DuplicateProfile`,
`DuplicateDialog`, `copyName`, `titleAddon`) and both touch `routes.go`,
`types.go`, `openapi.json` (generated — regenerate, don't merge), the `agent.ts`
i18n blocks and `mocks/handlers.ts`. Recommended: **land the copy branch first,
then rebase this one on `main`.** If that is slow, rebase onto the copy branch
(stacked) and retarget after it merges; do not duplicate its code here.

## UI wording

Terms follow the user's: **复制配置 / 切换到配置**; no "restore" anywhere.

| | en | zh |
|---|---|---|
| row label | Switch to config | 切换到配置 |
| custom chip | Custom | 自定义 |
| profile chip tooltip | Make the default a copy of this config; this config is not changed | 把默认配置换成此配置的副本；此配置本身不会改动 |
| lit chip tooltip | The default is identical to this config | 默认配置与此配置完全一致 |
| custom chip tooltip | The default has changes no saved config holds; switching away offers to save a copy first | 默认配置有尚未存成配置的改动；切走时会先问你是否存一份副本 |
| link | Save as profile… | 存为配置… |
| dialog title | Switch to "{{name}}" | 切换到“{{name}}” |
| dialog body | The default will become a copy of "{{name}}". "{{name}}" itself is not changed. | 默认配置将变成“{{name}}”的一份副本；“{{name}}”本身不会改动。 |
| checkbox | First save the current default as config | 先把当前默认配置存为配置 |
| unchecked warning | The current default config will be overwritten and cannot be recovered. | 当前默认配置会被覆盖，无法找回。 |
| confirm | Switch | 切换 |
| toast | Switched to "{{name}}" | 已切换到“{{name}}” |
| toast with backup | Switched to "{{name}}"; the previous default was saved as "{{saved}}" | 已切换到“{{name}}”；原默认配置已存为“{{saved}}” |
| failure | Failed to switch the default config | 切换默认配置失败 |

(ru follows the same keys.) The user-facing word for a profile in zh would be
"配置" here, while the rest of the zh UI says "方案" — see open questions.

## What happens to v1

v1 is unmerged (two commits on this branch), so nothing is migrated:

- **Removed:** `Config.ClaudeCodeDefaultSwitch` (+ the `claude_code_default_switch` key in config.json), `SwitchClaudeCodeDefaultToProfile`'s snapshot,
  `RestoreClaudeCodeDefault`, `DELETE …/default-switch`, the stored `profile_id/profile_name/in_sync/profile_exists` status and the "(edited)" / "(deleted)" chip states + i18n keys.
- **Kept and adapted:** the rule-overwrite loop (`copyCCRoutingDeep`, `setCCSlotLocked` usage) becomes step 3 above; `ccRoutingMatchesLocked` / `ccRoutingJSON`
  become the derived "which profile equals the default" check (run against each profile);
  the chip row component, `useSlotRouting.reload`, API wrappers and mock handlers are reshaped, not rewritten.
- **Tests:** snapshot/restore/persistence tests are replaced by: backup-then-overwrite order (including a failing backup name leaves the default untouched), custom vs matching detection, slot split/merge following the profile, source profile untouched and unaliased, and the handler refusing non-default scopes.

## Open questions

1. **Vocabulary.** The user says 配置; the zh UI calls profiles 方案. Using 配置 only in this row splits one concept across two words (principle 3). Either rename 方案→配置 everywhere (separate change) or use 方案 here.
2. **What "same config" covers.** v1 and this proposal copy *routing rules* only. A profile can also carry its own scenario flags and Claude Code overrides (`env`, `unset_env`, `default_mode`, which shape its generated settings file). Applying those to the default would rewrite gateway-wide flags and the user's `~/.claude/settings.json`; recommended: not in this change, the lit chip compares routing only.
3. **Backup default.** Checkbox pre-checked when the default is Custom (recommended), or always auto-save a `default-copy` silently (fewer clicks, but creates profiles the user did not ask for).
4. **Branch order** as above: wait for the copy branch to merge, or stack on it.
