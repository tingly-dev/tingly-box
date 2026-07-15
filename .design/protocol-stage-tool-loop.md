# Protocol Stage Tool Loop

> Status: Phase 4 canary active behind `--stage`. The in-process Beta
> implementation, production handler selection, and real HTTP harness path are
> complete; default rollout remains deferred.
>
> Canonical scope: MCP and server-tool model loops expressed as one
> `anthropic_beta` Protocol Stage. The broader chain and rollout remain defined
> in [`protocol-stage-chain.md`](./protocol-stage-chain.md).

## Decision

The production Tool Loop candidate uses `anthropic_beta` as its concrete
working protocol.

This is not a new canonical AST and not a claim that Beta replaces every
protocol boundary. It is a deliberate protocol choice for one Stage:

- Anthropic Beta has the most complete MCP, tool, thinking, and structured
  content surface already used by Tingly-Box;
- MCP tools can remain Anthropic-native instead of passing through an OpenAI
  DTO or a new neutral representation;
- existing Bridges convert other client/provider protocols at the Stage
  boundary;
- MCP remains a tool source/runtime and `servertool` remains an executor;
- one Tool Loop owns injection, interception, execution, and continuation.

The resulting shape has the same separation properties as multiple services or
HTTP middleware while remaining in-process:

```text
client protocol
  -> ingress Bridge
  -> Guardrail Stage (optional)
  -> Tool Loop Stage [anthropic_beta]
  -> provider Bridge
  -> Provider Endpoint
```

Requests move inward. Complete responses, stream events, errors, usage, and
side-effect facts return outward through the same levels.

## Why Beta Instead of a Neutral Tool DTO

Tool invocation is not just a function name plus JSON arguments. The native
protocol also defines:

- request tool definitions and tool-choice behavior;
- `tool_use` and `tool_result` content blocks;
- thinking/signature preservation;
- structured and nested tool-result content;
- stream event ordering and block indexes;
- MCP, Advisor, and server-tool extensions;
- stop reasons and usage fields.

A neutral DTO would have to reproduce this protocol surface and then maintain
another pair of conversions. That reduces type visibility while increasing
semantic conversion work. Beta already provides the required vocabulary, so
the Stage should use it directly.

The earlier OpenAI Chat Tool Loop remains useful as a lifecycle proof. It is
not the production MCP normalization layer.

## Goals

- Give MCP/server tools one complete non-stream and stream lifecycle.
- Keep the Tool Loop independent of Gin, HTTP/SSE writers, routing, and
  provider selection.
- Make ownership of every tool call explicit and deterministic.
- Reuse the existing MCP runtime, Anthropic adapter, servertool executor, and
  protocol assemblers.
- Support other protocols through explicit, capability-complete Bridges.
- Preserve request identity, usage, model, cancellation, stream ownership, and
  irreversible side-effect facts across multiple provider rounds.
- Keep activation additive behind the existing `--stage` process choice.

## Non-Goals

- No new user-facing stage mode, order editor, or MCP-specific startup flag.
- No replacement of all protocols with Anthropic Beta.
- No protocol-neutral tool/content AST.
- No separate MCP Stage and servertool Stage while both participate in the same
  model continuation loop.
- No default cutover or removal of the legacy MCP path in Phase 4.
- No UI, API, Swagger, database, or persistence-format change.
- No remote service boundary yet; the Stage contract merely keeps that option
  open.

## Component Boundaries

| Component | Owns | Does not own |
| --- | --- | --- |
| `tool_loop_anthropic_beta` | Round lifecycle, tool classification, execution sequencing, continuation, usage aggregation | Routing, HTTP, persistence |
| Beta Tool Provider | Injecting the exact enabled tools and returning their owned names | Executing calls or selecting providers |
| MCP runtime | Tool discovery, visibility, Advisor/server-tool configuration | Model round lifecycle |
| Servertool executor | Policy and actual tool execution | Protocol conversion or stream handling |
| Continuation store | One provider/session-scoped mixed continuation segment | General conversation history |
| Bridge | Request conversion inward and response/event conversion outward for one call | Tool ownership and execution |
| Provider Endpoint | One provider invocation in its concrete protocol | Tool continuation |
| Provider Observer | One provider-native request/response exchange per invocation | Stage snapshots |

