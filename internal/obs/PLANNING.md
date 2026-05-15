# obs Package Planning

Living planning doc for the `internal/obs` recording subsystem.

## Recording Phase 2 — Pipeline-Driven Design

Status: draft / proposal. No code yet. This section supersedes the
post-cleanup state delivered in phase 1.

### Why a phase 2

Phase 1 collapsed the duplicated `ScenarioRecorder` / `ProtocolRecorder`
pair, dropped dead types in `internal/obs`, and merged the two transform
recorders. The remaining shape is still recorder-object oriented:

- Handlers construct a `*ProtocolRecorder` early (`BeginProtocolRecording`)
  and may upgrade it later with provider/model/mode
  (`EnsureProtocolRecorder`).
- The recorder doubles as: (a) a mutable carrier of staged request/response
  data, (b) the stream chunk log, (c) the mode-aware emitter.
- The transform pipeline writes to the recorder through a side channel; the
  pipeline itself has no notion of recording.
- The `protocol/stream.StreamEventRecorder` interface is a 1-method back
  door from protocol back into server, so that protocol code can feed raw
  map events into the server-owned recorder.

These three roles in one type are why the call graph fans out into so many
helpers (`NewRecorderHooks`, `NewTransformRecorder`, `streamRecorder`,
ad-hoc `recorder.RecordError(...)` calls in handlers) and why every new
recording shape (new mode, new stage, new exporter) keeps touching `server`.

### Goal

Recording becomes a pipeline concern, not a server concern. After phase 2:

- `internal/server` owns **no** `*Recorder` type. Handlers do not call
  `RecordError` / `RecordResponse` directly.
- `internal/obs` owns the canonical record model and the emitter.
- `internal/protocol/transform` owns the pipeline carrier for in-flight
  request/response state and exposes a small observer point so that
  recording is the same as any other pipeline observer (metrics, tracing,
  guardrails).
- Mode (`RequestOnly`, `RequestResponse`, `Staged*`, `All`) is enforced at
  the **exporter** layer, not at the recorder. The hot path always builds
  the full record; trimming happens at egress.

### Target topology

```
       handler (gin)
            │ creates RecordCtx, attaches to transform.TransformContext
            ▼
  transform.Chain  ─ tap ─►  obs.Recorder (built-in observer)
            │                       │ on transform start/end:
            │                       │   - snapshot original/transformed req
            ▼                       │
       provider                     │
            │ stream events ───────►│ assembler + chunk log
            │ final response ──────►│ set final response
            ▼                       │
   handler completes / errors ─────►│ emit *obs.Record
                                    ▼
                          obs.Sink (mode-filtering exporter chain)
```

Key:

- `RecordCtx`: a small struct embedded in `transform.TransformContext`,
  holding session/scenario/provider/model plus pointers to original /
  transformed / provider / final + a chunk log. Replaces the
  `*ProtocolRecorder` field threaded through handlers.
- `obs.Recorder`: a built-in pipeline observer in `internal/obs`. Subscribes
  to `transform.Chain` lifecycle events plus a stream tap. Encapsulates the
  current `protocol_recording.go` logic without exposing a public type.
- Mode-filtering exporter: a wrapper exporter that strips fields per mode,
  so `Sink` callers don’t need to know about modes.
- Stream tap: `protocol/stream.EventTap` interface (1 method
  `OnEvent(eventType string, raw any)`). `obs.Recorder` implements it;
  protocol-side code calls `tap.OnEvent(...)` instead of asserting on a
  server-defined recorder type. Reverses the current `StreamEventRecorder`
  coupling direction (protocol no longer depends on server).

### Public API after phase 2

In `internal/obs`:

```go
type Recorder struct { /* unexported */ }
type RecorderOption func(*Recorder)

func NewRecorder(sink *Sink, opts ...RecorderOption) *Recorder
func (r *Recorder) Attach(ctx *transform.TransformContext)
func (r *Recorder) BindProvider(provider, model string)
func (r *Recorder) Tap() stream.EventTap
func (r *Recorder) Finish(err error)  // emits *Record
```

In `internal/protocol/transform`:

