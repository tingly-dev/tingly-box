# Bench Wireframes (pencil)

Wireframes for [`bench.md`](./bench.md).

Legend: `▤` = toggle group · `( )` = disabled w/ tooltip · `▸` = collapsed ·
`▾` = expanded · `⧉` = copy · `●` = overridden flag · `○` = inherited flag ·
`$VAR` = placeholder secret.

## 1. Page layout — three questions, three columns

```
┌ rail ┬──────────────────────────────────────────────────────────────────────────────┐
│      │ Bench                                   [history: ✅851ms ❌ ✅790ms]  [▶ Run] │
│  …   ├──────────────┬───────────────────────────────┬───────────────────────────────┤
│ ▷ 🧪 │ ┌ COMPOSE ──┐│ ┌ REQUEST ──────────────────┐ │ ┌ PAYLOAD ── ▤ Request│cURL ┐ │
│      │ │ Target     ││ │ Protocol: Anthropic ▾     │ │ │ POST http://localhost:9999│ │
│  …   │ │ [CC rule ▾]││ │ ┌───────────────────────┐ │ │ │   /tingly/claude_code     │ │
│      │ │            ││ │ │ {                     │ │ │ │   /v1/messages            │ │
│      │ │ Shape      ││ │ │  "system": "…",       │ │ │ │ ── headers ──  [+ header] │ │
│      │ │ ▤ NS │ Str ││ │ │  "messages": [        │ │ │ │ x-api-key: $TB_API_KEY    │ │
│      │ │ Scope      ││ │ │   {"role":"user", …}, │ │ │ │ anthropic-version: …      │ │
│      │ │ ▤ TB │ Dir ││ │ │   {"role":"system",…} │ │ │ │ ── body ──         [Edit] │ │
│      │ │────────────││ │ │  ]                    │ │ │ │ {                         │ │
│      │ │ (this is a ││ │ │ }                     │ │ │ │  "model": "…",            │ │
│      │ │ custom      ││ │ └───────────────────────┘ │ │ │  "max_completion_tokens":…│ │
│      │ │ request:    ││ │ [templates ▾]             │ │ │  "messages": [ … ],       │ │
│      │ │ Tool/Vision/││ │ [← back to the preset req]│ │ │  "stream": true           │ │
│      │ │ Thinking/   ││ ├ RESULT ───────────────────┤ │ │ }                      ⧉  │ │
│      │ │ Protocol    ││ │ ✅ Success · 850ms · 43tok │ │ │ (rebuilds, 500ms debounce)│ │
│      │ │ rows don't  ││ │ ▾ Journey (default OPEN)  │ │ └───────────────────────────┘ │
│      │ │ render, §4) ││ │   Rule    cc-rule · c_c   │ │                               │
│      │ │ PLUGINS    ││ │   Flags   applied: think= │ │   narrow viewport: PAYLOAD    │
│      │ │ 2 overridden│ │           high, max_c_t   │ │   drops below RESULT as a     │
│      │ │ [Reset all]││ │   Routing load_balancer   │ │   full-width collapsible      │
│      │ │  (see §3)  ││ │   Provider kimi → k2-0905 │ │                               │
│      │ └────────────┘│ │   Endpoint OpenAI Chat    │ │                               │
│      │  ← col scrolls│ │   Upstream https://…      │ │                               │
│      │               │ │ ▸ Response   ▸ Raw JSON   │ │                               │
│      │               │ └───────────────────────────┘ │                               │
└──────┴──────────────┴───────────────────────────────┴───────────────────────────────┘

  COMPOSE = "what do I send" · PAYLOAD = "what actually goes out" ·
  RESULT = "what happened".  No Advanced fold anywhere — every knob resident.
```

## 2. Unified target picker (no mode picker)

```
  [ Search rules & providers…                    ]
  ── Rules ────────────────────────────────
   ▸ Claude Code    cc-rule          claude_code
   ▸ Claude Code:p1 cc-profile-rule  claude_code:p1
   ▸ Codex          codex-rule       codex
  ── Providers ────────────────────────────
   ▸ Kimi        ├ kimi-k2-0905-preview   ← inline second level: model
                 └ kimi-latest
   ▸ OpenRouter  ├ …

  Picking an entry IS the target; rule-vs-provider is a property of the
  choice, never a question asked first. Deep links preselect:
    /bench?target=rule:{uuid}
    /bench?target=provider:{uuid}:{model}
```

## 3. Plugins overlay — three-state, registry-driven

```
  PLUGINS                          2 overridden · [Reset all]
  ── request ──────────────────────────────────────────────
  ○ custom_user_agent      (inherit: —)              [ off ]
  ● use_max_completion_tokens                     [ ON  ] ↺   ← highlighted
  ○ block_tools            (inherit: —)              [ off ]
  ── reasoning ────────────────────────────────────────────
  ● thinking_effort                            [ high ▾ ] ↺
  ── response ─────────────────────────────────────────────
  ○ skip_usage             inherit: on (scenario)    [ on* ]  ← concrete value,
  ── app ──────────────────────────────────────────────────      muted; * = inherited
  ○ claude_code_compat     inherit: on (rule default)[ on* ]
  ○ clean_header           inherit: on (rule default)[ on* ]

  ○ inherited  = not in overlay; shows the RESOLVED concrete baseline
                 (rule.Flags + scenario merge), muted.
  ● overridden = present in request `flags`; highlighted border + per-row ↺.

  Scope = Direct  ⇒  whole section disabled:
  ( PLUGINS — flags are TB middleware; Direct bypasses TB.
    Switch Scope to "Through TB" to test flags. )

  Authority note: baseline shown is a display-only shallow merge; the
  truth is the response's Applied Flags (journey row) — e.g. Claude OAuth
  suppressing clean_header shows up THERE, not as a UI prediction.
```

