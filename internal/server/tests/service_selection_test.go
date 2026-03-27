package tests

import (
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/tingly-dev/tingly-box/internal/loadbalance"
	"github.com/tingly-dev/tingly-box/internal/typ"
)

// TestServiceSelection_TokenBased tests the core logic of TokenBasedTactic
func TestServiceSelection_TokenBased(t *testing.T) {
	t.Run("Selects service with lowest token consumption", func(t *testing.T) {
		// Create three services with different token usage
		services := []*loadbalance.Service{
			{
				Provider:   "provider-a",
				Model:      "gpt-4",
				Weight:     1,
				Active:     true,
				TimeWindow: 300,
				Stats: loadbalance.ServiceStats{
					ServiceID:            "provider-a:gpt-4",
					TimeWindow:           300,
					WindowStart:          time.Now(),
					WindowTokensConsumed: 5000, // High usage
					WindowRequestCount:   50,
				},
			},
			{
				Provider:   "provider-b",
				Model:      "gpt-4",
				Weight:     1,
				Active:     true,
				TimeWindow: 300,
				Stats: loadbalance.ServiceStats{
					ServiceID:            "provider-b:gpt-4",
					TimeWindow:           300,
					WindowStart:          time.Now(),
					WindowTokensConsumed: 1000, // Low usage
					WindowRequestCount:   10,
				},
			},
			{
				Provider:   "provider-c",
				Model:      "gpt-4",
				Weight:     1,
				Active:     true,
				TimeWindow: 300,
				Stats: loadbalance.ServiceStats{
					ServiceID:            "provider-c:gpt-4",
					TimeWindow:           300,
					WindowStart:          time.Now(),
					WindowTokensConsumed: 2500, // Medium usage
					WindowRequestCount:   25,
				},
			},
		}

		typRule := &typ.Rule{
			UUID:     "test-rule-token-lowest",
			Services: services,
			Active:   true,
		}

		// TokenBasedTactic should select provider-b (lowest tokens)
		tactic := typ.NewTokenBasedTactic(2000)
		selected := tactic.SelectService(typRule)

		assert.NotNil(t, selected)
		assert.Equal(t, "provider-b", selected.Provider,
			"TokenBasedTactic should select service with lowest token consumption")
	})

	t.Run("Selects service with zero usage (prioritized)", func(t *testing.T) {
		services := []*loadbalance.Service{
			{
				Provider:   "provider-a",
				Model:      "gpt-4",
				Weight:     1,
				Active:     true,
				TimeWindow: 300,
				Stats: loadbalance.ServiceStats{
					ServiceID:            "provider-a:gpt-4",
					TimeWindow:           300,
					WindowStart:          time.Now(),
					WindowTokensConsumed: 5000,
					WindowRequestCount:   50,
				},
			},
			{
				Provider:   "provider-b",
				Model:      "gpt-4",
				Weight:     1,
				Active:     true,
				TimeWindow: 300,
				Stats: loadbalance.ServiceStats{
					ServiceID:            "provider-b:gpt-4",
					TimeWindow:           300,
					WindowStart:          time.Now(),
					WindowTokensConsumed: 0, // No usage yet
					WindowRequestCount:   0,
				},
			},
		}

		typRule := &typ.Rule{
			UUID:     "test-rule-zero-usage",
			Services: services,
			Active:   true,
		}

		tactic := typ.NewTokenBasedTactic(1000)
		selected := tactic.SelectService(typRule)

		assert.NotNil(t, selected)
		assert.Equal(t, "provider-b", selected.Provider,
			"Service with zero usage should be prioritized")
	})

	t.Run("Returns first service when all have equal usage", func(t *testing.T) {
		services := []*loadbalance.Service{
			{
				Provider:   "provider-a",
				Model:      "gpt-4",
				Weight:     1,
				Active:     true,
				TimeWindow: 300,
				Stats: loadbalance.ServiceStats{
					ServiceID:            "provider-a:gpt-4",
					TimeWindow:           300,
					WindowStart:          time.Now(),
					WindowTokensConsumed: 1000,
				},
			},
			{
				Provider:   "provider-b",
				Model:      "gpt-4",
				Weight:     1,
				Active:     true,
				TimeWindow: 300,
				Stats: loadbalance.ServiceStats{
					ServiceID:            "provider-b:gpt-4",
					TimeWindow:           300,
					WindowStart:          time.Now(),
					WindowTokensConsumed: 1000, // Same usage
				},
			},
		}

		typRule := &typ.Rule{
			UUID:     "test-rule-equal-usage",
			Services: services,
			Active:   true,
		}

		tactic := typ.NewTokenBasedTactic(1000)
		selected := tactic.SelectService(typRule)

		assert.NotNil(t, selected)
		// With equal tokens, should return the first service
		assert.Equal(t, "provider-a", selected.Provider,
			"With equal usage, first service should be selected")
	})
}

