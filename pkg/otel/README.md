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

## Metrics

| Instrument               | Type      | Unit      |
|--------------------------|-----------|-----------|
| `llm.token.usage.input`  | counter   | {token}   |
| `llm.token.usage.output` | counter   | {token}   |
| `llm.token.total`        | counter   | {token}   |
| `llm.token.cache.input`  | counter   | {token}   |
| `llm.token.system`       | counter   | {token}   |
| `llm.request.count`      | counter   | {request} |
| `llm.request.duration`   | histogram | ms        |
| `llm.request.errors`     | counter   | {error}   |

Attributes: `llm.provider`, `llm.provider.uuid`, `llm.model`,
`llm.request.model`, `llm.scenario`, `llm.streaming`, `llm.response.status`,
plus `llm.rule.uuid` / `llm.user.tier` / `llm.error.code` when set.

## Traces

- Spans batch through `sdktrace.WithBatcher` to the same OTLP endpoint as
  metrics.
- Sampling is parent-based: an incoming sampled `traceparent` is always
  honored; new traces sample at `TraceSampleRatio` (default: everything).
- `Tracer` provides `StartRequestSpan` (standard llm.* attributes),
  `RecordTokenUsageEvent`, `RecordError`, `EndSpan`.

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
