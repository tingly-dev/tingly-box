package otel

import "time"

// Config holds the configuration for the OTel meter setup.
type Config struct {
	// Enabled enables or disables OTel tracking
	Enabled bool

	// ExportInterval is the time between exports. Default: 10s
	ExportInterval time.Duration

	// ExportTimeout is the timeout for each export. Default: 30s
	ExportTimeout time.Duration

	// SQLiteEnabled enables SQLite export. Default: true
	SQLiteEnabled bool

	// SinkEnabled enables JSONL file sink export. Default: respect record-mode
	SinkEnabled bool

	// OTLPEndpoint is an optional OTLP endpoint for external observability backends
	OTLPEndpoint string
}

// DefaultConfig returns a config with sensible defaults.
func DefaultConfig() *Config {
	return &Config{
		Enabled:        true,
		ExportInterval: 10 * time.Second,
		ExportTimeout:  30 * time.Second,
		SQLiteEnabled:  true,
		SinkEnabled:    true,
	}
}
