package config

import (
	"math/rand"
)

// LoadBalancingTactic defines the interface for load balancing strategies
type LoadBalancingTactic interface {
	SelectService(rule *Rule) *Service
	GetName() string
	GetType() TacticType
}

// RoundRobinTactic implements round-robin load balancing based on request count
type RoundRobinTactic struct {
	RequestThreshold int64 // Number of requests per service before switching
}

// NewRoundRobinTactic creates a new round-robin tactic with optional threshold parameter
func NewRoundRobinTactic(requestThreshold ...int64) *RoundRobinTactic {
	threshold := int64(100) // Default 100 requests per service
	if len(requestThreshold) > 0 && requestThreshold[0] > 0 {
		threshold = requestThreshold[0]
	}
	return &RoundRobinTactic{RequestThreshold: threshold}
}

// SelectService selects the next service based on round-robin with request threshold
func (rr *RoundRobinTactic) SelectService(rule *Rule) *Service {
	if len(rule.Services) == 0 {
		return nil
	}

	// Filter active services
	var activeServices []*Service
	for i := range rule.Services {
		if rule.Services[i].Active {
			rule.Services[i].InitializeStats()
			activeServices = append(activeServices, &rule.Services[i])
		}
	}

	if len(activeServices) == 0 {
		return nil
	}

	// Get current service
	currentIndex := rule.CurrentServiceIndex % len(activeServices)
	currentService := activeServices[currentIndex]

	// Check if current service has exceeded the request threshold
	requests, _ := currentService.GetWindowStats()

	// If current service hasn't exceeded threshold, keep using it
	if requests < rr.RequestThreshold {
		return currentService
	}

	// Current service exceeded threshold, move to next service
	nextIndex := (rule.CurrentServiceIndex + 1) % len(activeServices)
	nextService := activeServices[nextIndex]

	// Update the rule's current index
	rule.CurrentServiceIndex = nextIndex

	return nextService
}

func (rr *RoundRobinTactic) GetName() string {
	return "Round Robin"
}

func (rr *RoundRobinTactic) GetType() TacticType {
	return TacticRoundRobin
}

// TokenBasedTactic implements load balancing based on token consumption
type TokenBasedTactic struct {
	TokenThreshold int64 // Threshold for token consumption before switching
}

// NewTokenBasedTactic creates a new token-based tactic
func NewTokenBasedTactic(tokenThreshold int64) *TokenBasedTactic {
	if tokenThreshold <= 0 {
		tokenThreshold = 10000 // Default threshold
	}
	return &TokenBasedTactic{TokenThreshold: tokenThreshold}
}

// SelectService selects service based on token consumption thresholds
func (tb *TokenBasedTactic) SelectService(rule *Rule) *Service {
	if len(rule.Services) == 0 {
		return nil
	}

	// Filter active services
	var activeServices []*Service
	for i := range rule.Services {
		if rule.Services[i].Active {
			activeServices = append(activeServices, &rule.Services[i])
		}
	}

	if len(activeServices) == 0 {
		return nil
	}

	// Check current service token usage
	currentIndex := rule.CurrentServiceIndex % len(activeServices)
	currentService := activeServices[currentIndex]

	_, windowTokens := currentService.GetWindowStats()

	// If current service hasn't exceeded threshold, keep using it
	if windowTokens < tb.TokenThreshold {
		return currentService
	}

	// Find service with lowest token usage in current window
	var selectedService *Service
	var lowestTokens int64 = -1

	for _, service := range activeServices {
		_, windowTokens := service.GetWindowStats()
		if lowestTokens == -1 || windowTokens < lowestTokens {
			lowestTokens = windowTokens
			selectedService = service
		}
	}

	return selectedService
}

func (tb *TokenBasedTactic) GetName() string {
	return "Token Based"
}

func (tb *TokenBasedTactic) GetType() TacticType {
	return TacticTokenBased
}

// HybridTactic implements a hybrid load balancing strategy
type HybridTactic struct {
	RequestThreshold int64 // Threshold for request count before switching
	TokenThreshold   int64 // Threshold for token consumption before switching
}

// NewHybridTactic creates a new hybrid tactic
func NewHybridTactic(requestThreshold, tokenThreshold int64) *HybridTactic {
	if requestThreshold <= 0 {
		requestThreshold = 100 // Default
	}
	if tokenThreshold <= 0 {
		tokenThreshold = 10000 // Default
	}
	return &HybridTactic{
		RequestThreshold: requestThreshold,
		TokenThreshold:   tokenThreshold,
	}
}