The Tool Loop is one Stage because injection, interception, execution, and
continuation are one indivisible model lifecycle. Splitting MCP and servertool
into peer Stages would give two levels competing to consume the same
`tool_use`. They may become separate dependencies or remote services, but not
independent protocol levels until their lifecycles are independently
meaningful.

## Native Contracts and Names

The concrete implementation names are part of the design vocabulary:

| Name | Meaning |
| --- | --- |
| `tool_loop_anthropic_beta` | Stage name used in topology and diagnostics |
| `AnthropicBetaToolProvider` | Prepares a Beta request and returns exact owned tool names |
| `AnthropicBetaStageExecutor` | Existing server-tool execution boundary used by the Stage |
| `AnthropicBetaContinuationStore` | Typed Beta mixed-continuation interface |
| `ProtocolStageBetaToolProvider` | Server adapter from MCP runtime to the Stage provider |
| `ProviderBetaContinuationStore` | Provider/session-scoped bounded store implementation |
| `WithServertoolProviders` | Server option for registering additional in-process tool providers across startup and config reload |

Ownership is an exact per-request snapshot returned by the tool provider. It is
not inferred from an MCP name prefix. A client tool and a server tool with the
same name are ambiguous and fail before the provider call.

## Complete Lifecycle

For each call, the Stage:

1. Validates and deep-clones the Beta request so caller-owned input is not
   mutated.
2. Applies one pending mixed continuation segment, if present.
3. Asks the Beta Tool Provider to inject enabled tools and return exact owned
   names.
4. Rejects empty names, duplicate ownership, claimed-but-not-injected tools,
   and collisions with client-declared tools.
5. Invokes the next Endpoint once for the current model round.
6. Extracts tool calls from the provider-native Beta message.
7. Classifies the response:

| Provider result | Behavior |
| --- | --- |
| No owned tool calls | Return the response outward |
| Owned tool calls only | Execute them, append Beta tool results, continue another provider round |
| External/client tool calls only | Return unchanged for the client to execute |
| Mixed owned + external, store available | Execute owned calls, store the owned continuation, hide owned calls, return external calls |
| Mixed owned + external, no store | Return unchanged and execute nothing |

8. Aggregates usage and model facts over every provider round.
9. Stops at the configured maximum round count.

Mixed behavior is conservative. Without a continuation store, executing only
the internal half would create a conversation that cannot be resumed safely,
so the Stage leaves the entire response outward.

## Mixed Continuation

When one response contains both server-owned and client-owned calls, the Stage
cannot send internal tool results back to the provider until the client returns
its external results.

The first request therefore:

1. executes only owned calls;
2. builds a typed Beta assistant + internal-result continuation segment;
3. stores the segment under session, provider, and protocol identity;
4. removes owned calls from the outward response;
5. returns only client-owned calls.

The following request:

1. consumes the segment exactly once;
2. merges the client tool results with the stored internal results;
3. resumes the provider conversation.

The store is bounded, provider-scoped, and single-consume. The Stage itself
does not know the storage key or routing identity.

## Streaming Lifecycle

### Round buffering is required

Anthropic streams may emit text or thinking blocks before a later `tool_use`.
No visible prefix proves that the round contains no internal tool call. If the
prefix were emitted immediately, a later internal call could not be hidden
without producing an invalid or partially leaked client stream.

The Beta Tool Loop therefore buffers exactly one provider round:

1. pull events only when the outer caller invokes `Next`;
2. assemble the round while retaining its native events;
3. close the inner provider stream exactly once;
4. classify the completed Beta message;
5. either continue internally or replay a client-visible round.

This is a protocol-correctness decision, not an implementation shortcut.
Future TTFT optimization must preserve the invariant and cannot assume tool
blocks arrive before text/thinking.

### Stream classification

| Round type | Outward events |
| --- | --- |
| No owned calls | Replay the complete native round |
| Owned calls only | Replay nothing; execute, append results, start next round |
| External calls only | Replay the complete native round |
| Mixed with store | Remove owned block start/delta/stop events and renumber remaining indexes |
| Mixed without store | Replay unchanged and execute nothing |

Usage, model, and `SideEffectsCommitted` are monotonic across all hidden and
visible rounds. Cancellation and provider errors propagate through the outer
stream. Every acquired inner stream is closed once.

