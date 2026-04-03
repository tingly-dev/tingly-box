package routing

import (
	"github.com/sirupsen/logrus"
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
