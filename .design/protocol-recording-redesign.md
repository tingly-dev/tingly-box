# Protocol Recording Redesign

> Status: proposed design for review; no production pipeline wiring is changed
> by this document.
>
> Scope: protocol request/response recording for complete, streaming,
> failover, Guardrail, and future Tool Loop/MCP Stage paths.

## Decision

Recording will be rebuilt as a request-scoped observation service with explicit
call, attempt, and provider-exchange lifecycles. It will not be implemented as
one protocol-owning Stage and it will not own HTTP writing, routing, retries,
tool execution, or provider calls.

Two thin observers will expose the real protocol boundaries:

- the HTTP adapter records the raw client request and the final serialized
  response/events actually sent to the client;
- a provider-endpoint observer records every concrete provider request and
  response/events immediately outside the terminal endpoint.

An outer request-execution scope is the only owner allowed to finish the
overall call record. The failover dispatcher owns attempts; provider errors
finish an attempt or exchange, not the call.

## Why the Existing Design Must Be Replaced

The current `ProtocolRecorder` combines unrelated responsibilities:

- it stores a `gin.Context` and reads request/response HTTP state;
- transform objects mutate it with pre/post request snapshots;
- failover rebinds one mutable recorder to different providers;
- protocol handlers, stream hooks, MCP, and error writers can all emit/finalize;
- Anthropic-specific assemblers synthesize streaming responses;
- it buffers generic stream chunks in memory;
- it selects record fields and emits directly to the sink.

This creates concrete correctness problems:

1. A retryable provider error can emit and release the call before failover
   succeeds.
2. One mutable provider binding cannot represent multiple attempts.
3. Tool-loop rounds and provider attempts are indistinguishable.
4. `ProviderResponse` exists in the record model but is not populated by the
   main recorder lifecycle.
5. Stream recording behavior depends on which protocol handler and assembler
   happened to run.
6. JSON round-trips through `map[string]any` lose type/raw-wire fidelity.
7. Full inbound headers are copied without a mandatory credential-redaction
   boundary.
8. Capturing stream chunks can consume memory even when the exported record
   only needs the assembled response.
9. Record modes mix capture depth, stream detail, and storage policy into one
   compound switch.

## Data Model

One client request produces exactly one `CallRecord`:

```text
CallRecord
├── identity: request/session/scenario/source protocol
├── client request
├── attempts[]
│   ├── route/provider/model/target protocol
│   ├── exchanges[]
│   │   ├── provider request
│   │   ├── provider response or stream summary
│   │   └── outcome/timing/error
│   └── outcome: succeeded | retryable_error | terminal_error | cancelled
├── client response or stream summary
└── outcome/timing/error
```

An **attempt** is one failover candidate. An **exchange** is one provider call
inside that attempt. A normal request has one attempt and one exchange; a Tool
Loop can have one attempt with several exchanges; failover can have several
attempts, each with one or more exchanges.

Payloads use an explicit boundary contract:

```go
type Payload struct {
    Protocol    protocol.APIType
    ContentType string
    Body        json.RawMessage
    Truncated   bool
    SHA256      string
}
```

Typed protocol/SDK values exist only while producing `Payload`. The stored
model preserves JSON bytes and protocol identity; recorder core never converts
one protocol to another.

## Ownership and Lifecycle

```text
HTTP adapter: BeginCall(raw client request)
  └── request execution scope (deferred FinishCall)
        └── failover dispatcher: BeginAttempt(candidate)
              └── provider observer: BeginExchange(provider request)
                    └── provider response/error: FinishExchange
              └── dispatcher: FinishAttempt
        └── HTTP adapter: observe final serialized response/events
  └── request execution scope: FinishCall exactly once
```

Rules:

1. `FinishCall` is idempotent and owned only by the outer request-execution
   scope, including failures that occur before provider dispatch.
2. `FinishAttempt` never emits an overall record.
3. A retryable error remains attached to the failed attempt.
4. Streaming success is determined by the same commitment gate used by
   failover, not by whether a recorder hook observed an event.
5. Cancellation is an explicit outcome, not a generic error string.
6. Recorder objects contain no Gin context and never write HTTP/SSE.
7. Sink/export failure cannot change request execution.

## Pipeline Integration

Recording is an observer of protocol boundaries, not a protocol feature that
forces all traffic into one canonical representation.

```text
client HTTP adapter + wire observer
  → Guardrail / Tool Loop stages
  → Bridge
  → provider recording endpoint wrapper (target protocol)
  → provider endpoint
```

Provider observation and client stream assembly use a protocol-specific
`CaptureCodec`:

```go
type CaptureCodec interface {
    Snapshot(value any) (Payload, error)
    NewStreamAccumulator(policy StreamPolicy) StreamAccumulator
}
```

The codec serializes complete typed values and optionally assembles stream
events. Raw HTTP request/response bytes can be captured directly. The codec
does not perform protocol conversion. Adding a protocol means registering one
codec, not adding recording branches to every protocol pair.

