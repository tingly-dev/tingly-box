# Claude Code: per-slot rules in unified mode

> Status: implemented. Background: `claude-code-config.md` (env shape, rule ↔ slot mapping).

![Slots row](images/claude-code-slots.png)

## Problem

Claude Code has two modes: **unified** (every model slot requests the `cc` rule) and **separate** (every slot has its own rule). The common need sits in between — "unified, but the subagent on another model" — and used to mean switching to separate and keeping five rules identical by hand.

## Design

The modes stay. Unified mode gains per-slot rules:

- The **default slot** (`ANTHROPIC_MODEL`) is the unified rule `builtin:<scenario>:cc` and is always there.
- Each other slot — `haiku` `sonnet` `opus` `fable` `subagent` — uses the unified rule unless it is in the scenario's **slot list**; then it requests its own rule `builtin:<scenario>:<slot>` (while that rule is on).
- Routing is untouched: the env carries a rule's `request_model`, which stays the routing key.

UI: in unified mode a **Slots** row sits under Base URL / API Key. `default` is always lit; clicking another slot gives it its own rule (the rules card then shows it) or hands it back. Separate mode, and separate-mode profiles, look and behave as before.

## Why an explicit slot list, not `Active`

`agent apply` (and older code) switches every Claude Code rule on, so `Active` can't mean "this slot has its own rule" without silently splitting the slots of users who once ran it. The list is written only by `SetClaudeCodeSlot`.

- Stored on the main scenario's record (`ScenarioConfig.ClaudeCodeSlots`) and on each profile (`ProfileMeta.ClaudeCodeSlots`). A profile keeps its own list so it never inherits the main one, and splitting a profile slot never creates a scenario record that would fork the profile from the main scenario's settings.
- Whole-record scenario writes never change the list (a profile page's GET-merge reads the main record through the fallback); a mode change clears the main list (switching back to unified merges every slot, as before), and the unified-mode rule sync keeps listed slots' rules on.
- Turning a slot on reuses its built-in rule. A rule without routing yet (the main scenario pre-seeds empty ones) — or a missing rule — starts as a copy of the unified rule's routing, so nothing changes until the user edits it. Turning it off only switches the rule off, so turning it on again restores it.
- Rejected: separate mode (main or profile), unknown profiles, the default slot.

## Implementation

- `config.SetClaudeCodeSlot` / `ClaudeCodeSlots` (`internal/config/cc_slots.go`); mode sync in `scenario.go`.
- `agent.ClaudeCodeSlotModels`: the one resolver behind the settings file and the tbclient env; `agent apply` in unified mode also writes the split slots.
- `agent apply` with a provider: in unified mode it updates only the main rule; split slots keep their own model and the other slot rules stay off. Separate mode (or a separate-mode apply) still points every rule at the provider.
- TUI quickstart: after choosing unified mode it asks for the subagent model ("same as the default" or another one); another one splits the subagent slot before the env is written.
- CLI: `profile list/show` and the `cc` profile picker show `unified + subagent` for split slots.
- API: `GET /api/v1/scenario/:scenario/claude-code/slots`, `PUT …/claude-code/slots/:slot {enabled}`.
- Frontend: `useSlotRouting` (Slots row, unified-mode rule list, fixed mode for profiles); `derivePrefsFromRules({ rules, mode, slots })`.

Changing a slot changes the env: the main page's Client Config status chip offers Reapply; profile settings are rebuilt at launch.
