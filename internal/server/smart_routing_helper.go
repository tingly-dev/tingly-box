package server

import (
	"github.com/anthropics/anthropic-sdk-go"
	"github.com/openai/openai-go/v3"
	"github.com/sirupsen/logrus"

	"github.com/tingly-dev/tingly-box/internal/loadbalance"
	smartrouting "github.com/tingly-dev/tingly-box/internal/smart_routing"
	"github.com/tingly-dev/tingly-box/internal/typ"
)

// ExtractRequestContext extracts RequestContext from request based on type
func (s *Server) ExtractRequestContext(req interface{}) (*smartrouting.RequestContext, error) {
	switch r := req.(type) {
	case *openai.ChatCompletionNewParams:
		return smartrouting.ExtractContextFromOpenAIRequest(r), nil
	case *anthropic.MessageNewParams:
		return smartrouting.ExtractContextFromAnthropicRequest(r), nil
	case *anthropic.BetaMessageNewParams:
		return smartrouting.ExtractContextFromBetaRequest(r), nil
	default:
		logrus.Debugf("[smart_routing] unknown request type %T, cannot extract context", req)
		return nil, nil
	}
}

// SelectServiceFromSmartRouting selects a service from matched smart routing services
// Creates a temporary rule with the matched services and uses the configured load balancing tactic
func (s *Server) SelectServiceFromSmartRouting(matchedServices []loadbalance.Service, rule *typ.Rule) (*loadbalance.Service, error) {
	if len(matchedServices) == 0 {
		return nil, nil
	}

	// Filter active services
	var activeServices []loadbalance.Service
	for _, service := range matchedServices {
		if service.Active {
			activeServices = append(activeServices, service)
		}
	}

	if len(activeServices) == 0 {
		return nil, nil
	}

	// For single service, return it directly
	if len(activeServices) == 1 {
		return &activeServices[0], nil
	}

	// Create a temporary rule with the matched services for load balancing
	tempRule := *rule // Copy the rule
	tempRule.Services = activeServices
	// Reset CurrentServiceIndex to 0 since this is a different set of services
	tempRule.CurrentServiceIndex = 0

	// Use the load balancer to select from the temporary rule
	selectedService, err := s.loadBalancer.SelectService(&tempRule)
	if err != nil {
		logrus.Debugf("[smart_routing] failed to select service from matched services: %v", err)
		return nil, err
	}

	return selectedService, nil
}
