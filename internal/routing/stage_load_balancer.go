package routing

import (
	"context"
	"fmt"

	"github.com/sirupsen/logrus"

	"github.com/tingly-dev/tingly-box/internal/loadbalance"
	"github.com/tingly-dev/tingly-box/internal/typ"
)

// LoadBalancerStage performs standard load balancing across all rule services.
// This stage always returns a service (or error), acting as the final fallback.
type LoadBalancerStage struct {
	loadBalancer LoadBalancer
}

// NewLoadBalancerStage creates a new load balancer stage
func NewLoadBalancerStage(lb LoadBalancer) *LoadBalancerStage {
	return &LoadBalancerStage{
		loadBalancer: lb,
	}
}

// Name returns the stage identifier
func (s *LoadBalancerStage) Name() string {
	return SourceLoadBalancer
}

// Evaluate selects a service using load balancing. This is the terminal
// stage: a failure here is a real error (there is no next stage to fall
// through to), so it is reported via err rather than a silent pass-through.
func (s *LoadBalancerStage) Evaluate(ctx *SelectionContext, candidates []*loadbalance.Service) ([]*loadbalance.Service, *SelectionResult, error) {
	// An empty candidate set here means the rule genuinely has no active
	// services: the pipeline driver (ServiceSelector.Select) restores the
	// active base pool between stages whenever a narrowing comes back empty,
	// so the terminal stage never has to compensate — it just reports the
	// config problem via SelectService's error.
	tempRule := *ctx.Rule
	tempRule.Services = candidates
	logOpenBreakerSkips(ctx, &tempRule)

	service, err := selectWithRequestContext(s.loadBalancer, selectionLogContext(ctx), &tempRule)
	if err != nil {
		return candidates, nil, fmt.Errorf("selection failed: %w", err)
	}

	if service == nil {
		return candidates, nil, fmt.Errorf("no service returned")
	}

	// When a smart partition matched, the candidate set IS that partition, so
	// label the pick smart_routing — preserving the pre-reorder observability
	// contract (source=smart_routing ⇒ picked from a smart-matched subset).
	source := SourceLoadBalancer
	if ctx.MatchedSmartRuleIndex >= 0 {
		source = SourceSmartRouting
	}
	result := NewResult(service, source)
	result.MatchedSmartRuleIndex = ctx.MatchedSmartRuleIndex
	return candidates, result, nil
}

func logOpenBreakerSkips(ctx *SelectionContext, rule interface {
	GetTacticType() loadbalance.TacticType
	GetActiveServices() []*loadbalance.Service
}) {
	if ctx == nil || ctx.Rule == nil || rule == nil || rule.GetTacticType() != loadbalance.TacticTier {
		return
	}
	store := loadbalance.DefaultBreakerStore()
	for _, svc := range rule.GetActiveServices() {
		if svc == nil {
			continue
		}
		state := store.Get(ctx.Rule.UUID, svc.ServiceID()).State()
		if state != loadbalance.BreakerOpen {
			continue
		}
		logrus.WithContext(selectionLogContext(ctx)).WithFields(logrus.Fields{
			"stage":         "routing_breaker_skipped",
			"rule_uuid":     ctx.Rule.UUID,
			"scenario":      string(ctx.Scenario),
			"request_model": ctx.Rule.RequestModel,
			"lb_tactic":     ctx.Rule.GetTacticType().String(),
			"service":       svc.ServiceID(),
			"provider_uuid": svc.Provider,
			"attempt_model": svc.Model,
			"tier":          svc.Tier,
			"breaker_state": state.String(),
		}).Warnf("[routing] skipped %s because breaker is %s", svc.ServiceID(), state.String())
	}
}

// contextLoadBalancer is implemented by balancers that can log against the
// request being routed (protocolserver.LoadBalancer). It is optional so the
// LoadBalancer interface and its test doubles stay context-free.
type contextLoadBalancer interface {
	SelectServiceCtx(ctx context.Context, rule *typ.Rule) (*loadbalance.Service, error)
}

func selectWithRequestContext(lb LoadBalancer, ctx context.Context, rule *typ.Rule) (*loadbalance.Service, error) {
	if clb, ok := lb.(contextLoadBalancer); ok {
		return clb.SelectServiceCtx(ctx, rule)
	}
	return lb.SelectService(rule)
}
