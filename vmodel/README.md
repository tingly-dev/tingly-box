# `vmodel`

Virtual models — synthetic, protocol-compliant provider implementations
that power the production `/virtual/v1/*` endpoint (used for onboarding,
demos, and dry-runs without configuring a real upstream provider). They
are wired into the virtual server at `vmodel/virtualserver`
and shipped in the production binary via `server.UseVirtualModelEndpoints`.

The same primitives are reused as an in-process LLM substitute by test
packages that need wire-format-correct fixtures (see
`internal/protocoltest`).

## Layout

```
vmodel/
├── interface.go        // base VirtualModel interface (provider-neutral)
├── types.go            // VirtualModelType, Model, ToolCallConfig, helpers
├── registry.go         // GenericRegistry[T] — shared thread-safe registry
├── base_mock.go        // BaseMockModel — shared identity/metadata methods
├── stream.go           // ResolveChunkDelay, EmitChunks — shared stream helpers
├── defaults_shared.go  // SharedDefaultMocks() + ExtendedErrorSpecs() — shared specs
├── error_injection.go  // ErrorInjection, ErrorInjectingModel, EmitGate
├── README.md
├── anthropic/          // Anthropic-protocol models + Registry alias
├── openai/             // OpenAI Chat-protocol models + Registry alias
├── virtualserver/      // Production Gin HTTP handler + Service wiring
└── benchmark/          // Load-test client + local server factory
    └── examples/       // Runnable server/client examples
```

The root package contains provider-neutral primitives shared by all sub-packages.
Concrete models, protocol-specific request/response types, stream events, and the
`Registry` alias live in the `anthropic` and `openai` sub-packages. The two
sub-packages do **not** import each other.

## Positioning & registration discipline

`vmodel` is a **business-first** package: it ships in production
to back the public `/virtual/v1/*` endpoint, and it is the single source of
truth for synthetic, protocol-compliant model behavior across the codebase.
Test packages are **secondary consumers** that reuse the same primitives.

| Role | Surface | Consumer |
|------|---------|----------|
| Primary (production) | `/virtual/v1/messages`, `/virtual/v1/chat/completions` | `internal/server` mounts `virtualserver.Service` for end-user demos / onboarding / dry-runs |
| Secondary (tests)    | In-process `GenericRegistry[T]`                       | `internal/protocoltest.Scenario`, `cli/harness --mock` |

**Registration discipline.** Anything added to `anthropic.RegisterDefaults` or
`openai.RegisterDefaults` is visible to **end users** of the production
endpoint. Therefore the defaults registry must contain only **user-facing
demo entries** — protocol-compliant, named clearly, useful for onboarding and
dry-runs (`echo-model`, `ask-user-question`, `virtual-claude-3`,
`virtual-gpt-4`, the compact transforms, etc.).

Test-only fixtures (protocol corner cases, wire-format edge cases,
scenario-specific stubs) **must not** be added to `RegisterDefaults`. Tests
that need bespoke synthetic models should construct their own
`GenericRegistry[T]` (the way `protocoltest` does for `Scenario`) and
register fixtures there — keeping the production defaults clean.

**Opt-in fixture sets.** Three named registration helpers ship alongside
`RegisterDefaults` for callers that want pre-built fixtures without polluting
the production endpoint:

| Helper | What it registers | When to call |
|--------|-------------------|--------------|
| `RegisterStreamTestMocks(reg)` | `virtual-stream-test`, `virtual-stream-test-tool` — advertise the full usage shape (prompt / completion / cached / cache-creation / reasoning) | Streaming-converter tests that need deterministic usage emission |
| `RegisterErrorMocks(reg)`      | `virtual-fail-precontent-{429,500}`, `virtual-fail-midstream-{close,event}` — always fail per the configured stage | Failover / resilience tests that need a deterministic broken upstream |
| `RegisterExtendedErrorMocks(reg)` | `virtual-fail-auth-{401,403}`, `virtual-fail-upstream-{502,503,504}`, `virtual-fail-invalid-400`, etc. — comprehensive error scenarios | Advanced testing with full error catalog |
| `RegisterAllErrorMocks(reg)` | Combines both basic and extended error models | Convenience helper for full error catalog |

