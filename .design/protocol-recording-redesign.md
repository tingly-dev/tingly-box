# Request Recording Redesign

> Status: R1–R5 and the additive R8 protocol-wide rollout are implemented.
> RequestRecord remains opt-in and requires both `--stage` and an enabled
> scenario `recording_v2` flag. R6 is proven with the in-process Tool Loop
> Stage; production MCP routing remains on legacy until the Stage canary is
> wired.
>
> Scope: the protocol request/response content retained for one incoming
> request. `UsageRecord`, request logging, and stage tracing are separate.

## Decision

The new unit is `RequestRecord`. It records the original request received at
the client-facing endpoint, the provider calls that actually happened, and the
final response returned outward after Stage and Bridge processing.

Recording attaches only at the two stable client/provider boundaries:

```text
HTTP Adapter
  → Request Scope: capture input_request
  → Stages / Bridges
  → Provider Observer
  → Provider Endpoint

Provider Endpoint response
  → Provider Observer: capture provider_response
  → Stages / Bridges
  → Client Output Adapter: final rewrites + capture final_response
  → HTTP/SSE writer
```

- **Request Scope** records the original client-protocol request before any
  Stage or Bridge and owns the completed `RequestRecord` across failover
  attempts.
- **Provider Observer** records the provider-native request passed into the
  terminal endpoint and the untouched provider-native response returned from
  it.
- **Client Output Adapter** records the final response after outward Stages,
  Bridges, response transforms, public-model rewriting, and other
  client-facing adjustments.

Recording does not snapshot every Stage. Stage insertion, removal, or
reordering must not change the persisted `RequestRecord` shape.

## What “Original Input Request” Means

`input_request` is the first protocol-native request created from the inbound
HTTP request and handed to the client-facing Endpoint. It is captured before
client preparation, Guardrail, Tool Loop, Bridge conversion, consistency, or
provider transforms.

It is the original endpoint request, not a Gin object and not a byte-for-byte
HTTP body/header capture. The HTTP Adapter remains responsible for parsing the
wire request; Recording receives an immutable protocol payload from it.

## What “Provider Request and Response” Means

The provider request is the final protocol-native value handed to the
`Provider Endpoint`, after all inward Stages, Bridges, consistency transforms,
and provider transforms.

The provider response is the protocol-native value returned by the
`Provider Endpoint`, before any outward Bridge or response-processing Stage.

These are the closest stable values owned by Tingly-Box around a provider
call. They are not an HTTP transport dump of SDK headers and bytes. Exact SDK
wire capture would be a separate transport feature and is not part of this
design.

## Data Model

One incoming request produces one `RequestRecord`:

```text
RequestRecord
├── request identity / scenario / outcome / duration
├── input_request
├── provider_exchanges[]
│   ├── sequence / attempt
│   ├── provider / model / protocol
│   ├── provider_request
│   ├── provider_response
│   └── outcome / error / duration
└── final_response?              // present for a successful request
```

```go
type RequestRecord struct {
    RequestID         string
    InputRequest      Payload
    ProviderExchanges []ProviderExchange
    FinalResponse     *Payload
    Outcome           Outcome
}

type ProviderExchange struct {
    Sequence int
    Attempt  int
    Provider string
    Model    string
    Protocol protocol.APIType
    Request  Payload
    Response *Payload
    Error    string
}

type Payload struct {
    Protocol    protocol.APIType
    ContentType string
    Body        json.RawMessage
}
```

`ProviderExchange` is flat and ordered. Failover creates entries with different
attempt numbers. A Tool Loop creates several ordered entries under the same
attempt. A separate nested attempt model is unnecessary for message recording.

## Three Stable Payload Boundaries

The core record contains only these payload boundaries:

1. `input_request`: the original client-protocol request.
2. `provider_exchanges[n]`: each actual provider request and raw provider
   response.
3. `final_response`: the client-protocol response after all outward processing.

A successful request always records `final_response`, including identity paths
where it equals the provider response. Readers never need fallback or equality
rules. A request that fails before producing a response may leave it empty.

No intermediate Stage request or response is stored.

## Ownership and Lifecycle

```text
request scope: BeginRequestRecord(input request)
  └── provider attempt
        └── Provider Observer: begin ProviderExchange
              └── Provider Endpoint
        └── Provider Observer: finish ProviderExchange
  └── optional failover / additional Tool Loop exchanges
  └── Client Output Adapter: capture final outward response
request execution scope: FinishRequestRecord exactly once
```

Rules:

1. The outer request-execution scope is the only owner of
   `FinishRequestRecord`.
2. A provider error finishes only its `ProviderExchange`; it does not finish
   the overall record while failover may continue.
3. Exchanges are appended in actual provider-call order.
4. The recorder never calls the provider and never controls failover.
5. Recording objects contain no Gin context and never write HTTP/SSE.
6. Recording cannot change request execution.

## Placement in Protocol Stage Topology

