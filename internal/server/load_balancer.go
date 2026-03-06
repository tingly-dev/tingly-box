package server

import (
	"fmt"
	"sync"
	"time"

	"github.com/tingly-dev/tingly-box/internal/loadbalance"
	"github.com/tingly-dev/tingly-box/internal/server/config"
	typ "github.com/tingly-dev/tingly-box/internal/typ"
)

// LoadBalancer manages load balancing across multiple services
type LoadBalancer struct {
	tactics      map[loadbalance.TacticType]typ.LoadBalancingTactic
	stats        map[string]*loadbalance.ServiceStats
	config       *config.Config
	healthFilter *typ.HealthFilter
	mutex        sync.RWMutex
}

// NewLoadBalancer creates a new load balancer
func NewLoadBalancer(cfg *config.Config, healthFilter *typ.HealthFilter) *LoadBalancer {
	lb := &LoadBalancer{
		tactics:      make(map[loadbalance.TacticType]typ.LoadBalancingTactic),
		stats:        make(map[string]*loadbalance.ServiceStats),
		config:       cfg,
		healthFilter: healthFilter,
	}

	// Initialize default tactics
	lb.initializeDefaultTactics()

	return lb
}

// initializeDefaultTactics initializes default load balancing tactics
func (lb *LoadBalancer) initializeDefaultTactics() {
	lb.tactics[loadbalance.TacticRoundRobin] = typ.NewRoundRobinTactic()
	lb.tactics[loadbalance.TacticTokenBased] = typ.NewTokenBasedTactic(10000)
	lb.tactics[loadbalance.TacticHybrid] = typ.NewHybridTactic(100, 10000)
}

// RegisterTactic registers a custom tactic
func (lb *LoadBalancer) RegisterTactic(tacticType loadbalance.TacticType, tactic typ.LoadBalancingTactic) {
	lb.mutex.Lock()
	defer lb.mutex.Unlock()

	lb.tactics[tacticType] = tactic
}

// SelectService selects the best service for a rule based on the configured tactic
func (lb *LoadBalancer) SelectService(rule *typ.Rule) (*loadbalance.Service, error) {
	if rule == nil {
		return nil, fmt.Errorf("rule is nil")
	}

	services := rule.GetServices()
	if len(services) == 0 {
		return nil, fmt.Errorf("no services configured for rule %s", rule.RequestModel)
	}

	// Filter active services
	var activeServices []*loadbalance.Service
	for _, service := range services {
		if service.Active {
			activeServices = append(activeServices, service)
		}
	}

	if len(activeServices) == 0 {
		return nil, fmt.Errorf("no active services for rule %s", rule.RequestModel)
	}

	// Filter healthy services using health filter
	healthyServices := lb.healthFilter.Filter(activeServices)

	// If no healthy services, return error
	if len(healthyServices) == 0 {
		return nil, fmt.Errorf("no healthy services available for rule %s", rule.RequestModel)
	}

	// For single healthy service rules, return it directly
	if len(healthyServices) == 1 {
		return healthyServices[0], nil
	}

	// Always instantiate tactic from rule's params to ensure correct parameters
	// State is now stored globally (globalRoundRobinStreaks) so this is safe
	actualTactic := rule.LBTactic.Instantiate()

	// Create a temporary rule with only healthy services for the tactic
	tempRule := &typ.Rule{
		UUID:             rule.UUID,
		RequestModel:     rule.RequestModel,
		ResponseModel:    rule.ResponseModel,
		CurrentServiceID: rule.CurrentServiceID,
		LBTactic:         rule.LBTactic,
		Active:           rule.Active,
		SmartEnabled:     rule.SmartEnabled,
		SmartRouting:     rule.SmartRouting,
	}
	// Set healthy services on the temp rule
	for _, svc := range healthyServices {
		tempRule.Services = append(tempRule.Services, svc)
	}

	// Select service using the tactic
	selectedService := actualTactic.SelectService(tempRule)
	if selectedService == nil {
		// Fallback to first healthy service
		return healthyServices[0], nil
	}

	return selectedService, nil
}

// getTactic retrieves a tactic by type
func (lb *LoadBalancer) getTactic(tacticType loadbalance.TacticType) (typ.LoadBalancingTactic, bool) {
	lb.mutex.RLock()
	defer lb.mutex.RUnlock()

	tactic, exists := lb.tactics[tacticType]
	return tactic, exists
}

// UpdateServiceIndex updates the current service ID for a rule
func (lb *LoadBalancer) UpdateServiceIndex(rule *typ.Rule, selectedService *loadbalance.Service) {
	if rule == nil || selectedService == nil {
		return
	}

	// Set the current service ID (provider:model format)
	rule.CurrentServiceID = selectedService.ServiceID()
}

// RecordUsage records usage for a service
// Deprecated: This method is no longer needed as usage is recorded directly in handlers
func (lb *LoadBalancer) RecordUsage(provider, model string, inputTokens, outputTokens int) {
	// No-op: usage is now recorded directly in handlers via trackUsageFromContext
	// This method is kept for backward compatibility during the migration
}

// Stop stops the load balancer and cleanup resources
func (lb *LoadBalancer) Stop() {
	// No-op: no resources to cleanup
}

// GetServiceStats returns statistics for a specific service
func (lb *LoadBalancer) GetServiceStats(provider, model string) *loadbalance.ServiceStats {
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
			service := rule.Services[i]
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
func (lb *LoadBalancer) GetAllServiceStats() map[string]*loadbalance.ServiceStats {
	result := make(map[string]*loadbalance.ServiceStats)

	// Read from config file (source of truth)
	if lb.config != nil {
		rules := lb.config.GetRequestConfigs()
		for _, rule := range rules {
			if !rule.Active {
				continue
			}
			for i := range rule.Services {
				service := rule.Services[i]
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
				stats := rule.Services[i].Stats
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
			// Reset current service ID to empty when services change
			if rule.CurrentServiceID != "" {
				rule.CurrentServiceID = ""
				modified = true
			}
			if modified {
				rules[ruleIdx] = rule
			}
		}
	}
}

// ValidateRule validates a rule configuration
func (lb *LoadBalancer) ValidateRule(rule *typ.Rule) error {
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
func (lb *LoadBalancer) GetRuleSummary(rule *typ.Rule) map[string]interface{} {
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
		"request_model":      rule.RequestModel,
		"response_model":     rule.ResponseModel,
		"tactic":             rule.GetTacticType().String(),
		"current_service_id": rule.CurrentServiceID,
		"active":             rule.Active,
		"is_legacy":          false,
		"services":           serviceSummaries,
	}
}

// HealthFilter returns the health filter for the load balancer
func (lb *LoadBalancer) HealthFilter() *typ.HealthFilter {
	return lb.healthFilter
}
