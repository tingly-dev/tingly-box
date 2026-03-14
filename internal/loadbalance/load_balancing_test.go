package loadbalance

import (
	"testing"
	"time"
)

// mockRule is a minimal mock of typ.Rule for testing
type mockRule struct {
	services []Service
}

func (m *mockRule) GetServices() []Service {
	if m.services == nil {
		return []Service{}
	}
	return m.services
}

func TestService_ServiceID(t *testing.T) {
	service := Service{
		Provider: "openai",
		Model:    "gpt-4",
	}

	expected := "openai:gpt-4"
	if got := service.ServiceID(); got != expected {
		t.Errorf("Service.ServiceID() = %v, want %v", got, expected)
	}
}

func TestServiceStats_RecordUsage(t *testing.T) {
	stats := &ServiceStats{
		ServiceID:   "test:provider",
		TimeWindow:  60, // 1 minute for testing
		WindowStart: time.Now(),
	}

	// Record initial usage
	stats.RecordUsage(80, 20) // 80 input, 20 output tokens
	if stats.RequestCount != 1 {
		t.Errorf("Expected RequestCount = 1, got %d", stats.RequestCount)
	}
	if stats.WindowTokensConsumed != 100 {
		t.Errorf("Expected WindowTokensConsumed = 100, got %d", stats.WindowTokensConsumed)
	}

	// Record second usage
	stats.RecordUsage(150, 50) // 150 input, 50 output tokens
	if stats.RequestCount != 2 {
		t.Errorf("Expected RequestCount = 2, got %d", stats.RequestCount)
	}
	if stats.WindowTokensConsumed != 300 {
		t.Errorf("Expected WindowTokensConsumed = 300, got %d", stats.WindowTokensConsumed)
	}

	// Check window stats
	requests, tokens := stats.GetWindowStats()
	if requests != 2 {
		t.Errorf("Expected window requests = 2, got %d", requests)
	}
	if tokens != 300 {
		t.Errorf("Expected window tokens = 300, got %d", tokens)
	}

	// Check detailed token stats
	requests, inputTokens, outputTokens := stats.GetWindowTokenDetails()
	if inputTokens != 230 {
		t.Errorf("Expected window input tokens = 230, got %d", inputTokens)
	}
	if outputTokens != 70 {
		t.Errorf("Expected window output tokens = 70, got %d", outputTokens)
	}
}

func TestServiceStats_WindowReset(t *testing.T) {
	stats := &ServiceStats{
		ServiceID:   "test:provider",
		TimeWindow:  1,                                // 1 second for testing
		WindowStart: time.Now().Add(-2 * time.Second), // Start 2 seconds ago
	}

	// Record usage to trigger window reset
	stats.RecordUsage(30, 20)

	// Window should be reset
	requests, tokens := stats.GetWindowStats()
	if requests != 1 {
		t.Errorf("Expected window requests = 1 after reset, got %d", requests)
	}
	if tokens != 50 {
		t.Errorf("Expected window tokens = 50 after reset, got %d", tokens)
	}
}

func TestServiceStats_ResetWindow(t *testing.T) {
	stats := &ServiceStats{
		ServiceID:            "test:provider",
		TimeWindow:           60,
		RequestCount:         10,
		WindowStart:          time.Now(),
		WindowRequestCount:   5,
		WindowTokensConsumed: 500,
		WindowInputTokens:    300,
		WindowOutputTokens:   200,
	}

	// Reset window
	stats.ResetWindow()

	// Check total stats remain unchanged
	if stats.RequestCount != 10 {
		t.Errorf("Expected total RequestCount = 10, got %d", stats.RequestCount)
	}

	// Check window stats are reset
	requests, tokens := stats.GetWindowStats()
	if requests != 0 {
		t.Errorf("Expected window requests = 0 after reset, got %d", requests)
	}
	if tokens != 0 {
		t.Errorf("Expected window tokens = 0 after reset, got %d", tokens)
	}

	// Check detailed window stats are reset
	requests, inputTokens, outputTokens := stats.GetWindowTokenDetails()
	if inputTokens != 0 {
		t.Errorf("Expected window input tokens = 0 after reset, got %d", inputTokens)
	}
	if outputTokens != 0 {
		t.Errorf("Expected window output tokens = 0 after reset, got %d", outputTokens)
	}
}

