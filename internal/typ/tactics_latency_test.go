package typ

import (
	"testing"

	"github.com/tingly-dev/tingly-box/internal/constant"
	"github.com/tingly-dev/tingly-box/internal/loadbalance"
)

// mockRuleForLatency is a minimal mock of Rule for latency testing
type mockRuleForLatency struct {
	services         []*loadbalance.Service
	currentServiceID string
}

func (m *mockRuleForLatency) GetActiveServices() []*loadbalance.Service {
	var active []*loadbalance.Service
	for i := range m.services {
		if m.services[i].Active {
			active = append(active, m.services[i])
		}
	}
	return active
}

func (m *mockRuleForLatency) GetCurrentServiceID() string {
	return m.currentServiceID
}

func TestLatencyBasedTactic_SelectService(t *testing.T) {
	// Create services with different latency profiles
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

	// Record different latencies for each service
	// Service 1: high latency (avg ~200ms)
	for i := 0; i < 5; i++ {
		service1.Stats.RecordLatency(200, 10)
	}

	// Service 2: medium latency (avg ~150ms)
	for i := 0; i < 5; i++ {
		service2.Stats.RecordLatency(150, 10)
	}

	// Service 3: low latency (avg ~100ms)
	for i := 0; i < 5; i++ {
		service3.Stats.RecordLatency(100, 10)
	}

	tests := []struct {
		name             string
		services         []*loadbalance.Service
		currentServiceID string
		thresholdMs      int64
		wantServiceID    string
	}{
		{
			name:             "selects lowest latency service when threshold exceeded",
			services:         []*loadbalance.Service{service1, service2, service3},
			currentServiceID: "provider1:model1", // Current has high latency (~200ms)
			thresholdMs:      150,                // Threshold below current latency
			wantServiceID:    "provider3:model3", // Should switch to lowest latency
		},
		{
			name:             "keeps current if within threshold",
			services:         []*loadbalance.Service{service1, service2, service3},
			currentServiceID: "provider3:model3", // Current has lowest latency (~100ms)
			thresholdMs:      2000,               // High threshold
			wantServiceID:    "provider3:model3", // Should keep current
		},
		{
			name:             "handles single service",
			services:         []*loadbalance.Service{service1},
			currentServiceID: "provider1:model1",
			thresholdMs:      2000,
			wantServiceID:    "provider1:model1",
		},
		{
			name:             "handles no active services",
			services:         []*loadbalance.Service{},
			currentServiceID: "",
			thresholdMs:      2000,
			wantServiceID:    "",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			tactic := NewLatencyBasedTactic(
				tt.thresholdMs,
				constant.DefaultLatencySampleWindow,
				constant.DefaultLatencyPercentile,
				"avg",
			)
			rule := &Rule{
				Services:         tt.services,
				CurrentServiceID: tt.currentServiceID,
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

func TestLatencyBasedTactic_getLatencyForService(t *testing.T) {
	service := &loadbalance.Service{
		Provider: "provider1",
		Model:    "model1",
		Stats: loadbalance.ServiceStats{
			ServiceID: "provider1:model1",
		},
	}

	// Record known latencies: 100, 150, 200, 250, 300
	latencies := []int64{100, 150, 200, 250, 300}
	for _, lat := range latencies {
		service.Stats.RecordLatency(lat, 10)
	}

	// Expected values:
	// avg = (100+150+200+250+300)/5 = 200
	// p50 (median) = 200
	// p95 = 290 (interpolated between 250 and 300)
	// p99 = 298 (interpolated between 250 and 300)

	tests := []struct {
		name           string
		comparisonMode string
		wantMin        float64
		wantMax        float64
	}{
		{
			name:           "avg mode",
			comparisonMode: "avg",
			wantMin:        195,
			wantMax:        205,
		},
		{
			name:           "p50 mode",
			comparisonMode: "p50",
			wantMin:        195,
			wantMax:        205,
		},
		{
			name:           "p95 mode",
			comparisonMode: "p95",
			wantMin:        285,
			wantMax:        295,
		},
		{
			name:           "p99 mode",
			comparisonMode: "p99",
			wantMin:        295,
			wantMax:        300,
		},
		{
			name:           "default mode (avg)",
			comparisonMode: "unknown",
			wantMin:        195,
			wantMax:        205,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			tactic := NewLatencyBasedTactic(
				constant.DefaultLatencyThresholdMs,
				constant.DefaultLatencySampleWindow,
				constant.DefaultLatencyPercentile,
				tt.comparisonMode,
			)

			got := tactic.getLatencyForService(service)
			if got < tt.wantMin || got > tt.wantMax {
				t.Errorf("getLatencyForService() = %v, want between %v and %v", got, tt.wantMin, tt.wantMax)
			}
		})
	}
}

func TestLatencyBasedTactic_getLatencyForService_NoSamples(t *testing.T) {
	service := &loadbalance.Service{
		Provider: "provider1",
		Model:    "model1",
		Stats: loadbalance.ServiceStats{
			ServiceID: "provider1:model1",
		},
	}

	tactic := NewLatencyBasedTactic(
		2000, // threshold
		constant.DefaultLatencySampleWindow,
		constant.DefaultLatencyPercentile,
		"avg",
	)

	// When no samples, should return threshold * 2 to deprioritize
	got := tactic.getLatencyForService(service)
	want := float64(4000)

	if got != want {
		t.Errorf("getLatencyForService() with no samples = %v, want %v", got, want)
	}
}

func TestLatencyBasedTactic_Defaults(t *testing.T) {
	tactic := NewLatencyBasedTactic(0, 0, 0, "")

	if tactic.LatencyThresholdMs != constant.DefaultLatencyThresholdMs {
		t.Errorf("Expected default LatencyThresholdMs = %d, got %d",
			constant.DefaultLatencyThresholdMs, tactic.LatencyThresholdMs)
	}
	if tactic.SampleWindowSize != constant.DefaultLatencySampleWindow {
		t.Errorf("Expected default SampleWindowSize = %d, got %d",
			constant.DefaultLatencySampleWindow, tactic.SampleWindowSize)
	}
	if tactic.Percentile != constant.DefaultLatencyPercentile {
		t.Errorf("Expected default Percentile = %f, got %f",
			constant.DefaultLatencyPercentile, tactic.Percentile)
	}
	if tactic.ComparisonMode != constant.DefaultLatencyComparisonMode {
		t.Errorf("Expected default ComparisonMode = %s, got %s",
			constant.DefaultLatencyComparisonMode, tactic.ComparisonMode)
	}
}

func TestLatencyBasedTactic_GetName(t *testing.T) {
	tactic := NewLatencyBasedTactic(
		constant.DefaultLatencyThresholdMs,
		constant.DefaultLatencySampleWindow,
		constant.DefaultLatencyPercentile,
		"avg",
	)

	if got := tactic.GetName(); got != "Latency Based" {
		t.Errorf("GetName() = %v, want %v", got, "Latency Based")
	}
}

func TestLatencyBasedTactic_GetType(t *testing.T) {
	tactic := NewLatencyBasedTactic(
		constant.DefaultLatencyThresholdMs,
		constant.DefaultLatencySampleWindow,
		constant.DefaultLatencyPercentile,
		"avg",
	)

	if got := tactic.GetType(); got != loadbalance.TacticLatencyBased {
		t.Errorf("GetType() = %v, want %v", got, loadbalance.TacticLatencyBased)
	}
}

func TestDefaultLatencyBasedParams(t *testing.T) {
	params := DefaultLatencyBasedParams()
	lp, ok := params.(LatencyBasedParams)
	if !ok {
		t.Fatal("DefaultLatencyBasedParams() did not return LatencyBasedParams")
	}

	if lp.LatencyThresholdMs != constant.DefaultLatencyThresholdMs {
		t.Errorf("Expected LatencyThresholdMs = %d, got %d",
			constant.DefaultLatencyThresholdMs, lp.LatencyThresholdMs)
	}
	if lp.SampleWindowSize != constant.DefaultLatencySampleWindow {
		t.Errorf("Expected SampleWindowSize = %d, got %d",
			constant.DefaultLatencySampleWindow, lp.SampleWindowSize)
	}
	if lp.Percentile != constant.DefaultLatencyPercentile {
		t.Errorf("Expected Percentile = %f, got %f",
			constant.DefaultLatencyPercentile, lp.Percentile)
	}
	if lp.ComparisonMode != constant.DefaultLatencyComparisonMode {
		t.Errorf("Expected ComparisonMode = %s, got %s",
			constant.DefaultLatencyComparisonMode, lp.ComparisonMode)
	}
}

func TestAsLatencyBasedParams(t *testing.T) {
	// Test with correct type
	params := LatencyBasedParams{
		LatencyThresholdMs: 1000,
		SampleWindowSize:   50,
		Percentile:         0.90,
		ComparisonMode:     "p95",
	}

	lp, ok := AsLatencyBasedParams(params)
	if !ok {
		t.Error("AsLatencyBasedParams() returned false for LatencyBasedParams")
	}
	if lp.LatencyThresholdMs != 1000 {
		t.Errorf("Expected LatencyThresholdMs = 1000, got %d", lp.LatencyThresholdMs)
	}

	// Test with wrong type
	rp := RoundRobinParams{}
	_, ok = AsLatencyBasedParams(rp)
	if ok {
		t.Error("AsLatencyBasedParams() returned true for RoundRobinParams")
	}
}

func TestParseTacticFromMap_LatencyBased(t *testing.T) {
	params := map[string]interface{}{
		"latency_threshold_ms": int64(1500),
		"sample_window_size":   int64(50),
		"percentile":           0.90,
		"comparison_mode":      "p95",
	}

	tactic := ParseTacticFromMap(loadbalance.TacticLatencyBased, params)

	if tactic.Type != loadbalance.TacticLatencyBased {
		t.Errorf("Expected tactic type = TacticLatencyBased, got %v", tactic.Type)
	}

	lp, ok := AsLatencyBasedParams(tactic.Params)
	if !ok {
		t.Fatal("Failed to convert params to LatencyBasedParams")
	}

	if lp.LatencyThresholdMs != 1500 {
		t.Errorf("Expected LatencyThresholdMs = 1500, got %d", lp.LatencyThresholdMs)
	}
	if lp.SampleWindowSize != 50 {
		t.Errorf("Expected SampleWindowSize = 50, got %d", lp.SampleWindowSize)
	}
	if lp.Percentile != 0.90 {
		t.Errorf("Expected Percentile = 0.90, got %f", lp.Percentile)
	}
	if lp.ComparisonMode != "p95" {
		t.Errorf("Expected ComparisonMode = p95, got %s", lp.ComparisonMode)
	}
}

func TestIsValidTactic_LatencyBased(t *testing.T) {
	if !IsValidTactic("latency_based") {
		t.Error("IsValidTactic(\"latency_based\") returned false")
	}
	if !IsValidTactic("LATENCY_BASED") {
		t.Error("IsValidTactic(\"LATENCY_BASED\") returned false (should be case-insensitive)")
	}
}
