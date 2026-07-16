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

func TestMeterSetup_Shutdown_NilProvider(t *testing.T) {
	// Shutdown on a zero-value setup must not panic
	setup := &MeterSetup{}

	if err := setup.Shutdown(context.Background()); err != nil {
		t.Errorf("Shutdown with nil provider should not return error: %v", err)
	}
}

func TestMeterSetup_Tracker_NilSetup(t *testing.T) {
	setup := &MeterSetup{}
	if setup.Tracker() != nil {
		t.Error("Tracker should be nil when not initialized")
	}
}

func TestNewMeterSetup_Disabled(t *testing.T) {
	setup, err := NewMeterSetup(context.Background(), &Config{Enabled: false})
	if err != nil {
		t.Errorf("NewMeterSetup with disabled config should not return error: %v", err)
	}
	if setup != nil {
		t.Error("NewMeterSetup with disabled config should return nil setup")
	}
}

func TestNewMeterSetup_NoOTLP(t *testing.T) {
	// Without OTLP the provider has no reader; instruments must still be
	// usable and Shutdown must succeed.
	ctx := context.Background()
	setup, err := NewMeterSetup(ctx, &Config{
		Enabled:        true,
		ExportInterval: 10 * time.Second,
		ExportTimeout:  30 * time.Second,
	})
	if err != nil {
		t.Fatalf("NewMeterSetup failed: %v", err)
	}
	if setup == nil {
		t.Fatal("MeterSetup should not be nil")
	}
	if setup.Tracker() == nil {
		t.Error("Tracker should not be nil")
	}

	if err := setup.Shutdown(ctx); err != nil {
		t.Errorf("Shutdown failed: %v", err)
	}
}
