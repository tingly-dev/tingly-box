package server

import (
	"fmt"
	"time"

	"github.com/sirupsen/logrus"

	"github.com/tingly-dev/tingly-box/internal/loadbalance"
	"github.com/tingly-dev/tingly-box/internal/server/config"
	typ "github.com/tingly-dev/tingly-box/internal/typ"
)

// LoadBalancer is the tactic-selection engine: it narrows a rule's services
// to the healthy active set and delegates the final pick to the rule's
// configured tactic (rule.LBTactic.Instantiate()). It also serves as the
// stats/admin surface consumed by LoadBalancerAPI.
type LoadBalancer struct {
	config       *config.Config
	healthFilter *typ.HealthFilter
}

// NewLoadBalancer creates a new load balancer
func NewLoadBalancer(cfg *config.Config, healthFilter *typ.HealthFilter) *LoadBalancer {
	return &LoadBalancer{
		config:       cfg,
		healthFilter: healthFilter,
	}
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

	// Filter healthy services using health filter. When every active service is
	// currently marked unhealthy (e.g. a transient 429 on a single-service rule,
	// or all services inside the recovery window at once), fall back to the full
	// active set instead of failing the whole rule. Trying an unhealthy upstream
	// is strictly better than a hard "no service available": the service may have
	// already recovered, and if it really is still failing the caller gets the
	// real upstream error (e.g. 429) rather than a confusing routing error.
	healthyServices := lb.healthFilter.Filter(activeServices)
	if len(healthyServices) == 0 {
		logrus.Warnf("[load_balancer] all %d active services for rule %s are unhealthy; "+
			"falling back to active set", len(activeServices), rule.RequestModel)
		healthyServices = activeServices
	}

	// For single healthy service rules, return it directly
	if len(healthyServices) == 1 {
		return healthyServices[0], nil
	}

	// Always instantiate tactic from rule's params to ensure correct parameters
	actualTactic := rule.LBTactic.Instantiate()
	logTierConfigIgnored(rule, activeServices, actualTactic.GetType())

	// Breaker-aware pick for horizontal tactics: filter to breaker-available
	// services (rule-scoped, non-consuming), pick within that subset, and
	// claim the picked service's probe slot. This keeps a tripped peer out of
	// flat-shape selection (previously only per-request failover masked it)
	// and admits exactly one probe to a recovering peer. TierTactic is left
	// alone — it owns the same walk per tier bucket with its own
	// degrade-to-T0 semantics.
	if actualTactic.GetType() != loadbalance.TacticTier {
		chosen := typ.PickBreakerAvailable(rule.UUID, healthyServices, func(candidates []*loadbalance.Service) *loadbalance.Service {
			return actualTactic.SelectService(ruleView(rule, candidates))
		})
		if chosen != nil {
			return chosen, nil
		}
		// Every healthy service is breaker-open (or its probe is in flight) —
		// degrade to the unfiltered pick below so the request reaches an
		// upstream and the client sees the real upstream error.
		logrus.Warnf("[load_balancer] all %d healthy services for rule %s are breaker-unavailable; "+
			"degrading to unfiltered selection", len(healthyServices), rule.RequestModel)
	}

	// Select service using the tactic (tier walk, or horizontal degrade path).
	selectedService := actualTactic.SelectService(ruleView(rule, healthyServices))
	if selectedService == nil {
		// Fallback to first healthy service
		return healthyServices[0], nil
	}

	return selectedService, nil
}

// ruleView returns a shallow copy of rule narrowed to the given services, so
// a tactic can select within a candidate subset without mutating the source.
func ruleView(rule *typ.Rule, services []*loadbalance.Service) *typ.Rule {
	return &typ.Rule{
		UUID:             rule.UUID,
		RequestModel:     rule.RequestModel,
		ResponseModel:    rule.ResponseModel,
		CurrentServiceID: rule.CurrentServiceID,
		LBTactic:         rule.LBTactic,
		Active:           rule.Active,
		SmartEnabled:     rule.SmartEnabled,
		SmartRouting:     rule.SmartRouting,
		Services:         services,
	}
}

