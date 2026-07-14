# Protocol Recording Redesign

> Status: proposed design for review; no production pipeline wiring is changed
> by this document.
>
> Scope: complete AI request/response recording for non-streaming, streaming,
> failover, Guardrail, and future Tool Loop/MCP Stage paths. Usage accounting
> is a separate subsystem and is not redesigned here.

## Decision

Recording will be rebuilt as a request-scoped observation service. One client
request produces one complete `AIRequestRecord`, including the client-visible
request/response and the provider exchanges that produced it. It will not be
implemented as one protocol-owning Stage and it will not own HTTP writing,
routing, retries, tool execution, provider calls, or usage accounting.

Two thin observers will expose the real protocol boundaries:

- the HTTP adapter records the complete client request and final logical
  response;
- a provider-endpoint observer records every concrete provider request and
  complete response immediately outside the terminal endpoint.

An outer request-execution scope is the only owner allowed to finish the
overall AI request record. The failover dispatcher owns attempts; provider
errors finish an attempt or exchange, not the overall record.

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
7. Message assembly depends on usage objects supplied by converters, coupling
   two records with different purposes.
8. Generic stream chunks are retained even though the desired artifact is the
   final complete AI response.

## Data Model

One client request produces exactly one `AIRequestRecord`:

```text
AIRequestRecord
├── identity: request/session/scenario/source protocol
├── client request
├── attempts[]
│   ├── route/provider/model/target protocol
│   ├── exchanges[]
│   │   ├── provider request
│   │   ├── provider response
│   │   └── outcome/timing/error
│   └── outcome: succeeded | retryable_error | terminal_error | cancelled
├── client response
└── outcome/timing/error
```

`AIRequestRecord` is deliberately named after the API request, not
`MessageRecord`: one API request can contain many entries in `messages[]` and
several Tool Loop rounds.

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
}
```

Typed protocol/SDK values exist only while producing `Payload`. The stored
model preserves JSON bytes and protocol identity; recorder core never converts
one protocol to another.

## Ownership and Lifecycle

```text
HTTP adapter: BeginAIRequest(raw client request)
  └── request execution scope (deferred FinishAIRequest)
        └── failover dispatcher: BeginAttempt(candidate)
              └── provider observer: BeginExchange(provider request)
                    └── provider response/error: FinishExchange
              └── dispatcher: FinishAttempt
        └── HTTP adapter: observe final response
  └── request execution scope: FinishAIRequest exactly once
```

Rules:

1. `FinishAIRequest` is idempotent and owned only by the outer
   request-execution scope, including failures before provider dispatch.
2. `FinishAttempt` never emits an overall record.
3. A retryable error remains attached to the failed attempt.
4. Streaming success is determined by the same commitment gate used by
   failover, not by whether a recorder hook observed an event.
5. Cancellation is an explicit outcome, not a generic error string.
6. Recorder objects contain no Gin context and never write HTTP/SSE.
7. Recording completion cannot change request execution.

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
    NewStreamAssembler() StreamAssembler
}
```

The codec serializes complete typed values and assembles stream events into one
final protocol response. Raw HTTP request/response bytes can be captured
directly. The codec does not perform protocol conversion. Adding a protocol
means registering one codec, not adding recording branches to every protocol
pair.

The adapter observer receives the raw inbound body and the final complete
response after public-model rewriting, so the client snapshot matches the
logical message the user sent and received. For a stream, the protocol codec
assembles events into that complete response; raw event history is not part of
the first design. Recorder core still has no Gin dependency: the adapter
passes immutable payload bytes and status metadata.

The provider observer is immediately outside the terminal endpoint, so every
Tool Loop round naturally becomes a separate exchange. It is an Endpoint
wrapper with complete and stream observation, but it never controls the
provider call or consumes the stream independently.

## Recording Scope

The first implementation has one behavior when enabled:

- record the complete client request;
- record every complete provider request/response exchange;
- record the final complete client response;
- for streaming, assemble events into the same logical response shape used by
  non-streaming recording;
- record outcomes needed to explain failover and Tool Loop ordering.

It does not introduce capture-depth modes or raw stream-event recording. The
first milestone is only the correct lifecycle and the complete AI message.

## Relation to Other Observability

AI message recording, request logging, and usage accounting are separate
systems:

| Concern | Source of truth | Recording relationship |
| --- | --- | --- |
| Request timeline and diagnostics | structured logging | Join by the same `request_id`; recording does not emit lifecycle logs on its behalf |
| Token/cost accounting | `UsageRecord` / usage tracking | Independent of `AIRequestRecord`; neither creates nor finalizes the other |
| Provider health and retry | failover/load balancing | Recording observes attempt outcomes after the routing decision; it never changes health or retry state |
| AI request inspection | `AIRequestRecord` / recording | Owns protocol payloads and attempt/exchange history |

The shared `request_id` is correlation, not shared mutable ownership. Recording
must work when usage tracking is disabled, and usage tracking must work when
recording is disabled.

A protocol response may naturally contain a native `usage` field inside its
captured JSON body. That field remains part of the unmodified AI response; the
recorder does not parse it into normalized token fields and does not write a
`UsageRecord`.

## Schema Evolution

The new persisted schema will be version 4. V3 files remain readable; V4 adds
`attempts` and `exchanges` rather than overloading `transformed_request` and
`provider_response`. The recorder produces one completed `AIRequestRecord`.

## Additive Migration Plan

| Checkpoint | Change | Production effect |
| --- | --- | --- |
| R1 — Foundation | V4 `AIRequestRecord` types, lifecycle state machine, in-memory recorder tests | None |
| R2 — Codecs | Beta, V1, Chat, Responses complete/stream capture codecs | None |
| R3 — Shadow harness | Observe Stage calls into an in-memory new recorder and compare with client/provider fixtures | No persisted output |
| R4 — Single-route canary | Beta identity Stage route behind an internal experimental switch | Opt-in only |
| R5 — Failover | Dispatcher owns attempts and the single final commit | Opt-in only |
| R6 — Tool Loop | Each provider round becomes an exchange | Opt-in only |
| R7 — Persistence/UI | V4 writer, reader, and product surface | Opt-in only |
| R8 — Cutover | Map the existing recording switch to `AIRequestRecord` | Discuss before changing defaults |
| R9 — Cleanup | Remove Gin recorder context, transform recorder, protocol hooks, and MCP recorder interface | After parity proof |

R1–R3 do not modify the active request pipeline. Work stops for review before
R4 touches production handler/topology wiring.

## Required Verification

- exactly one AI request record for complete, stream, error, and cancellation;
- retryable failure followed by success retains both attempts and commits once;
- Tool Loop retains all provider exchanges in order;
- client/provider protocols and payloads remain distinct across Bridges;
- final assembled stream record matches the complete logical client response;
- recording works with usage tracking disabled;
- usage tracking works with recording disabled;
- both records can be correlated by `request_id` without shared ownership;
- disabled recording preserves Stage and legacy behavior;
- harness covers all twelve current Stage routes in complete and stream modes.

## Explicit Non-Goals

- Recording does not drive retries, health, usage accounting, or affinity.
- Recording does not create, update, or finalize `UsageRecord`.
- Recording does not become a canonical protocol AST.
- Recording does not execute or filter tools.
- Recording does not own HTTP response writing or SSE framing.
- R1–R3 do not replace or modify the existing recorder.
