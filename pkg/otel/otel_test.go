package otel

import (
	"context"
	"testing"
	"time"
)

func TestDefaultConfig(t *testing.T) {
	cfg := DefaultConfig()

	if cfg == nil {
		t.Fatal("DefaultConfig should not return nil")
	}

	if !cfg.Enabled {
		t.Error("Enabled should be true by default")
	}

	if cfg.ExportInterval != 10*time.Second {
		t.Errorf("ExportInterval should be 10s, got %v", cfg.ExportInterval)
	}

	if cfg.ExportTimeout != 30*time.Second {
		t.Errorf("ExportTimeout should be 30s, got %v", cfg.ExportTimeout)
	}

	if cfg.OTLP.Enabled {
		t.Error("OTLP.Enabled should be false by default")
	}
}

func TestOTLPConfig_Defaults(t *testing.T) {
	cfg := OTLPConfig{}

	if cfg.Protocol != "" {
		t.Errorf("Default Protocol should be empty, got %s", cfg.Protocol)
	}

	if cfg.Endpoint != "" {
		t.Errorf("Default Endpoint should be empty, got %s", cfg.Endpoint)
	}
}

func TestSetup_Shutdown_ZeroValue(t *testing.T) {
	// Shutdown on a zero-value setup must not panic
	setup := &Setup{}

	if err := setup.Shutdown(context.Background()); err != nil {
		t.Errorf("Shutdown with nil providers should not return error: %v", err)
	}
}

func TestNewSetup_Disabled(t *testing.T) {
	setup, err := NewSetup(context.Background(), &Config{Enabled: false})
	if err != nil {
		t.Errorf("NewSetup with disabled config should not return error: %v", err)
	}
	if setup != nil {
		t.Error("NewSetup with disabled config should return nil setup")
	}
}

func TestNewSetup_NoOTLP(t *testing.T) {
	// Without OTLP the meter provider has no reader and no tracer provider
	// is installed; instruments and the tracer helper must still be usable
	// and Shutdown must succeed.
	ctx := context.Background()
	setup, err := NewSetup(ctx, &Config{
		Enabled:        true,
		ExportInterval: 10 * time.Second,
		ExportTimeout:  30 * time.Second,
	})
	if err != nil {
		t.Fatalf("NewSetup failed: %v", err)
	}
	if setup == nil {
		t.Fatal("Setup should not be nil")
	}
	if setup.Tracker() == nil {
		t.Error("Tracker should not be nil")
	}
	if setup.Tracer() == nil {
		t.Error("Tracer should not be nil - it must be safe to instrument unconditionally")
	}

	// Spans must be no-ops (not sampled) rather than recorded-and-dropped.
	sctx, span := setup.Tracer().StartRequestSpan(ctx, "openai", "gpt-4", "chat")
	if span.IsRecording() {
		t.Error("spans must not record when no OTLP endpoint is configured")
	}
	setup.Tracer().EndSpan(span, nil)
	_ = sctx

	if err := setup.Shutdown(ctx); err != nil {
		t.Errorf("Shutdown failed: %v", err)
	}
}

func TestNewSetup_WithOTLP(t *testing.T) {
	// OTLP exporters connect lazily, so setup succeeds offline. Spans must
	// actually record when tracing is wired.
	ctx := context.Background()
	setup, err := NewSetup(ctx, &Config{
		Enabled:        true,
		ExportInterval: time.Hour, // never fires during the test
		ExportTimeout:  time.Second,
		OTLP: OTLPConfig{
			Enabled:  true,
			Endpoint: "localhost:4317",
			Insecure: true,
		},
	})
	if err != nil {
		t.Fatalf("NewSetup failed: %v", err)
	}
	if setup == nil {
		t.Fatal("Setup should not be nil")
	}

	_, span := setup.Tracer().StartRequestSpan(ctx, "openai", "gpt-4", "chat")
	if !span.IsRecording() {
		t.Error("spans should record when OTLP tracing is configured")
	}
	setup.Tracer().EndSpan(span, nil)

	sctx, scancel := context.WithTimeout(ctx, 2*time.Second)
	defer scancel()
	// Shutdown flushes to an unreachable endpoint; the exporter error is
	// expected — what matters is that it returns rather than hangs.
	_ = setup.Shutdown(sctx)
}

func TestTraceSampler(t *testing.T) {
	tests := []struct {
		ratio float64
		desc  string
	}{
		{0, "zero value samples everything"},
		{1, "1.0 samples everything"},
		{-0.5, "negative samples everything"},
		{0.25, "fractional ratio"},
	}
	for _, tt := range tests {
		if s := traceSampler(tt.ratio); s == nil {
			t.Errorf("traceSampler(%v) returned nil (%s)", tt.ratio, tt.desc)
		}
	}
}
