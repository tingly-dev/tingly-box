package exporter

import (
	"context"
	"fmt"

	"go.opentelemetry.io/otel/exporters/otlp/otlptrace/otlptracegrpc"
	"go.opentelemetry.io/otel/exporters/otlp/otlptrace/otlptracehttp"
	sdktrace "go.opentelemetry.io/otel/sdk/trace"
)

// NewOTLPTraceExporter creates a new OTLP span exporter for the configured
// protocol ("grpc" by default, or "http/protobuf"). Traces share the same
// endpoint configuration as metrics — one OTLP egress for all signals.
func NewOTLPTraceExporter(cfg OTLPConfig) (sdktrace.SpanExporter, error) {
	ctx := context.Background()

	switch cfg.Protocol {
	case "http/protobuf":
		exp, err := otlptracehttp.New(ctx, otlpOptions(cfg,
			otlptracehttp.WithEndpoint, otlptracehttp.WithTimeout,
			otlptracehttp.WithInsecure, otlptracehttp.WithHeaders)...)
		if err != nil {
			return nil, fmt.Errorf("failed to create OTLP HTTP trace exporter: %w", err)
		}
		return exp, nil

	case "grpc", "":
		exp, err := otlptracegrpc.New(ctx, otlpOptions(cfg,
			otlptracegrpc.WithEndpoint, otlptracegrpc.WithTimeout,
			otlptracegrpc.WithInsecure, otlptracegrpc.WithHeaders)...)
		if err != nil {
			return nil, fmt.Errorf("failed to create OTLP gRPC trace exporter: %w", err)
		}
		return exp, nil

	default:
		return nil, fmt.Errorf("unsupported OTLP protocol: %s", cfg.Protocol)
	}
}
