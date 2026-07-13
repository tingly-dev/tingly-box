# Protocol Stage Chain

> Status: Phases 1–2 are implemented additively. `tingly-box start --stage`
> now opts OpenAI Chat → Anthropic Beta and Anthropic Beta → Anthropic Beta
> attempts into the production Stage pipeline; all other routes and requests
> requiring MCP, Guardrails, or protocol recording remain on legacy.
>
> Scope: LLM request/response data plane for non-streaming and streaming calls.

## Decision

Tingly-Box will evolve protocol-bound features into an ordered chain of
in-process **Protocol Stages**. A stage exposes the same complete and streaming
operations as the endpoint it wraps. Bidirectional protocol bridges adapt the
client protocol to the stage protocol and the stage protocol to the selected
provider protocol.

The first implementation is deliberately旁路: it adds contracts and tests in a
new package but does not connect any existing handler, Guardrail, MCP, or
server-tool path. Existing behavior remains the default until each migrated
stage passes parity, real-path harness, and canary validation.

## Why This Change

Guardrails, MCP, and server-tool behavior currently attach to a mixture of:

- concrete SDK request/response types;
- request transform chains;
- `protocol.HandleContext` stream hooks;
- protocol-pair dispatch branches;
- an MCP-owned multi-round stream loop;
- Gin response and SSE writers.

This spreads lifecycle ownership across features. In particular, a stream may
be consumed, filtered, recorded, guarded, and written by different layers. A
feature that should conceptually be “one processing level” must understand
several protocols and several execution modes.

Normal endpoint composition makes the order explicit: requests travel inward;
responses and stream events travel outward through the same wrappers.

## Terminology

| Term | Meaning |
| --- | --- |
| Protocol | A concrete API shape such as `openai_chat`, `openai_responses`, `anthropic_v1`, or `anthropic_beta` |
| Endpoint | A complete non-stream and stream implementation of one protocol |
| Protocol Stage | A named full-duplex endpoint wrapper implementing one native protocol |
| Bridge | A bidirectional adapter between two protocols: request inward, response/events/errors outward |
| Terminal Endpoint | The innermost provider-facing endpoint |
| HTTP Adapter | The only component allowed to parse ingress HTTP and commit response bytes/SSE |

Do not call Protocol Stages “tiers”. Tier already means provider failover
priority in Tingly-Box.

## Target Shape

```text
client
  | client protocol
  v
HTTP Adapter
  |
Ingress Bridge              client protocol <-> stage protocol
  |
Guardrails Stage            request pre-check / final response post-check
  |
Tool Loop Stage             tool catalog, interception, execution, continuation
  |
Provider Bridge             stage protocol <-> selected provider protocol
  |
Provider Endpoint
```

The arrows for requests point downward. Complete responses, stream events, and
errors return upward through the same wrappers.

## Core Contracts

The first foundation defines four concepts:

```go
type Endpoint interface {
    Protocol() protocol.APIType
    Complete(context.Context, Call) (*Response, error)
    Stream(context.Context, Call) (EventStream, error)
}

type Stage interface {
    Name() string
    Protocol() protocol.APIType
    Wrap(next Endpoint) Endpoint
}

type EventStream interface {
    Next(context.Context) (Event, error)
    Close() error
    Result() StreamResult
}

func Compose(terminal Endpoint, stages ...Stage) (Endpoint, error)
```

`Compose(terminal, guardrails, tools)` means:

```text
guardrails(tools(terminal))
```

The stage list is written in request order, outermost to innermost. Composition
fails before execution when a stage protocol differs from the endpoint it
wraps. This keeps an accidental implicit conversion out of feature code.

## Contract Invariants

1. **One native protocol per endpoint** — `Protocol()` is concrete and never an
   alias such as “OpenAI-compatible”.
2. **Both execution modes are required** — a stage implements `Complete` and
   `Stream`; pass-through implementations are valid.
3. **No transport ownership** — a stage cannot depend on Gin, write headers, or
   frame SSE.
4. **One stream consumer chain** — each wrapper pulls from the next stream and
   returns an event outward. Only the HTTP adapter drives the outermost stream.
5. **Explicit close** — the caller closes any successfully returned stream.
   Wrappers propagate close to the upstream stream.