## Protocol Topology

### Client ingress to the Beta Tool Loop

| Client protocol | Ingress |
| --- | --- |
| `anthropic_beta` | Identity |
| `anthropic_v1` | V1→Beta subset Bridge |
| `openai_chat` | Chat→Beta Bridge |
| `openai_responses` | Responses→Beta Bridge |

### Beta Tool Loop to provider

| Provider protocol | Provider boundary |
| --- | --- |
| `anthropic_beta` | Identity |
| `openai_chat` | Beta→Chat Bridge |
| `openai_responses` | Beta→Responses Bridge |
| `anthropic_v1` | Not currently supported below the Beta Stage |

Exact-pair registration remains mandatory. The topology builder does not search
for arbitrary transitive conversion paths at runtime.

## Anthropic V1 Compatibility Contract

V1 and Beta are distinct protocols even though V1 create-message requests are
a structural Beta subset.

### Request direction

V1→Beta request conversion uses JSON marshal/unmarshal as the contract. It is
lossless for the currently pinned V1 create-message request surface and avoids
manual field copying.

### Response direction

The compatibility guarantee stops at request promotion. Beta→V1 complete
responses and stream events keep the existing permissive JSON projection in
this phase; they are not checked against a maintained V1 response subset.
Beta-only output may therefore lack equivalent V1 typed semantics.

This is an explicit scope decision rather than a production blocker for the
V1-with-MCP canary. Strict response/event subset validation is deferred until
a concrete compatibility requirement or production evidence justifies it.

This decision does not make the Bridge unconditional bidirectional
compatibility. In particular, a provider whose concrete protocol is
`anthropic_v1` still cannot sit below the Beta Tool Loop without a separately
designed Beta→V1 request Bridge.

SDK regeneration must continue to verify V1→Beta request compatibility. A
future strict response contract would require its own maintained response and
event compatibility suite.

## Guardrail Ordering

The intended order is:

```text
Guardrails(ToolLoop(Provider))
```

Consequences:

- inbound Guardrails inspect original user content before tool injection;
- ToolLoop consumes internal rounds and produces the actual final response;
- outbound Guardrails evaluate only the client-visible final result;
- tool authorization occurs immediately before executor invocation.

This order is fixed by product semantics. Phase 4 does not introduce a user
setting for stage order.

Under `--stage`, V1 Guardrails independently promote the request to the Beta
working protocol. With MCP enabled, both Beta-native stages compose at that
same boundary. This promotion applies only to requests; outward V1 responses
continue to use the existing permissive projection and do not establish a new
strict Beta-to-V1 compatibility contract. Without `--stage`, V1 Guardrails
retain the complete legacy lifecycle.

## Recording and Usage

`RequestRecord` and `UsageRecord` remain separate concerns.

Recording attaches only at stable boundaries:

```text
original client request
  -> Stages / Bridges
  -> Provider Observer: exchange 1..N
  -> Stages / Bridges
  -> final client response
```

The Tool Loop adds no recording hook. Each provider round naturally crosses
the already-observed Provider Endpoint and becomes one ordered
`ProviderExchange` under the same attempt. One incoming request still has one
original input and one final outward response. Intermediate Stage responses are
not persisted.

Usage aggregation remains a protocol-neutral Stage result. Disabling recording
must not disable usage, and disabling usage must not disable recording.

## Errors, Side Effects, and Failover

Two commitment facts are independent:

- output committed: a client-visible event has been written;
- side effects committed: a server-owned tool succeeded.

After a tool succeeds, later provider, conversion, or stream failures carry
`SideEffectsCommitted=true`. The outer failover orchestrator must not restart
the whole attempt on another provider after either commitment boundary.

Failed tool execution produces a tool result marked as an error and does not by
itself mark a successful side effect. Tool-specific retry or idempotency is a
future extension and requires stable request/tool-call deduplication keys.

## Production Activation

Activation reuses the existing `--stage` server startup choice. No new mode or
flag is introduced.

Selection is per provider attempt:

| Condition | Pipeline |
| --- | --- |
| `--stage` disabled | Complete legacy lifecycle |
| No MCP/tool-loop feature | Existing plain Stage selection |
| MCP enabled and exact Beta Tool Loop topology is complete | Beta Tool Loop Stage |
| Missing Bridge/capability/dependency | Complete legacy lifecycle before provider invocation |

