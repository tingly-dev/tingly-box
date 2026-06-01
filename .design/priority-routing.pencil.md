# Tier Routing — Pencil Graph

Visual companion to `priority-routing.md`. Shows the runtime flow of tier-based service routing and its two complementary failover levels: **cross-request** (tier tactic + circuit breaker) and **in-request** (the passive `firstChunkGate` + stream priming). The in-request path is a **layered hand-off**: the producer emits chunks normally, a passive gate buffers them, and the orchestrator owns the retry decision.

Contents:

- Two complementary failover levels (map)
- Cross-request — TierTactic bucket walk
- Cross-request — circuit breaker state machine
- Cross-request — recorder → breaker feedback loop
- In-request — three layers
- In-request — dispatchWithPriorityFailover flow
- In-request — stream priming (`firstEventReplayStream`)
- In-request — per-attempt timelines (two scenarios)
- firstChunkGate state machine
- Commit-seam coverage map
- selectFallbackService filter pipeline
- Key invariants

## Two complementary failover levels

```
                     one logical "use A, else B" rule
                                  │
        ┌─────────────────────────┴──────────────────────────┐
        ▼                                                     ▼
CROSS-REQUEST  (between requests)                IN-REQUEST  (within one request)
TierTactic + circuit breaker                    firstChunkGate + stream priming
        │                                                     │
  request N hits broken A (T0)                   request N's attempt on A
  → recorder trips A's breaker                   fails pre-stream (429/5xx)
        │                                                     │
  request N+1 selection skips A,                 → gate discards buffer,
  picks B (next tier T1)                           orchestrator retries B
        │                                              in the SAME request
  A's breaker half-opens after 30s                         │
  → request N+k probes A, snaps back             client sees B's success;
                                                  no error, no client retry
        │                                                     │
        └── feedback: recorder.RecordResponse/RecordError ────┘
                 writes the breaker the next request reads
```

## Cross-request — TierTactic bucket walk

`TierTactic.SelectService` (`internal/typ/tactics.go`) runs once per request inside `LoadBalancer.SelectService`. It groups the rule's active services into tier buckets and walks them lowest-number-first (T0 = highest priority), taking the first tier with a breaker-permitted service.

```
active = rule.GetActiveServices()
       │
       ▼
groupServicesByTier(active)         ← ascending; tier 0 is highest priority, tried first
       │
   buckets: [ {tier 0: A,A'}, {tier 1: B}, {tier 2: C} ]
       │
       ├─ T0 (tier=0) ─ store.Allow(A)? store.Allow(A')?
       │       │
       │       ├─ ≥1 allowed → pickWithinTier(allowed) ──► RETURN
       │       │     (1 svc → that svc; N svc → WithinTierTactic, default random)
       │       │
       │       └─ none allowed (all breakers Open) → next tier
       │
       ├─ T1 (tier=1) ─ store.Allow(B)?
       │       └─ allowed → pickWithinTier ──► RETURN
       │
       ├─ T2 (tier=2) ─ store.Allow(C)?
       │       └─ allowed → pickWithinTier ──► RETURN
       │
       └─ EVERY tier tripped
              └─ pickWithinTier(fallback = first/highest-priority bucket, T0) ──► RETURN
                   (degrade, don't disappear: surface the real upstream 5xx
                    instead of rejecting locally)
```

## Cross-request — circuit breaker state machine

Per-service three-state breaker (`internal/loadbalance/breaker.go`), keyed by `serviceID = provider.UUID + ":" + model`. No scheduler — the Open→HalfOpen flip is lazy, evaluated on the next `Allow()`.

```
                         RecordSuccess()
            ┌──────────────────────────────────────────┐
            │                                           │
            ▼                                           │
   ┌─────────────────┐  consecFails ≥ FailureThreshold  │
   │     CLOSED      │  (default 3)                     │
   │ Allow() = true  │ ───────────────────────────────►│
   │ count failures  │                                  ▼
   └─────────────────┘                        ┌──────────────────┐
            ▲                                  │      OPEN        │
            │ RecordSuccess()                  │ Allow() = false  │
            │ (probe ok)                       │ openedAt = now   │
            │                                  └────────┬─────────┘
   ┌─────────────────┐                                  │
   │    HALF-OPEN    │  time.Since(openedAt) ≥           │
   │ Allow():        │  OpenDuration (default 30s)      │
   │  1st caller=true│ ◄────────────────────────────────┘
   │  others = false │      (lazy flip on next Allow())
   └────────┬────────┘
            │ RecordFailure()  → back to OPEN, openedAt reset, probe slot freed
            └────────────────────────────────────────────────────────┘

State() applies the same lazy Open→HalfOpen flip for read-consistent
introspection (BreakerStore.Snapshot() for future UI surfacing).
```

