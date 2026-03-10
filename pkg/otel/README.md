# pkg/otel - OpenTelemetry Observability for Tingly-Box

Package otel provides OpenTelemetry-based observability for LLM token usage in tingly-box.

## Features

- **Token Usage Metrics**: Track input/output tokens, request counts, latency, and errors
- **Multi-Exporter Support**: Export to SQLite, OTLP backends, and JSONL files simultaneously
- **Distributed Tracing**: Trace request lifecycle with token usage events
- **Semantic Conventions**: Follows OpenLLMetry attribute naming conventions

## Package Structure

```
pkg/otel/
├── attributes.go         # Semantic convention attribute keys
├── config.go             # Configuration types (Config, SQLiteConfig, OTLPConfig, SinkConfig)
├── meter.go              # MeterSetup initialization and lifecycle
├── tracer.go             # Distributed tracing support
├── tracker/
│   └── token_tracker.go  # TokenTracker for recording token usage
└── exporter/
    ├── multi.go          # MultiExporter for multiple backends
    ├── sqlite.go         # SQLite exporter
    ├── otlp.go           # OTLP exporter (gRPC/HTTP)
    ├── sink.go           # JSONL file sink exporter
    └── util.go           # Utility functions
```

## Usage

### Basic Setup

```go
import (
    "context"
    "github.com/tingly-dev/tingly-box/pkg/otel"
    "github.com/tingly-dev/tingly-box/pkg/otel/tracker"
)

// Create configuration
cfg := &otel.Config{
    Enabled:        true,
    ExportInterval: 10 * time.Second,
    ExportTimeout:  30 * time.Second,
    SQLite: otel.SQLiteConfig{Enabled: true},
    Sink:   otel.SinkConfig{Enabled: true},
    OTLP:   otel.OTLPConfig{Enabled: false},
}

// Initialize meter setup
setup, err := otel.NewMeterSetup(ctx, cfg, &otel.StoreRefs{
    StatsStore: statsStore,
    UsageStore: usageStore,
    Sink:       sink,
})
if err != nil {
    // handle error
}
defer setup.Shutdown(ctx)

// Get token tracker
tracker := setup.Tracker()

// Record token usage
tracker.RecordUsage(ctx, tracker.UsageOptions{
    Provider:     "openai",
    ProviderUUID: "uuid-123",
    Model:        "gpt-4",
    RequestModel: "gpt-4",
    Scenario:     "openai",
    InputTokens:  100,
    OutputTokens: 50,
    Streamed:     true,
    Status:       "success",
    LatencyMs:    250,
})
```

### Using Tracer

```go
// Get tracer
tracer := setup.Tracer()

// Start a span for LLM request
ctx, span := tracer.StartRequestSpan(ctx, "openai", "gpt-4", "openai")
defer tracer.EndSpan(span, nil)

// Record token usage as span event
tracer.RecordTokenUsageEvent(ctx, 100, 50)
```

### OTLP Export

```go
cfg := &otel.Config{
    Enabled:        true,
    ExportInterval: 10 * time.Second,
    OTLP: otel.OTLPConfig{
        Enabled:  true,
        Endpoint: "localhost:4317",
        Protocol: "grpc",
        Insecure: true,
    },
}
```

## Metrics

| Metric Name | Type | Description |
|-------------|------|-------------|
| `llm.token.usage.input` | Counter | Input/prompt token usage |
| `llm.token.usage.output` | Counter | Output/completion token usage |
| `llm.token.total` | Counter | Total tokens consumed |
| `llm.request.count` | Counter | Number of LLM requests |
| `llm.request.duration` | Histogram | Request duration in milliseconds |
| `llm.request.errors` | Counter | Number of request errors |

## Semantic Attributes

| Attribute | Key | Example |
|-----------|-----|---------|
| Provider | `llm.provider` | "openai", "anthropic" |
| Model | `llm.model` | "gpt-4", "claude-3-opus" |
| Request Model | `llm.request.model` | User-requested model |
| Token Type | `llm.token.type` | "input", "output" |
| Scenario | `llm.scenario` | "openai", "anthropic", "claude_code" |
| Streaming | `llm.streaming` | true, false |
| Status | `llm.response.status` | "success", "error", "canceled" |
| Error Code | `llm.error.code` | Error code if failed |
| Rule UUID | `llm.rule.uuid` | Load balancer rule |
| Provider UUID | `llm.provider.uuid` | Provider UUID |
| User Tier | `llm.user.tier` | "enterprise", "standard" |
| Latency | `llm.latency.ms` | Request latency in ms |

## Configuration

### Config

| Field | Type | Default | Description |
|-------|------|---------|-------------|
| Enabled | bool | true | Enable/disable OTel tracking |
| ExportInterval | duration | 10s | Time between exports |
| ExportTimeout | duration | 30s | Timeout for each export |
| BufferSize | int | 10000 | Max metrics to buffer |
| SQLite | SQLiteConfig | - | SQLite exporter config |
| OTLP | OTLPConfig | - | OTLP exporter config |
| Sink | SinkConfig | - | JSONL sink config |

### SQLiteConfig

| Field | Type | Default | Description |
|-------|------|---------|-------------|
| Enabled | bool | true | Enable SQLite export |

### OTLPConfig

| Field | Type | Default | Description |
|-------|------|---------|-------------|
| Enabled | bool | false | Enable OTLP export |
| Endpoint | string | "" | OTLP endpoint (host:port) |
| Protocol | string | "grpc" | "grpc" or "http/protobuf" |
| Insecure | bool | false | Disable TLS |
| Headers | map[string]string | nil | Additional headers |

### SinkConfig

| Field | Type | Default | Description |
|-------|------|---------|-------------|
| Enabled | bool | true | Enable JSONL sink export |

## Migration from internal/obs/otel

The new `pkg/otel` package replaces `internal/obs/otel`:

1. Import path changed: `internal/obs/otel` → `pkg/otel`
2. TokenTracker moved: `otel.TokenTracker` → `tracker.TokenTracker`
3. UsageOptions moved: `otel.UsageOptions` → `tracker.UsageOptions`
4. Config structure updated with nested exporter configs
5. Metric names updated for clarity:
   - `llm.token.usage` → `llm.token.usage.input` / `llm.token.usage.output`

## Dependencies

- go.opentelemetry.io/otel (v1.42.0)
- go.opentelemetry.io/otel/sdk (v1.42.0)
- go.opentelemetry.io/otel/exporters/otlp (v1.42.0)
- go.opentelemetry.io/otel/exporters/stdout (v1.42.0)

## Related Documentation

- Specification: `docs/spec/20260309-otel-token-usage-collector-spec.md`
- Architecture: `docs/arch/otel-arch.md`
