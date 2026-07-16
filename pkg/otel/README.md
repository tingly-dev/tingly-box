# pkg/otel - OpenTelemetry Metrics for Tingly-Box

Package otel wires OpenTelemetry metrics for LLM token usage.

## Design

Metrics have exactly **one egress**: an optional OTLP endpoint.

- **Aggregated metrics** (token counters, request counts, duration histogram)
  answer "how much / how fast" and leave the process only via OTLP.
- **Per-request artifacts** are written at the source, not derived from
  metrics: usage records by `internal/server/usage_tracking.go` (straight to
  the SQLite usage store), request recordings by the recording pipeline
  (`internal/server/recording` → `internal/obs.Sink`).

Reconstructing per-request records from aggregated metric data points is
lossy (values, request ids and timing are gone) and, with cumulative
temporality, re-emits every known series each export cycle. Earlier versions
of this package did exactly that via SQLite/Sink "exporters"; both have been
removed.

When OTLP is not configured (the default), the meter provider is created
**without a reader** — instrument calls are near-free no-ops and nothing is
exported anywhere.

## Package Structure

```
pkg/otel/
├── config.go             # Config / OTLPConfig
├── meter.go              # MeterSetup initialization and lifecycle
├── tracker/
│   └── token_tracker.go  # TokenTracker for recording token usage
└── exporter/
    └── otlp.go           # OTLP exporter (gRPC/HTTP)
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
}

setup, err := otel.NewMeterSetup(context.Background(), cfg)
if err != nil {
    // handle error
}
if setup != nil {
    defer setup.Shutdown(context.Background())
    tracker := setup.Tracker()
    _ = tracker // pass to the request pipeline; call RecordUsage per request
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

## Cardinality rules

Every distinct attribute set becomes a data point the cumulative SDK retains
for the lifetime of the process. Two rules keep that bounded (see #1255):

- **Never attach near-unique values as attributes** (latency, request ids,
  raw error text). Latency is the histogram *value*; error codes are capped
  at 64 bytes.
- **Detach attribute strings from request buffers** — `RecordUsage` clones
  model / request-model / error-code strings so a retained attribute cannot
  pin a multi-megabyte parsed request body.
