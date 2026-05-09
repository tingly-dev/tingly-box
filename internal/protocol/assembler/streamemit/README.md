# streamemit

Decoupled emission layer on top of the Anthropic stream assemblers in
`internal/protocol/assembler`.

## Why this package exists

The provider-side stream from Anthropic interleaves several kinds of
content blocks — `text`, `thinking`, `tool_use`, and various tool-result
variants — inside one event sequence. Today the passthrough handler in
`internal/protocol/stream/anthropic_passthrough.go` forwards each event
to the SSE consumer the moment it arrives:

```
provider ──► ProcessStream ──► SSEvent ──► consumer
```

That is fine when the consumer is happy to render partial content as it
arrives. It breaks down for two real use cases:

1. **Independent visibility into tool vs. message assembly.** Recording,
   auditing, and routing code want to ask "have I finished the tool
   call yet?" or "what text has the model produced so far?" without
   reimplementing the per-block state machine each time.
2. **Atomic tool-call delivery.** Some downstream consumers cannot act
   on a half-formed tool call — they need the whole `tool_use` block
   (id, name, fully-parsed input) before any of it reaches them, while
   text and thinking continue to stream live in parallel.

The buffer-then-decide pattern that solves (2) already lives in
`internal/guardrails/mutate/anthropic_stream.go`, but it is wired
specifically into the guardrails rewrite path. `streamemit` generalizes
that pattern into a reusable layer so non-guardrails callers can
compose the same shape, and so guardrails itself can eventually
collapse onto it.

## Design

### One emitter, two responsibilities

A `StreamEmitter` does two things every time `Feed` is called:

1. **Always feed the inner `*assembler.AnthropicStreamAssembler`** so
   `MessageAssembler()` and `Finish()` reflect every event the emitter
   has seen, regardless of any buffering decisions.
2. **Decide what to release right now.** Per-kind `EmissionPolicy`
   selects between `EmitImmediate` (1:1 with arrival) and
   `EmitOnComplete` (buffer the entire content block, flush as one
   ordered slice on `content_block_stop`).

Both responsibilities are independent: the assembled `*anthropic.Message`
returned by `Finish` is correct whether or not anything was buffered.

### Routing

```
                  ┌─────────────────────────────────────────────┐
                  │                StreamEmitter                │
                  │                                             │
 evt ─► Feed ─►   │   ┌─ inner *AnthropicStreamAssembler        │
                  │   │     (RecordV1Event / RecordV1BetaEvent) │
                  │   │                                         │
                  │   ├─ kinds[index]    ── learned at          │
                  │   │                     content_block_start │
                  │   │                                         │
                  │   ├─ toolBufs[index] ── only when policy    │
                  │   │                     for that kind is    │
                  │   │                     EmitOnComplete      │
                  │   │                                         │
                  │   └─ OnToolBlockComplete hook ── invoked at │
                  │                                  cb_stop    │
                  │                                  on flush   │
                  └─────────────────────────────────────────────┘
                              │
                              ▼
                    []BufferedEvent  (zero, one, or many — caller
                                      forwards each to the consumer)
```

`message_start`, `message_delta`, `message_stop` are always immediate.
`content_block_start` learns the block kind, opens a buffer if needed,
and either holds or releases the start event. `content_block_delta`
routes by the previously-recorded kind. `content_block_stop` either
flushes a buffered block (running the optional decision hook first) or
passes through.

### Emission timeline

```
events arriving:   ms_start  cb_start(text)  txt_d  txt_d  cb_stop  cb_start(tool)  json_d  json_d  cb_stop  ms_delta  ms_stop
                       │           │           │      │       │            │           │       │       │         │         │
emitter.Feed   ───►   1ev         1ev         1ev    1ev     1ev          0ev         0ev     0ev    ┌─4ev─┐    1ev      1ev
                       ▼           ▼           ▼      ▼       ▼            ▼           ▼       ▼     ▼     ▼     ▼         ▼
 SSE wire:           ms_start    cb_start(t)  d      d     cb_stop      ░░░░░░░░░  ░░░░░░  ░░░░░░  cb_start(tool)  ms_delta  ms_stop
                                                                                                   json_d
                                                                                                   json_d
                                                                                                   cb_stop
                                                                  ▲
                                                                  │
                                                          held in toolBufs[1] until
                                                          cb_stop, then drained as
                                                          one ordered slice
```