Both helpers live in each per-protocol sub-package (`anthropic.Register…` and
`openai.Register…`), source their specs from the root `defaults_shared.go`,
and are kept out of `RegisterDefaults` so production registries stay clean.

## Design

### GenericRegistry

`GenericRegistry[T VirtualModel]` is a thread-safe, generic registry that
underpins all per-protocol registries in this module:

```go
// anthropic.Registry and openai.Registry are type aliases:
type Registry = virtualmodel.GenericRegistry[VirtualModel]
```

Any package that needs to store objects satisfying `virtualmodel.VirtualModel`
can instantiate its own `GenericRegistry` directly — `protocoltest.Scenario`
does this for test scenarios.

### One registry per protocol

```go
anthropicReg := anthropic.NewRegistry()
anthropic.RegisterDefaults(anthropicReg)

openaiReg := openai.NewRegistry()
openai.RegisterDefaults(openaiReg)
```

A model registered in `anthropic.Registry` is callable only via
`/virtual/v1/messages`; a model in `openai.Registry` is callable only
via `/virtual/v1/chat/completions`. The registry **is** the protocol
context — there is no runtime `Protocols()` declaration, no `byProtocol`
index, and no protocol type assertions in lookup paths.

When a client requests a model that does not exist in the registry for
the endpoint it called, the handler returns **404 Not Found** (not 501).
A model is either registered for that protocol or it isn't.

### ID collisions across registries are legal

The same ID can exist in both registries simultaneously, holding two
independent concrete instances. `echo-model`, `ask-user-question`,
`ask-confirmation`, and `web-search-example` are registered in both
defaults exactly this way. Each instance only implements its own
protocol's interface, so there is no possibility of a model "lying"
about which protocols it speaks.

### Anthropic v1 vs. beta

The real Anthropic API distinguishes `MessageNewParams` (v1) and
`BetaMessageNewParams` (beta) on the wire, gated by `?beta=true`. The
virtual server accepts both and canonicalizes to the beta superset at
the HTTP boundary (`virtualserver/handler.go`). Vmodels see exactly one
request type — `*protocol.AnthropicBetaMessagesRequest` — so the
protocol-version distinction does not leak into the `VirtualModel` interface.

## Interfaces

### Base (`interface.go`)

```go
type VirtualModel interface {
    GetID() string
    GetName() string
    GetDescription() string
    GetType() VirtualModelType
    SimulatedDelay() time.Duration
    ToModel() Model
}
```

### Anthropic sub-interface (`anthropic/interface.go`)

```go
type VirtualModel interface {
    virtualmodel.VirtualModel
    HandleAnthropic(req *protocol.AnthropicBetaMessagesRequest) (VModelResponse, error)
    HandleAnthropicStream(req *protocol.AnthropicBetaMessagesRequest, emit func(any)) error
}
```

### OpenAI sub-interface (`openai/interface.go`)

```go
type VirtualModel interface {
    virtualmodel.VirtualModel
    HandleOpenAIChat(req *protocol.OpenAIChatCompletionRequest) (VModelResponse, error)
    HandleOpenAIChatStream(req *protocol.OpenAIChatCompletionRequest, emit func(any)) error
}
```

## Shared root-package primitives

### BaseMockModel

`BaseMockModel` implements the six identity/metadata methods of the base
`VirtualModel` interface (`GetID`, `GetName`, `GetDescription`, `GetType`,
`SimulatedDelay`, `ToModel`). Protocol-specific mock types embed it and only
add their `Handle*` methods:

```go
type MockModel struct {
    virtualmodel.BaseMockModel
    cfg *MockModelConfig
}
```

### ResolveChunkDelay / EmitChunks

`ResolveChunkDelay(totalDelay, chunkCount)` distributes a model's simulated
latency evenly across stream chunks. `EmitChunks` is the shared inner loop —
it calls the emit closure once per chunk with the appropriate sleep in between.
Both the anthropic and openai `DefaultStream` helpers use these.

### SharedDefaultMocks

`SharedDefaultMocks()` returns the specs for the four mocks that are registered
in **both** default registries. Each protocol's `RegisterDefaults` calls this
and wraps each spec in its own `MockModel`:

