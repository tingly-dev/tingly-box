# Bench Wireframes (pencil)

Wireframes for [`bench.md`](./bench.md). Wireframes drift from the real
layout faster than the design doc does — where the two disagree, `bench.md`
(and the actual code) is authoritative; §2 and §1's rule-based target picker
below are superseded by `bench.md` §3 (provider+model only, `ModelSelectDialog`
card picker), kept here only as a record of a discarded alternative.

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

## 4. Compose: Protocol → Content → Scope → Parameters → Presets

Content is one menu, not a mode gate: it lists Message (the axis-driven
builder) alongside this protocol's whole-body templates, so there's no
"pick a mode, then discover the starting-point menu inside it" two-step
(bench.md §6.3, §14).

```
  Compose (Content: Message)               Compose (Content: Multi-turn)
  ┌ COMPOSE ──────────────────┐            ┌ COMPOSE ──────────────────┐
  │ Target    [provider·model]│            │ Target    [provider·model]│
  │                             │            │                             │
  │ Protocol  ▤ O.Chat│O.Resp│A│            │ Protocol  ▤ O.Chat│O.Resp│A│
  │ Content   [ Message      ▾]│            │ Content   [ Multi-turn    ▾]│
  │ Scope     ▤ Through TB│Dir │            │ Scope     ▤ Through TB│Dir │
  │ PARAMETERS ───────────────  │            │ PARAMETERS ───────────────  │
  │  Request  ▤ Nonstream│Stream│           │  Request  ▤ Nonstream│Stream│
  │  Thinking ──●──────────    │            │  (Thinking ── ●, greyed —   │
  │ PRESETS ──────────────────  │            │   reference only, hover for │
  │  Tool     ▤ Off│On         │            │   "not applied" hint)       │
  │  Vision   ▤ Off│User│Tool  │            │  (Tool/Vision same — greyed,│
  │                             │            │   not unmounted)            │
  └─────────────────────────────┘            └─────────────────────────────┘

  ┌ REQUEST (Message) ────────┐             ┌ REQUEST (Multi-turn) ─────────┐
  │ Message                   │             │       [Copy the preset req.]  │
  │ [Hello, this is a test…]  │             │ ┌ JSON ─────────────────────┐ │
  └────────────────────────────┘             │ │ { "messages": [ … ] }    │ │
                                              │ └────────────────────────────┘ │
                                              └────────────────────────────────┘
```

Switching **Protocol** or **Content** never leaves a mismatched body
behind:

- Content → Message: `raw` clears; the unified Protocol control carries
  the custom body's protocol back onto `axes.protocol` so it doesn't
  silently change value just because Content flipped.
- Content → a template: body is replaced wholesale with that template's
  body in the current protocol.
- Protocol changed while Content is already non-Message: the same *kind*
  of content carries forward if the new protocol has it (e.g. Multi-turn
  exists for all three); otherwise it falls back to Blank. Protocol
  decides the body's grammar, Content decides its meaning.

The Content menu (`ContentMenu` in `BenchAxes.tsx`) lists:

```
  [ Message                                    ▾]
    ○ Message           (axis-driven; Thinking/Tool/Vision below apply)
    ○ Blank
    ○ Multi-turn         ○ Tool round-trip
    ○ Image              ○ Mid-conversation system (Anthropic only)
```

"Copy the preset request" (in the Request panel, not the Content menu —
it's a dynamic snapshot of the axes' current output, not a static
template) must stay correct even deep in a non-Message session — Payload's
own curl reflects whatever's CURRENTLY active (the custom body, once one
exists), so it can't source this. BenchPage runs a second, independent
curl fetch with raw forced off, only while a non-Message content is
active (bench.md §6.3).

The Probe dialog gets the lightest possible touch: it keeps its own
frequency-ordered layout (Shape/Scope resident, everything else in
Advanced) — only Protocol moves above the fold, because it's the
coordinate system, not because it's frequently touched. No Content menu,
no custom request editor, no Presets/Parameters relabeling. Its
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