// TestServiceSelection_TokenBasedKeepsCurrent tests the token-based logic (hybrid was removed)
func TestServiceSelection_TokenBasedKeepsCurrent(t *testing.T) {
	t.Run("Keeps current service if within threshold", func(t *testing.T) {
		// Create services with different request/token combinations
		services := []*loadbalance.Service{
			{
				Provider: "provider-a",
				Model:    "gpt-4",
				Weight:   1,
				Active:   true,
				Stats: loadbalance.ServiceStats{
					ServiceID:            "provider-a:gpt-4",
					TimeWindow:           300,
					WindowStart:          time.Now(),
					WindowRequestCount:   5,    // Low requests
					WindowTokensConsumed: 9000, // High tokens
				},
			},
			{
				Provider: "provider-b",
				Model:    "gpt-4",
				Weight:   1,
				Active:   true,
				Stats: loadbalance.ServiceStats{
					ServiceID:            "provider-b:gpt-4",
					TimeWindow:           300,
					WindowStart:          time.Now(),
					WindowRequestCount:   100, // High requests
					WindowTokensConsumed: 500, // Low tokens
				},
			},
		}

		typRule := &typ.Rule{
			UUID:     "test-rule-hybrid",
			Services: services,
			Active:   true,
		}

		tactic := typ.NewTokenBasedTactic(10000) // High token threshold

		// provider-a: tokens=5000 (< 10000) - within threshold
		// Since it's the first service (default) and hasn't exceeded thresholds, keep using it
		selected := tactic.SelectService(typRule)

		assert.NotNil(t, selected)
		assert.Equal(t, "provider-a", selected.Provider,
			"TokenBasedTactic should keep current service if within threshold")
	})

	t.Run("Selects service with better score when thresholds exceeded", func(t *testing.T) {
		// Create services where current service exceeds thresholds
		services := []*loadbalance.Service{
			{
				Provider: "provider-a",
				Model:    "gpt-4",
				Weight:   1,
				Active:   true,
				Stats: loadbalance.ServiceStats{
					ServiceID:            "provider-a:gpt-4",
					TimeWindow:           300,
					WindowStart:          time.Now(),
					WindowRequestCount:   100,  // High requests
					WindowTokensConsumed: 9000, // High tokens
				},
			},
			{
				Provider: "provider-b",
				Model:    "gpt-4",
				Weight:   1,
				Active:   true,
				Stats: loadbalance.ServiceStats{
					ServiceID:            "provider-b:gpt-4",
					TimeWindow:           300,
					WindowStart:          time.Now(),
					WindowRequestCount:   10,  // Low requests
					WindowTokensConsumed: 500, // Low tokens
				},
			},
		}

		typRule := &typ.Rule{
			UUID:             "test-rule-hybrid-switch",
			Services:         services,
			Active:           true,
			CurrentServiceID: "provider-a:gpt-4", // Current is provider-a
		}

		tactic := typ.NewTokenBasedTactic(5000) // Lower threshold

		// provider-a: tokens=9000 (>= 5000) - exceeded threshold
		// provider-b: tokens=500 (< 5000) - within threshold
		// provider-b should be selected (lower token usage)
		selected := tactic.SelectService(typRule)

		assert.NotNil(t, selected)
		assert.Equal(t, "provider-b", selected.Provider,
			"TokenBasedTactic should select service with lower token usage when threshold exceeded")
	})
}

// TestServiceSelection_WindowReset tests window reset behavior
func TestServiceSelection_WindowReset(t *testing.T) {
	t.Run("Selects based on current window after reset", func(t *testing.T) {
		// Create a service with an old window
		oldStartTime := time.Now().Add(-10 * time.Second) // 10 seconds ago
		service := &loadbalance.Service{
			Provider:   "provider-a",
			Model:      "gpt-4",
			Weight:     1,
			Active:     true,
			TimeWindow: 5, // 5 second window for testing
			Stats: loadbalance.ServiceStats{
				ServiceID:            "provider-a:gpt-4",
				TimeWindow:           5,
				WindowStart:          oldStartTime,
				WindowTokensConsumed: 10000, // High usage in old window
				WindowRequestCount:   100,
			},
		}

		services := []*loadbalance.Service{service}

		typRule := &typ.Rule{
			UUID:     "test-rule-window-reset",
			Services: services,
			Active:   true,
		}

		// Trigger window reset by recording new usage
		service.RecordUsage(100, 50)

		// Check that window was reset
		requests, tokens := service.GetWindowStats()
		assert.Equal(t, int64(1), requests, "Request count should be reset to 1")
		assert.Equal(t, int64(150), tokens, "Token count should be reset to new usage only")

		// TokenBasedTactic should see the reset stats and select the service
		tactic := typ.NewTokenBasedTactic(1000)
		selected := tactic.SelectService(typRule)

		assert.NotNil(t, selected)
		assert.Equal(t, "provider-a", selected.Provider,
			"TokenBasedTactic should select based on reset window stats")
	})
}