## Cross-request — recorder → breaker feedback loop

The dispatch hot path needs **zero** changes: `ProtocolRecorder` already observes every outcome, and two lines bridge it to the breaker store the selection logic reads.

```
   request N                                   request N+1 selection
       │                                              ▲
       ▼                                              │
  dispatch attempt on serviceID                       │
       │                                              │
       ├─ success → recorder.RecordResponse           │
       │              └─ RecordServiceSuccess(id) ──┐  │
       │                                            ▼  │
       └─ failure → recorder.RecordError            DefaultBreakerStore
                      └─ RecordServiceFailure(id) ─►  (same keys: UUID:model)
                                                     │
                                                     └─ store.Allow(id) consulted
                                                        by TierTactic above
   In-request failover re-binds the recorder per attempt
   (rec.SetActiveService) so a 2nd-attempt failure trips the
   SECOND service's breaker, not the first's.
```

## In-request — three layers

```
┌── Producer (protocol handler) ─────────────────────────────────────┐
│   emits SSE chunks / writes responses normally, unaware of failover │
│   on its FIRST real chunk → CommitFirstChunk(c)  (one signal)       │
└─────────────────────────────────────────────────────────────────────┘
                 │ writes through c.Writer
                 ▼
┌── firstChunkGate (passive byte buffer) ────────────────────────────┐
│   protocol-agnostic; NO decisions in Write/WriteHeader             │
│   records bytes until CommitFirstChunk / CommitIfBuffered          │
└─────────────────────────────────────────────────────────────────────┘
                 │ committed / status read back
                 ▼
┌── Orchestrator (dispatchWithPriorityFailover) ─────────────────────┐
│   owns retry: committed→done, retryable status→Discard+next tier   │
│   installs the gate ONLY when len(activeServices) > 1              │
└─────────────────────────────────────────────────────────────────────┘
```

## In-request — dispatchWithPriorityFailover flow

```
Request arrives at handler (e.g. AnthropicMessagesV1Beta)
│
├─ transform request body (once, before loop)
│
└─ dispatchWithPriorityFailover(rule, initialProvider, attempt)
       │
       ├─ len(activeServices) ≤ 1 ? ──YES──► attempt(...) directly, return
       │       (common case never touches the buffer — zero blast radius)
       │
       ├─ wrap c.Writer in firstChunkGate  ◄──────────────────────────────┐
       │                                                                   │
       ├─ for i = 0..len(activeServices)-1:                               │
       │       │                                                           │
       │       ├─ mark tried[serviceID]                                   │
       │       ├─ rec.SetActiveService(provider, model)  ← rebind         │
       │       │                                                           │
       │       └─ attempt(provider, model)  ← runs dispatchChainResult    │
       │              │                                                    │
       │              ├── streaming path ───────────────────────────────► │
       │              │      ForwardOpenAIResponsesStream()                │
       │              │         │                                          │
       │              │         ├─ err ≠ nil → handlePreStreamFailure     │
       │              │         │     (status 500 buffered in gate)        │
       │              │         │                                          │
       │              │         └─ err = nil → PrimeResponsesStream()     │
       │              │               │                                    │
       │              │               ├─ stream.Next() fails              │
       │              │               │   → handlePreStreamFailure (500)  │
       │              │               │                                    │
       │              │               ├─ stream.Next() false + no err     │
       │              │               │   → return bare SDK stream        │
       │              │               │                                    │
       │              │               └─ stream.Next() true               │
       │              │                   → return firstEventReplayStream  │
       │              │                                                    │
       │              │      Producer reaches first real chunk            │
       │              │         └─ CommitFirstChunk(c)                     │
       │              │               gate.committed = true               │
       │              │               flush hdr+status+buf → real wire    │
       │              │               subsequent writes → pass-through     │
       │              │                                                    │
       │              │      Commit signal seams (one per producer kind): │
       │              │        • ProcessStream  (hc-based handlers)        │
       │              │        • StreamLoop      (raw-c handlers)          │
       │              │        • responses→anthropic message_start sender  │
       │              │        • google→anthropic message_start sender     │
       │              │        • assembly path → NEVER commits (terminal   │
       │              │              c.JSON, behaves non-streaming)        │
       │              │                                                    │
       │              └── non-streaming path                              │
       │                     ForwardRequest() → resp                      │
       │                     status buffered in gate                      │
       │                                                                   │
       ├─ gate.Committed()? ──YES──► return (first chunk on wire, done)   │
       │                                                                   │
       ├─ isRetryableStatus(gate.Status())? ──NO──► return (terminal err) │
       │                                                                   │
       ├─ selectFallbackService(tried, sameAPIStyle)                       │
       │       │                                                           │
       │       ├─ no candidates → Debugf, return                          │
       │       ├─ LB error    → Warnf, return                             │
       │       └─ found next  → Infof, gate.Discard() ───────────────────►│
       │                          (reset buf, status, headers)             │
       │                          provider = nextProvider                   │
       │                          model = nextService.Model                 │
       │                                                                    │
       └─ deferred on return: c.Writer = realWriter; gate.CommitIfBuffered()
              │
              ├─ committed? → no-op (already on wire)
              ├─ buf empty + status 0? → no-op
              └─ else → flush last buffered error to real writer
```

