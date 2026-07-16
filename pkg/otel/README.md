# pkg/otel - OpenTelemetry Metrics & Traces for Tingly-Box

Package otel wires OpenTelemetry metrics and traces for LLM requests.

## Design

Telemetry has exactly **one egress**: an optional OTLP endpoint, shared by
all signals.

- **Aggregated metrics** (token counters, request counts, duration histogram)
  answer "how much / how fast".
- **Traces** answer "what happened inside this request" and propagate across
  the gateway hop via W3C `traceparent` (propagator installed when tracing
  is on).
- **Per-request artifacts** are written at the source, not derived from
  metrics: usage records by `internal/server/usage_tracking.go` (straight to
  the SQLite usage store), request recordings by the recording pipeline
  (`internal/server/recording` → `internal/obs.Sink`).

Reconstructing per-request records from aggregated metric data points is
lossy (values, request ids and timing are gone) and, with cumulative
temporality, re-emits every known series each export cycle. Earlier versions
of this package did exactly that via SQLite/Sink "exporters"; both have been
removed.

When OTLP is not configured (the default), the meter provider has **no
reader** and **no tracer provider is installed**: instrument calls are
near-free no-ops and spans are never sampled (`IsRecording() == false`), so
instrumentation can stay in place unconditionally. A provider is only
constructed when it has somewhere to send data — never the
"record-then-drop" middle ground.

## Package Structure

```
pkg/otel/
├── config.go             # Config / OTLPConfig (incl. TraceSampleRatio)
├── setup.go              # Setup: providers, propagator, lifecycle
├── tracer.go             # Tracer helper (request spans, events, errors)
├── attributes.go         # Semantic convention attribute keys
├── tracker/
│   └── token_tracker.go  # TokenTracker for recording token usage
└── exporter/
    ├── otlp.go           # OTLP metrics exporter (gRPC/HTTP)
    └── otlp_trace.go     # OTLP trace exporter (gRPC/HTTP)
```

## Usage

```go
import (
    "context"

    "github.com/tingly-dev/tingly-box/pkg/otel"
)

cfg := otel.DefaultConfig()
cfg.OTLP = otel.OTLPConfig{
    Enabled:  true,
    Endpoint: "localhost:4317",
    Protocol: "grpc", // or "http/protobuf"
    Insecure: true,
    // TraceSampleRatio: 0.1, // sample 10% of new traces; 0 = everything
}

setup, err := otel.NewSetup(context.Background(), cfg)
if err != nil {
    // handle error
}
if setup != nil {
    defer setup.Shutdown(context.Background())

    tracker := setup.Tracker() // call RecordUsage per request
    tracer := setup.Tracer()   // always non-nil; no-op when OTLP is off

    ctx, span := tracer.StartRequestSpan(ctx, provider, model, scenario)
    defer func() { tracer.EndSpan(span, err) }()
}
```

## Metrics — OTel GenAI semantic conventions

This package emits the standard GenAI client metrics
(https://github.com/open-telemetry/semantic-conventions-genai, Development
status; adopted wholesale before any consumer existed, so there is no legacy
`llm.*` namespace to migrate):

| Instrument                        | Type      | Unit    |
|-----------------------------------|-----------|---------|
| `gen_ai.client.token.usage`       | histogram | {token} |
| `gen_ai.client.operation.duration`| histogram | s       |

There are deliberately no separate request/error counters: the duration
histogram's count **is** the request count, and failures are classified by
the `error.type` attribute on it — that is the standard shape.

Standard attributes: `gen_ai.operation.name` (default `chat`),
`gen_ai.provider.name`, `gen_ai.request.model`, `gen_ai.response.model`,
`gen_ai.token.type` (`input` / `output`, extended with `cache_read` /
`system`), `error.type` on failures.

Gateway-specific dimensions stay in their own namespace: `tingly.scenario`,
`tingly.provider.uuid`, `tingly.rule.uuid`, `tingly.streaming`,
`tingly.user.tier`.

## Traces

- Spans batch through `sdktrace.WithBatcher` to the same OTLP endpoint as
  metrics.
- Sampling is parent-based: an incoming sampled `traceparent` is always
  honored; new traces sample at `TraceSampleRatio` (default: everything).
- Inference spans follow the GenAI convention: named
  `"{operation} {request model}"` (e.g. `chat gpt-4`), kind CLIENT, with
  `gen_ai.operation.name` / `gen_ai.provider.name` / `gen_ai.request.model`
  and `gen_ai.usage.input_tokens` / `gen_ai.usage.output_tokens` set via
  `Tracer.SetTokenUsage`.
- `Tracer` provides `StartRequestSpan`, `SetTokenUsage`, `RecordError`,
  `EndSpan`.

## Cardinality rules

Every distinct attribute set becomes a data point the cumulative SDK retains
for the lifetime of the process. Two rules keep that bounded (see #1255):

- **Never attach near-unique values as metric attributes** (latency, request
  ids, raw error text). Latency is the histogram *value*; error codes are
  capped at 64 bytes. (Spans are different: per-request values belong on
  spans, which are exported and released, not retained.)
- **Detach attribute strings from request buffers** — `RecordUsage` clones
  model / request-model / error-code strings so a retained attribute cannot
  pin a multi-megabyte parsed request body.