Text and thinking flow live; the tool block goes silent on the wire and
re-emerges as a contiguous burst at `content_block_stop`.

### `BufferedEvent` is a type alias

```go
type BufferedEvent = protocol.GuardrailsBufferedEvent
```

The alias (not a named type) means the output of `streamemit` and the
output of the guardrails rewriter are interchangeable. A handler can
chain them, a slice from one can be appended to a slice from the
other, and the existing `sendAnthropicStreamEvent` consumes both
without translation.

### Hook composition

`Config.OnToolBlockComplete` is the integration point for tool-level
post-processing — credential masking, allow/deny verdicts, content
rewriting:

```
            ┌──────────────────────────┐
            │ OnToolBlockComplete      │
            │   ├─ allow  → nil        │   (flush buffered as-is)
            │   ├─ rewrite→ Replace[]  │   (emit replacement instead)
            │   └─ drop   → Drop:true  │   (emit nothing)
            └──────────────────────────┘
```

This is the same decision shape as the guardrails
`AnthropicToolUseDecision`; an adapter is one screen of code.

### Scope

- **In:** Anthropic v1 (`MessageStreamEventUnion`) and v1beta
  (`BetaRawMessageStreamEventUnion`).
- **Out:** OpenAI Chat, OpenAI Responses, Google streams. Those have
  their own assemblers; analogous emitters can be added later.
- **One version per emitter.** Mixing v1 and v1beta events on a single
  emitter returns `ErrMixedVersions`. Use one emitter per request.

## Usage

### Pass-through (default — same behavior as today's handler)

```go
e := streamemit.New(streamemit.Config{}) // zero value: emit everything immediately

for streamResp.Next() {
    evt := streamResp.Current()
    out, err := e.FeedV1(&evt)
    if err != nil {
        return err
    }
    for _, ev := range out {
        sendAnthropicStreamEvent(c, ev.EventType, ev.Payload, c.Writer)
    }
}

_, msg := e.Finish(model, inputTokens, outputTokens)
// `msg` is *anthropic.Message reflecting the full stream.
```

### Hold tool calls until complete

```go
e := streamemit.New(streamemit.Config{
    ToolPolicy: streamemit.EmitOnComplete,
})

for streamResp.Next() {
    evt := streamResp.Current()
    out, _ := e.FeedV1(&evt)
    for _, ev := range out {
        sendAnthropicStreamEvent(c, ev.EventType, ev.Payload, c.Writer)
    }
}

// On error or client disconnect, salvage anything still buffered:
//   pending := e.Drain()
//   for _, ev := range pending { ... }
```

Text and thinking events still reach the consumer the moment they
arrive. Each tool block is silent on the wire from `content_block_start`
through every `input_json_delta`, then bursts out as a single ordered
slice at `content_block_stop`.

### Compose with a verdict hook

```go
e := streamemit.New(streamemit.Config{
    ToolPolicy: streamemit.EmitOnComplete,
    OnToolBlockComplete: func(toolID string, idx int, buffered []streamemit.BufferedEvent) (*streamemit.ToolDecision, error) {
        verdict, err := guardrails.Inspect(toolID, buffered)
        if err != nil {
            return nil, err
        }
        switch verdict.Kind {
        case guardrails.Allow:
            return nil, nil                              // flush as-is
        case guardrails.Block:
            return &streamemit.ToolDecision{Replace: verdict.ErrorEvents}, nil
        case guardrails.Drop:
            return &streamemit.ToolDecision{Drop: true}, nil
        }
        return nil, nil
    },
})
```

### Inspect each side independently

```go
// Read-only access to the inner state machine; safe to call at any time.
asm := e.MessageAssembler()
_ = asm // e.g. inspect blocks accumulated so far

// Snapshot the events buffered for a specific block index.
if buf, ok := e.ToolBuffer(1); ok {
    fmt.Println("tool block 1 has", len(buf), "events buffered so far")
}
```

