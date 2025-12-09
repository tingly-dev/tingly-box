package server

import (
	"testing"

	"tingly-box/internal/config"
)

func TestLoadBalancer_RoundRobin(t *testing.T) {
	// Create stats middleware
	statsMW := NewStatsMiddleware()
	defer statsMW.Stop()

	// Create load balancer
	lb := NewLoadBalancer(statsMW)
	defer lb.Stop()

	// Create test rule with multiple services
	rule := &config.Rule{
		RequestModel: "test",
		Services: []config.Service{
			{
				Provider:   "provider1",
				Model:      "model1",
				Weight:     1,
				Active:     true,
				TimeWindow: 300,
			},
			{
				Provider:   "provider2",
				Model:      "model2",
				Weight:     1,
				Active:     true,
				TimeWindow: 300,
			},
		},
		CurrentServiceIndex: 0, // Start with first service
		Tactic:              "round_robin",
		Active:              true,
	}

	// Test round-robin selection
	for i := 0; i < 4; i++ {
		service, err := lb.SelectService(rule)
		if err != nil {
			t.Fatalf("SelectService failed: %v", err)
		}

		if service == nil {
			t.Fatal("SelectService returned nil")
		}

		// Check that we're cycling through services
		// Note: round-robin increments index first, so iteration 0 returns provider2
		expectedProvider := "provider2"
		if i%2 == 1 {
			expectedProvider = "provider1"
		}

		if service.Provider != expectedProvider {
			t.Errorf("Iteration %d: expected provider %s, got %s", i, expectedProvider, service.Provider)
		}
	}
}

func TestLoadBalancer_WeightedRandom(t *testing.T) {
	// Create stats middleware
	statsMW := NewStatsMiddleware()
	defer statsMW.Stop()

	// Create load balancer and register random tactic
	lb := NewLoadBalancer(statsMW)
	defer lb.Stop()

	randomTactic := config.NewRandomTactic()
	lb.RegisterTactic(config.TacticRoundRobin, randomTactic)

	// Create test rule with weighted services
	rule := &config.Rule{
		RequestModel: "test",
		Services: []config.Service{
			{
				Provider:   "provider1",
				Model:      "model1",
				Weight:     3, // Higher weight
				Active:     true,
				TimeWindow: 300,
			},
			{
				Provider:   "provider2",
				Model:      "model2",
				Weight:     1, // Lower weight
				Active:     true,
				TimeWindow: 300,
			},
		},
		Tactic: "round_robin", // Will use our registered random tactic
		Active: true,
	}

	// Test weighted selection (run multiple times to see distribution)
	provider1Count := 0
	provider2Count := 0
	total := 100

	for i := 0; i < total; i++ {
		service, err := lb.SelectService(rule)
		if err != nil {
			t.Fatalf("SelectService failed: %v", err)
		}

		if service.Provider == "provider1" {
			provider1Count++
		} else if service.Provider == "provider2" {
			provider2Count++
		}
	}

	// Check that provider1 gets roughly 3x more selections
	// Allow some tolerance for randomness
	provider1Ratio := float64(provider1Count) / float64(provider2Count)
	if provider1Ratio < 2.0 || provider1Ratio > 4.0 {
		t.Errorf("Expected provider1 to get ~3x more selections than provider2, got ratio %.2f (%d vs %d)",
			provider1Ratio, provider1Count, provider2Count)
	}

	t.Logf("Distribution: provider1: %d, provider2: %d, ratio: %.2f",
		provider1Count, provider2Count, provider1Ratio)
}

func TestLoadBalancer_EnabledFilter(t *testing.T) {
	// Create stats middleware
	statsMW := NewStatsMiddleware()
	defer statsMW.Stop()

	// Create load balancer
	lb := NewLoadBalancer(statsMW)
	defer lb.Stop()

	// Create test rule with mixed enabled/disabled services
	rule := &config.Rule{
		RequestModel: "test",
		Services: []config.Service{
			{
				Provider:   "enabled1",
				Model:      "model1",
				Weight:     1,
				Active:     true,
				TimeWindow: 300,
			},
			{
				Provider:   "disabled1",
				Model:      "model2",
				Weight:     10,
				Active:     false, // Disabled
				TimeWindow: 300,
			},
			{
				Provider:   "enabled2",
				Model:      "model3",
				Weight:     1,
				Active:     true,
				TimeWindow: 300,
			},
		},
		Tactic: "round_robin",
		Active: true,
	}

	// Test that only enabled services are selected
	for i := 0; i < 10; i++ {
		service, err := lb.SelectService(rule)
		if err != nil {
			t.Fatalf("SelectService failed: %v", err)
		}

		if service == nil {
			t.Fatal("SelectService returned nil")
		}

		if service.Provider == "disabled1" {
			t.Errorf("Iteration %d: disabled service was selected", i)
		}

		// Should only alternate between enabled1 and enabled2
		if service.Provider != "enabled1" && service.Provider != "enabled2" {
			t.Errorf("Iteration %d: unexpected provider %s", i, service.Provider)
		}
	}
}

