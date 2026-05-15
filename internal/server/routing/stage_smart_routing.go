package routing

import (
	"context"

	"github.com/sirupsen/logrus"

	"github.com/tingly-dev/tingly-box/internal/loadbalance"
	smartrouting "github.com/tingly-dev/tingly-box/internal/smart_routing"
	"github.com/tingly-dev/tingly-box/internal/typ"
	pkgobs "github.com/tingly-dev/tingly-box/pkg/obs"
)

// SmartRoutingStage evaluates smart routing rules and returns matched services.
// If multiple services match, applies load balancing within the matched set.
type SmartRoutingStage struct {
	loadBalancer  LoadBalancer
	affinityStore AffinityStore
	multiLogger   *pkgobs.MultiLogger // optional; used to emit structured smart-routing logs
}

// NewSmartRoutingStage creates a new smart routing stage
func NewSmartRoutingStage(lb LoadBalancer, affinity AffinityStore) *SmartRoutingStage {
	return &SmartRoutingStage{
		loadBalancer:  lb,
		affinityStore: affinity,
	}
}

// SetMultiLogger attaches the multi-logger so the stage can emit structured
// smart-routing evaluation logs viewable from the frontend system log page.
func (s *SmartRoutingStage) SetMultiLogger(ml *pkgobs.MultiLogger) {
	s.multiLogger = ml
}

// Name returns the stage identifier
func (s *SmartRoutingStage) Name() string {
	return "smart_routing"
}

// requestHeadLen is intentionally small — the per-op trace already records a
// window around the matched substring (or a small head for non-text ops), so
// the snapshot only needs to surface enough context to recognize *which*
// request a row corresponds to. We avoid storing the full message bodies.
const requestHeadLen = 80

func requestHead(s string) string {
	if len(s) <= requestHeadLen {
		return s
	}
	return s[:requestHeadLen] + "…"
}

// requestSnapshot captures key request fields used during smart routing
// evaluation so operators can correlate decisions with what the request
// actually looked like. Long message bodies are truncated to a short head;
// the per-op trace owns the matched-substring windowing so we don't duplicate
// large payloads in memory.
func requestSnapshot(reqCtx *smartrouting.RequestContext) map[string]interface{} {
	if reqCtx == nil {
		return nil
	}
	return map[string]interface{}{
		"model":            reqCtx.Model,
		"thinking_enabled": reqCtx.ThinkingEnabled,
		"latest_role":      reqCtx.LatestRole,
		"latest_type":      reqCtx.LatestContentType,
		"estimated_tokens": reqCtx.EstimatedTokens,
		"tool_uses":        reqCtx.ToolUses,
		"latest_user_head": requestHead(reqCtx.GetLatestUserMessage()),
		"system_msg_count": len(reqCtx.SystemMessages),
		"user_msg_count":   len(reqCtx.UserMessages),
	}
}

// emitTrace writes a structured entry describing the smart-routing evaluation
// to both the standard system log (logrus.Debug) and the dedicated
// smart_routing memory sink (when a multiLogger is configured) so the
// frontend can render an inspectable history.
func (s *SmartRoutingStage) emitTrace(
	ctx *SelectionContext,
	reqCtx *smartrouting.RequestContext,
	trace []smartrouting.RuleEvalResult,
	matchedRuleIndex int,
	matchedServicesCount int,
	finalActiveCount int,
	selectedService *loadbalance.Service,
	outcome string,
	reason string,
) {
	matched := matchedRuleIndex >= 0
	fields := logrus.Fields{
		"rule_uuid":           ctx.Rule.UUID,
		"scenario":            string(ctx.Scenario),
		"request_model":       ctx.Rule.RequestModel,
		"matched":             matched,
		"matched_rule_index":  matchedRuleIndex,
		"outcome":             outcome,
		"reason":              reason,
		"matched_services":    matchedServicesCount,
		"final_active_count":  finalActiveCount,
		"trace":               trace,
		"request":             requestSnapshot(reqCtx),
		"rules_total":         len(ctx.Rule.SmartRouting),
	}
	if matched && matchedRuleIndex < len(trace) {
		fields["matched_rule_description"] = trace[matchedRuleIndex].Description
	}
	if selectedService != nil {
		fields["selected_provider"] = selectedService.Provider
		fields["selected_model"] = selectedService.Model
	}
	if ctx.GinContext != nil {
		fields["client_ip"] = ctx.GinContext.ClientIP()
		fields["request_id"] = ctx.GinContext.GetString("X-Request-Id")
	}

	if s.multiLogger != nil {
		s.multiLogger.GetLogrusLogger(pkgobs.LogSourceSmartRouting).
			WithFields(fields).
			Info(formatTraceMessage(matched, matchedRuleIndex, ctx.Rule.RequestModel, outcome))
	}
	logrus.WithFields(fields).Debugf("[smart_routing] %s", outcome)
}