## Public API surface

```go
// Construction
func New(cfg Config) *StreamEmitter

// Feeding
func (e *StreamEmitter) Feed(event interface{}) ([]BufferedEvent, error)
func (e *StreamEmitter) FeedV1(*anthropic.MessageStreamEventUnion) ([]BufferedEvent, error)
func (e *StreamEmitter) FeedV1Beta(*anthropic.BetaRawMessageStreamEventUnion) ([]BufferedEvent, error)

// Inspection
func (e *StreamEmitter) MessageAssembler() *assembler.AnthropicStreamAssembler
func (e *StreamEmitter) ToolBuffer(index int) ([]BufferedEvent, bool)

// Termination
func (e *StreamEmitter) Drain() []BufferedEvent
func (e *StreamEmitter) Finish(model string, inputTokens, outputTokens int) ([]BufferedEvent, *anthropic.Message)

// Sentinels
var ErrMixedVersions = errors.New(...)

// Config
type Config struct {
    TextPolicy          EmissionPolicy
    ThinkingPolicy      EmissionPolicy
    ToolPolicy          EmissionPolicy
    OnToolBlockComplete func(toolID string, index int, buffered []BufferedEvent) (*ToolDecision, error)
}

type EmissionPolicy int
const (
    EmitImmediate EmissionPolicy = iota
    EmitOnComplete
)

type ToolDecision struct {
    Replace []BufferedEvent
    Drop    bool
}

type BufferedEvent = protocol.GuardrailsBufferedEvent
```

## Design notes and caveats

- **The emitter owns the inner assembler.** Callers that today drive
  their own `AnthropicStreamAssembler` in parallel (e.g.
  `internal/server/scenario_recording.go`) should pick one or the other
  to avoid double-feeding. `MessageAssembler()` is the bridge for
  callers that want the assembled message without a second instance.
- **`Drain()` does not call `OnToolBlockComplete`.** It is the salvage
  path for error/cancel handling, not a completion signal. If the hook
  must run for partial tool blocks too, the caller should invoke it
  explicitly before calling `Drain`.
- **`MessageAssembler()` is read-oriented.** Feeding events into the
  returned assembler directly will desync it from the emitter's
  routing state. Use `Feed*` only.
- **`Drop` and `Replace` are independent.** If both are set, `Drop`
  wins. A nil `*ToolDecision` flushes the buffered events unchanged.
- **Thinking blocks under `EmitOnComplete`.** Mechanically supported
  (the same buffer machinery is reused) but no caller exercises this
  yet. Treat as latent capability.

## Migration plan

This package landed library-only. The handler-side migration happens
in follow-ups:

1. Swap `anthropic_passthrough.go` to drive a `StreamEmitter` with
   `Config{}` (byte-identical default behavior).
2. Move the guardrails rewriter behind `OnToolBlockComplete`; delete
   `GuardrailsStreamState.AnthropicToolEvents` /
   `AnthropicToolIDs`.
3. Expose a per-request opt-in (header or `HandleContext` flag) that
   flips `ToolPolicy` to `EmitOnComplete` for consumers that prefer
   atomic tool delivery.
4. Optionally retire the inline state machine in
   `internal/protocol/stream/openai_to_anthropic.go::streamState` by
   having the conversion handler emit Anthropic events into a
   `StreamEmitter`.

## Tests

`go test ./internal/protocol/assembler/streamemit/...` covers:

- text-only streams (v1 + v1beta) with `EmitImmediate`
- tool-only streams (v1 + v1beta) with `EmitOnComplete`
- mixed text + tool with text live, tool buffered
- ordering invariant: a tool flush returns only the tool's events,
  even when text deltas on a different block were already emitted
- `Drain()` flushing an unclosed tool buffer
- `Finish()` returning pending events plus an assembled message
- `OnToolBlockComplete` `Replace` / `Drop` / error propagation
- `ErrMixedVersions` when v1 and v1beta are fed to the same emitter
- byte-compatibility with `protocol.GuardrailsBufferedEvent`
