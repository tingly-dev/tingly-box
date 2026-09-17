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

## 4. Request editor: a preset request, or a custom one (not tabs)

A preset request and a custom request are different jobs, not two states of
the same control — the confusion in earlier drafts came from half-coupling
them (raw disabling *some* axes, Protocol pretending to stay live). A preset
request isn't "Bench borrowing Probe's UI" — it's literally the probe (the
scenario granularity) projected down onto a request: a probe is by nature a
preset, so materializing one *is* a preset request (bench.md §1 "三种粒度").
The fix isn't a smarter coupling, it's no coupling: one default view (the
preset request), one one-way action into a self-contained second view (the
custom request), never a peer switch.

```
  default view: a preset request (= the Probe dialog, unlabeled — it's just the page)
  ┌ REQUEST ───────────────────────────────┐
  │ Message                                 │
  │ [Hello, this is a test message. …]      │
  │                                          │
  │ PARAMETERS ───────────────────────────  │  ← AxisGroup: a real,
  │ Thinking   ──●──────────────            │     independently-valued field
  │ Protocol   ▤ OpenAI Chat│Responses│Ant  │     (bench.md §1 "四种归类")
  │ CONTENT ──────────────────────────────  │  ← AxisGroup: a fixed blob,
  │ Tool       ▤ Off │ On                   │     toggled on/off — same kind
  │ Vision     ▤ Off │ User │ Tool          │     of thing as a Template,
  │                                          │     just fragment-sized
  │ Need something the knobs can't express?  │
  │ [ Write the request yourself → ]         │
  └──────────────────────────────────────────┘
        │  click "Write the request yourself"     (one-way)
        │  (PAYLOAD's [Edit] is the other door — same
        │   destination, pre-seeded with the current body)
        ▼
  ┌ REQUEST ──────────────────────────────────────┐
  │ Protocol: Anthropic Messages ▾  ← chosen here; picking a
  │                                    different one REPLACES the
  │                                    body with ITS starting
  │                                    template (same move as a
  │                                    template pick — never just
  │                                    a relabel)
  │ ┌ JSON ───────────────────────────────────┐    │
  │ │ {                                       │    │
  │ │   "messages": [ … ]                     │    │
  │ │ }                                       │    │
  │ └───────────────────────────────────────────┘    │
  │              [← back to the preset request] [Change starting point ▾]│
  └─────────────────────────────────────────────────┘
    [Change starting point ▾] opens the exact same StartingPointMenu the
    door did — one mechanism, two trigger points, not three separate
    controls (Edit the builder's request / Templates used to be split):
        ○ Copy the preset request   (only when seeded — see below)
        ○ Blank
        ○ Multi-turn      ○ Tool round-trip
        ○ Image           ○ Mid-convo system (Anthropic only)
    "Copy the preset request" must stay correct even after crossing the
    door — Payload's own curl reflects whatever's CURRENTLY active (the
    custom body, once one exists), so it can't source this. BenchPage runs
    a second, independent curl fetch with raw forced off, only while a
    custom request is active (bench.md §6.3).

    No Parameters / Content AxisGroup in the Compose column while this view
    is active — not disabled, not rendered.
    They aren't this view's knobs.
```

State diagram — why this is a door, not a tab bar:

```
        ┌──────────────┐   click "Write the      ┌──────────────┐
        │ preset request│   request yourself"      │ custom request│
        │ (= the Probe  ├─────────────────────────▶│ editor        │
        │  dialog)      │        (one-way)          │               │
        └──────────────┘                            └──────────────┘
              ▲                                             │
              │        pick a new target / start over        │
              └────────────────────────────────────────────┘

  ✗ Rejected: two named tabs you flip between, each keeping its own
    state. Nobody actually toggles back and forth mid-session — once
    you've written custom JSON, the axes have nothing to resync to.
    Tabs would dress up the same binary choice with a false symmetry;
    a one-way door names what people actually do.
```

The Probe dialog never gets a door of its own — it stays exactly the
left-hand box, forever, with one new exit:

```
   ┌ Probe dialog ────────┐
   │ axes + Message        │
   │ (in-place, 2 clicks    │
   │  to a conclusion)      │
   │                        │
   │ [Open in Bench →]      │
   └────────────────────────┘
              │  carries target + axes + message
              ▼
       Bench's preset request (above) — never straight into the
       custom editor. Complex operations only exist on Bench;
       the dialog gets a signpost to them, not the operations
       themselves.
```

Raw body goes out as E2ERequest.request + request_protocol; the probe only
fills `model` (and max_tokens / stream_options defaults). Protocol choices
are constrained by the target: a rule's scenario family, or what the
provider speaks.

[templates ▾]  (one set per protocol; doubles as "starting point" list)
  Multi-turn            — user→assistant→user; conversion fidelity baseline
  Tool round-trip       — tools + assistant tool_use/tool_calls → tool result
  Image                 — user message with an image block
  Mid-convo system      — Anthropic only: the claude_code_compat shape

Insert = replace the body, same as a protocol switch. Each entry's caption
says what it exercises — the harness fixture knowledge, surfaced.

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
