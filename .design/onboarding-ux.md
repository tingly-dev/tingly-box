# Onboarding UX — guidance for first-time users

Three friction points new users hit, and the smallest changes that remove them.
All three follow the UX-First principles (see `ux-principles.md`): embed education
in the product (#8), hand over the next-step artifact (#11), and prefer smart
defaults over toggles (#6).

## 1. Routing graph — "what does each button / card do?"

The graph already ships interactive education (`EntryGuideDialog` Direct/Smart
walkthrough, `TierGuideDialog`), but it was only reachable by *hovering* a node —
undiscoverable for a first-time user who doesn't know to hover.

**Fix:** a persistent `?` affordance ("How routing works") in the rule header
(`ModelRequestHeader`), wired through `UnifiedRoutingGraph` to open
`EntryGuideDialog` **in the mode the user is currently looking at**
(`effectiveMode`), so the walkthrough matches the graph in front of them.

- `ModelRequestHeader` gains an optional `onShowGuide` prop; the button only
  renders when a handler is supplied (hidden in `guideMode` demo graphs).
- No intrusive auto-popup — discoverable, not forced (principle #10: education
  stays available, never blocks).

Files: `ModelRequestHeader.tsx`, `UnifiedRoutingGraph.tsx`.

## 2. Connect AI picker — "custom or find one?"

The picker opened with a flat "Pick a provider" line that didn't tell users the
list is the fast path and Custom is the fallback.

**Fix:** the intro copy now steers users to **search the pre-configured list
first**, and names **Custom endpoint** explicitly as the "not listed?" escape
hatch. The Custom card's caption changed from "Any compatible API" to
"Not listed? Bring your own URL" so the card itself answers the question.

File: `ConnectProviderDialog.tsx`.

## 3. Custom endpoint form — "/v1 and OpenAI vs Anthropic?"

The `/v1` tooltip already existed. The missing half was the protocol choice:
a custom URL has no provider template to tell us which API it speaks, so users
guessed between OpenAI and Anthropic with no steer.

**Fix (smart default + recommendation, principle #6):**
- In custom add mode the form now **pre-selects OpenAI** (the overwhelmingly
  common case) instead of leaving both unchecked.
- `ProtocolSelector` gains a `recommendOpenAI` flag (set in `customMode`) that
  shows a **Recommended** chip on OpenAI and swaps the helper text to:
  - OpenAI: "Most endpoints speak the OpenAI API — start here unless you know otherwise."
  - Anthropic: "Only if your endpoint explicitly supports the Anthropic (Claude) API."

Files: `ProviderFormDialog.tsx`, `providerFormDialog/ProtocolSelector.tsx`.

i18n keys added under `providerDialog.apiStyle`: `recommendedBadge`,
`customOpenAIHint`, `customAnthropicHint` (en + zh).