The adapter observer receives the raw inbound body and the final serialized
body/SSE events after public-model rewriting, so the client snapshot matches
what the user actually sent and received. Recorder core still has no Gin
dependency: the adapter passes immutable payload bytes and status metadata.

The provider observer is immediately outside the terminal endpoint, so every
Tool Loop round naturally becomes a separate exchange. It is an Endpoint
wrapper with complete and stream observation, but it never controls the
provider call or consumes the stream independently.

## Capture Policy

Capture depth and stream detail are separate axes:

| Axis | Values | Default when recording is enabled |
| --- | --- | --- |
| Capture depth | metadata, request, conversation, trace | conversation |
| Stream detail | final, sampled, full | final |
| Retention/export | exporter configuration | independent |

- `conversation` records client request and final client response.
- `trace` additionally records attempts, provider exchanges, stage decisions,
  and transformed provider payloads.
- `final` stores the assembled final response plus counts/timing.
- `sampled` stores bounded first/last events and errors.
- `full` is an explicit diagnostic mode with hard byte/event limits.

The common user choice should remain “record this conversation”. Trace and
full stream capture are advanced diagnostics, not a required mode picker.

Existing `recording_v2` values can be translated during migration:

| Existing value | New capture depth |
| --- | --- |
| request | request |
| request_response | conversation |
| staged_request_response | trace |

## Safety and Resource Limits

- Header capture uses an allowlist. `Authorization`, `X-API-Key`, cookies, and
  provider credentials are always redacted before enqueue/export.
- Payload redaction runs before content-addressed hashing so secret bytes never
  enter blob storage.
- Stream accumulators have hard event and byte budgets.
- Full event capture records truncation explicitly; it never grows without
  bound.
- The disabled path performs no serialization and does not allocate stream
  buffers.
- Export remains asynchronous and non-blocking, with dropped-record metrics.

## Relation to Other Observability

Recording, request logging, and usage accounting remain separate systems:

| Concern | Source of truth | Recording relationship |
| --- | --- | --- |
| Request timeline and diagnostics | structured logging | Join by the same `request_id`; recording does not emit lifecycle logs on its behalf |
| Token/cost accounting | usage tracking | Recording may preserve provider/client usage fields as payload evidence, but never updates counters |
| Provider health and retry | failover/load balancing | Recording observes attempt outcomes after the routing decision; it never changes health or retry state |
| Conversation inspection | recording | Owns bounded request/response artifacts and attempt/exchange history |

The shared `request_id` is correlation, not shared mutable ownership. A failure
or disabled state in any one observability subsystem must not change the
others.

## Schema Evolution

The new exporter schema will be version 4. V3 files remain readable; V4 adds
`attempts` and `exchanges` rather than overloading `transformed_request` and
`provider_response`.

Exporter/CAS deduplication remains downstream of capture. The recorder produces
one immutable completed `CallRecord`; exporters decide file layout, batching,
compression, and retention.

## Additive Migration Plan

| Checkpoint | Change | Production effect |
| --- | --- | --- |
| R1 — Foundation | V4 types, lifecycle state machine, redaction, budgets, in-memory exporter tests | None |
| R2 — Codecs | Beta, V1, Chat, Responses complete/stream capture codecs | None |
| R3 — Shadow harness | Observe Stage calls into an in-memory new recorder and compare with client/provider fixtures | No persisted output |
| R4 — Single-route canary | Beta identity Stage route behind an internal experimental switch | Opt-in only |
| R5 — Failover | Dispatcher owns attempts and the single final commit | Opt-in only |
| R6 — Tool Loop | Each provider round becomes an exchange | Opt-in only |
| R7 — Export/UI | V4 exporter, reader, and product surface | Opt-in only |
| R8 — Cutover | Translate existing recording settings to the new policies | Discuss before changing defaults |
| R9 — Cleanup | Remove Gin recorder context, transform recorder, protocol hooks, and MCP recorder interface | After parity proof |

R1–R3 do not modify the active request pipeline. Work stops for review before
R4 touches production handler/topology wiring.

## Required Verification

- exactly one call record for complete, stream, error, and cancellation;
- retryable failure followed by success retains both attempts and commits once;
- Tool Loop retains all provider exchanges in order;
- client/provider protocols and payloads remain distinct across Bridges;
- final stream artifact matches the bytes/events observed by the real client;
- full capture stays within configured memory/byte limits;
- credentials never appear in records, hashes, or blobs;
- exporter backpressure/failure does not affect the model response;
- disabled recording preserves Stage and legacy behavior and allocation budget;
- harness covers all twelve current Stage routes in complete and stream modes.

## Explicit Non-Goals

- Recording does not drive retries, health, usage accounting, or affinity.
- Recording does not become a canonical protocol AST.
- Recording does not execute or filter tools.
- Recording does not own HTTP response writing or SSE framing.
- R1–R3 do not replace or modify the existing recorder.
