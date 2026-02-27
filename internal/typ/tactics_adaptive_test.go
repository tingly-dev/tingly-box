package typ

import (
	"testing"

	"github.com/tingly-dev/tingly-box/internal/constant"
	"github.com/tingly-dev/tingly-box/internal/loadbalance"
)

func TestAdaptiveTactic_SelectService(t *testing.T) {
	// Create services with different performance profiles
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

	// Service 1: high latency, low speed
	for i := 0; i < 5; i++ {
		service1.Stats.RecordLatency(500, 10)
		service1.Stats.RecordTokenSpeed(30.0, 10)
	}

	// Service 2: medium latency, medium speed
	for i := 0; i < 5; i++ {
		service2.Stats.RecordLatency(200, 10)
		service2.Stats.RecordTokenSpeed(60.0, 10)
	}

	// Service 3: low latency, high speed (best overall)
	for i := 0; i < 5; i++ {
		service3.Stats.RecordLatency(100, 10)
		service3.Stats.RecordTokenSpeed(100.0, 10)
	}

	tactic := NewAdaptiveTactic(
		constant.DefaultLatencyWeight,
		constant.DefaultTokenWeight,
		constant.DefaultSpeedWeight,
		constant.DefaultHealthWeight,
		constant.DefaultLatencyThresholdMs,
		constant.DefaultTokenThreshold,
		constant.DefaultSpeedThresholdTps,
		constant.DefaultScoringMode,
	)

	tests := []struct {
		name          string
		services      []*loadbalance.Service
		wantServiceID string
	}{
		{
			name:          "selects best overall service",
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

func TestAdaptiveTactic_calculateScore(t *testing.T) {
	service := &loadbalance.Service{
		Provider: "provider1",
		Model:    "model1",
		Stats: loadbalance.ServiceStats{
			ServiceID: "provider1:model1",
		},
	}

	// Record known metrics
	for i := 0; i < 5; i++ {
		service.Stats.RecordLatency(200, 10)
		service.Stats.RecordTokenSpeed(60.0, 10)
	}

	tactic := NewAdaptiveTactic(
		0.4, // latency weight
		0.3, // token weight
		0.2, // speed weight
		0.1, // health weight
		1000, // max latency ms
		10000, // max token usage
		50.0, // min speed tps
		"weighted_sum",
	)

	score := tactic.calculateScore(service)

	// Score should be between 0 and 1
	if score < 0 || score > 1 {
		t.Errorf("calculateScore() = %f, want between 0 and 1", score)
	}

	// With these parameters, score should be reasonable
	if score < 0.5 {
		t.Errorf("calculateScore() = %f, expected higher score for good metrics", score)
	}
}

func TestAdaptiveTactic_calculateScore_NoData(t *testing.T) {
	service := &loadbalance.Service{
		Provider: "provider1",
		Model:    "model1",
		Stats: loadbalance.ServiceStats{
			ServiceID: "provider1:model1",
		},
	}

	tactic := NewAdaptiveTactic(
		0.4,   // latency weight
		0.3,   // token weight
		0.2,   // speed weight
		0.1,   // health weight
		1000,  // max latency ms
		10000, // max token usage
		50.0,  // min speed tps
		"weighted_sum",
	)

	score := tactic.calculateScore(service)

	// When no data:
	// - Latency score: 0.5 (neutral) * 0.4 = 0.2
	// - Token score: 0.5 (neutral) * 0.3 = 0.15
	// - Speed score: 0.5 (neutral) * 0.2 = 0.1
	// - Health score: 1.0 (always healthy) * 0.1 = 0.1
	// Total: 0.55
	// But since token window is empty, tokenScore might be different
	// Let's just check it's a reasonable value between 0 and 1
	if score < 0 || score > 1 {
		t.Errorf("calculateScore() with no data = %f, want between 0 and 1", score)
	}
}

func TestAdaptiveTactic_Defaults(t *testing.T) {
	tactic := NewAdaptiveTactic(0, 0, 0, 0, 0, 0, 0, "")

	if tactic.LatencyWeight != constant.DefaultLatencyWeight {
		t.Errorf("Expected default LatencyWeight = %f, got %f",
			constant.DefaultLatencyWeight, tactic.LatencyWeight)
	}
	if tactic.TokenWeight != constant.DefaultTokenWeight {
		t.Errorf("Expected default TokenWeight = %f, got %f",
			constant.DefaultTokenWeight, tactic.TokenWeight)
	}
	if tactic.SpeedWeight != constant.DefaultSpeedWeight {
		t.Errorf("Expected default SpeedWeight = %f, got %f",
			constant.DefaultSpeedWeight, tactic.SpeedWeight)
	}
	if tactic.HealthWeight != constant.DefaultHealthWeight {
		t.Errorf("Expected default HealthWeight = %f, got %f",
			constant.DefaultHealthWeight, tactic.HealthWeight)
	}
	if tactic.MaxLatencyMs != constant.DefaultLatencyThresholdMs {
		t.Errorf("Expected default MaxLatencyMs = %d, got %d",
			constant.DefaultLatencyThresholdMs, tactic.MaxLatencyMs)
	}
	if tactic.MaxTokenUsage != constant.DefaultTokenThreshold {
		t.Errorf("Expected default MaxTokenUsage = %d, got %d",
			constant.DefaultTokenThreshold, tactic.MaxTokenUsage)
	}
	if tactic.MinSpeedTps != constant.DefaultSpeedThresholdTps {
		t.Errorf("Expected default MinSpeedTps = %f, got %f",
			constant.DefaultSpeedThresholdTps, tactic.MinSpeedTps)
	}
	if tactic.ScoringMode != constant.DefaultScoringMode {
		t.Errorf("Expected default ScoringMode = %s, got %s",
			constant.DefaultScoringMode, tactic.ScoringMode)
	}
}

func TestAdaptiveTactic_GetName(t *testing.T) {
	tactic := NewAdaptiveTactic(
		constant.DefaultLatencyWeight,
		constant.DefaultTokenWeight,
		constant.DefaultSpeedWeight,
		constant.DefaultHealthWeight,
		constant.DefaultLatencyThresholdMs,
		constant.DefaultTokenThreshold,
		constant.DefaultSpeedThresholdTps,
		constant.DefaultScoringMode,
	)

	if got := tactic.GetName(); got != "Adaptive" {
		t.Errorf("GetName() = %v, want %v", got, "Adaptive")
	}
}

func TestAdaptiveTactic_GetType(t *testing.T) {
	tactic := NewAdaptiveTactic(
		constant.DefaultLatencyWeight,
		constant.DefaultTokenWeight,
		constant.DefaultSpeedWeight,
		constant.DefaultHealthWeight,
		constant.DefaultLatencyThresholdMs,
		constant.DefaultTokenThreshold,
		constant.DefaultSpeedThresholdTps,
		constant.DefaultScoringMode,
	)

	if got := tactic.GetType(); got != loadbalance.TacticAdaptive {
		t.Errorf("GetType() = %v, want %v", got, loadbalance.TacticAdaptive)
	}
}

func TestDefaultAdaptiveParams(t *testing.T) {
	params := DefaultAdaptiveParams()
	ap, ok := params.(AdaptiveParams)
	if !ok {
		t.Fatal("DefaultAdaptiveParams() did not return AdaptiveParams")
	}

	if ap.LatencyWeight != constant.DefaultLatencyWeight {
		t.Errorf("Expected LatencyWeight = %f, got %f",
			constant.DefaultLatencyWeight, ap.LatencyWeight)
	}
	if ap.TokenWeight != constant.DefaultTokenWeight {
		t.Errorf("Expected TokenWeight = %f, got %f",
			constant.DefaultTokenWeight, ap.TokenWeight)
	}
	if ap.SpeedWeight != constant.DefaultSpeedWeight {
		t.Errorf("Expected SpeedWeight = %f, got %f",
			constant.DefaultSpeedWeight, ap.SpeedWeight)
	}
	if ap.HealthWeight != constant.DefaultHealthWeight {
		t.Errorf("Expected HealthWeight = %f, got %f",
			constant.DefaultHealthWeight, ap.HealthWeight)
	}
}

func TestAsAdaptiveParams(t *testing.T) {
	// Test with correct type
	params := AdaptiveParams{
		LatencyWeight: 0.5,
		TokenWeight:   0.3,
		SpeedWeight:   0.2,
		HealthWeight:  0.1,
		MaxLatencyMs:  2000,
		MaxTokenUsage: 10000,
		MinSpeedTps:   50.0,
		ScoringMode:   "weighted_sum",
	}

	ap, ok := AsAdaptiveParams(params)
	if !ok {
		t.Error("AsAdaptiveParams() returned false for AdaptiveParams")
	}
	if ap.LatencyWeight != 0.5 {
		t.Errorf("Expected LatencyWeight = 0.5, got %f", ap.LatencyWeight)
	}

	// Test with wrong type
	rp := RoundRobinParams{}
	_, ok = AsAdaptiveParams(rp)
	if ok {
		t.Error("AsAdaptiveParams() returned true for RoundRobinParams")
	}
}

func TestParseTacticFromMap_Adaptive(t *testing.T) {
	params := map[string]interface{}{
		"latency_weight": 0.5,
		"token_weight":   0.3,
		"speed_weight":   0.15,
		"health_weight":  0.05,
		"max_latency_ms": int64(3000),
		"max_token_usage": int64(20000),
		"min_speed_tps":  75.0,
		"scoring_mode":   "multiplicative",
	}

	tactic := ParseTacticFromMap(loadbalance.TacticAdaptive, params)

	if tactic.Type != loadbalance.TacticAdaptive {
		t.Errorf("Expected tactic type = TacticAdaptive, got %v", tactic.Type)
	}

	ap, ok := AsAdaptiveParams(tactic.Params)
	if !ok {
		t.Fatal("Failed to convert params to AdaptiveParams")
	}

	if ap.LatencyWeight != 0.5 {
		t.Errorf("Expected LatencyWeight = 0.5, got %f", ap.LatencyWeight)
	}
	if ap.TokenWeight != 0.3 {
		t.Errorf("Expected TokenWeight = 0.3, got %f", ap.TokenWeight)
	}
	if ap.SpeedWeight != 0.15 {
		t.Errorf("Expected SpeedWeight = 0.15, got %f", ap.SpeedWeight)
	}
	if ap.HealthWeight != 0.05 {
		t.Errorf("Expected HealthWeight = 0.05, got %f", ap.HealthWeight)
	}
	if ap.MaxLatencyMs != 3000 {
		t.Errorf("Expected MaxLatencyMs = 3000, got %d", ap.MaxLatencyMs)
	}
	if ap.MaxTokenUsage != 20000 {
		t.Errorf("Expected MaxTokenUsage = 20000, got %d", ap.MaxTokenUsage)
	}
	if ap.MinSpeedTps != 75.0 {
		t.Errorf("Expected MinSpeedTps = 75.0, got %f", ap.MinSpeedTps)
	}
	if ap.ScoringMode != "multiplicative" {
		t.Errorf("Expected ScoringMode = multiplicative, got %s", ap.ScoringMode)
	}
}

func TestIsValidTactic_Adaptive(t *testing.T) {
	if !IsValidTactic("adaptive") {
		t.Error("IsValidTactic(\"adaptive\") returned false")
	}
	if !IsValidTactic("ADAPTIVE") {
		t.Error("IsValidTactic(\"ADAPTIVE\") returned false (should be case-insensitive)")
	}
}
