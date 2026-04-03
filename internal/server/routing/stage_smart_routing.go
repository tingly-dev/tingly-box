package routing

import (
	"github.com/sirupsen/logrus"

	"github.com/tingly-dev/tingly-box/internal/loadbalance"
	smartrouting "github.com/tingly-dev/tingly-box/internal/smart_routing"
	"github.com/tingly-dev/tingly-box/internal/typ"
)

// SmartRoutingStage evaluates smart routing rules and returns matched services.
// If multiple services match, applies load balancing within the matched set.
type SmartRoutingStage struct {
	loadBalancer LoadBalancer
}

// NewSmartRoutingStage creates a new smart routing stage
func NewSmartRoutingStage(lb LoadBalancer) *SmartRoutingStage {
	return &SmartRoutingStage{
		loadBalancer: lb,
	}
}

// Name returns the stage identifier
func (s *SmartRoutingStage) Name() string {
	return "smart_routing"
}

// Evaluate evaluates smart routing rules and selects a service
func (s *SmartRoutingStage) Evaluate(ctx *SelectionContext, state *selectionState) (*SelectionResult, bool) {
	rule := ctx.Rule

	// Skip if smart routing not enabled
	if !rule.SmartEnabled || len(rule.SmartRouting) == 0 || ctx.Request == nil {
		return nil, false
	}

	logrus.Debugf("[smart_routing] evaluating %d rules for model %s", len(rule.SmartRouting), rule.RequestModel)

	// Extract request context
	reqCtx, err := ExtractRequestContext(ctx.Request)
	if err != nil {
		logrus.Debugf("[smart_routing] failed to extract context: %v", err)
		return nil, false
	}

	// Create router and evaluate
	router, err := smartrouting.NewRouter(rule.SmartRouting)
	if err != nil {
		logrus.Debugf("[smart_routing] failed to create router: %v", err)
		return nil, false
	}

	matchedServices, matchedRuleIndex, matched := router.EvaluateRequestWithIndex(reqCtx)
	if !matched || len(matchedServices) == 0 {
		logrus.Debugf("[smart_routing] no rule matched")
		return nil, false
	}

	if state != nil && len(state.candidateServices) > 0 {
		matchedServices = IntersectServices(matchedServices, state.candidateServices)
	}
	if len(matchedServices) == 0 {
		logrus.Debugf("[smart_routing] matched rule has no services in current candidate set")
		return nil, false
	}

	ctx.MatchedSmartRuleIndex = matchedRuleIndex

	logrus.Debugf("[smart_routing] rule %d matched, selecting from %d services",
		matchedRuleIndex, len(matchedServices))

	// Filter active services
	activeServices := FilterActiveServices(matchedServices)
	if len(activeServices) == 0 {
		logrus.Debugf("[smart_routing] no active services in matched set")
		return nil, false
	}

	// Single service? Return it directly
	if len(activeServices) == 1 {
		result := NewResult(activeServices[0], "smart_routing")
		result.MatchedSmartRuleIndex = matchedRuleIndex
		return result, true
	}

	// Multiple services: apply load balancing within matched set
	service := s.selectFromServices(activeServices, rule)
	if service == nil {
		return nil, false
	}

	result := NewResult(service, "smart_routing")
	result.MatchedSmartRuleIndex = matchedRuleIndex
	return result, true
}

// selectFromServices applies load balancing to select one service from the matched set
func (s *SmartRoutingStage) selectFromServices(services []*loadbalance.Service, rule *typ.Rule) *loadbalance.Service {
	if len(services) == 0 {
		return nil
	}

	if len(services) == 1 {
		return services[0]
	}

	// Create a temporary rule with only the matched services for load balancing
	tempRule := *rule // Copy the rule
	tempRule.Services = services
	tempRule.CurrentServiceID = "" // Reset service ID for this sub-selection

	service, err := s.loadBalancer.SelectService(&tempRule)
	if err != nil {
		logrus.Debugf("[smart_routing] load balancer selection failed: %v", err)
		return nil
	}
	return service
}
