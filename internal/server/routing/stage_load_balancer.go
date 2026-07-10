package routing

import (
	"github.com/sirupsen/logrus"

	"github.com/tingly-dev/tingly-box/internal/loadbalance"
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
	return "load_balancer"
}

// Evaluate selects a service using load balancing
func (s *LoadBalancerStage) Evaluate(ctx *SelectionContext, state *selectionState) (*SelectionResult, bool) {
	tempRule := *ctx.Rule
	if state != nil {
		tempRule.Services = state.candidateServices
	}
	logOpenBreakerSkips(ctx, &tempRule)

	service, err := s.loadBalancer.SelectService(&tempRule)
	if err != nil {
		logrus.Errorf("[load_balancer] selection failed: %v", err)
		return nil, false
	}

	if service == nil {
		logrus.Errorf("[load_balancer] no service returned")
		return nil, false
	}

	result := NewResult(service, "load_balancer")
	return result, true
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
