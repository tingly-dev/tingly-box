package exporter

import (
	"context"
	"fmt"
	"time"

	"go.opentelemetry.io/otel/exporters/otlp/otlpmetric/otlpmetricgrpc"
	"go.opentelemetry.io/otel/exporters/otlp/otlpmetric/otlpmetrichttp"
	sdkmetric "go.opentelemetry.io/otel/sdk/metric"
	"go.opentelemetry.io/otel/sdk/metric/metricdata"
)

// OTLPExporter exports metrics to an OTLP backend via gRPC or HTTP.
type OTLPExporter struct {
	exporter sdkmetric.Exporter
	config   OTLPConfig
}

// OTLPConfig holds the OTLP exporter configuration
type OTLPConfig struct {
	Endpoint string
	Protocol string
	Insecure bool
	Headers  map[string]string
	Timeout  time.Duration
}

// NewOTLPExporter creates a new OTLP exporter.
func NewOTLPExporter(cfg OTLPConfig) (*OTLPExporter, error) {
	ctx := context.Background()
	var exporter sdkmetric.Exporter
	var err error

	// Set default timeout if not specified
	if cfg.Timeout == 0 {
		cfg.Timeout = 30 * time.Second
	}

	switch cfg.Protocol {
	case "http/protobuf":
		opts := []otlpmetrichttp.Option{
			otlpmetrichttp.WithEndpoint(cfg.Endpoint),
			otlpmetrichttp.WithTimeout(cfg.Timeout),
		}

		if cfg.Insecure {
			opts = append(opts, otlpmetrichttp.WithInsecure())
		}

		if len(cfg.Headers) > 0 {
			opts = append(opts, otlpmetrichttp.WithHeaders(cfg.Headers))
		}

		exporter, err = otlpmetrichttp.New(ctx, opts...)
		if err != nil {
			return nil, fmt.Errorf("failed to create OTLP HTTP exporter: %w", err)
		}

	case "grpc", "":
		opts := []otlpmetricgrpc.Option{
			otlpmetricgrpc.WithEndpoint(cfg.Endpoint),
			otlpmetricgrpc.WithTimeout(cfg.Timeout),
		}

		if cfg.Insecure {
			opts = append(opts, otlpmetricgrpc.WithInsecure())
		}

		if len(cfg.Headers) > 0 {
			opts = append(opts, otlpmetricgrpc.WithHeaders(cfg.Headers))
		}

		exporter, err = otlpmetricgrpc.New(ctx, opts...)
		if err != nil {
			return nil, fmt.Errorf("failed to create OTLP gRPC exporter: %w", err)
		}

	default:
		return nil, fmt.Errorf("unsupported OTLP protocol: %s", cfg.Protocol)
	}

	return &OTLPExporter{
		exporter: exporter,
		config:   cfg,
	}, nil
}

// Temporality returns the Temporality to use for an instrument kind.
func (e *OTLPExporter) Temporality(kind sdkmetric.InstrumentKind) metricdata.Temporality {
	return metricdata.CumulativeTemporality
}

// Aggregation returns the Aggregation to use for an instrument kind.
func (e *OTLPExporter) Aggregation(kind sdkmetric.InstrumentKind) sdkmetric.Aggregation {
	return sdkmetric.DefaultAggregationSelector(kind)
}

// Export exports metrics to the OTLP endpoint.
func (e *OTLPExporter) Export(ctx context.Context, res *metricdata.ResourceMetrics) error {
	return e.exporter.Export(ctx, res)
}

// ForceFlush forces a flush of pending data.
func (e *OTLPExporter) ForceFlush(ctx context.Context) error {
	return e.exporter.ForceFlush(ctx)
}

// Shutdown shuts down the exporter.
func (e *OTLPExporter) Shutdown(ctx context.Context) error {
	return e.exporter.Shutdown(ctx)
}
