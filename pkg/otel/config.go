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

	// OTLP exporter configuration. OTLP is the only metrics egress:
	// persistent usage records and request recordings are written at the
	// source (internal/server/usage_tracking.go and the recording pipeline),
	// not derived from aggregated metric data points.
	OTLP OTLPConfig
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

// DefaultConfig returns a config with sensible defaults
func DefaultConfig() *Config {
	return &Config{
		Enabled:        true,
		ExportInterval: 10 * time.Second,
		ExportTimeout:  30 * time.Second,
		OTLP: OTLPConfig{
			Enabled: false,
		},
	}
}
