// Package otel wires OpenTelemetry metrics and traces for LLM requests.
//
// Design: telemetry has exactly one egress — an optional OTLP endpoint,
// shared by all signals. Aggregated metrics (counters, histograms) answer
// "how much / how fast"; spans answer "what happened inside this request";
// per-request artifacts (usage records, request recordings) are written at
// the source by internal/server/usage_tracking.go and the recording
// pipeline, never reconstructed from aggregated metric data points.
//
// When OTLP is not configured, no providers are constructed at all:
// Tracker() returns nil (callers already nil-guard) and Tracer() wraps an
// explicit no-op provider, so telemetry costs nothing per request.
package otel

import (
	"context"
	"fmt"

	"go.opentelemetry.io/otel"
	"go.opentelemetry.io/otel/attribute"
	"go.opentelemetry.io/otel/propagation"
	"go.opentelemetry.io/otel/sdk/metric"
	"go.opentelemetry.io/otel/sdk/resource"
	sdktrace "go.opentelemetry.io/otel/sdk/trace"
	semconv "go.opentelemetry.io/otel/semconv/v1.37.0"
	tracenoop "go.opentelemetry.io/otel/trace/noop"

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
//
// The process-global OTel providers and propagator are installed only after
// every construction step has succeeded — a failed NewSetup leaves the
// globals untouched (default no-ops) instead of pointing at shut-down
// providers.
func NewSetup(ctx context.Context, cfg *Config) (*Setup, error) {
	if !cfg.Enabled {
		return nil, nil
	}

	// Without an OTLP endpoint there is deliberately no pipeline at all: a
	// nil Tracker skips all per-request attribute work (callers nil-guard),
	// and the no-op Tracer never records spans. The previous stdout
	// fallback — which would have printed metrics to the server console
	// every interval — is intentionally gone.
	if !cfg.OTLP.Enabled || cfg.OTLP.Endpoint == "" {
		return &Setup{tracer: NewTracer(tracenoop.NewTracerProvider())}, nil
	}

	// Create resource with service info
	resAttrs := []attribute.KeyValue{semconv.ServiceName("tingly-box")}
	if cfg.ServiceVersion != "" {
		resAttrs = append(resAttrs, semconv.ServiceVersion(cfg.ServiceVersion))
	}
	res, err := resource.New(ctx, resource.WithAttributes(resAttrs...))
	if err != nil {
		return nil, fmt.Errorf("failed to create resource: %w", err)
	}

	otlpCfg := exporter.OTLPConfig{
		Endpoint: cfg.OTLP.Endpoint,
		Protocol: cfg.OTLP.Protocol,
		Insecure: cfg.OTLP.Insecure,
		Headers:  cfg.OTLP.Headers,
		Timeout:  cfg.ExportTimeout,
	}

	// Metrics
	metricExp, err := exporter.NewOTLPExporter(otlpCfg)
	if err != nil {
		return nil, fmt.Errorf("failed to create OTLP metric exporter: %w", err)
	}
	meterProvider := metric.NewMeterProvider(
		metric.WithResource(res),
		metric.WithReader(metric.NewPeriodicReader(
			metricExp,
			metric.WithInterval(cfg.ExportInterval),
			metric.WithTimeout(cfg.ExportTimeout),
		)),
	)

	// Traces
	traceExp, err := exporter.NewOTLPTraceExporter(otlpCfg)
	if err != nil {
		_ = meterProvider.Shutdown(ctx)
		return nil, fmt.Errorf("failed to create OTLP trace exporter: %w", err)
	}
	tracerProvider := sdktrace.NewTracerProvider(
		sdktrace.WithResource(res),
		sdktrace.WithBatcher(traceExp),
		sdktrace.WithSampler(traceSampler(cfg.OTLP.TraceSampleRatio)),
	)

	// Token tracker
	tokenTracker, err := tracker.NewTokenTracker(meterProvider.Meter("tingly-box"))
	if err != nil {
		_ = meterProvider.Shutdown(ctx)
		_ = tracerProvider.Shutdown(ctx)
		return nil, fmt.Errorf("failed to create token tracker: %w", err)
	}

	// Everything succeeded — install the globals. W3C context propagation
	// lets trace ids survive the gateway hop in both directions once
	// handlers/clients are instrumented.
	otel.SetMeterProvider(meterProvider)
	otel.SetTracerProvider(tracerProvider)
	otel.SetTextMapPropagator(propagation.NewCompositeTextMapPropagator(
		propagation.TraceContext{},
		propagation.Baggage{},
	))

	return &Setup{
		meterProvider:  meterProvider,
		tracerProvider: tracerProvider,
		tracker:        tokenTracker,
		tracer:         NewTracer(tracerProvider),
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

// Tracker returns the token tracker, or nil when OTLP export is not
// configured (or on a nil Setup) — callers must nil-guard, which lets them
// skip all per-request metric work when there is no egress.
func (s *Setup) Tracker() *tracker.TokenTracker {
	if s == nil {
		return nil
	}
	return s.tracker
}

// Tracer returns the tracing helper. It is safe to call on a nil Setup and
// always returns a usable Tracer; spans are no-ops unless OTLP is
// configured.
func (s *Setup) Tracer() *Tracer {
	if s == nil || s.tracer == nil {
		return NewTracer(tracenoop.NewTracerProvider())
	}
	return s.tracer
}

// Shutdown flushes pending exports and shuts down the providers.
func (s *Setup) Shutdown(ctx context.Context) error {
	if s == nil {
		return nil
	}

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
