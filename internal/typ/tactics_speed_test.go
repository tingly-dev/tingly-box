package typ

import (
	"testing"

	"github.com/tingly-dev/tingly-box/internal/constant"
	"github.com/tingly-dev/tingly-box/internal/loadbalance"
)

func TestSpeedBasedTactic_SelectService(t *testing.T) {
	// Create services with different speed profiles
	service1 := &loadbalance.Service{
		Provider: "provider1",
		Model:    "model1",
		Active:   true,
		Stats: loadbalance.ServiceStats{
			ServiceID: "provider1:model1",
		},
	}
	service2 := &loadbalance.Service{
		Provider: "provider2",
		Model:    "model2",
		Active:   true,
		Stats: loadbalance.ServiceStats{
			ServiceID: "provider2:model2",
		},
	}
	service3 := &loadbalance.Service{
		Provider: "provider3",
		Model:    "model3",
		Active:   true,
		Stats: loadbalance.ServiceStats{
			ServiceID: "provider3:model3",
		},
	}

	// Record different speeds for each service
	// Service 1: low speed (avg ~30 tps)
	for i := 0; i < 5; i++ {
		service1.Stats.RecordTokenSpeed(30.0, 10)
	}

	// Service 2: medium speed (avg ~60 tps)
	for i := 0; i < 5; i++ {
		service2.Stats.RecordTokenSpeed(60.0, 10)
	}

	// Service 3: high speed (avg ~100 tps)
	for i := 0; i < 5; i++ {
		service3.Stats.RecordTokenSpeed(100.0, 10)
	}

	tactic := NewSpeedBasedTactic(
		constant.DefaultMinSpeedSamples,
		constant.DefaultSpeedThresholdTps,
		constant.DefaultSpeedSampleWindow,
	)

	tests := []struct {
		name          string
		services      []*loadbalance.Service
		wantServiceID string
	}{
		{
			name:          "selects highest speed service",
			services:      []*loadbalance.Service{service1, service2, service3},
			wantServiceID: "provider3:model3",
		},
		{
			name:          "handles single service",
			services:      []*loadbalance.Service{service1},
			wantServiceID: "provider1:model1",
		},
		{
			name:          "handles no active services",
			services:      []*loadbalance.Service{},
			wantServiceID: "",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			rule := &Rule{
				Services: tt.services,
			}

			got := tactic.SelectService(rule)
			var gotID string
			if got != nil {
				gotID = got.ServiceID()
			}

			if gotID != tt.wantServiceID {
				t.Errorf("SelectService() = %v, want %v", gotID, tt.wantServiceID)
			}
		})
	}
}

func TestSpeedBasedTactic_InsufficientSamples(t *testing.T) {
	// Create services with insufficient samples
	service1 := &loadbalance.Service{
		Provider: "provider1",
		Model:    "model1",
		Active:   true,
		Stats: loadbalance.ServiceStats{
			ServiceID: "provider1:model1",
		},
	}
	service2 := &loadbalance.Service{
		Provider: "provider2",
		Model:    "model2",
		Active:   true,
		Stats: loadbalance.ServiceStats{
			ServiceID: "provider2:model2",
		},
	}

	// Service 1: only 2 samples (below default min of 5)
	for i := 0; i < 2; i++ {
		service1.Stats.RecordTokenSpeed(100.0, 10)
	}

	// Service 2: 5 samples (meets min requirement)
	for i := 0; i < 5; i++ {
		service2.Stats.RecordTokenSpeed(50.0, 10)
	}

	tactic := NewSpeedBasedTactic(
		5, // min samples required
		constant.DefaultSpeedThresholdTps,
		constant.DefaultSpeedSampleWindow,
	)

	rule := &Rule{
		Services: []*loadbalance.Service{service1, service2},
	}

	// Should select service2 because service1 has insufficient samples
	got := tactic.SelectService(rule)
	if got == nil {
		t.Fatal("SelectService() returned nil")
	}
	if got.ServiceID() != "provider2:model2" {
		t.Errorf("SelectService() = %v, want provider2:model2", got.ServiceID())
	}
}

func TestSpeedBasedTactic_Defaults(t *testing.T) {
	tactic := NewSpeedBasedTactic(0, 0, 0)

	if tactic.MinSamplesRequired != constant.DefaultMinSpeedSamples {
		t.Errorf("Expected default MinSamplesRequired = %d, got %d",
			constant.DefaultMinSpeedSamples, tactic.MinSamplesRequired)
	}
	if tactic.SpeedThresholdTps != constant.DefaultSpeedThresholdTps {
		t.Errorf("Expected default SpeedThresholdTps = %f, got %f",
			constant.DefaultSpeedThresholdTps, tactic.SpeedThresholdTps)
	}
	if tactic.SampleWindowSize != constant.DefaultSpeedSampleWindow {
		t.Errorf("Expected default SampleWindowSize = %d, got %d",
			constant.DefaultSpeedSampleWindow, tactic.SampleWindowSize)
	}
}

