package config

import (
	"testing"
	"time"
)

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

func TestRule_GetServices_Single(t *testing.T) {
	rule := &Rule{
		RequestModel: "test",
		Services: []Service{
			{
				Provider:   "openai",
				Model:      "gpt-4",
				Weight:     1,
				Active:     true,
				TimeWindow: 300,
			},
		},
		Active: true,
	}

	services := rule.GetServices()

	if len(services) != 1 {
		t.Errorf("Expected 1 service, got %d", len(services))
	}

	service := services[0]
	if service.Provider != "openai" {
		t.Errorf("Expected provider = openai, got %s", service.Provider)
	}
	if service.Model != "gpt-4" {
		t.Errorf("Expected model = gpt-4, got %s", service.Model)
	}
	if service.Weight != 1 {
		t.Errorf("Expected default weight = 1, got %d", service.Weight)
	}
	if !service.Active {
		t.Errorf("Expected service to be active, got %v", service.Active)
	}
	if service.TimeWindow != 300 {
		t.Errorf("Expected default time_window = 300, got %d", service.TimeWindow)
	}
}

func TestRule_GetServices_New(t *testing.T) {
	rule := &Rule{
		RequestModel: "test",
		Services: []Service{
			{
				Provider:   "openai",
				Model:      "gpt-4",
				Weight:     2,
				Active:     true,
				TimeWindow: 600,
			},
			{
				Provider:   "anthropic",
				Model:      "claude-3",
				Weight:     1,
				Active:     false,
				TimeWindow: 300,
			},
		},
	}

	services := rule.GetServices()

	if len(services) != 2 {
		t.Errorf("Expected 2 services, got %d", len(services))
	}

	// Check first service
	if services[0].Provider != "openai" {
		t.Errorf("Expected first provider = openai, got %s", services[0].Provider)
	}
	if services[0].Weight != 2 {
		t.Errorf("Expected first weight = 2, got %d", services[0].Weight)
	}

	// Check second service
	if services[1].Provider != "anthropic" {
		t.Errorf("Expected second provider = anthropic, got %s", services[1].Provider)
	}
	if services[1].Active {
		t.Errorf("Expected second service to be inactive, got %v", services[1].Active)
	}
}

func TestRule_GetTacticType(t *testing.T) {
	// Rule with explicit tactic (token_based)
	ruleWithTactic := &Rule{
		RequestModel: "test",
		LBTactic: Tactic{
			Type:   TacticTokenBased,
			Params: DefaultTokenBasedParams(),
		},
	}
	if ruleWithTactic.GetTacticType() != TacticTokenBased {
		t.Errorf("Expected TacticTokenBased, got %v", ruleWithTactic.GetTacticType())
	}

	// Rule without tactic (should default to round robin)
	ruleWithoutTactic := &Rule{
		RequestModel: "test",
		LBTactic: Tactic{
			Type:   0, // Type 0 means uninitialized
			Params: nil,
		},
	}
	if ruleWithoutTactic.GetTacticType() != TacticRoundRobin {
		t.Errorf("Expected TacticRoundRobin as default, got %v", ruleWithoutTactic.GetTacticType())
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
