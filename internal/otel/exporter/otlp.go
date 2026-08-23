// Package exporter provides the OTLP exporters used by pkg/otel.
package exporter

import (
	"context"
	"fmt"
	"time"

	"go.opentelemetry.io/otel/exporters/otlp/otlpmetric/otlpmetricgrpc"
	"go.opentelemetry.io/otel/exporters/otlp/otlpmetric/otlpmetrichttp"
	sdkmetric "go.opentelemetry.io/otel/sdk/metric"
)

// OTLPConfig holds the OTLP exporter configuration, shared by all signals.
type OTLPConfig struct {
	Endpoint string
	Protocol string
	Insecure bool
	Headers  map[string]string
	Timeout  time.Duration
}

// timeout returns the configured timeout or the 30s default.
func (cfg OTLPConfig) timeout() time.Duration {
	if cfg.Timeout == 0 {
		return 30 * time.Second
	}
	return cfg.Timeout
}

// otlpOptions assembles the option list for one OTLP exporter flavor. The
// per-signal, per-protocol option types are distinct but structurally
// identical, so each constructor passes its own With* functions.
func otlpOptions[O any](cfg OTLPConfig, withEndpoint func(string) O, withTimeout func(time.Duration) O, withInsecure func() O, withHeaders func(map[string]string) O) []O {
	opts := []O{withEndpoint(cfg.Endpoint), withTimeout(cfg.timeout())}
	if cfg.Insecure {
		opts = append(opts, withInsecure())
	}
	if len(cfg.Headers) > 0 {
		opts = append(opts, withHeaders(cfg.Headers))
	}
	return opts
}

// NewOTLPExporter creates a new OTLP metrics exporter for the configured
// protocol ("grpc" by default, or "http/protobuf"). The returned exporter is
// the SDK exporter itself — no wrapping is needed. Construction is lazy;
// no connection is dialed until the first export.
func NewOTLPExporter(cfg OTLPConfig) (sdkmetric.Exporter, error) {
	ctx := context.Background()

	switch cfg.Protocol {
	case "http/protobuf":
		exp, err := otlpmetrichttp.New(ctx, otlpOptions(cfg,
			otlpmetrichttp.WithEndpoint, otlpmetrichttp.WithTimeout,
			otlpmetrichttp.WithInsecure, otlpmetrichttp.WithHeaders)...)
		if err != nil {
			return nil, fmt.Errorf("failed to create OTLP HTTP exporter: %w", err)
		}
		return exp, nil

	case "grpc", "":
		exp, err := otlpmetricgrpc.New(ctx, otlpOptions(cfg,
			otlpmetricgrpc.WithEndpoint, otlpmetricgrpc.WithTimeout,
			otlpmetricgrpc.WithInsecure, otlpmetricgrpc.WithHeaders)...)
		if err != nil {
			return nil, fmt.Errorf("failed to create OTLP gRPC exporter: %w", err)
		}
		return exp, nil

	default:
		return nil, fmt.Errorf("unsupported OTLP protocol: %s", cfg.Protocol)
	}
}