func logTierConfigIgnored(rule *typ.Rule, services []*loadbalance.Service, tacticType loadbalance.TacticType) {
	if rule == nil || tacticType == loadbalance.TacticTier {
		return
	}
	tiers := make(map[int]struct{})
	for _, svc := range services {
		if svc == nil || !svc.Active {
			continue
		}
		tiers[svc.Tier] = struct{}{}
		if len(tiers) > 1 {
			logrus.WithFields(logrus.Fields{
				"stage":     "tier_config_ignored",
				"rule_uuid": rule.UUID,
				"tactic":    tacticType.String(),
			}).Warnf("[load_balancer] rule %s has multiple service tiers but lb_tactic=%s; tier priority is ignored",
				rule.UUID, tacticType.String())
			return
		}
	}
}

// UpdateServiceIndex updates the current service ID for a rule
func (lb *LoadBalancer) UpdateServiceIndex(rule *typ.Rule, selectedService *loadbalance.Service) {
	if rule == nil || selectedService == nil {
		return
	}

	// Set the current service ID (provider:model format)
	rule.CurrentServiceID = selectedService.ServiceID()
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
					sm := lb.config.StoreManager()
					if sm == nil {
						continue
					}
					store := sm.Stats()
					if store == nil {
						continue
					}
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

// ClearServiceStats clears statistics for a specific provider:model — both
// the persisted stats store and the in-memory ServiceStats on every active
// rule service that matches (stats are global per provider:model, shared
// across rules). The previous implementation only touched an internal map
// that was never populated, so the clear was a no-op.
func (lb *LoadBalancer) ClearServiceStats(provider, model string) {
	// 1. Persisted stats store.
	if lb.config != nil {
		if sm := lb.config.StoreManager(); sm != nil {
			if store := sm.Stats(); store != nil {
				if err := store.ClearService(provider, model); err != nil {
					logrus.Warnf("[load_balancer] failed to clear persisted stats for %s/%s: %v", provider, model, err)
				}
			}
		}
	}

	// 2. In-memory ServiceStats on matching active services across all rules.
	// Reset the same field set as ClearAllStats (cumulative + window), so a
	// single-service clear matches the all-clear scope service-by-service.
	if lb.config != nil {
		rules := lb.config.GetRequestConfigs()
		for ruleIdx, rule := range rules {
			if !rule.Active {
				continue
			}
			for i := range rule.Services {
				svc := rule.Services[i]
				if svc.Active && svc.Provider == provider && svc.Model == model {
					stats := &svc.Stats
					stats.RequestCount = 0
					stats.WindowRequestCount = 0
					stats.WindowTokensConsumed = 0
					stats.WindowInputTokens = 0
					stats.WindowOutputTokens = 0
					stats.WindowStart = time.Now()
					stats.LastUsed = time.Time{}
				}
			}
			rules[ruleIdx] = rule
		}
	}
}

// ClearAllStats clears all statistics (both in-memory and persisted in config)
func (lb *LoadBalancer) ClearAllStats() {
	// Clear persisted stats from the dedicated stats store
	if lb.config != nil {
		if sm := lb.config.StoreManager(); sm != nil {
			if store := sm.Stats(); store != nil {
				_ = store.ClearAll()
			}
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
				"avg_latency_ms":       stats.AvgLatencyMs,
				"p50_latency_ms":       stats.P50LatencyMs,
				"p95_latency_ms":       stats.P95LatencyMs,
				"p99_latency_ms":       stats.P99LatencyMs,
				"avg_ttft_ms":          stats.AvgTTFTMs,
				"p95_ttft_ms":          stats.P95TTFTMs,
				"avg_token_speed":      stats.AvgTokenSpeed,
				"cache_hit_rate":       stats.CacheHitRate,
				"window_cache_hits":    stats.WindowCacheHits,
				"window_cache_misses":  stats.WindowCacheMisses,
				"window_cost_tokens":   stats.WindowCostTokens,
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
