# Onboarding UX — guidance for first-time users

Three friction points new users hit, and the smallest changes that remove them.
All three follow the UX-First principles (see `ux-principles.md`): embed education
in the product (#8), hand over the next-step artifact (#11), and prefer smart
defaults over toggles (#6).

## 1. Routing graph — "what does each button / card do?"

The graph already ships interactive education (`EntryGuideDialog` Direct/Smart
walkthrough, `TierGuideDialog`), but it was only reachable by *hovering* a node —
undiscoverable for a first-time user who doesn't know to hover.

**Fix (discoverability):** one persistent `?` affordance ("How routing works")
at the **page level** — in the rule-list toolbar (`TemplatePageActions`, after
`New Rule`), not per rule card. `TemplatePage` owns the `EntryGuideDialog` and
opens it in Direct mode. A single, page-scoped entry avoids repeating the icon
on every card (principle #9, reduce noise) and doubles as a standing hint that
the guide can be reopened there.

- The graph's `EntryNode` keeps its contextual "View direct/smart guide →"
  links (mode-specific, inline in the selector) — those are separate from the
  toolbar entry.

**Fix (first-run):** the Direct guide auto-opens once per user (new *and*
existing) the first time they land on a populated routing page, then records
the dismissal in `localStorage` (`tb.routingGuideAutoShown`) and never
auto-opens again. The toolbar `?` reopens it on demand (principle #10:
education stays available, never blocks).

**Fix (recognisability):** the Connect-AI / Add-model steps reference toolbar
buttons that aren't in the graph, so the guide renders a faithful mock toolbar
(`GuideToolbarPreview`) above the diagram with the relevant button pulsing and
a "Click here" tag — users can see exactly what to click.

**Fix (content gap):** the Direct walkthrough used to start mid-stream at
"single provider" and never explained how you *get* there. It now starts from
zero and covers the actions, not just the routing theory:

1. **Connect an AI provider** — you need a credential first (Connect AI toolbar).
2. **Add your first model** — `＋ Add model` in an empty rule; `New Rule` for more.
3. **Change or remove a model** — click a service card to edit/swap, hover→trash.
4. **Load balancing within a tier** — same-tier services share traffic.
5. **Tier-based fallback chain** — T0 primary, cascade to T1/T2 on failure.

Smart mode keeps its 3 conceptual steps. The dialog's step model was refactored
from index-magic (`< 3` / `>= 3`) to a `mode` field on each `GuideStep` plus
plain filtered-local indexing — adding/reordering steps no longer needs offset
arithmetic. Step i18n keys moved from numeric (`steps.1`…`6`) to semantic
(`steps.connectAI`, `addModel`, `editModel`, `loadBalance`, `tierFallback`,
`smartIntro`, `smartConditions`, `smartAdvanced`) in en + zh.

Files: `ModelRequestHeader.tsx`, `UnifiedRoutingGraph.tsx`,
`tier/EntryGuideDialog.tsx`, `tier/diagrams.ts`, i18n en/zh.

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
