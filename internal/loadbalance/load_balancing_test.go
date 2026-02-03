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
