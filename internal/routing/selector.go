package routing

import (
	"fmt"
	"time"

	"github.com/sirupsen/logrus"

	"github.com/tingly-dev/tingly-box/internal/clock"
	"github.com/tingly-dev/tingly-box/internal/loadbalance"
	"github.com/tingly-dev/tingly-box/internal/typ"
	pkgobs "github.com/tingly-dev/tingly-box/pkg/obs"
)

// ProviderResolver resolves providers by UUID and persists config state.
type ProviderResolver interface {
	GetProviderByUUID(uuid string) (*typ.Provider, error)
	// GetEffectiveAffinity returns the effective affinity TTL for a rule,
	// considering both scenario default and rule override. Returns 0 if disabled.
	GetEffectiveAffinity(rule *typ.Rule) time.Duration
}

// LoadBalancer defines the interface for load balancing operations.
// This avoids importing the server package (which would create circular imports).
type LoadBalancer interface {
	SelectService(rule *typ.Rule) (*loadbalance.Service, error)
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

// ServiceSelector is the main entry point for service selection.
// It orchestrates a pipeline of selection stages and validates the final result.
type ServiceSelector struct {
	config        ProviderResolver
	affinityStore AffinityStore
	loadBalancer  LoadBalancer

	// One pipeline serves every rule — each stage self-guards (see the comment
	// in NewServiceSelectorWithLogger). Built once at construction.
	pipeline []SelectionStage
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

	// Deduplicate services by service ID while preserving first-seen order.
	// Map iteration order in Go is randomized, so building the candidate slice
	// from a map would make routing (and tests) non-deterministic. We keep an
	// ordered slice and use the index map only to dedupe / override in place.
	services := make([]*loadbalance.Service, 0, len(rule.Services))
	indexByID := make(map[string]int)

	add := func(svc *loadbalance.Service) {
		if svc == nil {
			return
		}
		id := svc.GetServiceID().String()
		if i, ok := indexByID[id]; ok {
			// Override the existing entry in place, keeping its position.
			services[i] = svc
			return
		}
		indexByID[id] = len(services)
		services = append(services, svc)
	}

	// Add default services.
	for _, svc := range rule.Services {
		add(svc)
	}

	// Add smart_routing services (override defaults if same ID).
	for _, sr := range rule.SmartRouting {
		for _, svc := range sr.Services {
			add(svc)
		}
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
	}

	var healthFilter *typ.HealthFilter
	if p, ok := lb.(healthFilterProvider); ok {
		healthFilter = p.HealthFilter()
	}

	newSmart := func() *SmartRoutingStage {
		stage := NewSmartRoutingStage(affinity)
		if multiLogger != nil {
			stage.SetMultiLogger(multiLogger)
		}
		return stage
	}

	// One pipeline serves every rule — order is health → smart → affinity →
	// strategy. Each stage self-guards, so there is no need for per-mode
	// variants:
	//   - Health  narrows the candidate set by 429/auth health; passes through
	//     when no filter is set or filtering would empty the set (degrade).
	//   - Smart   runs BEFORE affinity: it decides the content partition (and
	//     always runs processor ops, which mutate the request) by narrowing
	//     the candidate set to the matched subset. Running it after affinity
	//     let a first-request pin defeat content routing for the whole
	//     session TTL — and skip processor ops entirely for pinned sessions.
	//     Passes through when smart routing is off or unmatched.
	//   - Affinity returns nothing when affinity is disabled or there's no
	//     session. Pins are scoped per matched partition (AffinitySessionKey),
	//     so stickiness lives INSIDE the content decision, never above it.
	//     The breaker-driven (500) signal is consulted inside the affinity
	//     tier check and the tier tactic; Health feeds it the 429/auth signal.
	//   - LB      always selects (the terminal fallback) within the narrowed
	//     candidate set, labeling the result smart_routing when a partition
	//     matched.
	s.pipeline = []SelectionStage{
		NewHealthStage(healthFilter),
		newSmart(),
		NewAffinityStage(affinity),
		NewLoadBalancerStage(lb),
	}

	return s
}

// Select is the main entry point for service selection.
// It picks a pre-built pipeline based on rule configuration and executes it.
func (s *ServiceSelector) Select(ctx *SelectionContext) (*SelectionResult, error) {
	state := newSelectionState(ctx.Rule)
	evaluatedStages := make([]string, 0, len(s.pipeline))

	logrus.Debugf("[selector] executing pipeline with %d stages for rule %s",
		len(s.pipeline), ctx.Rule.UUID)

	// Execute pipeline stages in order
	for _, stage := range s.pipeline {
		stageName := stage.Name()
		evaluatedStages = append(evaluatedStages, stageName)
		logrus.Debugf("[selector] evaluating stage: %s", stageName)

		result, handled := stage.Evaluate(ctx, state)

		if result != nil {
			if result.FilteredServices != nil {
				state.candidateServices = result.FilteredServices
			}
		}

		if handled {
			if result != nil {
				// A filter stage's result is intentionally replaced as the
				// pipeline advances. Attach the cumulative path at the terminal
				// boundary so observability reports every stage that actually ran,
				// including pass-through stages that returned nil.
				result.EvaluatedStages = append([]string(nil), evaluatedStages...)
			}
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

// postProcess handles post-selection logic like affinity locking.
// Locks affinity whenever affinity is enabled and the source is not SourceAffinity
// (i.e., don't re-lock an already-locked entry).
func (s *ServiceSelector) postProcess(ctx *SelectionContext, result *SelectionResult) {
	if result.Source == SourceAffinity || ctx.SessionID.IsEmpty() {
		return
	}

	ttl := s.config.GetEffectiveAffinity(ctx.Rule)
	if ttl == 0 {
		// Affinity disabled for this rule.
		return
	}
	// Pin inside the partition this request was routed by, so one session can
	// hold independent pins per request kind (see AffinitySessionKey).
	affinityKey := AffinitySessionKey(ctx.SessionID.String(), ctx.MatchedSmartRuleIndex)
	s.affinityStore.Set(ctx.Rule.UUID, affinityKey, &AffinityEntry{
		Service:   result.Service,
		LockedAt:  clock.Now(),
		ExpiresAt: clock.Now().Add(ttl),
	})
	logrus.Infof("[affinity] locked service %s -> %s for session key %s",
		result.Provider.Name, result.Service.Model, affinityKey)
}