## In-request — stream priming (`firstEventReplayStream`)

SDK streams are lazy: `ResponsesNewStreaming(...)` returns a `*Stream` without issuing HTTP, so a 503 only surfaces inside the first `Next()`. `PrimeResponsesStream` (`internal/protocol/stream/prime.go`) forces that first `Next()` out-of-band so failover can act before any byte is gated, then replays the read event so the handler's per-event loop is unchanged. (Distinct from the circuit-breaker probe and `client.ProbeResponsesStream`'s synthetic health check — priming sends nothing extra.)

```
PrimeResponsesStream(sdkStream)
       │
       ├─ inner.Next() == false
       │     ├─ inner.Err() ≠ nil → return (nil, err)   → handlePreStreamFailure (500, retryable)
       │     └─ inner.Err() == nil → return (bareStream, nil)   (empty stream, nothing to replay)
       │
       └─ inner.Next() == true   (first event already pulled & cached)
             └─ return (&firstEventReplayStream{inner, first: cached}, nil)

firstEventReplayStream replays the cached first event:
       Next() call #1  → nextCount=1, return true   (inner NOT advanced again)
       Current() @ #1  → returns the cached first event
       Next() call #≥2 → delegates inner.Next()
       Current() @ #≥2 → delegates inner.Current()
       Err()/Close()   → always delegate to inner
```

## In-request — per-attempt timelines

Two decisive scenarios, showing Producer ↔ firstChunkGate ↔ Orchestrator over time.

```
SCENARIO A — pre-stream failure on T0, success on T1
────────────────────────────────────────────────────────────
Orchestrator  Producer (attempt)        firstChunkGate            real wire
   │ wrap ───────────────────────────────► (buffered)
   │ attempt(T0) ─► forward + prime T0
   │                 prime fails ─► handlePreStreamFailure
   │                   WriteHeader(500)+body ─► buf=500/body          (nothing)
   │ ◄─ return
   │ Committed()? no
   │ Status()=500 retryable? yes
   │ selectFallbackService → T1
   │ gate.Discard() ─────────────────────► buf reset, status 0
   │ attempt(T1) ─► forward + prime T1
   │                 prime ok ─► first chunk
   │                   CommitFirstChunk ──► commit: hdr+200+buf ───► FLUSHED
   │                   write events ──────► pass-through ──────────► streamed
   │ ◄─ return
   │ Committed()? YES → return
   └ defer CommitIfBuffered() → no-op (already committed)

SCENARIO B — first chunk commits, later mid-stream error (no retry)
────────────────────────────────────────────────────────────
   │ attempt(T0) ─► prime ok ─► first chunk
   │                 CommitFirstChunk ────► commit ────────────────► FLUSHED
   │                 stream events… ───────► pass-through ─────────► streamed
   │                 upstream dies mid-stream
   │                 SSE error event ──────► pass-through ─────────► streamed (honest error)
   │ ◄─ return
   │ Committed()? YES → return       (retry impossible: bytes already on wire)
   └ defer CommitIfBuffered() → no-op
```

## firstChunkGate State Machine

