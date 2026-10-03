# Request Failover (pencil)

Mid-request failover for the priority-routing tactic, **after** it was lifted so
each attempt re-transforms the request for the candidate it is about to call.
This is what enables heterogeneous (cross-API-style) failover.

Scope: this file owns the one-time prologue / per-candidate transformation
sequence. Cross-request tier selection and breaker recovery are explained in
[`tier-routing.pencil.md`](./tier-routing.pencil.md); gate/priming mechanics
there are shared with this path. Pipeline placement is authoritative in
[`protocol-stage-pipeline.md`](./protocol-stage-pipeline.md), with a separate
[`pencil companion`](./protocol-stage-pipeline.pencil.md).

Legend: `║`/`▼` = control flow · boxes = phases · `┐┘` brackets group the
"mutate-in-place" steps.

```text
HTTP entrypoint (Anthropic V1/Beta, OpenAI Chat/Responses)
  │
  ├─ ONCE: parse → rule → vision proxy → context hint → initial selection
  │        session/tracking/recorder → pristine request snapshot
  ▼
dispatchWithPriorityFailover(initial provider, model, attempt)
  │ install firstChunkGate for eligible multi-service rules
  ▼
PER CANDIDATE (provider, model)
  ├─ clone pristine input (multi-service path)
  ├─ planAttempt: dual endpoint / target / flags / preBase / preVendor
  ├─ transformRequest: Source → Base/Bridge → Target → Vendor
  │    Stage path: Source once; Target half before each upstream round
  │    guardrails/tool-round integration follows the chosen serving path
  └─ dispatch / serve Stage → actual upstream call
  ▼
INSPECT ATTEMPT
  ├─ committed response or committed tool effect? → stop, never retry
  ├─ retryable status / setup failure?
  │    ├─ fallback candidate → discard buffered response, repeat attempt
  │    └─ exhausted → flush the last buffered upstream error
  └─ success or non-retryable client error → flush and finish
```

`CommitFirstChunk` controls response commitment; `MarkFirstToken` records TTFT
on the first content delta. They are separate signals (`ttft.md`). A structural
SSE event can commit the response before the first content token, so “no content
yet” alone is not proof a retry remains safe. The detailed gate timeline is in
`tier-routing.pencil.md`; the per-attempt transform order is in
`protocol-stage-pipeline.md`.


## Concrete cross-style run (the thing that was impossible before)

```
  tier rule:  T0 = Anthropic-style provider     T1 = OpenAI-style provider     (streaming)

  client ──POST /v1/messages──►  [ gate: buffering ]
  ┌─────────────────────────────┐        ┌─────────────────────────────┐
  │ attempt 1   (T0, claude)     │        │ attempt 2   (T1, gpt)        │
  │ clone → transform → Anthropic│        │ clone → transform → OpenAI   │
  │ POST  anthropic upstream     │        │ POST  openai upstream        │
  │      └─ 529  (buffered)      │        │      └─ 200, first chunk     │
  │ status retryable             │        │ CommitFirstChunk             │
  │ Discard buffer               │        │ gate → pass-through          │
  │ selectFallback(style="") → T1│───────►│ bytes stream to client ✓     │
  └─────────────────────────────┘        └─────────────────────────────┘
                                              ▲ after commit, retry is impossible
```

## Why 401/403 is retryable

Inbound auth is settled by middleware long before the gate exists, so a 401 that
reaches the loop always came from a provider: a token caught mid-refresh, a
rotated or revoked key, a provider-side account flag. That is a property of one
credential, not of the request — which is exactly what a sibling service is for.
The attempt is also reported to the health monitor (like 429), so the broken
credential stops winning selection on the next request instead of 401ing every
caller until someone notices.

## Why it's safe (the one invariant)

```
  per-attempt clone  +  buffered gate
        │                    │
        │                    └─ no failed attempt's bytes leak; the client sees exactly
        │                       ONE response — the first success, or the last error.
        └─ preChain/guardrails/transform mutate in place, so every retry MUST start
           from a pristine request; reusing a once-transformed body is the old bug.
```

**Old vs new, one line:** *old* = transform once → loop only re-dispatches the
same Anthropic-shaped body → fallback pinned to the same API style. *new* = the
loop wraps the transform → each attempt re-shapes the pristine request for its
own provider → fallback spans any style.

## Map to code

| Phase | Where |
| --- | --- |
| Prologue + per-attempt split | `internal/protocolserver/anthropic_message.go` (V1/Beta), `openai_chat.go`, `openai_responses.go` (entrypoint / per-attempt handlers) |
| Loop + gate + retry decision | `internal/protocolserver/failover_dispatch.go` (`dispatchWithPriorityFailover`, `firstChunkGate`, `isRetryableStatus`, `failAttemptSetup`) |
| Cross-style candidate pool | `selectFallbackService(rule, tried, "")` in `failover_dispatch.go` |
| Pristine per-attempt clone | `internal/protocolserver/protocol_clone.go` |

See also `tier-routing.pencil.md` for tier/breaker selection that feeds this.