```go
for _, spec := range virtualmodel.SharedDefaultMocks() {
    _ = reg.Register(NewMockModel(&MockModelConfig{
        ID: spec.ID, Name: spec.Name, Content: spec.Content,
        ToolCall: spec.ToolCall, Delay: spec.Delay,
    }))
}
```

## Model categories

`VirtualModelType` (declared in `types.go`) tags every vmodel so the
extension UI and registry consumers can group by behavior:

| Type     | Meaning                                                                                             | Examples                                    |
| -------- | --------------------------------------------------------------------------------------------------- | ------------------------------------------- |
| `static` | Returns a fixed text response                                                                        | `virtual-claude-3`, `virtual-gpt-4`, `echo-model` |
| `tool`   | Returns a `tool_use` / `tool_calls` block                                                           | `ask-user-question`, `ask-confirmation`, `web-search-example` |
| `proxy`  | Applies a transform chain (no upstream call; same model also runs in real proxy paths)              | `compact-round-only`, `claude-code-compact`   |

## Error models

Error models are virtual models that always fail with a configurable error.
They enable testing of failover, resilience, and error handling without
requiring a real upstream provider or ad-hoc test servers.

### Error model categories

Error models are categorized by `ErrorCategory` (defined in `error_injection.go`):

| Category      | Description | HTTP Status | Retryable |
| ------------- | ----------- | ----------- | --------- |
| `rate_limit`  | Rate limiting (429) | 429 | Yes |
| `upstream`    | Upstream server errors (5xx) | 500, 502, 503, 504 | Yes |
| `timeout`      | Request timeout | 504 | Yes (pre-content), No (mid-stream) |
| `overloaded`   | Service overloaded | 503 | Yes |
| `invalid`      | Invalid request | 400 | No |
| `auth`         | Authentication/authorization | 401, 403 | No |
| `network`      | Network connectivity | Various | Yes (pre-content), No (mid-stream) |
| `malformed`    | Malformed response | Various | No |

### Error stages

Error models operate at two distinct stages:

1. **Pre-content** (`ErrorStagePreContent`): Failure before any response bytes are written
   - Handler returns HTTP error status immediately
   - No streaming starts
   - **Failover SHOULD retry** (firstChunkGate stays buffered)

2. **Mid-stream** (`ErrorStageMidStream`): Failure after streaming has started
   - Handler emits some content events, then fails
   - Two modes: connection close or error event
   - **Failover MUST NOT retry** (gate committed, bytes on the wire)

### Basic error models (4)

Registered in `SharedDefaultMocks()` (always available):

| Model ID | Stage | Status | Retryable | Use case |
| -------- | ----- | ------ | --------- | -------- |
| `virtual-fail-429` | Pre-content | 429 | Yes | Rate limit testing |
| `virtual-fail-500` | Pre-content | 500 | Yes | Upstream error testing |
| `virtual-fail-midstream-close` | Mid-stream | — | No | TCP disconnect testing |
| `virtual-fail-midstream-event` | Mid-stream | — | No | SSE error event testing |

### Extended error models (5)

Registered by `RegisterExtendedErrorMocks()`:

| Model ID | Stage | Status | Category | Use case |
| -------- | ----- | ------ | -------- | -------- |
| `virtual-fail-auth-401` | Pre-content | 401 | auth | Authentication failure |
| `virtual-fail-502` | Pre-content | 502 | upstream | Bad gateway |
| `virtual-fail-503` | Pre-content | 503 | overloaded | Service unavailable |
| `virtual-fail-400` | Pre-content | 400 | invalid | Invalid request |
| `virtual-fail-timeout` | Mid-stream | — | timeout | Mid-stream timeout |

### Using error models

```go
// In tests - basic error models are already available via SharedDefaultMocks
reg := anthropic.NewRegistry()
anthropic.RegisterDefaults(reg) // Includes virtual-fail-429, virtual-fail-500, etc.

// For extended error models (opt-in)
anthropic.RegisterExtendedErrorMocks(reg)

// In production (for demo/onboarding)
service := virtualserver.NewService(anthropicReg, openaiReg)
// Basic error models ARE included by default
// Extended error models require RegisterExtendedErrorMocks
```

