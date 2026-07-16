// Package otel wires OpenTelemetry metrics for LLM token usage.
//
// Design: metrics have exactly one egress — an optional OTLP endpoint.
// Aggregated metrics (counters, histograms) answer "how much / how fast";
// per-request artifacts (usage records, request recordings) are written at
// the source by internal/server/usage_tracking.go and the recording
// pipeline, never reconstructed from aggregated metric data points. When
// OTLP is not configured, the meter provider is created without a reader,
// so instrument calls are cheap no-ops.
package otel

import (
	"context"
	"fmt"

	"go.opentelemetry.io/otel"
	"go.opentelemetry.io/otel/sdk/metric"
	"go.opentelemetry.io/otel/sdk/resource"
	semconv "go.opentelemetry.io/otel/semconv/v1.24.0"

	"github.com/tingly-dev/tingly-box/pkg/otel/exporter"
	"github.com/tingly-dev/tingly-box/pkg/otel/tracker"
)

// MeterSetup holds the meter provider and token tracker.
type MeterSetup struct {
	meterProvider *metric.MeterProvider
	tracker       *tracker.TokenTracker
}

// NewMeterSetup creates a new meter setup with the provided config.
// It returns (nil, nil) when cfg.Enabled is false.
func NewMeterSetup(ctx context.Context, cfg *Config) (*MeterSetup, error) {
	if !cfg.Enabled {
		return nil, nil
	}

	// Create resource with service info
	res, err := resource.New(ctx,
		resource.WithAttributes(
			semconv.ServiceName("tingly-box"),
			semconv.ServiceVersion("1.0.0"),
		),
	)
	if err != nil {
		return nil, fmt.Errorf("failed to create resource: %w", err)
	}

	providerOpts := []metric.Option{metric.WithResource(res)}

	// OTLP is the only exporter. Without it there is deliberately no reader:
	// a reader-less provider keeps instrument calls near-free and avoids
	// pushing metrics anywhere nobody asked for (the previous stdout
	// fallback would have spammed the server console).
	if cfg.OTLP.Enabled && cfg.OTLP.Endpoint != "" {
		otlpExp, err := exporter.NewOTLPExporter(exporter.OTLPConfig{
			Endpoint: cfg.OTLP.Endpoint,
			Protocol: cfg.OTLP.Protocol,
			Insecure: cfg.OTLP.Insecure,
			Headers:  cfg.OTLP.Headers,
			Timeout:  cfg.ExportTimeout,
		})
		if err != nil {
			return nil, fmt.Errorf("failed to create OTLP exporter: %w", err)
		}
		providerOpts = append(providerOpts, metric.WithReader(metric.NewPeriodicReader(
			otlpExp,
			metric.WithInterval(cfg.ExportInterval),
			metric.WithTimeout(cfg.ExportTimeout),
		)))
	}

	meterProvider := metric.NewMeterProvider(providerOpts...)

	// Set global meter provider
	otel.SetMeterProvider(meterProvider)

	// Create meter and token tracker
	tokenTracker, err := tracker.NewTokenTracker(meterProvider.Meter("tingly-box"))
	if err != nil {
		_ = meterProvider.Shutdown(ctx)
		return nil, fmt.Errorf("failed to create token tracker: %w", err)
	}

	return &MeterSetup{
		meterProvider: meterProvider,
		tracker:       tokenTracker,
	}, nil
}

// Tracker returns the token tracker.
func (ms *MeterSetup) Tracker() *tracker.TokenTracker {
	return ms.tracker
}

// Shutdown flushes pending exports and shuts down the meter provider.
func (ms *MeterSetup) Shutdown(ctx context.Context) error {
	if ms.meterProvider == nil {
		return nil
	}
	if err := ms.meterProvider.Shutdown(ctx); err != nil {
		return fmt.Errorf("shutdown error: %w", err)
	}
	return nil
}
