# Probe Panel Redesign (pencil)

Scope: panel interaction and state resolution. Production diagnostic contracts are in [probe.md](./probe.md); Bench wireframes remain separate in [bench.pencil.md](./bench.pencil.md).

Wireframes for `.sdlc/docs/probe-panel-redesign-20260820.spec.md`.

Legend: `▤` = toggle group · `( )` = disabled w/ tooltip · `▸` = collapsed section ·
`$VAR` = placeholder secret · `→` = derived/resolved value.

## 1. Axis model — before vs after

```
  BEFORE (one mixed enum + hidden knobs)              AFTER (orthogonal axes)

  test_mode ──┬─ simple     (nonstream)               Shape     ▤ Nonstream | Stream
              ├─ streaming  (stream)                  Tool      ▤ Off | On
              └─ tool       (stream + tools)    ⇒     Thinking  ▤ none | low | medium | high
  endpoint: exists in API, NO UI                      Protocol  ▤ OA-Chat | OA-Resp | Anthropic
  direct:  ▤ Through TB | Direct                      Scope     ▤ Through TB | Direct
                (openai-only, 2 values)               Message   [ editable, default per Shape/Tool ]

One knob per axis; every future knob is one more row, not a new mode.
Protocol has NO "Auto" — always a concrete wire protocol, reduced per target (see §2).
Labels are brand-first (matches ApiStyleBadge vocabulary): full labels in the UI are
"OpenAI Chat" / "OpenAI Responses" / "Anthropic" — bare "Responses"/"Messages" would
assume SDK knowledge; "Anthropic" needs no suffix (single protocol in that family).
```

## 2. Dialog layout (provider target — richest case)

Left control rail (instrument panel) + right results column (the visual
anchor), with cURL spanning the full width at the bottom (its lines are long
and it belongs to the whole panel). The dialog itself is resizable
(`resize: both`, min bounds). Primary axes stay visible; everything else
folds behind Advanced.

```
┌─ Probe · Kimi · kimi-k2-0905-preview ──────────────── [⧉ cURL] [⟳] [▶ Run] ─┐
│ ┌─ Request Config ──┐  ┌─ Results ────────────────────────────────────────┐ │
│ │ Shape             │  │ ┌─ Not run yet (dashed, fills column) ──────┐   │ │
│ │  ▤ Nonstream│Stream│ │ │ Set the config on the left, then Run Test │   │ │
│ │ Scope             │  │ └───────────────────────────────────────────┘   │ │
│ │  ▤ Through TB│Direct│ │                                                │ │
│ │ ▸ Advanced        │  │ (after run: ✅ Success · 850ms · 43 tokens      │ │
│ │   ├ Tool  ▤ Off│On │  │  ▸ Request Journey   ▸ Response                │ │
│ │   ├ Thinking ─●─  │  │  ▸ Raw JSON           [each block copyable])   │ │
│ │   │  (slider w/   │  │                                                │ │
│ │   │   marks)      │  │                                                │ │
│ │   ├ Protocol      │  │                                                │ │
│ │   │  ▤ OChat│OResp│A│ └────────────────────────────────────────────────┘ │
│ │   └ Message [ … ] │                                                    │
│ └───────────────────┘                                                    │
│ ▸ cURL ── full width ──────────────────────────────────────────────────   │
│   curl -N http://localhost:9999/tingly/openai/v1/chat/completions \       │
│     -H 'Authorization: Bearer $TB_API_KEY' \                              │
│     -d '{                                                                  │
│       "model": "kimi-k2-…", "stream": true, …   (pretty, single-quoted)    │
│     }'  ⧉ copy   caption: replace $TB_API_KEY with your gateway key        │
└─────────────────────────────────────────────────────────────────────────────┘
```

Rail styling notes (learned the hard way):
- Groups fill the rail width with **equal-width options** — layout-only
  deltas over the shared theme style; padding/colors/shape stay themed.
- Never wrap ToggleButtons in Tooltip inside a group (breaks direct-child
  CSS); hover detail goes on the axis label instead.
- Protocol buttons are abbreviated (`O Chat` / `O Resp.` / `A`) with the full
  name in the label tooltip; Thinking is a marked slider (labels visible),
  inset + clipped so end-mark labels don't overflow the rail.

Target-type degradation (Protocol reduces to the concrete protocols the target
speaks — unavailable options simply don't render; single-protocol ⇒ locked):

```
  openai-style target:    Protocol ▤ OpenAI Chat │ OpenAI Resp.    (2 options)
  codex OAuth provider:   Protocol ▤ OpenAI Resp. │ OpenAI Chat     (default Responses)
  anthropic-style target: Protocol ( Anthropic )  ← locked to the only option
  dual-base provider:     Protocol ▤ OpenAI Chat │ OA-Resp │ Anthropic (3 options)
  google-style target:    Protocol ( – )  ← disabled; own SDK, out of scope
                          Scope   ( Direct )  ← locked; no loopback route
  rule target:            Scope  ( Through TB )  ← locked; tooltip: "rule probes must
                                                          traverse TB middleware"
  Tool axis needs no lock: both Tool×Shape combos are valid (non-stream lifts
  structured tool_calls; stream keeps raw chunks) — verified in helper.go.
```

## 3. Open-time state resolution (the stream-association fix)

```
                 dialog opens
                      │
                      ▼
        ┌───────────────────────────────┐
        │ explicit props?               │── yes ──► use them (entries stop hardcoding)
        └───────────────────────────────┘
                      │ no
                      ▼
        ┌───────────────────────────────┐
        │ initialResult present?        │── yes ──► Shape := result.stream
        └───────────────────────────────┘            (show that result, state matches it)
                      │ no
                      ▼
        ┌───────────────────────────────┐
        │ last-used config for this     │── yes ──► restore (localStorage,
        │ target-type?                  │            key: tb.probe.config.{rule|provider})
        └───────────────────────────────┘
                      │ no / first ever
                      ▼
              default: Stream · Tool off · Thinking none · Protocol = target's
                       primary (OpenAI→Chat · Codex→Responses · Anthropic→Messages)
                       · Through TB

  any manual change or re-run ──► persists back to tb.probe.config.{target-type}
```

## 4. cURL generation path (backend, shares the real request builder)

```
  dialog config change ──debounce──► POST /api/v2/probe/curl   (does NOT execute)
                                          │
                                          ▼
                          ┌───────────────────────────────────────┐
                          │ ValidateE2ERequest  (same as /probe)  │
                          │ BuildProbeRequest(req)  ← SAME builder │
                          │   the prober uses (single source of    │
                          │   truth — no parallel impl)            │
                          └───────────────────────────────────────┘
                                          │
                          ┌───────────────┴────────────────┐
                          ▼                                ▼
                  Scope = Through TB                Scope = Direct
                  URL: localhost:port               URL: provider.APIBase + path
                      /tingly/{scenario}/v1/…           (real upstream)
                  Auth: $TB_API_KEY                   Auth: $UPSTREAM_API_KEY
                  (header per scenario protocol)      (header per provider protocol)
                          └───────────────┬────────────────┘
                                          ▼
                          { command, request: {method, url, headers, body} }
                                          │
                                          ▼
                          dialog cURL section renders + copy ⧉
```