The Provider Observer wraps the terminal before topology construction. The
request scope is created before the failover loop, while the Client Output
Adapter records the value immediately before HTTP/SSE serialization:

```text
recorder := BeginRequestRecord(input)
for each provider attempt:
    provider := ObserveProvider(terminal, recorder, attempt)
    endpoint := BuildTopology(provider, stages, bridges)
    ServeClientOutput(endpoint, recorder)
FinishRequestRecord(recorder)
```

Capturing only the value returned by `BuildTopology` is too early. Existing
client adapters still apply protocol response transforms, public-model
rewriting, and stream-event adjustments before writing; `final_response` must
reflect those operations.

This placement has stable semantics for all topologies:

| Topology | Client boundary records | Provider Observer records |
| --- | --- | --- |
| Identity | original request and final response | provider request/response |
| Cross-protocol Bridge | source-protocol input and converted output | target-protocol request/response |
| Guardrail | request before inbound policy; response after outbound policy | request after inbound policy; raw provider response |
| Tool Loop | original request and only the final outward response | every provider round |
| Failover | original request and only the response ultimately returned | every provider call by attempt |

Recording every Stage boundary is deliberately rejected for the core record:

- payload count would grow with topology depth;
- internal Stage order would become a storage contract;
- the same content would usually be duplicated;
- Guardrail and Tool Loop implementation details would leak into the request
  artifact.

An ordered stage trace may separately record stage name, protocol, duration,
and outcome. It is diagnostics metadata, not request/response content.

## Complete and Streaming

Complete calls snapshot the input request in the request scope and the
request/response pair at each Provider Observer invocation. The Client Output
Adapter snapshots the final response after its last response rewrite.

For streaming, each observer wraps the stream returned by the next endpoint
and assembles events in that observer's native protocol while the normal caller
pulls them. The observer does not consume the stream independently:

- Provider Observer assembles the complete raw provider response for every
  provider exchange.
- Client Output Adapter assembles the final outward events after client-facing
  rewrites into one complete response.
- Raw stream chunks are not stored in the first implementation.

Recording reuses protocol-owned typed values, Wire DTOs, and stream assemblers.
It never performs protocol conversion itself and does not maintain a second
recording-specific codec registry.

## Separation from Usage

`RequestRecord` and `UsageRecord` are independent:

- neither creates, updates, or finalizes the other;
- either feature works when the other is disabled;
- both may use the same `request_id` for correlation;
- Recording does not parse or normalize token counts.

A provider response may contain a native `usage` field inside its captured
body. It remains ordinary response content and does not make the records share
ownership.

## Legacy Field Mapping

The target semantics map as follows:

| Legacy concept | New field |
| --- | --- |
| Client/pre-transform snapshot (`original_request`) | `input_request` |
| Final provider-bound request (`transformed_request` was the closest implementation) | `provider_exchanges[n].provider_request` |
| Raw provider response (`provider_response`, previously not reliably populated) | `provider_exchanges[n].provider_response` |
| Final outward result (`final_response`) | `final_response` |
| Transform step names | separate stage trace |

## Implementation Containment

The new implementation starts in one isolated package:

```text
internal/record/
├── record.go              // RequestRecord, ProviderExchange, Payload
├── recorder.go            // request-scoped lifecycle
├── provider_endpoint.go   // terminal Endpoint observer
└── stream.go              // observes EventStream through protocol assemblers
```

This package has no Gin, HTTP writer, routing, Guardrail, MCP, or legacy
recorder dependency. It may wrap `protocol/stage.Endpoint`; the Stage core does
not import Recording.

Protocol handling stays in the existing protocol packages:

- complete request/response values remain the typed values already carried by
  `stage.Call`, `stage.Response`, and `wire` DTOs;
- Provider observation delegates through the existing `stage.Endpoint` and
  `stage.EventStream` contracts;
- stream reconstruction reuses the existing Anthropic V1/Beta, OpenAI Chat,
  and OpenAI Responses assemblers in `internal/protocol/assembler`;
- client output capture reuses the same Wire DTO/event values already produced
  by the HTTP/SSE adapters.

If Recording exposes a real common gap, the protocol layer is enhanced once.
The expected addition is a small protocol-owned assembler interface/factory
over the existing implementations, for example:

```go
type StreamAssembler interface {
    Add(value any) error
    Finish() (any, error)
}

func NewStreamAssembler(api protocol.APIType) (StreamAssembler, error)
```

The concrete type switches for SDK and Wire events belong to the protocol
assembler package, not `internal/record`. No source→target protocol-pair logic
is added for Recording.

Production integration is limited to three seams per client protocol:

1. The protocol prologue creates one recorder and captures `input_request`
   before the failover loop.
2. The Stage target builder wraps the selected Provider Endpoint.
3. The client output adapter captures the post-rewrite complete response or
   outgoing stream events.

