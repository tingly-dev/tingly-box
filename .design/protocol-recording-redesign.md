# Request Recording Redesign

> Status: proposed design for review; no production pipeline wiring is changed
> by this document.
>
> Scope: the protocol request/response content retained for one incoming
> request. `UsageRecord`, request logging, and stage tracing are separate.

## Decision

The new unit is `RequestRecord`. It records the provider calls that actually
happened and, when different, the final response returned outward after Stage
and Bridge processing.

Recording attaches only at two stable endpoint boundaries:

```text
HTTP Adapter
  → Output Observer
  → Stages / Bridges
  → Provider Observer
  → Provider Endpoint
```

- **Provider Observer** records the provider-native request passed into the
  terminal endpoint and the untouched provider-native response returned from
  it.
- **Output Observer** sees the response after every outward Stage and Bridge.
  It stores `output_response` only when that response differs from the final
  successful provider response.

Recording does not snapshot every Stage. Stage insertion, removal, or
reordering must not change the persisted `RequestRecord` shape.

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
├── provider_exchanges[]
│   ├── sequence / attempt
│   ├── provider / model / protocol
│   ├── provider_request
│   ├── provider_response
│   └── outcome / error / duration
└── output_response?             // only when different
```

```go
type RequestRecord struct {
    RequestID         string
    ProviderExchanges []ProviderExchange
    OutputResponse    *Payload
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

The incoming client body is not stored as another core payload. The provider
request is the request that can explain or replay the actual provider call. If
client-ingress capture is later needed for a specific product use case, it can
be added as a separate boundary field without changing provider exchange
semantics.

## When `output_response` Exists

The outward response is stored when any of these is true:

- a Bridge changed the response protocol;
- a response Stage changed its content;
- the outer chain produced a response without a provider response.

For an identity path whose final response equals the final successful provider
response, `output_response` is omitted. Readers treat the provider response as
the returned response in that case. This preserves the old useful distinction
without storing the same payload twice.

The comparison happens between captured protocol payloads at the two
boundaries. Individual Stages do not need to report whether they mutated a
response.

## Ownership and Lifecycle

```text
request execution scope: BeginRequestRecord
  └── provider attempt
        └── Provider Observer: begin ProviderExchange
              └── Provider Endpoint
        └── Provider Observer: finish ProviderExchange
  └── optional failover / additional Tool Loop exchanges
  └── Output Observer: capture final outward response
request execution scope: FinishRequestRecord exactly once
```

Rules:

1. The outer request-execution scope is the only owner of
   `FinishRequestRecord`.
2. A provider error finishes only its `ProviderExchange`; it does not finish
   the overall record while failover may continue.
3. Exchanges are appended in actual provider-call order.
4. The Output Observer never calls the provider and never controls failover.
5. Recording objects contain no Gin context and never write HTTP/SSE.
6. Recording cannot change request execution.

## Placement in Protocol Stage Topology

The Provider Observer wraps the terminal before topology construction. The
Output Observer wraps the completed client-facing topology:

```text
provider := ObserveProvider(terminal, recorder)
topology := BuildTopology(provider, stages, bridges)
endpoint := ObserveOutput(topology, recorder)
```

This placement has stable semantics for all topologies:

| Topology | Provider Observer sees | Output Observer sees |
| --- | --- | --- |
| Identity | provider request/response | same response; normally omitted |
| Cross-protocol Bridge | target-protocol request/response | source-protocol converted response |
| Guardrail | request after inbound policy; raw provider response | response after outbound policy |
| Tool Loop | every provider round | only the final outward response |
| Failover | every provider call by attempt | only the response ultimately returned |

Recording every Stage boundary is deliberately rejected for the core record:

- payload count would grow with topology depth;
- internal Stage order would become a storage contract;
- the same content would usually be duplicated;
- Guardrail and Tool Loop implementation details would leak into the request
  artifact.

An ordered stage trace may separately record stage name, protocol, duration,
and outcome. It is diagnostics metadata, not request/response content.

## Complete and Streaming

Complete calls snapshot the request and response values at each observer.

For streaming, each observer wraps the stream returned by the next endpoint
and assembles events in that observer's native protocol while the normal caller
pulls them. The observer does not consume the stream independently:

- Provider Observer assembles the complete raw provider response for every
  provider exchange.
- Output Observer assembles the complete final outward response.
- Raw stream chunks are not stored in the first implementation.

Each protocol therefore needs one capture codec for complete values and stream
assembly. Recording never performs protocol conversion itself.

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
| Final provider-bound request (`transformed_request` was the closest implementation) | `provider_exchanges[n].provider_request` |
| Raw provider response (`provider_response`, previously not reliably populated) | `provider_exchanges[n].provider_response` |
| Converted or response-processed result (`final_response`) | optional `output_response` |
| Client/pre-transform snapshot (`original_request`) | no first-version core field |
| Transform step names | separate stage trace |

## Additive Migration Plan

| Checkpoint | Change | Production effect |
| --- | --- | --- |
| R1 — Foundation | `RequestRecord`, ordered exchanges, lifecycle, in-memory tests | None |
| R2 — Capture codecs | Beta, V1, Chat, Responses complete/stream assembly | None |
| R3 — Boundary harness | Verify provider and output snapshots across all Stage routes | No persisted output |
| R4 — Single-route canary | Beta identity route behind an internal experimental switch | Opt-in only |
| R5 — Failover | Multiple exchanges, one final record | Opt-in only |
| R6 — Tool Loop | Multiple exchanges in one attempt | Opt-in only |
| R7 — Persistence/UI | New writer, reader, and request inspection surface | Opt-in only |
| R8 — Cutover | Map the existing recording switch to `RequestRecord` | Discuss before changing defaults |
| R9 — Cleanup | Remove Gin recorder, transform recorder, stream hooks, and MCP recorder interface | After parity proof |

R1–R3 do not modify the active request pipeline. Work stops for review before
R4 touches production topology wiring.

## Required Verification

- identity complete/stream records one provider pair and omits duplicate
  `output_response`;
- each cross-protocol route records target-protocol provider payloads and the
  source-protocol output response;
- same-protocol response mutation produces `output_response`;
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