func formatTraceMessage(matched bool, idx int, model, outcome string) string {
	if matched {
		return "smart routing matched rule " + indexToString(idx) + " for " + model + " (" + outcome + ")"
	}
	return "smart routing fell through for " + model + " (" + outcome + ")"
}

func indexToString(i int) string {
	if i < 0 {
		return "-1"
	}
	// avoid pulling strconv just for this
	if i == 0 {
		return "0"
	}
	digits := []byte{}
	for i > 0 {
		digits = append([]byte{byte('0' + i%10)}, digits...)
		i /= 10
	}
	return string(digits)
}

// Evaluate evaluates smart routing rules and selects a service. When a
// matched rule carries op-level processors, the stage runs them (the
// "implicit bypass") and then returns (nil, false) so the LoadBalancer
// (global fallback) picks an upstream with the mutated request. The
// mutated request is NOT re-evaluated against the smart-routing rules —
// re-entering evaluation with a post-processor request invites surprising
// matches and is intentionally avoided.
func (s *SmartRoutingStage) Evaluate(ctx *SelectionContext, state *selectionState) (*SelectionResult, bool) {
	rule := ctx.Rule

	// Skip if smart routing not enabled
	if !rule.SmartEnabled || len(rule.SmartRouting) == 0 || ctx.Request == nil {
		logrus.Debugf("[smart_routing] skipped - SmartEnabled=%v, SmartRoutingCount=%d, Request=%v",
			rule.SmartEnabled, len(rule.SmartRouting), ctx.Request != nil)
		return nil, false
	}

	logrus.Debugf("[smart_routing] evaluating %d rules for model %s", len(rule.SmartRouting), rule.RequestModel)

	// Extract request context
	reqCtx, err := ExtractRequestContext(ctx.Request)
	if err != nil {
		logrus.Debugf("[smart_routing] failed to extract context: %v", err)
		s.emitTrace(ctx, nil, nil, -1, 0, 0, nil, "extract_failed", err.Error())
		return nil, false
	}
	if reqCtx == nil {
		s.emitTrace(ctx, nil, nil, -1, 0, 0, nil, "no_context", "request type not supported for smart routing")
		return nil, false
	}

	// Annotate the request context with the agent.claude_code request kind when
	// the scenario is claude_code. Other scenarios leave the field empty so the
	// agent.claude_code SmartOp simply does not match.
	if ctx.Scenario == typ.ScenarioClaudeCode {
		reqCtx.ClaudeCodeRequestKind = smartrouting.DetectClaudeCodeRequestKind(reqCtx)
	}

	// Pre-collect capacity info for all services across all smart routing rules.
	// evaluateRule will filter this down to per-rule services when evaluating.
	reqCtx.ServiceCapacity = s.collectAllCapacityInfo(rule.SmartRouting)

	// Create router and evaluate
	router, err := smartrouting.NewRouter(rule.SmartRouting)
	if err != nil {
		logrus.Debugf("[smart_routing] failed to create router: %v", err)
		s.emitTrace(ctx, reqCtx, nil, -1, 0, 0, nil, "router_invalid", err.Error())
		return nil, false
	}

	matchedServices, matchedRuleIndex, matched, trace := router.Evaluate(reqCtx)
	if !matched || len(matchedServices) == 0 {
		logrus.Debugf("[smart_routing] no rule matched - matched=%v, services=%d", matched, len(matchedServices))
		s.emitTrace(ctx, reqCtx, trace, -1, 0, 0, nil, "no_match", "no rule matched the request")
		return nil, false
	}

	matchedRule := rule.SmartRouting[matchedRuleIndex]

	// If this rule was already bypassed in an earlier pass, do not run its
	// processors again — fall through to the LoadBalancer instead.
	if _, already := ctx.BypassedSmartRules[matchedRuleIndex]; already {
		s.emitTrace(ctx, reqCtx, trace, matchedRuleIndex, len(matchedServices), 0, nil,
			"bypass_already_done", "rule already bypassed by op-level processor; falling through to LoadBalancer")
		return nil, false
	}

	// Implicit bypass: if the matched rule's ops carry registered processors,
	// run them (they mutate ctx.Request in place) and return (nil, false).
	// The LoadBalancer stage then picks an upstream from the parent rule's
	// top-level Services with the mutated request. We do NOT re-evaluate
	// smart-routing rules against the mutated request — keeping the bypass
	// strictly one-shot prevents post-processor inputs from triggering
	// unintended matches downstream.
	type collectedProc struct {
		op   smartrouting.SmartOp
		proc smartrouting.OpProcessor
	}
	var procs []collectedProc
	for _, op := range matchedRule.Ops {
		if p, ok := smartrouting.LookupProcessor(op.Position, op.Operation); ok {
			procs = append(procs, collectedProc{op: op, proc: p})
		}
	}
	if len(procs) > 0 {
		processorCtx := context.Background()
		if ctx.GinContext != nil && ctx.GinContext.Request != nil {
			processorCtx = ctx.GinContext.Request.Context()
		}
		pctx := &smartrouting.ProcessorContext{
			Ctx:       processorCtx,
			Request:   ctx.Request,
			ReqCtx:    reqCtx,
			RuleIndex: matchedRuleIndex,
			Services:  matchedServices,
		}
		for _, cp := range procs {
			pctx.OpUUID = cp.op.UUID
			if err := cp.proc.Process(pctx); err != nil {
				logrus.Debugf("[smart_routing] processor %s/%s error: %v",
					cp.op.Position, cp.op.Operation, err)
			}
		}
		if ctx.BypassedSmartRules == nil {
			ctx.BypassedSmartRules = make(map[int]struct{})
		}
		ctx.BypassedSmartRules[matchedRuleIndex] = struct{}{}
		s.emitTrace(ctx, reqCtx, trace, matchedRuleIndex, len(matchedServices), 0, nil,
			"bypass_processor_run", "op-level processor ran; falling through to LoadBalancer")
		return nil, false
	}

	if state != nil && len(state.candidateServices) > 0 {
		beforeCount := len(matchedServices)
		matchedServices = IntersectServices(matchedServices, state.candidateServices)
		logrus.Debugf("[smart_routing] intersection: %d -> %d services", beforeCount, len(matchedServices))
	}
	if len(matchedServices) == 0 {
		logrus.Debugf("[smart_routing] matched rule has no services in current candidate set")
		s.emitTrace(ctx, reqCtx, trace, matchedRuleIndex, 0, 0, nil, "no_candidates",
			"matched rule has no services in current candidate set")
		return nil, false
	}

	ctx.MatchedSmartRuleIndex = matchedRuleIndex

	logrus.Debugf("[smart_routing] rule %d matched, selecting from %d services",
		matchedRuleIndex, len(matchedServices))

	// Filter active services
	activeServices := FilterActiveServices(matchedServices)
	if len(activeServices) == 0 {
		logrus.Debugf("[smart_routing] no active services in matched set")
		s.emitTrace(ctx, reqCtx, trace, matchedRuleIndex, len(matchedServices), 0, nil, "no_active_services",
			"matched rule has no active services")
		return nil, false
	}

	// Single service? Return it directly
	if len(activeServices) == 1 {
		result := NewResult(activeServices[0], "smart_routing")
		result.MatchedSmartRuleIndex = matchedRuleIndex
		s.emitTrace(ctx, reqCtx, trace, matchedRuleIndex, len(matchedServices), len(activeServices),
			activeServices[0], "selected", "single active service in matched set")
		return result, true
	}

	// Multiple services: apply load balancing within matched set
	service := s.selectFromServices(activeServices, rule)
	if service == nil {
		s.emitTrace(ctx, reqCtx, trace, matchedRuleIndex, len(matchedServices), len(activeServices),
			nil, "lb_failed", "load balancer returned no service")
		return nil, false
	}

	result := NewResult(service, "smart_routing")
	result.MatchedSmartRuleIndex = matchedRuleIndex
	s.emitTrace(ctx, reqCtx, trace, matchedRuleIndex, len(matchedServices), len(activeServices),
		service, "selected", "load balanced within matched set")
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

// collectAllCapacityInfo collects seat-capacity info for all services across all smart routing rules.
// Deduplicates by serviceID. evaluateRule will filter down to per-rule services.
func (s *SmartRoutingStage) collectAllCapacityInfo(rules []smartrouting.SmartRouting) []smartrouting.ServiceCapacityInfo {
	seen := make(map[string]struct{})
	var result []smartrouting.ServiceCapacityInfo
	for _, r := range rules {
		for _, svc := range r.Services {
			id := svc.ServiceID()
			if _, ok := seen[id]; ok {
				continue
			}
			seen[id] = struct{}{}

			cap := 0
			if svc.ModelCapacity != nil {
				cap = *svc.ModelCapacity
			}

			active := 0
			if s.affinityStore != nil {
				active = s.affinityStore.CountByService(id)
			}

			result = append(result, smartrouting.ServiceCapacityInfo{
				ServiceID:   id,
				Capacity:    cap,
				ActiveCount: active,
			})
		}
	}
	return result
}