The first Beta identity canary therefore touches only
`anthropic_message.go` and `protocol_stage_anthropic_beta.go` outside the new
package. It does not modify Stage/Bridge contracts, Guardrail, MCP, or the
legacy recorder.

Failover integration is a later, central change in `failover_dispatch.go`: it
provides the current attempt number while keeping the same request-scoped
recorder. Tool Loop requires no recording hook; repeated calls through the
already-wrapped Provider Endpoint naturally append exchanges.

Other source protocols are integrated one at a time through their existing
prologue and client output adapter, with an independent test and commit for
each. No all-protocol handler rewrite is required.

## Additive Migration Plan

| Checkpoint | Status | Change | Production effect |
| --- | --- | --- | --- |
| R1 — Foundation | Complete | `RequestRecord`, ordered exchanges, lifecycle, in-memory tests | None |
| R2 — Protocol capture support | Complete | Common interface over existing Beta, V1, Chat, and Responses assemblers | None |
| R3 — Boundary harness | Complete | Verify input, provider, and output snapshots across all Stage routes | No persisted output |
| R4 — Single-route canary | Complete | Beta identity, single service, no MCP | Opt-in only |
| R5 — Failover | Complete | Ordered attempt exchanges across homogeneous and cross-protocol failover, one final record | Opt-in only |
| R6 — Tool Loop | Stage proof complete; production pending | Multiple exchanges in one attempt through the real Provider Observer, with one final response | None yet; MCP still selects legacy |
| R7 — Persistence/UI | Partial | Native reader and request inspection surface; R4 already writes an additive `request_record` envelope through the existing sink | Opt-in only |
| R8 — Protocol-wide rollout | Complete | All twelve production Stage routes, complete and stream, Stage-compatible service sets and no MCP | Opt-in only; no default cutover |
| R9 — Cleanup | Not started | Remove Gin recorder, transform recorder, stream hooks, and MCP recorder interface | After parity proof |

### Current Activation Boundary

The new recording path is selected only when all of these are true:

- the server starts with `--stage`;
- the request scenario has `recording_v2` enabled;
- the route is one of the twelve explicitly registered production Stage routes;
- every active service resolves to a registered Stage provider protocol
  (current exclusion: Google-style providers);
- MCP is disabled.

Every other recording combination keeps the complete legacy lifecycle. Without
`recording_v2`, no new recorder or sink work is performed. Restarting without
`--stage` is the rollback. The harness compatibility label V1 → Beta resolves
to the V1 provider protocol at runtime; it exercises V1 identity rather than
introducing a V1/Beta Bridge.

The completed `RequestRecord` is persisted through the existing asynchronous
obs sink as an additive `request_record` envelope. Legacy readers may ignore
that field; non-Stage or unsupported feature combinations continue to use the
legacy behavior.

R8 verification ran the text matrix through the real server path with
`--stage --record-dir`: all 26 labeled complete/stream cases passed. The
persisted output contained 26 successful `RequestRecord` envelopes, each with
one input, one provider exchange, and one final response. Those 26 cases cover
the twelve production Stage routes twice, plus the two V1 → Beta compatibility
cases that normalize to V1 identity.

R5 keeps the recorder at request scope while the failover orchestrator exposes
the current attempt number only when recording is active. Each provider
terminal wrapper appends one exchange with that attempt number. The request is
finished once, after the failover gate has committed the winning response or
flushed the terminal error. Complete and stream tests cover all four source
protocols with cross-protocol failure → success, plus exhausted two-provider
failure. Rules containing a provider protocol outside the registered Stage
surface do not enter the new recording path.

R6 requires no new recording hook. The Chat-native Tool Loop calls the already
observed Provider Endpoint once per model round, so each round appends an
ordered exchange under the same attempt number. Complete/stream Stage tests
prove that an internal tool round followed by a final model round produces two
successful provider exchanges and one final outward response. This is an
in-process boundary proof, not production activation: requests with MCP remain
on the complete legacy path until the Tool Loop runtime adapters and handler
selection are wired.

## Required Verification

- identity complete/stream records the input, one provider pair, and the final
  response;
- every route preserves the original client-protocol request in
  `input_request` before any Stage or Bridge mutation;
- each cross-protocol route records target-protocol provider payloads and the
  source-protocol output response;
- same-protocol response mutation is reflected in `final_response`;
- retryable failure followed by success retains both provider exchanges and
  finishes one `RequestRecord`;
- Tool Loop retains every provider exchange in order but only one final output;
- stream assembly produces the same logical payload as complete recording;
- Recording works with Usage disabled and Usage works with Recording disabled;
- the harness covers all twelve current Stage routes in complete and stream
  modes.

## Explicit Non-Goals

- Recording does not drive routing, retries, health, usage, or affinity.
- Recording does not snapshot every Stage payload.
- Recording does not become a canonical protocol AST.
- Recording does not own HTTP response writing or SSE framing.
- R1–R3 do not replace or modify the existing recorder.
