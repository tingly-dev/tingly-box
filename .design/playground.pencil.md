# Playground Wireframes (pencil)

Wireframes for [`playground.md`](./playground.md).

Legend: `▤` = toggle group · `( )` = disabled w/ tooltip · `▸` = collapsed ·
`▾` = expanded · `⧉` = copy · `●` = overridden flag · `○` = inherited flag ·
`$VAR` = placeholder secret.

## 1. Page layout — three questions, three columns

```
┌ rail ┬──────────────────────────────────────────────────────────────────────────────┐
│      │ Playground                                   [history: ✅851ms ❌ ✅790ms]  [▶ Run] │
│  …   ├──────────────┬───────────────────────────────┬───────────────────────────────┤
│ ▷ 🧪 │ ┌ COMPOSE ──┐│ ┌ CONVERSATION ─────────────┐ │ ┌ PAYLOAD ── ▤ Request│cURL ┐ │
│      │ │ Target     ││ │ System                    │ │ │ POST http://localhost:9999│ │
│  …   │ │ [CC rule ▾]││ │ [You are a test agent…  ] │ │ │   /tingly/claude_code     │ │
│      │ │            ││ │ ┌ user ▾ ──────────── ✕ ┐ │ │ │   /v1/messages            │ │
│      │ │ Shape      ││ │ │ What's in ./src?      │ │ │ │ ── headers ──             │ │
│      │ │ ▤ NS │ Str ││ │ └───────────────────────┘ │ │ │ x-api-key: $TB_API_KEY    │ │
│      │ │ Scope      ││ │ ┌ assistant ▾ ──────── ✕ ┐ │ │ │ anthropic-version: …      │ │
│      │ │ ▤ TB │ Dir ││ │ │ Let me check…         │ │ │ │ ── body ──                │ │
│      │ │ Tool       ││ │ └───────────────────────┘ │ │ │ {                         │ │
│      │ │ ▤ Off │ On ││ │ ┌ system ▾ ──────────  ✕ ┐ │ │ │  "model": "…",            │ │
│      │ │ Vision     ││ │ │ <mid-convo directive> │ │ │ │  "max_completion_tokens":…│ │
│      │ │ ▤ N │ U │ T ││ │ └───────────────────────┘ │ │ │  "messages": [ … ],       │ │
│      │ │ Thinking   ││ │ [+ turn]     [templates ▾]│ │ │  "stream": true           │ │
│      │ │ ──●────    ││ ├ RESULT ───────────────────┤ │ │ }                      ⧉  │ │
│      │ │ Protocol   ││ │ ✅ Success · 850ms · 43tok │ │ │ (rebuilds, 500ms debounce)│ │
│      │ │ ▤ OC│OR│ A ││ │ ▾ Journey (default OPEN)  │ │ └───────────────────────────┘ │
│      │ │────────────││ │   Rule    cc-rule · c_c   │ │                               │
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
    /playground?target=rule:{uuid}
    /playground?target=provider:{uuid}:{model}
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

## 4. Conversation templates (embedded education)

```
  [templates ▾]
    Multi-turn            — user→assistant→user; conversion fidelity baseline
    Mid-convo system      — the claude_code_compat shape (system inside messages)
    Tool round-trip       — pair with the Tool axis; assistant call → tool result

  Insert = replace current turns (confirm if edited). Each entry's caption
  says what it exercises — the harness fixture knowledge, surfaced.
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