## 4. Compose: Protocol → Request mode → Scope → Parameters → Presets

This revises §4 as it stood earlier — Request mode was a one-way door
(a button at the bottom of the Request panel, tabs explicitly rejected).
The user redesigned Compose from scratch rather than inheriting that
structure: Protocol and Request mode are now formal, always-visible
controls at the *top* of Compose, in that order, not something you
discover scrolling through axes. This isn't the rejected "tabs" shape
either — there's still exactly one value for each control, not two
independent panels each remembering their own state; picking Custom
still means the body from here on is fully yours, same as before.

```
  Compose (preset mode)                    Compose (custom mode)
  ┌ COMPOSE ──────────────────┐            ┌ COMPOSE ──────────────────┐
  │ Target    [rule/provider ▾]│            │ Target    [rule/provider ▾]│
  │                             │            │                             │
  │ Protocol  ▤ O.Chat│O.Resp│A│            │ Protocol  ▤ O.Chat│O.Resp│A│
  │ Request mode ▤ Preset│Custom│           │ Request mode ▤ Preset│Custom│
  │ Scope     ▤ Full│Pinned    │            │ Scope     ▤ Full│Pinned    │
  │ PARAMETERS ───────────────  │            │ PARAMETERS ───────────────  │
  │  Request  ▤ Nonstream│Stream│           │  Request  ▤ Nonstream│Stream│
  │  Thinking ──●──────────    │            │  (Thinking not shown — only │
  │ PRESETS ──────────────────  │            │   shapes the preset builder)│
  │  Tool     ▤ Off│On         │            │                             │
  │  Vision   ▤ Off│User│Tool  │            │ (Presets group not rendered │
  │                             │            │  at all — not disabled)     │
  └─────────────────────────────┘            └─────────────────────────────┘

  ┌ REQUEST (preset) ─────────┐             ┌ REQUEST (custom) ─────────────┐
  │ Message                   │             │              [Change starting  │
  │ [Hello, this is a test…]  │             │               point ▾]         │
  └────────────────────────────┘             │ ┌ JSON ─────────────────────┐ │
                                              │ │ { "messages": [ … ] }    │ │
                                              │ └────────────────────────────┘ │
                                              └────────────────────────────────┘
```

Switching **Protocol** or **Request mode** never leaves a mismatched body
behind — same rule as before, tightened:

- Request mode Preset→Custom: `raw` seeds from `seedBody` (what the preset
  request would send right now) if there is one, else a blank body in the
  current protocol.
- Request mode Custom→Preset: the custom body's protocol is carried back
  onto `axes.protocol` first, so the unified Protocol control doesn't
  silently change value just because mode flipped.
- Protocol changed while already Custom: body resets to a **blank** request
  in the new protocol (not a template) — Protocol decides the body's
  grammar, not its content; **Change starting point** picks the content.

**Change starting point** (`StartingPointMenu`, one mechanism, one trigger
now that the door is gone) — same list as before:

```
  [Change starting point ▾]
    ○ Copy the preset request   (only when seeded)
    ○ Blank
    ○ Multi-turn      ○ Tool round-trip
    ○ Image           ○ Mid-convo system (Anthropic only)
```

"Copy the preset request" must stay correct even deep in a custom session —
Payload's own curl reflects whatever's CURRENTLY active (the custom body,
once one exists), so it can't source this. BenchPage runs a second,
independent curl fetch with raw forced off, only while a custom request is
active (bench.md §6.3).

The Probe dialog gets the lightest possible touch, not this redesign: it
keeps its own frequency-ordered layout (Shape/Scope resident, everything
else in Advanced) — only Protocol moves above the fold, because it's the
coordinate system, not because it's frequently touched. No Request mode
toggle, no custom request editor, no Presets/Parameters relabeling. Its
one door to Bench is unchanged:

```
   ┌ Probe dialog ────────┐
   │ Protocol               │  ← moved up, everything else unchanged
   │ Shape / Scope          │
   │ ▸ Advanced             │
   │ [Open in Bench →]      │
   └────────────────────────┘
              │  carries target + axes + message
              ▼
       Bench's preset request — never straight into the custom editor.
       Complex operations only exist on Bench.
```

[templates ▾] carries the same content described above — one set per
protocol, each entry's caption saying what it exercises (Multi-turn,
Tool round-trip, Image, Mid-convo system). Insert = replace the body,
same as a Protocol switch.

## 5. Flags overlay data flow (through-TB only)

```
  FlagOverlayPanel ──▶ E2ERequest.flags = {"use_max_completion_tokens":true,
                                           "thinking_effort":"high"}
        │                                   (only touched keys present)
        ▼
  POST /api/v2/probe ─▶ E2EProber ─▶ probeHeaderRoundTripper
                                       X-Tingly-Probe-Flags: base64url(JSON)
        ▼
  TB loopback /tingly/{scenario}/…
        ▼
  ResolveRuleFlagsWithScenario:
     rule flags → scenario inherit → ➊ OVERLAY → auto CleanHeader → OAuth suppress
                                     └ overrides config, never physics
        ▼
  X-Tingly-Applied-Flags ─▶ Result.AppliedFlags ─▶ Journey "Flags" row
                                                    (authoritative echo)

  Same body → POST /api/v2/probe/curl renders the PAYLOAD panel: identical
  param builders, so panel and run can never disagree.
```

## 6. Run history chips (session-scoped)

```
  [ ✅ 851ms · stream · 2 flags ] [ ❌ timeout · direct ] [ ✅ 790ms · tool ]
       ▲ click = show that result AND restore the config that produced it
         (request-echo axes + local config snapshot) — compare runs by
         flipping between chips. Cleared on reload (V1).
```
