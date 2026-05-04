# `internal/virtualmodel`

Virtual models — synthetic, in-process model implementations used for
testing, request shaping, and tool simulation without calling a real
upstream provider. They are wired into the virtual server at
`internal/virtualserver` and exposed under `/virtual/v1/...`.

## Layout

```
internal/virtualmodel/
├── interface.go        // base VirtualModel interface (provider-neutral)
├── types.go            // VirtualModelType, Model, ToolCallConfig, helpers
├── README.md
├── anthropic/          // Anthropic-protocol models + Registry
└── openai/             // OpenAI Chat-protocol models + Registry
```

The root package contains only provider-neutral primitives. Concrete
models, protocol-specific request/response types, stream events, and
the `Registry` itself live in the `anthropic` and `openai`
sub-packages. The two sub-packages do **not** import each other.

## Design

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
the HTTP boundary (`internal/virtualserver/handler.go`). Vmodels see
exactly one request type — `*protocol.AnthropicBetaMessagesRequest` —
so the protocol-version distinction does not leak into the
`VirtualModel` interface.

## Interfaces

### Base (`internal/virtualmodel/interface.go`)

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

## Model categories

`VirtualModelType` (declared in `types.go`) tags every vmodel so the
extension UI and registry consumers can group by behavior:

| Type     | Meaning                                               | Examples                                    |
| -------- | ----------------------------------------------------- | ------------------------------------------- |
| `static` | Returns a fixed text response                          | `virtual-claude-3`, `virtual-gpt-4`, `echo-model` |
| `tool`   | Returns a `tool_use` / `tool_calls` block              | `ask-user-question`, `ask-confirmation`, `web-search-example` |
| `proxy`  | Applies a transform chain (no upstream call here, but the same model also runs in real proxy paths) | `compact-thinking`, `claude-code-compact`   |

## Default model allocation

| Model ID                | Anthropic registry | OpenAI registry |
| ----------------------- | :----------------: | :-------------: |
| `virtual-claude-3`      |         X          |                 |
| `virtual-gpt-4`         |                    |        X        |
| `echo-model`            |         X          |        X        |
| `ask-user-question`     |         X          |        X        |
| `ask-confirmation`      |         X          |        X        |
| `web-search-example`    |         X          |        X        |
| `compact-thinking`      |         X          |                 |
| `compact-round-only`    |         X          |                 |
| `compact-round-files`   |         X          |                 |
| `claude-code-compact`   |         X          |                 |
| `claude-code-strategy`  |         X          |                 |

Compact transforms are Anthropic-only because they operate on the
Anthropic message shape. They could be ported to OpenAI by adding an
OpenAI-side `TransformModel`, but no production use case currently
calls for it.

## Adding a model

### Single protocol

```go
// in your wiring code
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
`Handle*Stream` methods to emit deltas via the `emit func(any)`
callback:

- `anthropic`: `StreamStartEvent`, `TextDeltaEvent`, `ToolUseEvent`, `DoneEvent`
- `openai`:    `DeltaEvent`, `ToolEvent`, `DoneEvent`

The virtual server (`internal/virtualserver/handler.go`) translates
these into the wire-format SSE frames expected by each protocol.

A `DefaultStream` helper in each sub-package converts a non-streaming
`Handle*` response into a sequence of stream events, so static and tool
mocks get streaming for free.

## Related packages

- `internal/virtualserver` — HTTP handler, routes, request/response
  shaping. Owns the v1 → beta lift for Anthropic.
- `internal/extension` — exposes registered vmodels as extension items
  with a `provider` metadata key for the UI split.
- `internal/protocol/transform` — transform chain types used by
  `anthropic.TransformModel` (e.g. compact-thinking).
- `internal/smart_compact` — concrete transform implementations.