Once a Stage attempt begins, it never falls back into legacy mid-attempt.
Rollback is a restart without `--stage`.

Compatibility never activates this path by itself: MCP enablement, Guardrails,
and a complete Beta topology are necessary feature conditions, but the
process-level `--stage` choice remains the first and mandatory gate.

The production wiring checkpoint implements:

1. replace the current unconditional MCP→legacy selection with an exact
   topology eligibility check;
2. construct the Beta tool provider, existing executor, and provider-bound
   continuation store for each attempt;
3. compose Guardrail, Tool Loop, provider Bridge, Provider Observer, and
   terminal endpoint in the documented order;
4. promote V1 MCP requests to Beta while preserving the current permissive
   Beta→V1 response/event projection;
5. add a debug-level entry naming `tool_loop_anthropic_beta` and every concrete
   protocol boundary;
6. verify through the real HTTP path with `harness matrix --stage --mcp` before
   calling the canary active.

All six items are implemented. The first real-path canary is the V1→Beta
promotion in complete and streaming modes through
`harness matrix --mode=single --stage --mcp`.

## Diagnostics and UX-First Review

Diagnostics must answer:

1. Did this request use Stage or legacy?
2. Which concrete protocols and named Stages did it traverse?
3. How many provider rounds and tool executions occurred?
4. Where did failure or commitment happen?

The design follows the product UX principles:

- reuse `--stage`; do not add a mode picker;
- display concrete protocol names such as `anthropic_beta`, not aliases;
- keep MCP enablement and Stage rollout as orthogonal axes;
- make smart fallback automatic when topology is unsupported;
- make harness diagnostics traverse the real path; dormant Bridge tests are
  labeled as such and cannot be presented as production evidence;
- keep rollback and re-entry explicit through process restart;
- scope side effects to the current attempt and stop failover after commitment.

## Current Implementation Checkpoint

Implemented and committed:

- Beta complete and streaming Tool Loop;
- exact owned/external/mixed tool classification;
- provider-scoped mixed continuation;
- direct MCP runtime→Beta tool definitions;
- reuse of the existing Anthropic adapter and servertool executor;
- lossless V1 request promotion and V1→Beta Bridge;
- complete/stream RequestRecord multi-exchange proof;
- composed V1→Beta Tool Loop tests;
- dormant 54-cell Bridge matrix including V1→Beta and V1→Beta Stage→Chat;
- exact two-boundary production selection for all four ingress protocols;
- production Beta Tool Loop assembly with MCP runtime, servertool executor,
  provider-scoped continuation, Guardrail ordering, and provider observation;
- failover suppression after successful tool side effects;
- real HTTP complete/stream V1→Beta MCP canary through `harness matrix`.
- real HTTP owned-tool fixture, driven by raw HTTP plus the official Anthropic
  and OpenAI Go SDKs, proving provider round 1 → local execution → provider
  round 2 → final response across all four ingress protocols;
- normalized converted Anthropic events expose their wire payload to protocol
  assemblers, so cross-protocol streams remain interceptable by the Beta loop;
- additional `servertool.ToolProvider` instances can enter through the formal
  server option and survive config-driven pipeline rebuilds.

Pending by design:

- optional strict Beta→V1 response/event validation, deferred until a concrete
  compatibility need appears;
- default rollout and legacy removal.

## Acceptance Criteria for Production Wiring

- Exact supported source/target pairs select the Beta Tool Loop only with
  `--stage` and the existing MCP feature enabled.
- Unsupported pairs choose legacy before any provider or tool side effect.
- Complete and stream paths produce the same final semantics as the existing
  MCP lifecycle for owned, external, and mixed calls.
- Provider rounds are recorded as ordered exchanges in one request/attempt.
- Usage and source-visible model remain correct across hidden rounds.
- Tool name collisions fail before the provider call.
- Later failures after tool success prevent failover replay.
- V1 MCP requests are promoted to Beta; outward responses/events retain the
  current permissive V1 projection behavior.
- `harness matrix --stage --mcp` traverses the production path and exposes the
  concrete Stage path in debug logs.
- Starting without `--stage` leaves current behavior unchanged.