### Error model naming convention

Error model IDs follow the pattern:
```
virtual-fail-{status}-{variant}
```

- `status`: HTTP status code (429, 500, 401, 502, 503, 400) or type (midstream-close, midstream-event, timeout)
- `variant`: Optional disambiguator (close, event, timeout, auth, etc.)

The stage (pre-content vs mid-stream) is implicit from the ErrorStage field, not the ID.

Examples:
- `virtual-fail-429` - Rate limit (pre-content)
- `virtual-fail-midstream-close` - Mid-stream connection close
- `virtual-fail-auth-401` - Authentication failure
- `virtual-fail-502` - Bad gateway error

## Default model allocation

| Model ID                | Anthropic registry | OpenAI registry |
| ----------------------- | :----------------: | :-------------: |
| `virtual-claude-3`      |         X          |                 |
| `virtual-gpt-4`         |                    |        X        |
| `echo-model`            |         X          |        X        |
| `ask-user-question`     |         X          |        X        |
| `ask-confirmation`      |         X          |        X        |
| `web-search-example`    |         X          |        X        |
| `compact-round-only`    |         X          |                 |
| `compact-round-files`   |         X          |                 |
| `claude-code-compact`   |         X          |                 |
| `claude-code-strategy`  |         X          |                 |

Compact transforms are Anthropic-only because they operate on the
Anthropic message shape. They could be ported to OpenAI by adding an
OpenAI-side `TransformModel`, but no production use case currently
calls for it.

The four shared mocks (`echo-model`, `ask-user-question`, `ask-confirmation`,
`web-search-example`) are defined once in `defaults_shared.go` and registered
by both `anthropic.RegisterDefaults` and `openai.RegisterDefaults`.

## Adding a model

### Single protocol

```go
reg := service.GetAnthropicRegistry()
_ = reg.Register(anthropic.NewMockModel(&anthropic.MockModelConfig{
    ID:      "my-mock",
    Name:    "My Mock",
    Content: "fixed reply",
    Delay:   50 * time.Millisecond,
}))
```

### Both protocols (same logical model)

Register two separate concrete instances under the same ID — one per
registry. They can share configuration but not state:

```go
cfg := myConfig{...}
_ = anthropicReg.Register(anthropic.NewMockModel(&anthropic.MockModelConfig{
    ID: "my-dual", Content: cfg.Reply,
}))
_ = openaiReg.Register(openai.NewMockModel(&openai.MockModelConfig{
    ID: "my-dual", Content: cfg.Reply,
}))
```

For richer dual-protocol models with shared logic, factor the logic
into a private core type and embed it in two thin wrappers — one in
each sub-package — that implement the respective `Handle*` methods.

### Custom (non-mock) model

Implement the relevant sub-interface directly. The sub-package must
own the type so it cannot accidentally implement the other protocol's
interface.

## Streaming

Each sub-package defines its own stream event types, used by the
`Handle*Stream` methods to emit deltas via the `emit func(any)` callback:

- `anthropic`: `StreamStartEvent`, `TextDeltaEvent`, `ToolUseEvent`, `DoneEvent`
- `openai`:    `DeltaEvent`, `ToolEvent`, `DoneEvent`

The virtual server (`virtualserver/handler.go`) translates these into the
wire-format SSE frames expected by each protocol.

`DefaultStream` in each sub-package converts a non-streaming `Handle*`
response into a stream event sequence using the shared `EmitChunks` helper,
so static and tool mocks get streaming for free.

## Error injection

A small facility lets a mock simulate an upstream failure without writing a
custom handler. It is **opt-in per model** — set `MockModelConfig.Error` (or
`MockScenario.Error`) to an `ErrorInjection`, and the virtual server handler
honors it. Models with no `Error` field set behave exactly as before.

```go
type ErrorInjection struct {
    Stage ErrorStage // ErrorStagePreContent or ErrorStageMidStream

    // Pre-content fields
    Status  int    // HTTP status (defaults to 500)
    Message string // rendered into the protocol-specific error envelope
    Type    string // defaults to "api_error"

    // Mid-stream fields
    AfterEvents   int           // emit N real events first (default 1)
    MidStreamMode MidStreamMode // ConnectionClose (TCP hijack) or ErrorEvent (SSE error frame)
}
```

