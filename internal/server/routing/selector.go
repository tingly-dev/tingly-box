package routing

import (
	"fmt"
	"time"

	"github.com/sirupsen/logrus"

	"github.com/tingly-dev/tingly-box/internal/loadbalance"
	"github.com/tingly-dev/tingly-box/internal/typ"
)

// ProviderResolver resolves providers by UUID and persists config state.
type ProviderResolver interface {
	GetProviderByUUID(uuid string) (*typ.Provider, error)
	SaveCurrentServiceID(ruleUUID string, serviceID string) error
}

// LoadBalancer defines the interface for load balancing operations.
// This avoids importing the server package (which would create circular imports).
type LoadBalancer interface {
	SelectService(rule *typ.Rule) (*loadbalance.Service, error)
	UpdateServiceIndex(rule *typ.Rule, service *loadbalance.Service)
}

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

// pipelineMode determines which pipeline configuration to use.
type pipelineMode int

const (
	pipelineModeNoAffinity     pipelineMode = iota // Smart Routing -> Load Balancer
	pipelineModeGlobalAffinity                     // Affinity -> Smart Routing -> Load Balancer
	pipelineModeSmartAffinity                      // Smart Routing -> Affinity -> Load Balancer
	pipelineModeCapacity                           // Capacity-based selection (uses CapacityStage)
)

// ServiceSelector is the main entry point for service selection.
// It orchestrates a pipeline of selection stages and validates the final result.
type ServiceSelector struct {
	config        ProviderResolver
	affinityStore AffinityStore
	loadBalancer  LoadBalancer
	capacityStage *CapacityStage

	// Pre-built pipelines keyed by mode, built once at construction
	pipelines map[pipelineMode][]SelectionStage
}

// NewServiceSelector creates a new service selector
func NewServiceSelector(
	cfg ProviderResolver,
	affinity AffinityStore,
	lb LoadBalancer,
	capacityStage *CapacityStage,
) *ServiceSelector {
	s := &ServiceSelector{
		config:        cfg,
		affinityStore: affinity,
		loadBalancer:  lb,
		capacityStage: capacityStage,
		pipelines:     make(map[pipelineMode][]SelectionStage),
	}

	// Pre-build all pipeline variants
	s.pipelines[pipelineModeNoAffinity] = []SelectionStage{
		NewSmartRoutingStage(lb),
		NewLoadBalancerStage(lb),
	}
	s.pipelines[pipelineModeGlobalAffinity] = []SelectionStage{
		NewAffinityStage(affinity, "global"),
		NewSmartRoutingStage(lb),
		NewLoadBalancerStage(lb),
	}
	s.pipelines[pipelineModeSmartAffinity] = []SelectionStage{
		NewSmartRoutingStage(lb),
		NewAffinityStage(affinity, "smart_rule"),
		NewLoadBalancerStage(lb),
	}

	// Build capacity-based pipeline if capacity stage is provided
	if capacityStage != nil {
		s.pipelines[pipelineModeCapacity] = []SelectionStage{
			capacityStage,
			NewLoadBalancerStage(capacityStage), // Fallback to capacity-based LB
		}
	}

	return s
}

// Select is the main entry point for service selection.
// It picks a pre-built pipeline based on rule configuration and executes it.
func (s *ServiceSelector) Select(ctx *SelectionContext) (*SelectionResult, error) {
	pipeline := s.selectPipeline(ctx.Rule)

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

// selectPipeline picks the appropriate pre-built pipeline based on rule configuration.
func (s *ServiceSelector) selectPipeline(rule *typ.Rule) []SelectionStage {
	// Check for capacity-based tactic first
	if rule.GetTacticType() == loadbalance.TacticCapacityBased && s.capacityStage != nil {
		return s.pipelines[pipelineModeCapacity]
	}

	if !rule.SmartAffinity {
		return s.pipelines[pipelineModeNoAffinity]
	}

	// Default to global affinity scope.
	// TODO: Read from rule.AffinityScope when field is added.
	return s.pipelines[pipelineModeGlobalAffinity]
}

// postProcess handles post-selection logic like affinity locking.
// Locks affinity whenever affinity is enabled and the source is not "affinity"
// (i.e., don't re-lock an already-locked entry).
func (s *ServiceSelector) postProcess(ctx *SelectionContext, result *SelectionResult) {
	if result.Source == "affinity" || !ctx.Rule.SmartAffinity || ctx.SessionID == "" {
		return
	}

	s.affinityStore.Set(ctx.Rule.UUID, ctx.SessionID, &AffinityEntry{
		Service:   result.Service,
		LockedAt:  time.Now(),
		ExpiresAt: time.Now().Add(2 * time.Hour), // TODO: make configurable
	})
	logrus.Infof("[affinity] locked service %s -> %s for session %s",
		result.Provider.Name, result.Service.Model, ctx.SessionID)
}

// UpdateServiceIndex updates the current service index for round-robin.
// This is called from the handler after selection to persist state.
func (s *ServiceSelector) UpdateServiceIndex(rule *typ.Rule, service *loadbalance.Service) error {
	s.loadBalancer.UpdateServiceIndex(rule, service)

	if err := s.config.SaveCurrentServiceID(rule.UUID, rule.CurrentServiceID); err != nil {
		return fmt.Errorf("failed to persist CurrentServiceID: %w", err)
	}

	return nil
}
