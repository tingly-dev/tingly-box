package otel

import (
	"context"
	"fmt"

	"go.opentelemetry.io/otel"
	"go.opentelemetry.io/otel/exporters/stdout/stdoutmetric"
	"go.opentelemetry.io/otel/sdk/metric"
	"go.opentelemetry.io/otel/sdk/resource"
	"go.opentelemetry.io/otel/sdk/trace"
	semconv "go.opentelemetry.io/otel/semconv/v1.24.0"

	"github.com/tingly-dev/tingly-box/internal/data/db"
	"github.com/tingly-dev/tingly-box/internal/obs"
	"github.com/tingly-dev/tingly-box/pkg/otel/exporter"
	"github.com/tingly-dev/tingly-box/pkg/otel/tracker"
)

// MeterSetup holds the meter provider, tracer provider, and token tracker.
type MeterSetup struct {
	meterProvider  *metric.MeterProvider
	tracerProvider *trace.TracerProvider
	tracker        *tracker.TokenTracker
	tracer         *Tracer
}

// StoreRefs holds references to the storage backends for exporters.
type StoreRefs struct {
	StatsStore *db.StatsStore
	UsageStore *db.UsageStore
	Sink       *obs.Sink
}

// NewMeterSetup creates a new meter setup with the provided config and stores.
func NewMeterSetup(ctx context.Context, cfg *Config, stores *StoreRefs) (*MeterSetup, error) {
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

	// Build metric exporters
	var metricExporters []metric.Exporter

	// SQLite exporter
	if cfg.SQLite.Enabled && stores.UsageStore != nil {
		sqliteExp := exporter.NewSQLiteExporter(stores.UsageStore)
		metricExporters = append(metricExporters, sqliteExp)
	}

	// Sink exporter
	if cfg.Sink.Enabled && stores.Sink != nil {
		sinkExp := exporter.NewSinkExporter(stores.Sink)
		metricExporters = append(metricExporters, sinkExp)
	}

	// OTLP exporter
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
		metricExporters = append(metricExporters, otlpExp)
	}

	// If no exporters, use stdout for debugging
	if len(metricExporters) == 0 {
		stdoutExp, err := stdoutmetric.New()
		if err != nil {
			return nil, fmt.Errorf("failed to create stdout exporter: %w", err)
		}
		metricExporters = append(metricExporters, stdoutExp)
	}

	// Create meter provider with periodic reader
	multiExporter := exporter.NewMultiExporter(metricExporters...)
	reader := metric.NewPeriodicReader(
		multiExporter,
		metric.WithInterval(cfg.ExportInterval),
		metric.WithTimeout(cfg.ExportTimeout),
	)

	meterProvider := metric.NewMeterProvider(
		metric.WithReader(reader),
		metric.WithResource(res),
	)

	// Set global meter provider
	otel.SetMeterProvider(meterProvider)

	// Create tracer provider
	var tracerProvider *trace.TracerProvider
	if cfg.OTLP.Enabled && cfg.OTLP.Endpoint != "" {
		// TODO: Add OTLP trace exporter when needed
		tracerProvider = trace.NewTracerProvider(
			trace.WithResource(res),
		)
	} else {
		tracerProvider = trace.NewTracerProvider(
			trace.WithResource(res),
			trace.WithSampler(trace.AlwaysSample()),
		)
	}

	// Set global tracer provider
	otel.SetTracerProvider(tracerProvider)

	// Create meter and token tracker
	meter := meterProvider.Meter("tingly-box")
	tracker, err := tracker.NewTokenTracker(meter)
	if err != nil {
		// Shutdown meter provider on error
		_ = meterProvider.Shutdown(ctx)
		_ = tracerProvider.Shutdown(ctx)
		return nil, fmt.Errorf("failed to create token tracker: %w", err)
	}

	// Create tracer
	tracer := NewTracer(tracerProvider)

	return &MeterSetup{
		meterProvider:  meterProvider,
		tracerProvider: tracerProvider,
		tracker:        tracker,
		tracer:         tracer,
	}, nil
}

// Tracker returns the token tracker.
func (ms *MeterSetup) Tracker() *tracker.TokenTracker {
	return ms.tracker
}

// Tracer returns the tracer.
func (ms *MeterSetup) Tracer() *Tracer {
	return ms.tracer
}

// Shutdown shuts down the meter and tracer providers.
func (ms *MeterSetup) Shutdown(ctx context.Context) error {
	var errs []error

	if ms.meterProvider != nil {
		if err := ms.meterProvider.Shutdown(ctx); err != nil {
			errs = append(errs, err)
		}
	}

	if ms.tracerProvider != nil {
		if err := ms.tracerProvider.Shutdown(ctx); err != nil {
			errs = append(errs, err)
		}
	}

	if len(errs) > 0 {
		return fmt.Errorf("shutdown errors: %v", errs)
	}
	return nil
}
