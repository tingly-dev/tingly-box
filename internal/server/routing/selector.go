package routing

import (
	"fmt"
	"time"

	"github.com/sirupsen/logrus"

	"github.com/tingly-dev/tingly-box/internal/loadbalance"
	"github.com/tingly-dev/tingly-box/internal/server/config"
	"github.com/tingly-dev/tingly-box/internal/typ"
)

// AffinityStore interface defines operations for session-service affinity
type AffinityStore interface {
	Get(ruleUUID, sessionID string) (*AffinityEntry, bool)
	Set(ruleUUID, sessionID string, entry *AffinityEntry)
}

// AffinityEntry represents a locked service for a session
type AffinityEntry struct {
	Service   *loadbalance.Service
	MessageID string
	LockedAt  time.Time
	ExpiresAt time.Time
}

// ServiceSelector is the main entry point for service selection.
// It orchestrates a pipeline of selection stages and validates the final result.
type ServiceSelector struct {
	config        *config.Config
	affinityStore AffinityStore
	loadBalancer  interface{} // *server.LoadBalancer
}

// NewServiceSelector creates a new service selector
func NewServiceSelector(
	cfg *config.Config,
	affinity AffinityStore,
	lb interface{},
) *ServiceSelector {
	return &ServiceSelector{
		config:        cfg,
		affinityStore: affinity,
		loadBalancer:  lb,
	}
}

// Select is the main entry point for service selection.
// It builds a pipeline based on rule configuration and executes it.
func (s *ServiceSelector) Select(ctx *SelectionContext) (*SelectionResult, error) {
	// Build pipeline based on rule configuration
	pipeline := s.buildPipeline(ctx.Rule)

	logrus.Debugf("[selector] executing pipeline with %d stages for rule %s",
		len(pipeline), ctx.Rule.UUID)

	// Execute pipeline stages in order
	for _, stage := range pipeline {
		stageName := stage.Name()
		logrus.Debugf("[selector] evaluating stage: %s", stageName)

		result, handled := stage.Evaluate(ctx)

		// Track that this stage was evaluated
		if result != nil {
			result.AddEvaluatedStage(stageName)
		}

		if handled {
			// Stage produced a result, validate and return
			if result == nil || result.Service == nil {
				logrus.Warnf("[selector] stage %s returned handled=true but nil result", stageName)
				continue
			}

			// Validate service is active
			if !result.Service.Active {
				logrus.Debugf("[selector] stage %s returned inactive service, trying next stage", stageName)
				continue
			}

			// Resolve provider
			provider, err := s.config.GetProviderByUUID(result.Service.Provider)
			if err != nil {
				logrus.Debugf("[selector] provider not found for service: %v, trying next stage", err)
				continue
			}

			if !provider.Enabled {
				logrus.Debugf("[selector] provider %s is disabled, trying next stage", provider.Name)
				continue
			}

			result.Provider = provider

			// Post-process: lock affinity if needed
			s.postProcess(ctx, result)

			logrus.Infof("[selector] selected service %s from provider %s via %s",
				result.Service.Model, provider.Name, result.Source)

			return result, nil
		}

		logrus.Debugf("[selector] stage %s passed to next stage", stageName)
	}

	return nil, fmt.Errorf("no service available for rule %s (model: %s)",
		ctx.Rule.UUID, ctx.Rule.RequestModel)
}

// buildPipeline constructs the selection pipeline based on rule configuration
func (s *ServiceSelector) buildPipeline(rule *typ.Rule) []SelectionStage {
	var pipeline []SelectionStage

	// Determine affinity scope from rule configuration
	// Default to "global" for backward compatibility
	affinityScope := "global"
	// TODO: Read from rule.AffinityScope when field is added

	if !rule.SmartAffinity {
		// No affinity: just smart routing + load balancer
		pipeline = append(pipeline, NewSmartRoutingStage(s.loadBalancer))
		pipeline = append(pipeline, NewLoadBalancerStage(s.loadBalancer))
		return pipeline
	}

	if affinityScope == "global" {
		// Global affinity: check affinity first, then smart routing, then LB
		pipeline = append(pipeline, NewAffinityStage(s.affinityStore, "global"))
		pipeline = append(pipeline, NewSmartRoutingStage(s.loadBalancer))
		pipeline = append(pipeline, NewLoadBalancerStage(s.loadBalancer))
	} else {
		// Smart rule affinity: evaluate smart routing first, then check affinity within matched rule
		pipeline = append(pipeline, NewSmartRoutingStage(s.loadBalancer))
		pipeline = append(pipeline, NewAffinityStage(s.affinityStore, "smart_rule"))
		pipeline = append(pipeline, NewLoadBalancerStage(s.loadBalancer))
	}

	return pipeline
}

// postProcess handles post-selection logic like affinity locking
func (s *ServiceSelector) postProcess(ctx *SelectionContext, result *SelectionResult) {
	// Lock affinity if applicable
	if result.Source == "smart_routing" && ctx.Rule.SmartAffinity && ctx.SessionID != "" {
		s.affinityStore.Set(ctx.Rule.UUID, ctx.SessionID, &AffinityEntry{
			Service:   result.Service,
			LockedAt:  time.Now(),
			ExpiresAt: time.Now().Add(2 * time.Hour), // TODO: make configurable
		})
		logrus.Infof("[affinity] locked service %s -> %s for session %s",
			result.Provider.Name, result.Service.Model, ctx.SessionID)
	}

	// TODO: Update metrics, persist CurrentServiceID, etc.
}

// UpdateServiceIndex updates the current service index for round-robin (legacy method)
// This is called from the handler after selection to persist state
func (s *ServiceSelector) UpdateServiceIndex(rule *typ.Rule, service *loadbalance.Service) error {
	// Type assert to LoadBalancer interface
	type loadBalancer interface {
		UpdateServiceIndex(rule *typ.Rule, service *loadbalance.Service)
	}

	if lb, ok := s.loadBalancer.(loadBalancer); ok {
		lb.UpdateServiceIndex(rule, service)
	}

	// Persist to database
	if err := s.config.SaveCurrentServiceID(rule.UUID, rule.CurrentServiceID); err != nil {
		return fmt.Errorf("failed to persist CurrentServiceID: %w", err)
	}

	return nil
}
