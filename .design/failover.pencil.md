# Failover (pencil)

Mid-request failover for the priority-routing tactic, **after** it was lifted so
each attempt re-transforms the request for the candidate it is about to call.
This is what enables heterogeneous (cross-API-style) failover.

Legend: `║`/`▼` = control flow · boxes = phases · `┐┘` brackets group the
"mutate-in-place" steps.

```
                         NEW FAILOVER — one prologue, then re-transform per attempt

  HandleAnthropicMessages(c)                                     ← real HTTP entrypoint (anthropic.go)
        │
        ▼
  ┌────────────────────────────────────────────────────────────────────────────┐
  │ ① PROLOGUE — runs ONCE, independent of which provider ends up serving        │
  ├────────────────────────────────────────────────────────────────────────────┤
  │   parse body (v1 │ beta)                                                     │
  │   determineRule                                                              │
  │   applyVisionProxy ........ mutates request BEFORE routing (data dependency) │
  │   detectContext1M ......... mutates rule                                     │
  │   SelectService ........... picks first candidate  →  p0 / model0            │
  │   sessionID→ctx · SetTracking · recorder(pristine body)                      │
  │   template = clone(req) ... pristine snapshot   (only if >1 service)         │
  └────────────────────────────────────────────────────────────────────────────┘
        │
        ▼
  ┌────────────────────────────────────────────────────────────────────────────┐
  │ ② dispatchWithPriorityFailover(p0, model0, attempt)   ← shared, transport-   │
  │    install firstChunkGate on c.Writer  (buffers bytes; multi-service only)   │   agnostic loop
  └────────────────────────────────────────────────────────────────────────────┘
        │
        ▼   for candidate (p, model), starting at (p0, model0):
  ╔══════════════════════════════════════════════════════════════════════════════╗
  ║ ③ ATTEMPT — runs PER candidate, fully provider-dependent                      ║
  ╟──────────────────────────────────────────────────────────────────────────────╢
  ║   req      = clone(template) ........ fresh, never-yet-mutated request        ║
  ║   provider = resolveDual(p)                                                   ║
  ║   maxAllowed = cap(p, model)                                                  ║
  ║   preChain(req)            ┐                                                  ║
  ║   guardrails(req)          │  mutate req in place  →  shape it into           ║
  ║   target = f(p.APIStyle)   │  THIS provider's wire format                     ║
  ║   transform(req → target)  ┘  (Anthropic / OpenAI / Google)                   ║
  ║   dispatchChainResult ────────────────────────────────► upstream HTTP call    ║
  ╚══════════════════════════════════════════════════════════════════════════════╝
        │
        ▼   inspect gate after the attempt
  ┌────────────────────────────────────────────────────────────────────────────┐
  │ committed?  (first real stream chunk already flushed) ──────────► DONE ✓     │
  │ retryable?  (429 / 5xx,  or setup-fail → failAttemptSetup=500)               │
  │      selectFallbackService(tried, style = "")   ← pool spans ALL styles      │
  │           next candidate → Discard buffer, loop ↺                            │
  │           exhausted      → flush last buffered error ──────────► DONE ✗      │
  │ else  (2xx success / 4xx client error) ── flush ──────────────► DONE         │
  └────────────────────────────────────────────────────────────────────────────┘
```

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
| Prologue + per-attempt split | `internal/server/anthropic_message_v1.go` (`AnthropicMessagesV1` / `runAnthropicV1Attempt`), `anthropic_message_beta.go`, `openai_chat.go`, `openai_responses.go` |
| Loop + gate + retry decision | `internal/server/failover_dispatch.go` (`dispatchWithPriorityFailover`, `firstChunkGate`, `isRetryableStatus`, `failAttemptSetup`) |
| Cross-style candidate pool | `selectFallbackService(rule, tried, "")` in `failover_dispatch.go` |
| Pristine per-attempt clone | `internal/server/request_clone.go` |

See also `tier-routing.pencil.md` for tier/breaker selection that feeds this.
