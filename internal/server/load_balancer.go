package server

import (
	"fmt"
	"sync"
	"time"
	"tingly-box/internal/server/middleware"

	"tingly-box/internal/config"
)

// LoadBalancer manages load balancing across multiple services
type LoadBalancer struct {
	tactics map[config.TacticType]config.LoadBalancingTactic
	stats   map[string]*config.ServiceStats
	statsMW *middleware.StatsMiddleware
	config  *config.Config
	mutex   sync.RWMutex
}

// NewLoadBalancer creates a new load balancer
func NewLoadBalancer(statsMW *middleware.StatsMiddleware, cfg *config.Config) *LoadBalancer {
	lb := &LoadBalancer{
		tactics: make(map[config.TacticType]config.LoadBalancingTactic),
		stats:   make(map[string]*config.ServiceStats),
		statsMW: statsMW,
		config:  cfg,
	}

	// Initialize default tactics
	lb.initializeDefaultTactics()

	return lb
}

// initializeDefaultTactics initializes default load balancing tactics
func (lb *LoadBalancer) initializeDefaultTactics() {
	lb.tactics[config.TacticRoundRobin] = config.NewRoundRobinTactic()
	lb.tactics[config.TacticTokenBased] = config.NewTokenBasedTactic(10000)
	lb.tactics[config.TacticHybrid] = config.NewHybridTactic(100, 10000)
}

// RegisterTactic registers a custom tactic
func (lb *LoadBalancer) RegisterTactic(tacticType config.TacticType, tactic config.LoadBalancingTactic) {
	lb.mutex.Lock()
	defer lb.mutex.Unlock()

	lb.tactics[tacticType] = tactic
}

// SelectService selects the best service for a rule based on the configured tactic
func (lb *LoadBalancer) SelectService(rule *config.Rule) (*config.Service, error) {
	if rule == nil {
		return nil, fmt.Errorf("rule is nil")
	}

	services := rule.GetServices()
	if len(services) == 0 {
		return nil, fmt.Errorf("no services configured for rule %s", rule.RequestModel)
	}

	// Filter active services
	var activeServices []config.Service
	for _, service := range services {
		if service.Active {
			activeServices = append(activeServices, service)
		}
	}

	if len(activeServices) == 0 {
		return nil, fmt.Errorf("no active services for rule %s", rule.RequestModel)
	}

	// For single service rules, return it directly
	if len(activeServices) == 1 {
		return &activeServices[0], nil
	}

	// Always instantiate tactic from rule's params to ensure correct parameters
	// State is now stored globally (globalRoundRobinStreaks) so this is safe
	actualTactic := rule.LBTactic.Instantiate()

	// Select service using the tactic
	selectedService := actualTactic.SelectService(rule)
	if selectedService == nil {
		// Fallback to first active service
		return &activeServices[0], nil
	}

	return selectedService, nil
}

// getTactic retrieves a tactic by type
func (lb *LoadBalancer) getTactic(tacticType config.TacticType) (config.LoadBalancingTactic, bool) {
	lb.mutex.RLock()
	defer lb.mutex.RUnlock()

	tactic, exists := lb.tactics[tacticType]
	return tactic, exists
}

// UpdateServiceIndex updates the current service index for a rule
func (lb *LoadBalancer) UpdateServiceIndex(rule *config.Rule, selectedService *config.Service) {
	if rule == nil || selectedService == nil {
		return
	}

	services := rule.GetServices()
	for i, service := range services {
		if service.ServiceID() == selectedService.ServiceID() {
			rule.CurrentServiceIndex = i
			break
		}
	}
}

// RecordUsage records usage for a service
func (lb *LoadBalancer) RecordUsage(provider, model string, inputTokens, outputTokens int) {
	serviceID := fmt.Sprintf("%s:%s", provider, model)
	lb.statsMW.RecordUsage(serviceID, inputTokens, outputTokens)
}

// GetServiceStats returns statistics for a specific service
func (lb *LoadBalancer) GetServiceStats(provider, model string) *config.ServiceStats {
	if lb.config == nil {
		return nil
	}

	// Find the service in the rules and return its stats
	rules := lb.config.GetRequestConfigs()
	for _, rule := range rules {
		if !rule.Active {
			continue
		}

		for i := range rule.Services {
			service := &rule.Services[i]
			if service.Active && service.Provider == provider && service.Model == model {
				// Return a copy of the service's stats
				statsCopy := service.Stats.GetStats()
				return &statsCopy
			}
		}
	}

	return nil
}

