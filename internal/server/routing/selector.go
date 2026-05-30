package routing

import (
	"fmt"
	"time"

	"github.com/sirupsen/logrus"

	"github.com/tingly-dev/tingly-box/internal/loadbalance"
	"github.com/tingly-dev/tingly-box/internal/typ"
	pkgobs "github.com/tingly-dev/tingly-box/pkg/obs"
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
	CountByService(serviceID string) int // count active sessions locked to this service
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
)

// ServiceSelector is the main entry point for service selection.
// It orchestrates a pipeline of selection stages and validates the final result.
type ServiceSelector struct {
	config        ProviderResolver
	affinityStore AffinityStore
	loadBalancer  LoadBalancer

	// Pre-built pipelines keyed by mode, built once at construction
	pipelines map[pipelineMode][]SelectionStage
}

type healthFilterProvider interface {
	HealthFilter() *typ.HealthFilter
}

type selectionState struct {
	candidateServices []*loadbalance.Service
}

func newSelectionState(rule *typ.Rule) *selectionState {
	if rule == nil {
		return &selectionState{candidateServices: nil}
	}

	// Use a map to deduplicate services by service ID
	serviceMap := make(map[string]*loadbalance.Service)

	// Add default services
	if rule.Services != nil {
		for _, svc := range rule.Services {
			if svc != nil {
				serviceMap[svc.GetServiceID().String()] = svc
			}
		}
	}

	// Add smart_routing services (override defaults if same ID)
	for _, sr := range rule.SmartRouting {
		if sr.Services != nil {
			for _, svc := range sr.Services {
				if svc != nil {
					serviceMap[svc.GetServiceID().String()] = svc
				}
			}
		}
	}

	// Convert map back to slice
	services := make([]*loadbalance.Service, 0, len(serviceMap))
	for _, svc := range serviceMap {
		services = append(services, svc)
	}

	return &selectionState{candidateServices: services}
}

// NewServiceSelector creates a new service selector
func NewServiceSelector(
	cfg ProviderResolver,
	affinity AffinityStore,
	lb LoadBalancer,
) *ServiceSelector {
	return NewServiceSelectorWithLogger(cfg, affinity, lb, nil)
}

// NewServiceSelectorWithLogger is like NewServiceSelector but also wires the
// multi-logger into smart-routing stages so each evaluation is captured to the
// dedicated smart_routing log source.
func NewServiceSelectorWithLogger(
	cfg ProviderResolver,
	affinity AffinityStore,
	lb LoadBalancer,
	multiLogger *pkgobs.MultiLogger,
) *ServiceSelector {
	s := &ServiceSelector{
		config:        cfg,
		affinityStore: affinity,
		loadBalancer:  lb,
		pipelines:     make(map[pipelineMode][]SelectionStage),
	}

	var healthFilter *typ.HealthFilter
	if p, ok := lb.(healthFilterProvider); ok {
		healthFilter = p.HealthFilter()
	}

	newSmart := func() *SmartRoutingStage {
		stage := NewSmartRoutingStage(lb, affinity)
		if multiLogger != nil {
			stage.SetMultiLogger(multiLogger)
		}
		return stage
	}

	// Pre-build all pipeline variants
	s.pipelines[pipelineModeNoAffinity] = []SelectionStage{
		newSmart(),
		NewLoadBalancerStage(lb),
	}
	s.pipelines[pipelineModeGlobalAffinity] = []SelectionStage{
		NewAffinityStage(affinity, "global"),
		newSmart(),
		NewLoadBalancerStage(lb),
	}
	s.pipelines[pipelineModeSmartAffinity] = []SelectionStage{
		NewHealthStage(healthFilter),
		NewAffinityStage(affinity, "smart_rule"),
		newSmart(),
		NewLoadBalancerStage(lb),
	}

	return s
}

// Select is the main entry point for service selection.
// It picks a pre-built pipeline based on rule configuration and executes it.
func (s *ServiceSelector) Select(ctx *SelectionContext) (*SelectionResult, error) {
	pipeline := s.selectPipeline(ctx.Rule)
	state := newSelectionState(ctx.Rule)

	logrus.Debugf("[selector] executing pipeline with %d stages for rule %s",
		len(pipeline), ctx.Rule.UUID)

	// Execute pipeline stages in order
	for _, stage := range pipeline {
		stageName := stage.Name()
		logrus.Debugf("[selector] evaluating stage: %s", stageName)

		result, handled := stage.Evaluate(ctx, state)

		// Track that this stage was evaluated
		if result != nil {
			result.AddEvaluatedStage(stageName)
			if result.FilteredServices != nil {
				state.candidateServices = result.FilteredServices
			}
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

			logrus.Debugf("[selector] selected service %s from provider %s via %s",
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
	logrus.Debugf("[selector] selectPipeline for rule %s: AffinityEnabled=%v, SmartEnabled=%v, SmartRouting count=%d",
		rule.RequestModel, rule.AffinityEnabled(), rule.SmartEnabled, len(rule.SmartRouting))
	if !rule.AffinityEnabled() {
		return s.pipelines[pipelineModeNoAffinity]
	}

	// Use global affinity scope: the affinity stage runs first and reads the
	// ruleUUID:sessionID key that postProcess writes. The smart_rule-scoped
	// pipeline can't read here because affinity is evaluated before smart
	// routing (MatchedSmartRuleIndex is still unset), so it would never pin.
	// TODO: Read from rule.AffinityScope when per-smart-rule scoping lands.
	return s.pipelines[pipelineModeGlobalAffinity]
}

// getHealthFilter returns the health filter from the load balancer
func (s *ServiceSelector) getHealthFilter() *typ.HealthFilter {
	if p, ok := s.loadBalancer.(healthFilterProvider); ok {
		return p.HealthFilter()
	}
	return nil
}

// postProcess handles post-selection logic like affinity locking.
// Locks affinity whenever affinity is enabled and the source is not "affinity"
// (i.e., don't re-lock an already-locked entry).
func (s *ServiceSelector) postProcess(ctx *SelectionContext, result *SelectionResult) {
	if result.Source == "affinity" || !ctx.Rule.AffinityEnabled() || ctx.SessionID.IsEmpty() {
		return
	}

	ttl := ctx.Rule.AffinityTTL()
	if ttl == 0 {
		// Legacy SmartAffinity bool — use a sensible default.
		ttl = 2 * time.Hour
	}
	s.affinityStore.Set(ctx.Rule.UUID, ctx.SessionID.String(), &AffinityEntry{
		Service:   result.Service,
		LockedAt:  time.Now(),
		ExpiresAt: time.Now().Add(ttl),
	})
	logrus.Infof("[affinity] locked service %s -> %s for session %s",
		result.Provider.Name, result.Service.Model, ctx.SessionID.String())
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
