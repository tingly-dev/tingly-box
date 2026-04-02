package routing

import (
	"fmt"
	"math/rand"

	"github.com/sirupsen/logrus"

	"github.com/tingly-dev/tingly-box/internal/loadbalance"
	"github.com/tingly-dev/tingly-box/internal/typ"
)

// CapacityStage performs capacity-based load balancing.
// It integrates with the affinity system: for existing sessions, it reuses affinity;
// for new sessions, it selects based on available capacity using weighted random.
type CapacityStage struct {
	tracker *loadbalance.SessionTracker
	store   AffinityStore
}

// NewCapacityStage creates a new capacity-based stage with session tracker and affinity store
func NewCapacityStage(tracker *loadbalance.SessionTracker, store AffinityStore) *CapacityStage {
	return &CapacityStage{
		tracker: tracker,
		store:   store,
	}
}

// Name returns the stage identifier
func (s *CapacityStage) Name() string {
	return "capacity"
}

// Evaluate performs capacity-based service selection.
// 1. Check affinity store for existing session (reuse existing affinity)
// 2. For new sessions, select based on available capacity using weighted random
// 3. Record activity for affinity sessions
func (s *CapacityStage) Evaluate(ctx *SelectionContext) (*SelectionResult, bool) {
	rule := ctx.Rule

	// Skip if capacity-based tactic is not configured
	if rule.GetTacticType() != loadbalance.TacticCapacityBased {
		return nil, false
	}

	// If no session ID, fall through to regular selection
	if ctx.SessionID == "" {
		return nil, false
	}

	// Check affinity store for existing session
	if s.store != nil {
		entry, ok := s.store.Get(rule.UUID, ctx.SessionID)
		if ok {
			// Session has existing affinity, record activity and use it
			if s.tracker != nil {
				s.tracker.RecordActivity(ctx.SessionID)
			}
			logrus.Debugf("[capacity] session %s has affinity to %s",
				ctx.SessionID, entry.Service.Model)
			result := NewResult(entry.Service, "affinity")
			result.MatchedSmartRuleIndex = ctx.MatchedSmartRuleIndex
			return result, true
		}
	}

	// New session: select based on capacity
	available := s.getAvailableServicesWithCapacity(rule.Services)
	if len(available) == 0 {
		logrus.Debugf("[capacity] no services with available capacity")
		return nil, false
	}

	// Select using weighted random by available capacity
	selected := s.weightedRandomSelect(available)
	if selected == nil {
		return nil, false
	}

	// Try to acquire capacity slot
	if s.tracker != nil {
		acquired, err := s.tracker.TryAcquire(ctx.SessionID, selected.Provider, selected.Model)
		if err != nil {
			logrus.Debugf("[capacity] failed to acquire capacity for %s: %v", selected.ServiceID(), err)
			return nil, false
		}
		if !acquired {
			logrus.Debugf("[capacity] capacity full for %s", selected.ServiceID())
			return nil, false
		}
	}

	logrus.Debugf("[capacity] selected %s for new session %s", selected.ServiceID(), ctx.SessionID)
	result := NewResult(selected, "capacity")
	result.MatchedSmartRuleIndex = ctx.MatchedSmartRuleIndex
	return result, true
}

// getAvailableServicesWithCapacity returns services with available capacity and their weights
func (s *CapacityStage) getAvailableServicesWithCapacity(services []*loadbalance.Service) []*loadbalance.Service {
	if s.tracker == nil {
		// No tracker, return all active services with equal weight
		return FilterActiveServices(services)
	}

	return s.tracker.GetAvailableServices(services)
}

// weightedRandomSelect selects a service weighted by available capacity
func (s *CapacityStage) weightedRandomSelect(services []*loadbalance.Service) *loadbalance.Service {
	if len(services) == 0 {
		return nil
	}
	if len(services) == 1 {
		return services[0]
	}

	// Calculate weights based on available capacity
	weights := make([]int64, len(services))
	totalWeight := int64(0)

	for i, svc := range services {
		var cap int64 = 100 // Default weight for unlimited services
		if s.tracker != nil {
			cap = s.tracker.GetAvailableCapacity(svc)
			if cap < 0 {
				cap = 100 // Treat unlimited (-1) as default weight
			}
		}
		weights[i] = cap
		totalWeight += cap
	}

	if totalWeight == 0 {
		// All at capacity, return first service
		return services[0]
	}

	// Random selection
	r := rand.Int63n(totalWeight)
	cumulative := int64(0)
	for i, w := range weights {
		cumulative += w
		if r < cumulative {
			return services[i]
		}
	}

	return services[len(services)-1]
}

// SelectService selects a service using capacity-based logic.
// This implements the LoadBalancer interface for integration with existing pipelines.
func (s *CapacityStage) SelectService(rule *typ.Rule) (*loadbalance.Service, error) {
	if rule == nil || len(rule.Services) == 0 {
		return nil, fmt.Errorf("no services available")
	}

	available := s.getAvailableServicesWithCapacity(rule.Services)
	if len(available) == 0 {
		return nil, fmt.Errorf("all services at capacity")
	}

	return s.weightedRandomSelect(available), nil
}

// UpdateServiceIndex is a no-op for capacity-based selection.
// Capacity is managed by SessionTracker, not by service index.
func (s *CapacityStage) UpdateServiceIndex(rule *typ.Rule, service *loadbalance.Service) {
	// No-op: capacity-based selection doesn't use service index
}