// GetAllServiceStats returns all service statistics from all active rules.
// Stats are keyed by provider:model since stats are global (shared across rules).
func (lb *LoadBalancer) GetAllServiceStats() map[string]*config.ServiceStats {
	result := make(map[string]*config.ServiceStats)

	// Read from config file (source of truth)
	if lb.config != nil {
		rules := lb.config.GetRequestConfigs()
		for _, rule := range rules {
			if !rule.Active {
				continue
			}
			for i := range rule.Services {
				service := &rule.Services[i]
				if service.Active {
					// Stats are global per provider:model, not per-rule
					store := lb.config.GetStatsStore()
					key := store.ServiceKey(service.Provider, service.Model)
					// Only add if not already present (services across rules may share provider:model)
					if _, exists := result[key]; !exists {
						statsCopy := service.Stats.GetStats()
						result[key] = &statsCopy
					}
				}
			}
		}
	}

	return result
}

// ClearServiceStats clears statistics for a specific service
func (lb *LoadBalancer) ClearServiceStats(provider, model string) {
	// Clear from internal stats map
	serviceID := fmt.Sprintf("%s:%s", provider, model)
	if stats, exists := lb.stats[serviceID]; exists {
		stats.ResetWindow()
	}
}

// ClearAllStats clears all statistics (both in-memory and persisted in config)
func (lb *LoadBalancer) ClearAllStats() {
	// Clear from internal stats map
	for _, stats := range lb.stats {
		stats.ResetWindow()
	}

	// Clear persisted stats from the dedicated stats store
	if lb.config != nil {
		if store := lb.config.GetStatsStore(); store != nil {
			_ = store.ClearAll()
		}
	}

	// Also clear stats from all rules in memory
	if lb.config != nil {
		rules := lb.config.GetRequestConfigs()
		for ruleIdx, rule := range rules {
			modified := false
			for i := range rule.Services {
				stats := &rule.Services[i].Stats
				if stats.RequestCount > 0 || stats.WindowRequestCount > 0 {
					stats.RequestCount = 0
					stats.WindowRequestCount = 0
					stats.WindowTokensConsumed = 0
					stats.WindowInputTokens = 0
					stats.WindowOutputTokens = 0
					stats.WindowStart = time.Now()
					stats.LastUsed = time.Time{}
					modified = true
				}
			}
			// Reset current service index to 0
			if rule.CurrentServiceIndex != 0 {
				rule.CurrentServiceIndex = 0
				modified = true
			}
			if modified {
				rules[ruleIdx] = rule
			}
		}
	}
}

// ValidateRule validates a rule configuration
func (lb *LoadBalancer) ValidateRule(rule *config.Rule) error {
	if rule == nil {
		return fmt.Errorf("rule is nil")
	}

	if rule.RequestModel == "" {
		return fmt.Errorf("request_model is required")
	}

	services := rule.GetServices()
	if len(services) == 0 {
		return fmt.Errorf("no services configured")
	}

	// Check if at least one service is active
	hasActiveService := false
	for _, service := range services {
		if service.Active {
			hasActiveService = true
			break
		}
	}

	if !hasActiveService {
		return fmt.Errorf("at least one service must be active")
	}

	// Validate tactic
	tacticType := rule.GetTacticType()
	_, exists := lb.getTactic(tacticType)
	if !exists {
		return fmt.Errorf("unsupported tactic: %s", tacticType.String())
	}

	return nil
}

// GetRuleSummary returns a summary of rule configuration and statistics
func (lb *LoadBalancer) GetRuleSummary(rule *config.Rule) map[string]interface{} {
	if rule == nil {
		return nil
	}

	services := rule.GetServices()
	serviceSummaries := make([]map[string]interface{}, 0, len(services))

	for _, service := range services {
		stats := lb.GetServiceStats(service.Provider, service.Model)
		summary := map[string]interface{}{
			"service_id":  service.ServiceID(),
			"provider":    service.Provider,
			"model":       service.Model,
			"weight":      service.Weight,
			"active":      service.Active,
			"time_window": service.TimeWindow,
		}

		if stats != nil {
			summary["stats"] = map[string]interface{}{
				"request_count":        stats.RequestCount,
				"window_request_count": stats.WindowRequestCount,
				"window_tokens":        stats.WindowTokensConsumed,
				"window_input_tokens":  stats.WindowInputTokens,
				"window_output_tokens": stats.WindowOutputTokens,
				"last_used":            stats.LastUsed,
			}
		}

		serviceSummaries = append(serviceSummaries, summary)
	}

	return map[string]interface{}{
		"request_model":         rule.RequestModel,
		"response_model":        rule.ResponseModel,
		"tactic":                rule.GetTacticType().String(),
		"current_service_index": rule.CurrentServiceIndex,
		"active":                rule.Active,
		"is_legacy":             false,
		"services":              serviceSummaries,
	}
}

// Stop stops the load balancer and cleanup resources
func (lb *LoadBalancer) Stop() {
	if lb.statsMW != nil {
		lb.statsMW.Stop()
	}
}
