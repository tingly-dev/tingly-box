# Claude Code: switch the default to a profile

Status: implemented.

## Problem

People often run the default Claude Code routing (`claude_code`) and want, for a while, exactly what one of their profiles routes to — then go back. Editing the default rule by hand, slot by slot, is slow and error-prone.

## Design

The default page has a **Switch** row above Slots: "default routing to  [My own] [ds] [sonnet] …". One chip per profile plus "My own"; the lit chip is what the default routes like now. Clicking a profile chip makes the default take a **copy** of that profile's routing; clicking "My own" restores. Everything is visible at once, so there is no menu to open and no separate restore action. A chip whose profile was edited-after-switch reads "name (edited)" and re-copies on click. Profiles never show the control — only the main scenario can be switched (the endpoint rejects `claude_code:<id>` and other scenarios).

- **Copy, not pointer.** Requests resolve rules by scenario; a pointer would put a second lookup on the hot path and make the default's rules read-only or ambiguous. A copy keeps the source profile untouched and independent (rules are deep-copied through their persisted JSON form; no `*Service` is shared), and later edits to the default or the profile do not leak into each other.
- **What is copied.** The default rule (`cc`) and every slot rule: services, LB tactic, smart routing, rule flags. A slot the profile has split becomes split in the default; a slot it has not split follows the default again (the default's own slot rule is only switched off, as with the Slots row). Scenario-level flags and the profile's `claude_code` env/permission overrides are *not* copied: those are gateway-wide or per-launch settings, not routing.
- **Restore.** The first switch stores the default's cc/slot rules (`Config.ClaudeCodeDefaultSwitch.Original`, persisted in config.json; `null` when not switched — not `omitempty`, because Save keeps absent keys). Switching profile → profile keeps that first snapshot, so restore always returns to the default from before any switch. Restore also removes slot rules the switch created from nothing, and discards edits made to the default since the switch.
- **Status.** `GET …/claude-code/default-switch` returns `switched`, the profile, `profile_exists` (false once it was deleted; the default keeps its copy and stays restorable) and `in_sync` (the default's routing still equals the profile's). The UI shows "Same routing as profile X" / "Started from profile X, edited since".
- **Refused.** An unknown profile, or one with no routing configured yet (it would leave the default unrouted). Nothing is changed on refusal.

## Not done on purpose

The applied `~/.claude/settings.json` is not rewritten: model env vars follow the *split slots*, so if the profile splits different slots than the default did, the existing client-config status shows the file as outdated and "Apply" refreshes it, as after any slot change.

Code: `internal/config/cc_default_switch.go`, `internal/server/module/scenario/handler_default_switch.go`, `frontend/src/pages/scenario/components/ClaudeCodeDefaultProfileSwitch.tsx`.
