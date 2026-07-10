package routing

import (
	"fmt"
	"strings"

	"github.com/gin-gonic/gin"
	"github.com/sirupsen/logrus"

	"github.com/tingly-dev/tingly-box/internal/constant"
	"github.com/tingly-dev/tingly-box/internal/loadbalance"
	"github.com/tingly-dev/tingly-box/internal/typ"
)

// SimpleSelector is the production-facing facade over the ServiceSelector
// pipeline. It adds the probe-pin bypass, gin session/observability plumbing,
// and debug headers on top of the raw stage pipeline. Request handlers should
// use it; only diagnostics (e.g. the LB simulator) drive ServiceSelector.Select
// directly.
type SimpleSelector struct {
	selector *ServiceSelector
}

// NewSimpleSelector creates a simplified selector
func NewSimpleSelector(selector *ServiceSelector) *SimpleSelector {
	return &SimpleSelector{selector: selector}
}

// SelectService resolves the session, runs the selection pipeline, validates
// the provider, and records the decision (gin context keys, log line, debug
// headers). The resolved session ID is stored in the gin context.
func (s *SimpleSelector) SelectService(
	c *gin.Context,
	scenario typ.RuleScenario,
	rule *typ.Rule,
	req interface{},
) (*typ.Provider, *loadbalance.Service, error) {
	// X-Tingly-Probe-Service: {provider_uuid}:{model} — bypass all pipeline
	// stages and pin to the specified service. Used by service-target probes
	// that need to route through TB's middleware stack (for flag application)
	// while targeting a specific provider+model.
	if probeService := c.GetHeader("X-Tingly-Probe-Service"); probeService != "" {
		if providerUUID, model, ok := strings.Cut(probeService, ":"); ok {
			provider, err := s.selector.config.GetProviderByUUID(providerUUID)
			if err != nil || provider == nil {
				return nil, nil, fmt.Errorf("probe service provider not found: %s", providerUUID)
			}
			if !provider.Enabled {
				return nil, nil, fmt.Errorf("probe service provider disabled: %s", providerUUID)
			}
			svc := &loadbalance.Service{Provider: providerUUID, Model: model, Active: true}
			logrus.Debugf("[routing] probe service pin: provider=%s model=%s", provider.Name, model)
			setRoutingDebugHeaders(c, provider.Name, provider.UUID, model, SourceProbePin, -1)
			return provider, svc, nil
		}
	}

	// Build context (session ID resolved internally)
	ctx := NewSelectionContext(rule, req, c, scenario)

	// Execute pipeline
	result, err := s.selector.Select(ctx)
	if err != nil {
		return nil, nil, err
	}

	if result.Provider == nil || result.Service == nil {
		return nil, nil, fmt.Errorf("selection returned nil result")
	}

	// Automatically store sessionID in gin context for downstream handlers
	c.Set(constant.CtxKeySessionID, ctx.SessionID.String())
	// The scoped affinity key (session + matched smart partition) — consumers
	// that write back to the affinity entry (e.g. message-id updates) must use
	// this, not the bare session, or they'll miss partition-scoped pins.
	c.Set(constant.CtxKeyAffinityKey, AffinitySessionKey(ctx.SessionID.String(), result.MatchedSmartRuleIndex))

	// Store result metadata for observability
	c.Set("routing_source", result.Source)

	// Store LB trajectory: which upstream was selected and via which tactic.
	// SourceSmartRouting and SourceAffinity bypass the normal LB tactic, so
	// label them explicitly; otherwise use the rule's configured tactic name.
	c.Set(constant.CtxKeyLBServiceID, result.Service.ServiceID())
	tacticName := result.Source
	if result.Source != SourceSmartRouting && result.Source != SourceAffinity {
		tacticName = rule.GetTacticType().String()
	}
	c.Set(constant.CtxKeyLBTactic, tacticName)

	logrus.WithContext(c.Request.Context()).WithFields(logrus.Fields{
		"stage":            "routing_selected",
		"rule_uuid":        rule.UUID,
		"scenario":         string(scenario),
		"request_model":    rule.RequestModel,
		"source":           result.Source,
		"lb_tactic":        tacticName,
		"service":          result.Service.ServiceID(),
		"provider":         result.Provider.Name,
		"provider_uuid":    result.Provider.UUID,
		"model":            result.Service.Model,
		"routed_model":     result.Service.Model,
		"routed_provider":  result.Provider.Name,
		"candidate_count":  len(ctx.Rule.GetActiveServices()),
		"evaluated_stages": result.EvaluatedStages,
	}).Infof("[routing] selected %s/%s via %s", result.Provider.UUID, result.Service.Model, result.Source)

	setRoutingDebugHeaders(c, result.Provider.Name, result.Provider.UUID, result.Service.Model, result.Source, result.MatchedSmartRuleIndex)

	return result.Provider, result.Service, nil
}

// setRoutingDebugHeaders emits X-Tingly-Selected-* response headers describing
// the routing decision, but only when the request opted in via
// X-Tingly-Debug-Routing: 1 (set by probes). matchedSmartRule < 0 means none.
func setRoutingDebugHeaders(c *gin.Context, providerName, providerUUID, model, source string, matchedSmartRule int) {
	if c.GetHeader("X-Tingly-Debug-Routing") != "1" {
		return
	}
	c.Header("X-Tingly-Selected-Provider", providerName)
	c.Header("X-Tingly-Selected-Provider-UUID", providerUUID)
	c.Header("X-Tingly-Selected-Model", model)
	c.Header("X-Tingly-Routing-Source", source)
	if matchedSmartRule >= 0 {
		c.Header("X-Tingly-Matched-Smart-Rule", fmt.Sprintf("%d", matchedSmartRule))
	}
}

// SelectServiceForEmbeddings is a variant of SelectService for embedding requests.
// Embedding requests don't carry chat-style context, so content-based smart routing
// is skipped (load balancing, affinity, and health filters still apply).
func (s *SimpleSelector) SelectServiceForEmbeddings(
	c *gin.Context,
	scenario typ.RuleScenario,
	rule *typ.Rule,
) (*typ.Provider, *loadbalance.Service, error) {
	return s.SelectService(c, scenario, rule, nil)
}

// SelectServiceForImageGeneration is a variant of SelectService for image generation
// requests. Image generation requests don't carry chat-style context, so content-based
// smart routing is skipped (load balancing, affinity, and health filters still apply).
func (s *SimpleSelector) SelectServiceForImageGeneration(
	c *gin.Context,
	scenario typ.RuleScenario,
	rule *typ.Rule,
) (*typ.Provider, *loadbalance.Service, error) {
	return s.SelectService(c, scenario, rule, nil)
}