```
                     ┌────────────────────────────────────────────────────┐
                     │                  BUFFERED                          │
                     │  status=0, buf=empty, hdr={}                       │
                     │                                                     │
  WriteHeader(code)  │  WriteHeader → status=code                         │
  Write(bytes)       │  Write       → buf += bytes (status defaults 200)  │
        ──────────── │  NO status inspection, NO commit decision         │
                     │  Flush() = no-op   Discard() = reset for retry     │
                     └───────────┬────────────────────────────────────────┘
                                 │ CommitFirstChunk()  (producer signal)
                                 │   — or —
                                 │ CommitIfBuffered()  (orchestrator defer)
                                 ▼
                     ┌────────────────────────────────────────────────────┐
                     │                 COMMITTED (pass-through)           │
                     │  committed=true                                    │
                     │  copy hdr→real, WriteHeader(status|200), flush buf │
                     │  all future reads/writes go direct to real writer  │
                     │  Flush() delegates, Discard()/CommitIfBuffered noop │
                     └────────────────────────────────────────────────────┘
```

## Commit-seam coverage map

Every streaming producer must raise `CommitFirstChunk` on its first real chunk. Three seams cover the families; two hand-rolled handlers call it directly; the assembly path deliberately never commits.

```
seam: ProcessStream (internal/protocol/context.go)      ← hc-based handlers
   ├─ HandleOpenAIChatStream
   ├─ Anthropic v1 / beta passthroughs
   ├─ HandleResponsesToOpenAIChatStream
   └─ generic-MCP stream dispatchers

seam: StreamLoop (internal/protocol/stream/loop.go)     ← raw-c handlers
   ├─ handleOpenAIToAnthropicStreamResponse (+ WithMCPHooks)
   ├─ AnthropicToOpenAIStream
   ├─ HandleOpenAIChatToResponsesStream
   ├─ HandleOpenAIResponsesStream
   └─ HandleAnthropicBetaToOpenAIResponsesStream

explicit CommitFirstChunk(c) in message_start senders   ← hand-rolled for…range
   ├─ HandleResponsesToAnthropicV1Stream      (openai_to_anthropic.go)
   ├─ HandleResponsesToAnthropicBetaStream    (openai_to_anthropic_beta.go)
   ├─ HandleGoogleToAnthropicStreamResponse   (google_to_any.go)
   └─ HandleGoogleToAnthropicBetaStreamResponse

NEVER commits (terminal c.JSON, behaves non-streaming)
   ├─ HandleResponsesToAnthropicV1Assembly
   └─ HandleResponsesToAnthropicBetaAssembly
        ↑ shares handlerResponsesToAnthropicStream with the *Stream variants,
          so the commit lives in the per-variant message_start sender, NOT
          in the shared core — otherwise assembly would flush an SSE 200
          before its single terminal JSON.
```

## selectFallbackService Filter Pipeline

```
rule.GetActiveServices()
       │
       ├─ exclude tried[svc.ServiceID()]
       ├─ skip if provider lookup fails
       ├─ skip if provider.APIStyle ≠ requireAPIStyle   ← same style only
       │
       └─ available[] → build tempRule (no affinity carryover)
              │
              └─ s.loadBalancer.SelectService(tempRule)
                     │
                     ├─ err → (nil,nil,err)  Warnf at call site
                     ├─ nil → (nil,nil,nil)  Debugf at call site
                     └─ svc → (provider, svc, nil)  Infof at call site
```

## Key Invariants

- The gate is **passive**: it makes no protocol or status decisions. The producer signals the first real chunk; the orchestrator decides retry. Each layer owns one concern, so a gate bug cannot pick the wrong tier and an orchestrator bug cannot corrupt bytes.
- Single-service requests (`len(activeServices) ≤ 1`) bypass the gate entirely — the common case stays on the original `c.Writer`.
- Transform happens once before the loop — only the provider/model pointer changes on retry; the serialized request body is reused as-is.
- `APIStyle` is pinned to `initialProvider.APIStyle` — cross-style fallback would require re-transformation and is explicitly out of scope.
- Once `committed`, the connection is on the wire; `Discard()` and `CommitIfBuffered()` both become no-ops.
- `Status() == 0` (untouched writer) ⇒ non-retryable — matches a client disconnect / no-write completion.
- Budget cap = `len(activeServices)` — worst case visits each service once.
- The assembly path (`HandleResponsesToAnthropic{V1,Beta}Assembly`) never commits mid-stream: it buffers a struct and emits one terminal `c.JSON`, so it flows through the gate like a non-streaming response.
