# Adaptive Endpoint Routing

**Date**: 2026-05-16  
**Branch**: `refactor/prefer0514`

---

## Problem

OpenAI-compatible providers vary in which HTTP endpoints they expose and whether they support streaming on each. The three variants in practice:

| Provider kind                     | Chat Completions (`/chat/completions`) | Responses API (`/responses`) |
|-----------------------------------|----------------------------------------|------------------------------|
| Standard OpenAI / third-party     | ✅                                      | optional                     |
| Codex (OAuth)                     | ❌                                      | ✅ only                       |
| Anthropic / Google (pass-through) | native (not OpenAI)                    | —                            |

Before this refactor, the server chose an endpoint by string-matching the model name (
`"codex"`) or the provider base URL (
`"chatgpt.com"`). That heuristic broke for providers that host Codex-style models under arbitrary names, and it could not express that a specific endpoint does not support streaming.

Two concrete failure modes:

1. A request routed to Chat Completions for a provider that only exposes Responses API returns a 404 or a protocol error.
2. A streaming request sent to an endpoint that accepts the base call but does not support
   `stream: true` hangs or fails mid-stream.

---

## Design

### Core principle: probe once, route per-request

The routing decision is made at request time from cached probe data, not from static configuration or model name patterns. The probe result is the source of truth; routing logic is code, not persisted state.

### Data model: `SupportsStream` is first-class

`EndpointStatus` (in-memory) and `ModelCapability` (SQLite) both carry a `SupportsStream` boolean alongside
`Available`. An endpoint that accepts non-streaming calls but hangs on streaming is treated as unavailable for streaming requests.

```
EndpointStatus {
    Available      bool
    SupportsStream bool
    LatencyMs      int
    ErrorMessage   string
    LastChecked    time.Time
}
```

The DB column `supports_stream` mirrors this. `SaveEndpointCapability` replaces the old `SaveCapability`.

### Probe layer (`adaptive_probe.go`)

`ProbeModelEndpoints` runs Chat and Responses probes concurrently under a single
`DefaultProbeTimeout` (10 s) context. Each sub-probe uses that same context directly — no nested timeout.

Codex providers skip the Chat probe entirely (they are known to not support it); the Chat status is set to unavailable immediately without a network round-trip.

The probe result is written to:

1. In-memory `ProbeCache` (fast path for subsequent requests)
2. SQLite via `ModelCapabilityStore` (survives restarts)

`PreferredEndpoint` is never persisted. It is always recalculated from raw `Available`/`SupportsStream` state by
`determinePreferredEndpoint`. This means routing behavior can be changed by a code deploy without a DB migration or cache flush.

### Routing layer (`adaptive_endpoint_router.go`)

`SelectOpenAIEndpoint` is the single entry point for all four OpenAI-style handlers:

```
SelectOpenAIEndpoint(
    ctx         context.Context,
    provider    *typ.Provider,
    modelID     string,
    incoming    IncomingAPIType,   // "chat" or "responses"
    isStreaming bool,
    responsesReq *protocol.ResponseCreateRequest,  // nil for chat incoming
) (*EndpointSelection, error)
```

Decision tree:

```
Codex provider?
  └─ yes → Responses (always)

No cached capability?
  └─ fire background probe; respect incoming API (non-blocking)

incoming = chat:
  Chat usable (available ∧ (non-stream ∨ supportsStream))?
    └─ yes → Chat
  Responses usable?
    └─ yes → Responses
  └─ fallback → Chat (pass through; let provider error propagate)

incoming = responses:
  Responses usable?
    └─ yes → Responses
  Chat usable?
    └─ can downgrade? (no previous_response_id, include, background, truncation, reasoning)
       └─ yes → Chat
       └─ no  → error 400 (unsupported_endpoint)
  └─ fallback → Responses
```

`endpointUsable(available, supportsStream, isStreaming bool) bool` encodes the stream compatibility check as a single pure function, making it trivially testable.

`CanDowngradeResponsesToChat` checks Responses-API-only fields that cannot be represented in Chat Completions. If any are set, the downgrade is rejected with a 400 rather than silently dropping request semantics.

### Client probe methods (`openai.go`, `probe.go`)

Two explicit probe methods replace the overloaded `ProbeStream`:

- `ProbeChatEndpoint(ctx, model, ProbeEndpointOptions)` — always hits `/chat/completions`
- `ProbeResponsesEndpoint(ctx, model, ProbeEndpointOptions)` — always hits `/responses`

`ProbeStream` is retained for backwards compatibility with the Anthropic client path but is deprecated for new endpoint-routing use.

`ProbeEndpointOptions` carries `Message`, `Stream`, and
`Mode` so callers control exactly what is sent without needing wrapper variants.

---

## Key invariants

1. **`PreferredEndpoint` is never read from the database.** It is always computed from current `Available` and
   `SupportsStream` state. Code changes take effect on the next `GetModelCapability` call.

2. **Routing never blocks a request on an unknown model.
   ** If capability is not cached, the incoming API is honoured and a background probe is scheduled. The probe result improves future routing without adding latency to the current request.

3. **Stream incompatibility is a hard routing constraint.** `endpointUsable` returns `false` when
   `isStreaming && !supportsStream`. The router tries the other endpoint before falling back. If neither endpoint is usable for streaming, the fallback is the primary-preference endpoint for that incoming type — the error comes from the upstream provider, which is the right place for it.

4. **Codex is special-cased at the provider identity level**, not by model name. The check is
   `provider.OAuthDetail.GetIssuer() == ai.IssuerCodex`, which is stable regardless of what model ID is requested.

---

## Files changed

| File                                               | Role                                                                                                              |
|----------------------------------------------------|-------------------------------------------------------------------------------------------------------------------|
| `internal/server/adaptive_endpoint_router.go`      | New: `SelectOpenAIEndpoint`, routing decision tree                                                                |
| `internal/server/adaptive_endpoint_router_test.go` | New: unit tests for `endpointUsable`, `defaultEndpointSelection`, `CanDowngradeResponsesToChat`                   |
| `internal/server/adaptive_probe.go`                | Refactored: Codex fast-path, `SupportsStream` propagation, single timeout per probe run                           |
| `internal/server/probe_cache.go`                   | `ModelEndpointCapability` + `EndpointStatus` gain `SupportsStream`/`ChatSupportsStream`/`ResponsesSupportsStream` |
| `internal/data/db/model_capability.go`             | DB schema gains `supports_stream`; new `SaveEndpointCapability`; `PreferredEndpoint` deprecated on struct         |
| `internal/client/openai.go`                        | `ProbeChatEndpoint`, `ProbeResponsesEndpoint` replacing overloaded `ProbeStream` for endpoint-explicit probing    |
| `internal/client/probe.go`                         | `ProbeEndpointOptions`; `ProbeStream` marked deprecated for routing                                               |
| `internal/server/anthropic_beta.go`                | Migrated to `SelectOpenAIEndpoint`                                                                                |
| `internal/server/anthropic_v1.go`                  | Migrated to `SelectOpenAIEndpoint`                                                                                |
| `internal/server/openai.go`                        | Migrated to `SelectOpenAIEndpoint`                                                                                |
| `internal/server/openai_responses.go`              | Migrated to `SelectOpenAIEndpoint`; `CanDowngradeResponsesToChat` guard                                           |
