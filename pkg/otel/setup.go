// Package otel wires OpenTelemetry metrics and traces for LLM requests.
//
// Design: telemetry has exactly one egress — an optional OTLP endpoint,
// shared by all signals. Aggregated metrics (counters, histograms) answer
// "how much / how fast"; spans answer "what happened inside this request";
// per-request artifacts (usage records, request recordings) are written at
// the source by internal/server/usage_tracking.go and the recording
// pipeline, never reconstructed from aggregated metric data points.
//
// When OTLP is not configured, no reader or span processor is installed:
// instrument calls are cheap no-ops and spans are never sampled, so
// instrumentation can stay in place unconditionally.
package otel

import (
	"context"
	"fmt"

	"go.opentelemetry.io/otel"
	"go.opentelemetry.io/otel/propagation"
	"go.opentelemetry.io/otel/sdk/metric"
	"go.opentelemetry.io/otel/sdk/resource"
	sdktrace "go.opentelemetry.io/otel/sdk/trace"
	semconv "go.opentelemetry.io/otel/semconv/v1.24.0"

	"github.com/tingly-dev/tingly-box/pkg/otel/exporter"
	"github.com/tingly-dev/tingly-box/pkg/otel/tracker"
)

// Setup holds the configured providers, the token tracker, and the tracer.
type Setup struct {
	meterProvider  *metric.MeterProvider
	tracerProvider *sdktrace.TracerProvider
	tracker        *tracker.TokenTracker
	tracer         *Tracer
}

// NewSetup initializes OTel metrics and tracing with the provided config.
// It returns (nil, nil) when cfg.Enabled is false.
func NewSetup(ctx context.Context, cfg *Config) (*Setup, error) {
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

	otlpOn := cfg.OTLP.Enabled && cfg.OTLP.Endpoint != ""

	// Metrics. Without OTLP there is deliberately no reader: a reader-less
	// provider keeps instrument calls near-free and avoids pushing metrics
	// anywhere nobody asked for (the previous stdout fallback would have
	// spammed the server console).
	meterOpts := []metric.Option{metric.WithResource(res)}
	if otlpOn {
		metricExp, err := exporter.NewOTLPExporter(exporter.OTLPConfig{
			Endpoint: cfg.OTLP.Endpoint,
			Protocol: cfg.OTLP.Protocol,
			Insecure: cfg.OTLP.Insecure,
			Headers:  cfg.OTLP.Headers,
			Timeout:  cfg.ExportTimeout,
		})
		if err != nil {
			return nil, fmt.Errorf("failed to create OTLP metric exporter: %w", err)
		}
		meterOpts = append(meterOpts, metric.WithReader(metric.NewPeriodicReader(
			metricExp,
			metric.WithInterval(cfg.ExportInterval),
			metric.WithTimeout(cfg.ExportTimeout),
		)))
	}
	meterProvider := metric.NewMeterProvider(meterOpts...)
	otel.SetMeterProvider(meterProvider)

	// Traces. The tracer provider is only constructed when it has somewhere
	// to send spans — a provider with a sampler but no processor records
	// spans and then drops them, which is worse than the global no-op.
	var tracerProvider *sdktrace.TracerProvider
	if otlpOn {
		traceExp, err := exporter.NewOTLPTraceExporter(exporter.OTLPConfig{
			Endpoint: cfg.OTLP.Endpoint,
			Protocol: cfg.OTLP.Protocol,
			Insecure: cfg.OTLP.Insecure,
			Headers:  cfg.OTLP.Headers,
			Timeout:  cfg.ExportTimeout,
		})
		if err != nil {
			_ = meterProvider.Shutdown(ctx)
			return nil, fmt.Errorf("failed to create OTLP trace exporter: %w", err)
		}
		tracerProvider = sdktrace.NewTracerProvider(
			sdktrace.WithResource(res),
			sdktrace.WithBatcher(traceExp),
			sdktrace.WithSampler(traceSampler(cfg.OTLP.TraceSampleRatio)),
		)
		otel.SetTracerProvider(tracerProvider)
		// W3C context propagation, so trace ids survive the gateway hop in
		// both directions once handlers/clients are instrumented.
		otel.SetTextMapPropagator(propagation.NewCompositeTextMapPropagator(
			propagation.TraceContext{},
			propagation.Baggage{},
		))
	}

	// Create meter and token tracker
	tokenTracker, err := tracker.NewTokenTracker(meterProvider.Meter("tingly-box"))
	if err != nil {
		_ = meterProvider.Shutdown(ctx)
		if tracerProvider != nil {
			_ = tracerProvider.Shutdown(ctx)
		}
		return nil, fmt.Errorf("failed to create token tracker: %w", err)
	}

	// The Tracer helper is always non-nil so call sites can instrument
	// unconditionally; when tracing is off it wraps the global no-op
	// provider and spans cost nothing.
	var tr *Tracer
	if tracerProvider != nil {
		tr = NewTracer(tracerProvider)
	} else {
		tr = NewTracer(nil)
	}

	return &Setup{
		meterProvider:  meterProvider,
		tracerProvider: tracerProvider,
		tracker:        tokenTracker,
		tracer:         tr,
	}, nil
}

// traceSampler maps the configured ratio to a parent-based sampler: an
// incoming sampled context is always honored; new traces are sampled at
// ratio. Ratios outside (0,1) — including the zero value — mean "sample
// everything", the sensible default for a gateway with moderate QPS.
func traceSampler(ratio float64) sdktrace.Sampler {
	if ratio > 0 && ratio < 1 {
		return sdktrace.ParentBased(sdktrace.TraceIDRatioBased(ratio))
	}
	return sdktrace.ParentBased(sdktrace.AlwaysSample())
}

// Tracker returns the token tracker.
func (s *Setup) Tracker() *tracker.TokenTracker {
	return s.tracker
}

// Tracer returns the tracing helper. It is non-nil on any Setup returned by
// NewSetup; spans are no-ops unless OTLP is configured.
func (s *Setup) Tracer() *Tracer {
	return s.tracer
}

// Shutdown flushes pending exports and shuts down the providers.
func (s *Setup) Shutdown(ctx context.Context) error {
	var errs []error

	if s.meterProvider != nil {
		if err := s.meterProvider.Shutdown(ctx); err != nil {
			errs = append(errs, err)
		}
	}

	if s.tracerProvider != nil {
		if err := s.tracerProvider.Shutdown(ctx); err != nil {
			errs = append(errs, err)
		}
	}

	if len(errs) > 0 {
		return fmt.Errorf("shutdown errors: %v", errs)
	}
	return nil
}