func TestParseTacticType(t *testing.T) {
	tests := []struct {
		input    string
		expected TacticType
	}{
		{"round_robin", TacticRoundRobin},
		{"token_based", TacticTokenBased},
		{"hybrid", TacticHybrid},
		{"random", TacticRandom},
		{"invalid", TacticRoundRobin}, // Default fallback
		{"", TacticRoundRobin},        // Empty string fallback
	}

	for _, test := range tests {
		if got := ParseTacticType(test.input); got != test.expected {
			t.Errorf("ParseTacticType(%s) = %v, want %v", test.input, got, test.expected)
		}
	}
}

func TestTacticType_String(t *testing.T) {
	tests := map[TacticType]string{
		TacticRoundRobin: "round_robin",
		TacticTokenBased: "token_based",
		TacticHybrid:     "hybrid",
		TacticRandom:     "random",
		TacticType(999):  "unknown", // Invalid type
	}

	for tacticType, expected := range tests {
		if got := tacticType.String(); got != expected {
			t.Errorf("TacticType(%d).String() = %v, want %v", tacticType, got, expected)
		}
	}
}

func TestService_PreferCompletions(t *testing.T) {
	tests := []struct {
		name     string
		model    string
		expected bool
	}{
		{"codex model lowercase", "codex-3", true},
		{"codex model uppercase", "CODEX-3", true},
		{"codex model mixed case", "CoDeX-3", true},
		{"gpt-4 model", "gpt-4", false},
		{"claude model", "claude-3", false},
		{"empty string", "", false},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			service := Service{
				Model: tt.model,
			}
			if got := service.PreferCompletions(); got != tt.expected {
				t.Errorf("Service.PreferCompletions() for model %s = %v, want %v", tt.model, got, tt.expected)
			}
		})
	}
}

func TestServiceStats_RecordLatency(t *testing.T) {
	stats := &ServiceStats{
		ServiceID:   "test:provider",
		TimeWindow:  60,
		WindowStart: time.Now(),
	}

	// Record some latency samples
	samples := []int64{100, 150, 200, 120, 180}
	for _, sample := range samples {
		stats.RecordLatency(sample, 10) // max 10 samples
	}

	// Check that stats were calculated
	avg, p50, p95, p99, count := stats.GetLatencyStats()

	if count != 5 {
		t.Errorf("Expected sample count = 5, got %d", count)
	}

	// Average should be around 150
	if avg < 140 || avg > 160 {
		t.Errorf("Expected avg around 150, got %f", avg)
	}

	// P50 (median) should be around 150
	if p50 < 140 || p50 > 160 {
		t.Errorf("Expected p50 around 150, got %f", p50)
	}

	// P95 and P99 should be higher
	if p95 < 190 {
		t.Errorf("Expected p95 >= 190, got %f", p95)
	}
	if p99 < 195 {
		t.Errorf("Expected p99 >= 195, got %f", p99)
	}
}

func TestServiceStats_RecordLatency_RollingWindow(t *testing.T) {
	stats := &ServiceStats{
		ServiceID:   "test:provider",
		TimeWindow:  60,
		WindowStart: time.Now(),
	}

	// Record more samples than the window size
	for i := 0; i < 15; i++ {
		stats.RecordLatency(int64(100+i*10), 10) // max 10 samples
	}

	// Check that only the last 10 samples are kept
	avg, _, _, _, count := stats.GetLatencyStats()

	if count != 10 {
		t.Errorf("Expected sample count = 10 (window size), got %d", count)
	}

	// Average should be based on the last 10 samples: 150-240
	// (150+160+170+180+190+200+210+220+230+240) / 10 = 195
	if avg < 190 || avg > 200 {
		t.Errorf("Expected avg around 195, got %f", avg)
	}
}

func TestServiceStats_RecordLatency_Empty(t *testing.T) {
	stats := &ServiceStats{
		ServiceID:   "test:provider",
		TimeWindow:  60,
		WindowStart: time.Now(),
	}

	// Don't record any samples
	avg, p50, p95, p99, count := stats.GetLatencyStats()

	if count != 0 {
		t.Errorf("Expected sample count = 0, got %d", count)
	}
	if avg != 0 {
		t.Errorf("Expected avg = 0 for empty samples, got %f", avg)
	}
	if p50 != 0 {
		t.Errorf("Expected p50 = 0 for empty samples, got %f", p50)
	}
	if p95 != 0 {
		t.Errorf("Expected p95 = 0 for empty samples, got %f", p95)
	}
	if p99 != 0 {
		t.Errorf("Expected p99 = 0 for empty samples, got %f", p99)
	}
}