```go
type TransformContext struct {
    // ... existing fields ...
    RecordCtx *RecordCtx // nil when recording disabled
}

type RecordCtx struct {
    Scenario           string
    Provider, Model    string
    SessionID          string
    OriginalRequest    *obs.RecordRequest
    TransformedRequest *obs.RecordRequest
    ProviderResponse   *obs.RecordResponse
    FinalResponse      *obs.RecordResponse
    Steps              []string
    Started            time.Time
}
```

In `internal/protocol/stream`:

```go
type EventTap interface {
    OnEvent(eventType string, raw any)
}

// Existing StreamEventRecorder removed.
```

Handlers shrink to:

```go
rec := obs.NewRecorder(sink, obs.WithScenario(scenario))
defer rec.Finish(retErr)  // emits *Record
rec.Attach(transformCtx)
hc.WithEventTap(rec.Tap())
```

No `RecordError`, no `RecordResponse`, no explicit `SetAssembledResponse`
from handler code — the recorder reads everything it needs off the
pipeline.

### Mode handling — at the exporter

`internal/obs` gains a `ModeFilterExporter`:

```go
func NewModeFilterExporter(inner RecordExporter, mode RecordMode) RecordExporter
```

It strips fields per mode before delegating:

| Mode                          | OriginalReq | TransformedReq | ProviderResp | FinalResp |
| ----------------------------- | ----------- | -------------- | ------------ | --------- |
| `All` / `Scenario`            | ✓           | ✓              | ✓            | ✓         |
| `RequestOnly`                 |             | ✓              |              |           |
| `RequestResponse`             |             | ✓              |              | ✓         |
| `StagedRequestResponse`       | ✓           | ✓              |              | ✓         |

`NewSink(baseDir, mode, ...)` becomes the place that wraps the underlying
exporters with this filter. The recorder always sets every field it has;
trimming happens once, at egress.

### Migration plan (sequential, each step ships independently)

1. **Extract `RecordCtx` into `transform.TransformContext`.** Server still
   owns `*ProtocolRecorder`, but it writes into `ctx.RecordCtx` instead of
   its own fields. Transform recorders go away; the base transform updates
   `RecordCtx.TransformedRequest` itself.
2. **Introduce `obs.Recorder` as a thin façade.** It wraps the existing
   `ProtocolRecorder` initially. Handlers switch their import to
   `obs.Recorder`.
3. **Move `streamRecorder` + `NewRecorderHooks` into `obs.Recorder`.**
   Hook construction becomes `rec.HandleContextHooks()`. Server-side
   `scenario_recording.go` shrinks to nothing.
4. **Add `stream.EventTap`, remove `stream.StreamEventRecorder`.**
   Protocol no longer depends on server.
5. **Add `ModeFilterExporter`, simplify `ProtocolRecorder.emit` to always
   build the full record.** Drop mode awareness from the recorder.
6. **Delete `ProtocolRecorder` from `internal/server`.** Only `obs.Recorder`
   remains.

Each step is mechanical, vet- and test-green on its own, and reviewable
without holding the full design in your head.

### Out of scope (phase 3+)

- A separate exporter for OTLP / remote collector. Current `RecordExporter`
  interface already supports it; phase 2 doesn’t need to land it.
- Replay tooling on top of the CAS exporter.
- Cross-session correlation (already partially supported via `SessionID`).

### Open questions

- Should `RecordCtx` live in `transform.TransformContext` or in a separate
  context carrier (e.g. `context.Context` value)? The pipeline already
  passes `*TransformContext` end-to-end so embedding is cheaper, but it
  fuses recording onto the transform package. Recommend embedding —
  any future "context carrier" abstraction can lift it later.
- `obs.Recorder` reads `gin.Context` today for response status/headers and
  `response_body`. After phase 2, who owns that read? Options: (a) keep gin
  coupling in `obs`, (b) plumb status/headers through `RecordCtx`. Lean
  toward (b) — it lets `obs` stay framework-agnostic.
- Where do the per-handler "synthesise final response from chunks"
  fallbacks live? Probably on `obs.Recorder.Finish`, since it already owns
  the assembler.