6. **Per-call state** — mutable conversion, Guardrail, tool-call, and usage state
   is created for one call/attempt, never stored globally in a shared Stage.
7. **Structured outcome** — usage, response model, trace, and committed side
   effects travel in `Response` or `StreamResult`, not in Gin context fields.
8. **No hidden bridge** — protocol changes happen only in a named Bridge.
9. **No retry ownership** — stages report failures and commitment state;
   routing/failover decides whether another provider attempt is allowed.

## Bidirectional Bridge Contract

A bridge must convert the whole protocol surface, not merely the request:

- parsed request and request-derived state;
- complete response;
- stream events, including one-to-many event expansion;
- terminal usage and finish reason;
- response model and identifiers;
- typed errors and retry hints.

Bridge instances are immutable configuration and must be concurrency-safe. Any
mutable assembler or correlation state belongs to a per-call bridge session
created while converting the request:

```go
type Bridge interface {
    Source() protocol.APIType
    Target() protocol.APIType
    Capabilities() Capabilities
    Open(context.Context, Call, Operation) (BridgeSession, error)
}

type BridgeSession interface {
    TargetCall() Call
    ConvertComplete(context.Context, *Response) (*Response, error)
    ConvertStream(context.Context, EventStream) (EventStream, error)
    ConvertError(context.Context, error) error
}
```

`Open` converts the request inward and creates exactly one session for that
call. `OperationComplete` and `OperationStream` are explicit because request
conversion may set different stream/usage fields. The session converts complete
responses, streams, and target errors outward. After successful stream
conversion, the converted stream owns and closes the target stream.

`Call.State` is a bounded `ProtocolState` carrier for request-derived facts that
remain necessary after changing protocol. The initial `OpenAIChat` field holds
the `OpenAIConfig` produced by Anthropic-to-Chat conversion for later provider
transforms. This state is per-call and is intentionally not an extensible
property bag.

An immutable `BridgeRegistry` resolves exact protocol pairs and capabilities.
The topology builder works from the provider terminal outward and inserts a
registered bridge whenever adjacent stages speak different protocols. This
allows every stage to implement one native protocol without requiring all
stages to choose the same protocol.

The generic Bridge foundation is additive. Concrete bridges must reuse existing
request/nonstream/stream converters rather than create a second conversion
implementation.

### Canonical Stage Protocol

`anthropic_beta` is the leading initial candidate because it is already the
normalization target for non-Anthropic requests sent to Anthropic providers and
can represent rich content and tools. It is not hard-coded as a universal
promise.

Before enabling a chain, a capability check must prove both bridge legs for the
requested features:

```text
source -> stage protocol -> provider target
```

Capabilities include complete response, streaming, tools, tool results, usage,
finish reason, and error fidelity. Known OpenAI Responses tool-use defects mean
some combinations must remain on the legacy path during migration.

## Feature Ownership

### Guardrails Stage

The Guardrails Stage owns:

- inbound user-content evaluation and permitted request mutation;
- outbound evaluation of the final client-visible response;
- stream accumulation required by a configured policy;
- credential masking state and cleanup;
- Guardrail-specific trace facts that do not expose protected content.

It does not inject tools, execute tools, select providers, or write errors to
HTTP.

### Tool Loop Stage

The Tool Loop Stage owns:

- server-visible tool catalog injection;
- complete and streaming tool-call assembly;
- classification of client/external versus server/internal tools;
- policy checks immediately before execution;
- invocation through a protocol-neutral `ToolExecutor`;
- appending tool results and continuing the next model round;
- max-round enforcement and usage accumulation.

MCP remains a tool catalog/runtime source. `servertool.Executor` remains an
execution backend. Neither needs to understand every client/provider protocol;
only the Tool Loop Stage understands its native stage protocol.

Splitting MCP and servertool into two protocol stages immediately would create
a false boundary because both would compete to own the same model tool-call
loop. They can become separately composable later only if their request and
response lifecycles become independently meaningful.

## Intended Stage Order

Default order:

```text
Guardrails(ToolLoop(Provider))
```

Consequences:

- the inbound Guardrail sees the original user content before tool injection;
- the Tool Loop consumes internal model tool calls and produces a final answer;
- the outbound Guardrail sees what will actually be returned to the user;
- tool authorization runs inside the Tool Loop before any executor call.

Current stream and non-stream paths do not express this order identically.
Migration must record the difference and intentionally converge on the order
above rather than silently claiming byte-for-byte parity.

## Streaming Lifecycle

1. The HTTP Adapter parses the request without committing SSE headers.
2. The composed endpoint returns an `EventStream` or a pre-stream error.
3. The HTTP Adapter installs the existing failover commit gate when applicable.
4. It pulls events from the outer stream.
5. The first real client-visible event commits the response.
6. It frames and writes that event in the client protocol.
7. On completion or error it reads `StreamResult`, records usage/trace, and
   closes the stream.

Converters and stages must support cancellation and backpressure by performing
work only when `Next` is called and honoring the passed context. No stage may
buffer the full response merely to simplify transport handling unless a
specific Guardrail policy explicitly requires bounded accumulation.

## Failover and Irreversible Side Effects

Two commitment boundaries matter:

1. **Output committed** — the first client-visible chunk has left the process.
2. **Side effects committed** — a server tool has successfully performed work
   that cannot be safely replayed.

After either boundary, the outer orchestrator must not discard the attempt and
restart the full chain on another provider.

The first tool-stage implementation will conservatively mark successful tool
execution as committed. Future work may allow retry for tools that explicitly
declare idempotency and use a stable `(request ID, tool call ID)` deduplication
key.

## Observability

Every execution should eventually emit an ordered stage trace such as:

```text
openai_chat -> anthropic_beta -> guardrails -> tool_loop
            -> openai_responses -> provider
```

Each entry records concrete protocol, duration, outcome, and safe counters. It
must not record prompts, credentials, masked content, or raw tool arguments by
default. Diagnostics must traverse the production chain; a direct provider
probe remains useful only as an explicit comparison path.

## Incremental Migration

### Phase 1 — Foundation, no traffic

- Add `internal/protocol/stage` contracts and composition validation.
- Add unit tests for complete and stream ordering, protocol mismatch, and close.
- Do not import the package from existing server code.

### Phase 2 — Bridge and concrete-chain harness

- Adapt existing request/nonstream/stream converters behind a bidirectional
  bridge session.
- Split protocol conversion from Consistency/Vendor provider finalization.
- Add a real composed A -> B -> C request path to the harness.
- Add capability checks and keep unsupported combinations on legacy.

Generic Bridge sessions, capability checks, an immutable exact-pair registry,
identity bridges, and mixed-protocol in-memory topology tests are implemented.
Concrete Anthropic v1/beta → OpenAI Chat and OpenAI Chat → Anthropic Beta
bridges are implemented for complete and stream. The dormant matrix now runs a
real Chat → Beta-native Stage → Chat topology in both modes. A production-path
harness remains pending until server dispatch is deliberately integrated with
the new topology; the dormant matrix must not be treated as evidence of that
wiring.

### Phase 3 — Guardrails canary

- Implement Guardrails for one native stage protocol.
- Compare dry-run decisions and mutations with legacy behavior.
- Enable only for allowlisted scenarios/providers; retain legacy fallback before
  response commitment.

### Phase 4 — Tool Loop canary

- Move generic complete/stream MCP loops behind one Stage.
- Inject `ToolCatalog`, `ToolPolicy`, and `ToolExecutor` dependencies.
- Validate using deterministic mocks and read-only tools before a production
  canary. Never dual-execute tools for shadow comparison.

### Phase 5 — Handler integration and default rollout

- Compose a fresh chain for each provider attempt from a pristine request.
- Preserve existing routing, load balancing, and first-chunk gate behavior.
- Progress from internal allowlist to default only after protocol matrix,
  official SDK, Duo, and failover validation.

The first opt-in integration selects Stage per provider attempt after routing
has resolved the concrete target protocol but before legacy Base conversion.
`--stage` is immutable for the server process. The Stage path reuses client
preparation, target consistency, rule, and vendor transforms as native protocol
stages; the provider endpoint and HTTP adapter retain their existing ownership.
Unsupported protocol pairs and MCP-enabled requests remain on legacy. Once a
Stage attempt has started, it is never replayed through legacy.