func TestParseTacticType_LatencyBased(t *testing.T) {
	tests := []struct {
		input    string
		expected TacticType
	}{
		{"latency_based", TacticLatencyBased},
		{"LATENCY_BASED", TacticRoundRobin}, // Case sensitive, falls back to default
		{"invalid", TacticRoundRobin},       // Default fallback
	}

	for _, test := range tests {
		if got := ParseTacticType(test.input); got != test.expected {
			t.Errorf("ParseTacticType(%s) = %v, want %v", test.input, got, test.expected)
		}
	}
}

func TestTacticType_String_LatencyBased(t *testing.T) {
	if got := TacticLatencyBased.String(); got != "latency_based" {
		t.Errorf("TacticLatencyBased.String() = %v, want %v", got, "latency_based")
	}
}

func TestServiceStats_RecordTokenSpeed(t *testing.T) {
	stats := &ServiceStats{
		ServiceID:   "test:provider",
		TimeWindow:  60,
		WindowStart: time.Now(),
	}

	// Record some speed samples
	samples := []float64{50.0, 75.0, 100.0, 60.0, 80.0}
	for _, sample := range samples {
		stats.RecordTokenSpeed(sample, 10) // max 10 samples
	}

	// Check that stats were calculated
	avg, count := stats.GetTokenSpeedStats()

	if count != 5 {
		t.Errorf("Expected sample count = 5, got %d", count)
	}

	// Average should be around 73
	if avg < 70 || avg > 80 {
		t.Errorf("Expected avg around 73, got %f", avg)
	}
}

func TestServiceStats_RecordTokenSpeed_RollingWindow(t *testing.T) {
	stats := &ServiceStats{
		ServiceID:   "test:provider",
		TimeWindow:  60,
		WindowStart: time.Now(),
	}

	// Record more samples than the window size
	for i := 0; i < 15; i++ {
		stats.RecordTokenSpeed(float64(50+i*10), 10) // max 10 samples
	}

	// Check that only the last 10 samples are kept
	avg, count := stats.GetTokenSpeedStats()

	if count != 10 {
		t.Errorf("Expected sample count = 10 (window size), got %d", count)
	}

	// Average should be based on the last 10 samples: 100-190
	// (100+110+120+130+140+150+160+170+180+190) / 10 = 145
	if avg < 140 || avg > 150 {
		t.Errorf("Expected avg around 145, got %f", avg)
	}
}

func TestServiceStats_RecordTokenSpeed_Empty(t *testing.T) {
	stats := &ServiceStats{
		ServiceID:   "test:provider",
		TimeWindow:  60,
		WindowStart: time.Now(),
	}

	// Don't record any samples
	avg, count := stats.GetTokenSpeedStats()

	if count != 0 {
		t.Errorf("Expected sample count = 0, got %d", count)
	}
	if avg != 0 {
		t.Errorf("Expected avg = 0 for empty samples, got %f", avg)
	}
}

func TestParseTacticType_SpeedBased(t *testing.T) {
	tests := []struct {
		input    string
		expected TacticType
	}{
		{"speed_based", TacticSpeedBased},
		{"SPEED_BASED", TacticRoundRobin}, // Case sensitive, falls back to default
		{"invalid", TacticRoundRobin},     // Default fallback
	}

	for _, test := range tests {
		if got := ParseTacticType(test.input); got != test.expected {
			t.Errorf("ParseTacticType(%s) = %v, want %v", test.input, got, test.expected)
		}
	}
}

func TestTacticType_String_SpeedBased(t *testing.T) {
	if got := TacticSpeedBased.String(); got != "speed_based" {
		t.Errorf("TacticSpeedBased.String() = %v, want %v", got, "speed_based")
	}
}

func TestParseTacticType_Adaptive(t *testing.T) {
	tests := []struct {
		input    string
		expected TacticType
	}{
		{"adaptive", TacticAdaptive},
		{"ADAPTIVE", TacticRoundRobin}, // Case sensitive, falls back to default
		{"invalid", TacticRoundRobin},  // Default fallback
	}

	for _, test := range tests {
		if got := ParseTacticType(test.input); got != test.expected {
			t.Errorf("ParseTacticType(%s) = %v, want %v", test.input, got, test.expected)
		}
	}
}

func TestTacticType_String_Adaptive(t *testing.T) {
	if got := TacticAdaptive.String(); got != "adaptive" {
		t.Errorf("TacticAdaptive.String() = %v, want %v", got, "adaptive")
	}
}
