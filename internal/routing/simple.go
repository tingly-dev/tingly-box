package routing

import (
	"fmt"
	"strings"
	"time"

	"github.com/gin-gonic/gin"
	"github.com/sirupsen/logrus"

	"github.com/tingly-dev/tingly-box/internal/constant"
	"github.com/tingly-dev/tingly-box/internal/limits"
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

// Limits returns the account-limits guard (nil when limits are off).
func (s *SimpleSelector) Limits() *LimitsGuard {
	if s == nil || s.selector == nil {
		return nil
	}
	return s.selector.limits
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
			setRoutingDebugHeaders(c, provider.Name, provider.UUID, model, SourceProbePin, -1, nil)
			return provider, svc, nil
		}
	}

	// Build context (session ID resolved internally)
	ctx := NewSelectionContext(rule, req, c, scenario)

	// Execute pipeline; with limits on, also take the account permit and
	// re-select if another request won the last slot in the meantime.
	result, err := s.selectAndAcquire(c, ctx)
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

	setRoutingDebugHeaders(c, result.Provider.Name, result.Provider.UUID, result.Service.Model, result.Source, result.MatchedSmartRuleIndex, result.EvaluatedStages)

	return result.Provider, result.Service, nil
}

// selectAndAcquire runs the pipeline and, when limits are enabled, acquires the
// selected service's permit into the request lease. The LimitStage already
// filtered on a read-only assessment; this re-checks atomically, so losing a
// race just excludes that service and selects again (bounded by the number of
// candidates; the queue wait budget is shared across retries).
func (s *SimpleSelector) selectAndAcquire(c *gin.Context, ctx *SelectionContext) (*SelectionResult, error) {
	guard := s.selector.limits
	if guard == nil {
		return s.selector.Select(ctx)
	}
	attempts := len(ctx.Rule.Services) + 1
	for _, sr := range ctx.Rule.SmartRouting {
		attempts += len(sr.Services)
	}
	for i := 0; i < attempts; i++ {
		result, err := s.selector.Select(ctx)
		if err != nil {
			return nil, err
		}
		permit, ok := guard.Acquire(ctx.Rule.UUID, result.Service, ctx.LimitsIgnored)
		if ok {
			if permit != nil {
				LeaseFromGin(c).Hold(result.Service.ServiceID(), permit)
				if c.GetHeader("X-Tingly-Debug-Routing") == "1" {
					c.Header(limitsDebugHeader, "held="+result.Service.ServiceID())
				}
			}
			return result, nil
		}
		if ctx.LimitExcluded == nil {
			ctx.LimitExcluded = map[string]bool{}
		}
		ctx.LimitExcluded[result.Service.ServiceID()] = true
		logrus.WithContext(c.Request.Context()).WithFields(logrus.Fields{
			"stage": "routing_limit_race_lost", "rule_uuid": ctx.Rule.UUID, "service": result.Service.ServiceID(),
		}).Debugf("[limits] lost the last slot of %s; re-selecting", result.Service.ServiceID())
	}
	return nil, &LimitError{RuleUUID: ctx.Rule.UUID, Reasons: limits.ReasonConcurrency, RetryAfter: time.Second}
}

// limitsDebugHeader is only emitted for X-Tingly-Debug-Routing probes.
const limitsDebugHeader = "X-Tingly-Limits"

// setRoutingDebugHeaders emits X-Tingly-Selected-* response headers describing
// the routing decision, but only when the request opted in via
// X-Tingly-Debug-Routing: 1 (set by probes). matchedSmartRule < 0 means none.
func setRoutingDebugHeaders(c *gin.Context, providerName, providerUUID, model, source string, matchedSmartRule int, evaluatedStages []string) {
	if c.GetHeader("X-Tingly-Debug-Routing") != "1" {
		return
	}
	c.Header("X-Tingly-Selected-Provider", providerName)
	c.Header("X-Tingly-Selected-Provider-UUID", providerUUID)
	c.Header("X-Tingly-Selected-Model", model)
	c.Header("X-Tingly-Routing-Source", source)
	if len(evaluatedStages) > 0 {
		c.Header("X-Tingly-Evaluated-Stages", strings.Join(evaluatedStages, ","))
	}
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
