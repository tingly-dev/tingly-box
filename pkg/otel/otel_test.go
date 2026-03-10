package otel

import (
	"context"
	"testing"
	"time"

	"github.com/tingly-dev/tingly-box/internal/obs"
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

	if cfg.BufferSize != 10000 {
		t.Errorf("BufferSize should be 10000, got %d", cfg.BufferSize)
	}

	if !cfg.SQLite.Enabled {
		t.Error("SQLite.Enabled should be true by default")
	}

	if cfg.OTLP.Enabled {
		t.Error("OTLP.Enabled should be false by default")
	}

	if !cfg.Sink.Enabled {
		t.Error("Sink.Enabled should be true by default")
	}
}

func TestConfig_CustomValues(t *testing.T) {
	cfg := &Config{
		Enabled:        true,
		ExportInterval: 5 * time.Second,
		ExportTimeout:  60 * time.Second,
		BufferSize:     5000,
		SQLite: SQLiteConfig{
			Enabled: false,
		},
		OTLP: OTLPConfig{
			Enabled:  true,
			Endpoint: "localhost:4317",
			Protocol: "grpc",
			Insecure: true,
		},
		Sink: SinkConfig{
			Enabled: true,
		},
	}

	if cfg.ExportInterval != 5*time.Second {
		t.Errorf("ExportInterval mismatch: %v", cfg.ExportInterval)
	}

	if cfg.ExportTimeout != 60*time.Second {
		t.Errorf("ExportTimeout mismatch: %v", cfg.ExportTimeout)
	}

	if cfg.BufferSize != 5000 {
		t.Errorf("BufferSize mismatch: %d", cfg.BufferSize)
	}

	if cfg.SQLite.Enabled {
		t.Error("SQLite.Enabled should be false")
	}

	if !cfg.OTLP.Enabled {
		t.Error("OTLP.Enabled should be true")
	}

	if cfg.OTLP.Endpoint != "localhost:4317" {
		t.Errorf("OTLP.Endpoint mismatch: %s", cfg.OTLP.Endpoint)
	}

	if cfg.OTLP.Protocol != "grpc" {
		t.Errorf("OTLP.Protocol mismatch: %s", cfg.OTLP.Protocol)
	}

	if !cfg.OTLP.Insecure {
		t.Error("OTLP.Insecure should be true")
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

func TestStoreRefs(t *testing.T) {
	refs := &StoreRefs{
		StatsStore: nil,
		UsageStore: nil,
		Sink:       nil,
	}

	if refs == nil {
		t.Fatal("StoreRefs should not be nil")
	}
}

func TestMeterSetup_Shutdown(t *testing.T) {
	// Test shutdown with nil providers (should not panic)
	setup := &MeterSetup{
		meterProvider:  nil,
		tracerProvider: nil,
		tracker:        nil,
		tracer:         nil,
	}

	ctx := context.Background()
	err := setup.Shutdown(ctx)
	if err != nil {
		t.Errorf("Shutdown with nil providers should not return error: %v", err)
	}
}

func TestMeterSetup_Tracker(t *testing.T) {
	// Test Tracker method
	setup := &MeterSetup{}
	tracker := setup.Tracker()
	if tracker != nil {
		t.Error("Tracker should be nil when not initialized")
	}
}

func TestMeterSetup_Tracer(t *testing.T) {
	// Test Tracer method
	setup := &MeterSetup{}
	tracer := setup.Tracer()
	if tracer != nil {
		t.Error("Tracer should be nil when not initialized")
	}
}

func TestNewMeterSetup_Disabled(t *testing.T) {
	ctx := context.Background()
	cfg := &Config{
		Enabled: false,
	}
	stores := &StoreRefs{}

	setup, err := NewMeterSetup(ctx, cfg, stores)
	if err != nil {
		t.Errorf("NewMeterSetup with disabled config should not return error: %v", err)
	}

	if setup != nil {
		t.Error("NewMeterSetup with disabled config should return nil setup")
	}
}

func TestNewMeterSetup_WithStores(t *testing.T) {
	ctx := context.Background()
	cfg := &Config{
		Enabled:        true,
		ExportInterval: 10 * time.Second,
		ExportTimeout:  30 * time.Second,
		SQLite: SQLiteConfig{
			Enabled: true,
		},
		Sink: SinkConfig{
			Enabled: true,
		},
		OTLP: OTLPConfig{
			Enabled: false,
		},
	}

	// Create with nil stores (should handle gracefully)
	stores := &StoreRefs{
		StatsStore: nil,
		UsageStore: nil,
		Sink:       nil,
	}

	setup, err := NewMeterSetup(ctx, cfg, stores)
	if err != nil {
		t.Fatalf("NewMeterSetup failed: %v", err)
	}

	if setup == nil {
		t.Fatal("MeterSetup should not be nil")
	}

	// Cleanup
	if err := setup.Shutdown(ctx); err != nil {
		t.Errorf("Shutdown failed: %v", err)
	}
}

func TestNewMeterSetup_WithMockStores(t *testing.T) {
	ctx := context.Background()
	cfg := &Config{
		Enabled:        true,
		ExportInterval: 10 * time.Second,
		ExportTimeout:  30 * time.Second,
		SQLite: SQLiteConfig{
			Enabled: true,
		},
		Sink: SinkConfig{
			Enabled: false, // Disable sink for this test
		},
		OTLP: OTLPConfig{
			Enabled: false,
		},
	}

	// Create mock sink
	sink := obs.NewSink(t.TempDir(), obs.RecordModeResponse)

	stores := &StoreRefs{
		StatsStore: nil,
		UsageStore: nil,
		Sink:       sink,
	}

	setup, err := NewMeterSetup(ctx, cfg, stores)
	if err != nil {
		t.Fatalf("NewMeterSetup failed: %v", err)
	}

	if setup == nil {
		t.Fatal("MeterSetup should not be nil")
	}

	if setup.Tracker() == nil {
		t.Error("Tracker should not be nil")
	}

	// Cleanup
	if err := setup.Shutdown(ctx); err != nil {
		t.Errorf("Shutdown failed: %v", err)
	}
}

func TestAttributes(t *testing.T) {
	// Test that all semantic convention attributes are defined
	tests := []struct {
		name string
		key  string
	}{
		{"Provider", string(AttrLLMProvider)},
		{"Model", string(AttrLLMModel)},
		{"RequestModel", string(AttrLLMRequestModel)},
		{"TokenType", string(AttrLLMTokenType)},
		{"Scenario", string(AttrLLMScenario)},
		{"Streaming", string(AttrLLMStreaming)},
		{"ResponseStatus", string(AttrLLMResponseStatus)},
		{"ErrorCode", string(AttrLLMErrorCode)},
		{"RuleUUID", string(AttrLLMRuleUUID)},
		{"ProviderUUID", string(AttrLLMProviderUUID)},
		{"UserTier", string(AttrLLMUserTier)},
		{"LatencyMs", string(AttrLLMLatencyMs)},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if tt.key == "" {
				t.Errorf("Attribute %s should not be empty", tt.name)
			}
		})
	}
}