The next native routes are explicitly `anthropic_beta → anthropic_beta` and
`anthropic_beta → openai_chat`. `anthropic_v1` remains a separate protocol and
does not inherit Beta's identity or Bridge registrations. The Beta routes stay
on legacy whenever MCP, Guardrails, or V2 protocol recording owns part of the
request/response lifecycle.

### Phase 6 — Legacy removal

- Remove protocol-specific feature hooks and MCP transforms in separate changes.
- Remove per-protocol MCP experiment toggles after all supported traffic uses
  the new chain.
- Keep rollback available until the cleanup change itself is proven stable.

## Rollout and Rollback

- `legacy` remains the default at the beginning.
- `tingly-box start --stage` activates Stage selection; restart without the flag
  is the rollback artifact.
- Pure conversion and dry-run Guardrail behavior may run in shadow.
- Tool execution may only run once and therefore uses explicit canaries.
- Fallback is allowed only before output or side effects are committed.
- No persisted schema change is required for the foundation or first canaries.
- Removing legacy code is never combined with initially enabling a migrated
  Stage.

## Security Requirements

- Stage metadata cannot become an unbounded feature-specific property bag.
- Sensitive request/response data must not enter stage traces.
- Tool execution requires the same callable/permission checks as the current
  server-tool pipeline.
- Cancellation and close paths must release Guardrail buffers and provider
  streams.
- Unsupported protocol capabilities fail closed or stay on legacy during the
  migration; they are never silently omitted.
- Side-effect commitment must be propagated even when a later model round
  fails.

## UX-First Review

- **Vocabulary**: “Protocol Stage” avoids collision with routing Tier.
- **Smart defaults**: existing feature enablement builds one default order; no
  mode picker is introduced.
- **Concrete values**: diagnostics show `anthropic_beta`, not an alias.
- **Real path**: harness and diagnostics exercise the same composed endpoint
  used by requests.
- **Reversibility**: legacy remains available through canary rollout and cleanup
  is a later decision.
- **Scoped effects**: shadow mode excludes tool execution and fallback stops at
  commitment boundaries.

## Alternatives Rejected for the Initial Migration

- Expanding raw protocol hooks as the target architecture.
- Creating a user-configurable stage-order editor.
- Running every stage as a separate HTTP process before in-process semantics are
  complete.
- Making servertool a separate protocol stage while the Tool Loop still owns
  tool-call assembly and continuation.
- Replacing all protocol dispatch paths in one change.

## Implementation Checkpoint — 2026-07-13

The following are implemented under `internal/protocol/stage` without any
production import:

- complete and streaming Endpoint contracts;
- ordered same-protocol Stage composition;
- per-call bidirectional Bridge sessions;
- core and semantic capability checks;
- exact-pair immutable Bridge registry plus identity fallback;
- topology construction that inserts Bridges between differently typed Stages;
- monotonic propagation of usage/model fallback and committed side effects;
- complete and streaming in-memory multi-hop harnesses.

Bidirectional Anthropic Beta/OpenAI Chat Bridges are implemented. Both response
directions expose transport-neutral complete and stream conversion entrypoints
while existing `Handle*` functions remain the production wrappers. The dormant
42-cell Bridge matrix includes a concrete Chat → Beta-native Stage → Chat
topology and verifies text, tool-use, and tool-result semantics in complete and
streaming modes.

Runtime integration is opt-in through `--stage`. For each OpenAI Chat provider
attempt whose concrete target is Anthropic Beta, the server builds a fresh Chat
preparation → Bridge → Beta provider-finalization → provider endpoint topology.
For an Anthropic Beta request, it builds Beta preparation followed by either
the Beta identity path or the Beta → OpenAI Chat Bridge, then the concrete
provider-finalization and endpoint. Streaming and complete responses return
through the same endpoint chain and the outer Beta HTTP adapter. Anthropic V1,
capability-missing pairs, feature-owned legacy lifecycles, and the explicit
response-roundtrip diagnostic remain on legacy. Debug routing exposes the
concrete `X-Tingly-Protocol-Pipeline: stage|legacy` decision.
