package otel

import (
	"time"
)

// Config holds the configuration for the OTel observability setup.
type Config struct {
	// Enabled enables or disables OTel tracking
	Enabled bool

	// ExportInterval is the time between exports. Default: 10s
	ExportInterval time.Duration

	// ExportTimeout is the timeout for each export. Default: 30s
	ExportTimeout time.Duration

	// BufferSize is the max number of metrics to buffer. Default: 10000
	BufferSize int

	// SQLite exporter configuration
	SQLite SQLiteConfig

	// OTLP exporter configuration
	OTLP OTLPConfig

	// Sink exporter configuration
	Sink SinkConfig
}

// SQLiteConfig holds SQLite exporter configuration
type SQLiteConfig struct {
	// Enabled enables SQLite export
	Enabled bool
}

// OTLPConfig holds OTLP exporter configuration
type OTLPConfig struct {
	// Enabled enables OTLP export
	Enabled bool

	// Endpoint is the OTLP endpoint (gRPC or HTTP)
	Endpoint string

	// Protocol is the OTLP protocol ("grpc" or "http/protobuf")
	Protocol string

	// Insecure disables TLS for the connection
	Insecure bool

	// Headers are optional headers to send with each request
	Headers map[string]string
}

// SinkConfig holds JSONL sink exporter configuration
type SinkConfig struct {
	// Enabled enables JSONL sink export
	Enabled bool
}

// DefaultConfig returns a config with sensible defaults
func DefaultConfig() *Config {
	return &Config{
		Enabled:        true,
		ExportInterval: 10 * time.Second,
		ExportTimeout:  30 * time.Second,
		BufferSize:     10000,
		SQLite: SQLiteConfig{
			Enabled: true,
		},
		OTLP: OTLPConfig{
			Enabled: false,
		},
		Sink: SinkConfig{
			Enabled: true,
		},
	}
}