func TestSpeedBasedTactic_GetName(t *testing.T) {
	tactic := NewSpeedBasedTactic(
		constant.DefaultMinSpeedSamples,
		constant.DefaultSpeedThresholdTps,
		constant.DefaultSpeedSampleWindow,
	)

	if got := tactic.GetName(); got != "Speed Based" {
		t.Errorf("GetName() = %v, want %v", got, "Speed Based")
	}
}

func TestSpeedBasedTactic_GetType(t *testing.T) {
	tactic := NewSpeedBasedTactic(
		constant.DefaultMinSpeedSamples,
		constant.DefaultSpeedThresholdTps,
		constant.DefaultSpeedSampleWindow,
	)

	if got := tactic.GetType(); got != loadbalance.TacticSpeedBased {
		t.Errorf("GetType() = %v, want %v", got, loadbalance.TacticSpeedBased)
	}
}

func TestDefaultSpeedBasedParams(t *testing.T) {
	params := DefaultSpeedBasedParams()
	sp, ok := params.(SpeedBasedParams)
	if !ok {
		t.Fatal("DefaultSpeedBasedParams() did not return SpeedBasedParams")
	}

	if sp.MinSamplesRequired != constant.DefaultMinSpeedSamples {
		t.Errorf("Expected MinSamplesRequired = %d, got %d",
			constant.DefaultMinSpeedSamples, sp.MinSamplesRequired)
	}
	if sp.SpeedThresholdTps != constant.DefaultSpeedThresholdTps {
		t.Errorf("Expected SpeedThresholdTps = %f, got %f",
			constant.DefaultSpeedThresholdTps, sp.SpeedThresholdTps)
	}
	if sp.SampleWindowSize != constant.DefaultSpeedSampleWindow {
		t.Errorf("Expected SampleWindowSize = %d, got %d",
			constant.DefaultSpeedSampleWindow, sp.SampleWindowSize)
	}
}

func TestAsSpeedBasedParams(t *testing.T) {
	// Test with correct type
	params := SpeedBasedParams{
		MinSamplesRequired: 10,
		SpeedThresholdTps:  75.0,
		SampleWindowSize:   50,
	}

	sp, ok := AsSpeedBasedParams(params)
	if !ok {
		t.Error("AsSpeedBasedParams() returned false for SpeedBasedParams")
	}
	if sp.MinSamplesRequired != 10 {
		t.Errorf("Expected MinSamplesRequired = 10, got %d", sp.MinSamplesRequired)
	}

	// Test with wrong type
	rp := RoundRobinParams{}
	_, ok = AsSpeedBasedParams(rp)
	if ok {
		t.Error("AsSpeedBasedParams() returned true for RoundRobinParams")
	}
}

func TestParseTacticFromMap_SpeedBased(t *testing.T) {
	params := map[string]interface{}{
		"min_samples_required": int64(10),
		"speed_threshold_tps":  75.0,
		"sample_window_size":   int64(50),
	}

	tactic := ParseTacticFromMap(loadbalance.TacticSpeedBased, params)

	if tactic.Type != loadbalance.TacticSpeedBased {
		t.Errorf("Expected tactic type = TacticSpeedBased, got %v", tactic.Type)
	}

	sp, ok := AsSpeedBasedParams(tactic.Params)
	if !ok {
		t.Fatal("Failed to convert params to SpeedBasedParams")
	}

	if sp.MinSamplesRequired != 10 {
		t.Errorf("Expected MinSamplesRequired = 10, got %d", sp.MinSamplesRequired)
	}
	if sp.SpeedThresholdTps != 75.0 {
		t.Errorf("Expected SpeedThresholdTps = 75.0, got %f", sp.SpeedThresholdTps)
	}
	if sp.SampleWindowSize != 50 {
		t.Errorf("Expected SampleWindowSize = 50, got %d", sp.SampleWindowSize)
	}
}

func TestIsValidTactic_SpeedBased(t *testing.T) {
	if !IsValidTactic("speed_based") {
		t.Error("IsValidTactic(\"speed_based\") returned false")
	}
	if !IsValidTactic("SPEED_BASED") {
		t.Error("IsValidTactic(\"SPEED_BASED\") returned false (should be case-insensitive)")
	}
}