// SelectService selects service based on both request count and token consumption
func (ht *HybridTactic) SelectService(rule *Rule) *Service {
	if len(rule.Services) == 0 {
		return nil
	}

	// Filter active services
	var activeServices []*Service
	for i := range rule.Services {
		if rule.Services[i].Active {
			activeServices = append(activeServices, &rule.Services[i])
		}
	}

	if len(activeServices) == 0 {
		return nil
	}

	// Check current service usage
	currentIndex := rule.CurrentServiceIndex % len(activeServices)
	currentService := activeServices[currentIndex]

	requests, tokens := currentService.GetWindowStats()

	// If current service hasn't exceeded either threshold, keep using it
	if requests < ht.RequestThreshold && tokens < ht.TokenThreshold {
		return currentService
	}

	// Score services based on combined usage (lower score is better)
	var selectedService *Service
	var lowestScore int64 = -1

	for _, service := range activeServices {
		requests, tokens := service.GetWindowStats()
		// Weight tokens more heavily than requests
		score := requests*10 + tokens

		if lowestScore == -1 || score < lowestScore {
			lowestScore = score
			selectedService = service
		}
	}

	return selectedService
}

func (ht *HybridTactic) GetName() string {
	return "Hybrid"
}

func (ht *HybridTactic) GetType() TacticType {
	return TacticHybrid
}

// RandomTactic implements random selection with weighted probability
type RandomTactic struct{}

// NewRandomTactic creates a new random tactic
func NewRandomTactic() *RandomTactic {
	return &RandomTactic{}
}

// SelectService selects a service randomly based on weights
func (rt *RandomTactic) SelectService(rule *Rule) *Service {
	if len(rule.Services) == 0 {
		return nil
	}

	// Filter active services
	var activeServices []*Service
	var totalWeight int
	for i := range rule.Services {
		if rule.Services[i].Active {
			activeServices = append(activeServices, &rule.Services[i])
			if rule.Services[i].Weight > 0 {
				totalWeight += rule.Services[i].Weight
			}
		}
	}

	if len(activeServices) == 0 {
		return nil
	}

	// If no weights or all weights are 0, select randomly
	if totalWeight == 0 {
		return activeServices[rand.Intn(len(activeServices))]
	}

	// Weighted random selection
	random := rand.Intn(totalWeight)

	currentWeight := 0
	for _, service := range activeServices {
		currentWeight += service.Weight
		if random < currentWeight {
			return service
		}
	}

	// Fallback (shouldn't reach here)
	return activeServices[0]
}

func (rt *RandomTactic) GetName() string {
	return "Random"
}

func (rt *RandomTactic) GetType() TacticType {
	return TacticRoundRobin // Treat as round-robin type for compatibility
}

// Pre-created singleton tactic instances
var (
	defaultRoundRobinTactic = NewRoundRobinTactic()
	defaultTokenBasedTactic = NewTokenBasedTactic(10000)
	defaultHybridTactic     = NewHybridTactic(100, 10000)
	defaultRandomTactic     = NewRandomTactic()
)

// CreateTactic creates a tactic instance based on type and parameters
// Returns singleton instances for default configurations, creates new instances only when custom parameters are provided
func CreateTactic(tacticType TacticType, params map[string]interface{}) LoadBalancingTactic {
	switch tacticType {
	case TacticRoundRobin:
		if params != nil {
			if requestThreshold, ok := params["request_threshold"].(int64); ok && requestThreshold > 0 {
				return NewRoundRobinTactic(requestThreshold)
			}
		}
		return defaultRoundRobinTactic
	case TacticTokenBased:
		if params != nil {
			if tokenThreshold, ok := params["token_threshold"].(int64); ok && tokenThreshold > 0 && tokenThreshold != 10000 {
				return NewTokenBasedTactic(tokenThreshold)
			}
		}
		return defaultTokenBasedTactic
	case TacticHybrid:
		if params != nil {
			var requestThreshold, tokenThreshold int64 = 100, 10000
			hasCustomParams := false

			if rt, ok := params["request_threshold"].(int64); ok && rt > 0 {
				requestThreshold = rt
				hasCustomParams = true
			}
			if tt, ok := params["token_threshold"].(int64); ok && tt > 0 {
				tokenThreshold = tt
				hasCustomParams = true
			}

			if hasCustomParams {
				return NewHybridTactic(requestThreshold, tokenThreshold)
			}
		}
		return defaultHybridTactic
	default:
		return defaultRoundRobinTactic // Default fallback
	}
}
