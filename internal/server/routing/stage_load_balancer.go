package routing

import (
	"github.com/sirupsen/logrus"

	"github.com/tingly-dev/tingly-box/internal/loadbalance"
	"github.com/tingly-dev/tingly-box/internal/typ"
)

// LoadBalancerStage performs standard load balancing across all rule services.
// This stage always returns a service (or error), acting as the final fallback.
type LoadBalancerStage struct {
	loadBalancer interface{} // Will be *server.LoadBalancer
}

// NewLoadBalancerStage creates a new load balancer stage
func NewLoadBalancerStage(lb interface{}) *LoadBalancerStage {
	return &LoadBalancerStage{
		loadBalancer: lb,
	}
}

// Name returns the stage identifier
func (s *LoadBalancerStage) Name() string {
	return "load_balancer"
}

// Evaluate selects a service using load balancing
func (s *LoadBalancerStage) Evaluate(ctx *SelectionContext) (*SelectionResult, bool) {
	// Type assert to LoadBalancer interface
	type loadBalancer interface {
		SelectService(rule *typ.Rule) (*loadbalance.Service, error)
	}

	lb, ok := s.loadBalancer.(loadBalancer)
	if !ok {
		logrus.Errorf("[load_balancer] invalid load balancer type")
		return nil, false
	}

	service, err := lb.SelectService(ctx.Rule)
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