func TestLoadBalancer_RecordUsage(t *testing.T) {
	// Create stats middleware
	statsMW := NewStatsMiddleware()
	defer statsMW.Stop()

	// Create load balancer
	lb := NewLoadBalancer(statsMW)
	defer lb.Stop()

	// Record usage for a service
	lb.RecordUsage("test-provider", "test-model", 120, 30) // 120 input, 30 output tokens

	// Check that statistics were recorded
	stats := lb.GetServiceStats("test-provider", "test-model")
	if stats == nil {
		t.Fatal("Expected stats to be recorded")
	}

	statsCopy := stats.GetStats()
	if statsCopy.RequestCount != 1 {
		t.Errorf("Expected RequestCount = 1, got %d", statsCopy.RequestCount)
	}
	if statsCopy.WindowTokensConsumed != 150 {
		t.Errorf("Expected WindowTokensConsumed = 150, got %d", statsCopy.WindowTokensConsumed)
	}
	if statsCopy.WindowInputTokens != 120 {
		t.Errorf("Expected WindowInputTokens = 120, got %d", statsCopy.WindowInputTokens)
	}
	if statsCopy.WindowOutputTokens != 30 {
		t.Errorf("Expected WindowOutputTokens = 30, got %d", statsCopy.WindowOutputTokens)
	}
}

func TestLoadBalancer_ValidateRule(t *testing.T) {
	// Create stats middleware
	statsMW := NewStatsMiddleware()
	defer statsMW.Stop()

	// Create load balancer
	lb := NewLoadBalancer(statsMW)
	defer lb.Stop()

	// Test valid rule
	validRule := &config.Rule{
		RequestModel: "test",
		Services: []config.Service{
			{
				Provider:   "provider1",
				Model:      "model1",
				Weight:     1,
				Active:     true,
				TimeWindow: 300,
			},
		},
		Tactic: "round_robin",
		Active: true,
	}

	if err := lb.ValidateRule(validRule); err != nil {
		t.Errorf("Valid rule validation failed: %v", err)
	}

	// Test rule with no services
	invalidRule1 := &config.Rule{
		RequestModel: "test",
		Services:     []config.Service{},
		Tactic:       "round_robin",
		Active:       true,
	}

	if err := lb.ValidateRule(invalidRule1); err == nil {
		t.Error("Expected validation error for rule with no services")
	}

	// Test rule with no enabled services
	invalidRule2 := &config.Rule{
		RequestModel: "test",
		Services: []config.Service{
			{
				Provider:   "provider1",
				Model:      "model1",
				Weight:     1,
				Active:     false, // Disabled
				TimeWindow: 300,
			},
		},
		Tactic: "round_robin",
		Active: true,
	}

	if err := lb.ValidateRule(invalidRule2); err == nil {
		t.Error("Expected validation error for rule with no enabled services")
	}
}

func TestLoadBalancer_GetRuleSummary(t *testing.T) {
	// Create stats middleware
	statsMW := NewStatsMiddleware()
	defer statsMW.Stop()

	// Create load balancer
	lb := NewLoadBalancer(statsMW)
	defer lb.Stop()

	// Create test rule
	rule := &config.Rule{
		RequestModel: "test",
		Services: []config.Service{
			{
				Provider:   "provider1",
				Model:      "model1",
				Weight:     2,
				Active:     true,
				TimeWindow: 300,
			},
		},
		Tactic: "hybrid",
		Active: true,
	}

	// Get rule summary
	summary := lb.GetRuleSummary(rule)

	// Check summary content
	if summary["request_model"] != "test" {
		t.Errorf("Expected request_model = test, got %v", summary["request_model"])
	}

	if summary["tactic"] != "hybrid" {
		t.Errorf("Expected tactic = hybrid, got %v", summary["tactic"])
	}

	if summary["active"] != true {
		t.Errorf("Expected active = true, got %v", summary["active"])
	}

	if summary["is_legacy"] != false {
		t.Errorf("Expected is_legacy = false, got %v", summary["is_legacy"])
	}

	// Check services
	services, ok := summary["services"].([]map[string]interface{})
	if !ok {
		t.Fatal("Expected services to be a slice")
	}

	if len(services) != 1 {
		t.Errorf("Expected 1 service in summary, got %d", len(services))
	}

	service := services[0]
	if service["provider"] != "provider1" {
		t.Errorf("Expected service provider = provider1, got %v", service["provider"])
	}

	if service["model"] != "model1" {
		t.Errorf("Expected service model = model1, got %v", service["model"])
	}

	if service["weight"] != 2 {
		t.Errorf("Expected service weight = 2, got %v", service["weight"])
	}
}