The two stages correspond to two **distinct gateway paths** (and are exactly
the cases the priority-routing `firstChunkGate` must handle differently):

| Stage | Wire behavior | What it exercises |
|-------|---------------|-------------------|
| `ErrorStagePreContent` | Handler returns `Status` + protocol-shaped error envelope before any streaming starts. No SSE frames. | Gate stays buffered → retryable; failover MUST retry. |
| `ErrorStageMidStream`  | Handler writes 200 + headers + `AfterEvents` real chunks, then either hijacks and closes the TCP connection or emits a final SSE error frame. | Gate already committed → bytes on the wire; failover MUST NOT retry. |

### Architecture: model declares, handler enforces

Mock stream loops are kept **gate-free**: `DefaultStream` and
`MockModel.Handle*Stream` simply emit every event they would normally emit.
The virtualserver handler owns the mid-stream cutoff:

1. Before invoking `Handle*Stream`, the handler asks the model whether it
   implements `ErrorInjectingModel` and configures an injection.
2. If a mid-stream injection is configured, the handler wraps the model's
   `emit` callback in a counting gate. After `AfterEvents` events have been
   admitted, subsequent events (including terminal `DoneEvent` /
   `UsageEvent`) are silently dropped.
3. Once the model's stream loop returns, the handler applies the configured
   `MidStreamMode` (`hijackAndClose` or `applyMidStreamBreak*`).

For pre-content injection there's no gate — the handler short-circuits with
`writePreContentError{OpenAI,Anthropic}` before dispatching to the model at
all.

This split keeps the failure-injection surface narrow (one small facility,
isolated to the handler) and leaves the common mock path uncluttered.

### Pre-registered fail mocks

Basic error models (429, 500, midstream-close, midstream-event) are included in
`SharedDefaultMocks()` and registered by default via `RegisterDefaults()`.
Extended error models require opt-in registration via `RegisterExtendedErrorMocks()`.

| Model ID | Behavior |
|----------|----------|
| `virtual-fail-429`  | HTTP 429 + `rate_limit_error` envelope (retryable) |
| `virtual-fail-500`  | HTTP 500 + `api_error` envelope (retryable) |
| `virtual-fail-midstream-close` | One real chunk then TCP close (not retryable) |
| `virtual-fail-midstream-event` | One real chunk then SSE error frame (not retryable) |

Failover e2e tests (`internal/protocoltest`) use these directly via
`SetupFailoverRoute(... primaryFailModel: pt.FailMockPreContent429)` instead
of standing up ad-hoc `httptest.Server` instances.

## Benchmarking (`benchmark/`)

`benchmark.NewLocalServer()` boots a `virtualserver.Service` with the
default registries as an in-process HTTP server. `BenchmarkClient` drives
load against any HTTP endpoint that speaks the virtual server API.

```go
srv := benchmark.NewLocalServer()
defer srv.Close()

client := benchmark.NewBenchmarkClient(srv.URL())
result, _ := client.RunChatBenchmark(ctx, benchmark.BenchmarkConfig{
    Concurrency: 10,
    Requests:    100,
})
fmt.Printf("TPS: %.1f  p99: %v\n", result.TPS, result.P99Latency)
```

See `benchmark/examples/` for runnable server and client programs.

## Related packages

- `vmodel/virtualserver` — Production Gin HTTP handler, routes,
  request/response shaping. Owns the v1 → beta lift for Anthropic.
- `internal/protocoltest` — Test-only consumer that **reuses**
  `GenericRegistry[Scenario]` as a primitive (its `Scenario` type satisfies
  `vmodel.VirtualModel`). Serves pre-rendered byte/SSE payloads for
  wire-format protocol testing. It does **not** inherit production defaults
  from `RegisterDefaults`; it owns its own registry of test fixtures.
- `internal/protocol/transform` — Transform chain types used by
  `anthropic.TransformModel` (e.g. compact-round-only).
- `internal/smart_compact` — Concrete transform implementations.
