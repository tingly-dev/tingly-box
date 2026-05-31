package smartrouting

import (
	"fmt"
	"log"
	"strings"

	"github.com/gobwas/glob"

	"github.com/tingly-dev/tingly-box/internal/loadbalance"
)

// Router evaluates requests against smart routing rules
type Router struct {
	rules []SmartRouting
}

// NewRouter creates a new smart routing router
func NewRouter(rules []SmartRouting) (*Router, error) {
	for i, rule := range rules {
		if err := ValidateSmartRouting(&rule); err != nil {
			return nil, fmt.Errorf("rule[%d]: %w", i, err)
		}
	}
	return &Router{rules: rules}, nil
}

// Evaluate runs the rule list against ctx in a single pass and returns the
// matched services (or nil), the matched rule index (or -1), the matched flag,
// and the per-rule trace. All other public Evaluate* / TraceEvaluation methods
// are thin wrappers around this.
func (r *Router) Evaluate(ctx *RequestContext) (services []*loadbalance.Service, ruleIdx int, matched bool, trace []RuleEvalResult) {
	trace = make([]RuleEvalResult, 0, len(r.rules))
	ruleIdx = -1
	for i := range r.rules {
		rule := &r.rules[i]
		ruleRes := r.evaluateRule(ctx, rule, i)
		trace = append(trace, ruleRes)
		if ruleRes.Matched {
			services = rule.Services
			ruleIdx = i
			matched = true
			break
		}
	}
	return
}

// EvaluateRequest evaluates a request against smart routing rules.
// Returns the matched services and true if a rule matched, otherwise nil and false.
func (r *Router) EvaluateRequest(ctx *RequestContext) ([]*loadbalance.Service, bool) {
	services, _, matched, _ := r.Evaluate(ctx)
	return services, matched
}

// EvaluateRequestWithIndex is EvaluateRequest plus the matched rule index.
func (r *Router) EvaluateRequestWithIndex(ctx *RequestContext) ([]*loadbalance.Service, int, bool) {
	services, idx, matched, _ := r.Evaluate(ctx)
	return services, idx, matched
}

// evaluateRule evaluates a single rule and returns the per-op trace plus the
// composite Matched flag. Evaluation short-circuits at the first failed op so
// the returned Ops slice may end on a non-matching entry.
func (r *Router) evaluateRule(ctx *RequestContext, rule *SmartRouting, idx int) RuleEvalResult {
	origStats := ctx.ServiceStats
	origCap := ctx.ServiceCapacity
	ctx.ServiceStats = collectRuleStats(rule.Services)
	ctx.ServiceCapacity = filterCapacityForRule(ctx.ServiceCapacity, rule.Services)
	defer func() {
		ctx.ServiceStats = origStats
		ctx.ServiceCapacity = origCap
	}()

	res := RuleEvalResult{
		RuleIndex:   idx,
		Description: rule.Description,
		OpsTotal:    len(rule.Ops),
		Matched:     true,
		Ops:         make([]OpEvalResult, 0, len(rule.Ops)),
	}
	for i := range rule.Ops {
		opRes := r.evaluateOp(ctx, &rule.Ops[i])
		res.Ops = append(res.Ops, opRes)
		if !opRes.Matched {
			res.Matched = false
			break
		}
	}
	res.OpsEvaluated = len(res.Ops)
	return res
}

// evaluateOp dispatches to the per-position evaluator. The returned
// OpEvalResult always has Position/Operation/Value populated; per-position
// methods set Matched plus optional Reason and Actual.
func (r *Router) evaluateOp(ctx *RequestContext, op *SmartOp) OpEvalResult {
	switch op.Position {
	case PositionModel:
		return r.evaluateModelOp(ctx, op)
	case PositionThinking:
		return r.evaluateThinkingOp(ctx, op)
	case PositionContextSystem:
		return r.evaluateContextSystemOp(ctx, op)
	case PositionContextUser:
		return r.evaluateContextUserOp(ctx, op)
	case PositionLatestUser:
		return r.evaluateLatestUserOp(ctx, op)
	case PositionToolUse:
		return r.evaluateToolUseOp(ctx, op)
	case PositionToken:
		return r.evaluateTokenOp(ctx, op)
	case PositionServiceTTFT:
		return r.evaluateServiceTTFTOp(ctx, op)
	case PositionServiceCapacity:
		return r.evaluateServiceCapacityOp(ctx, op)
	case PositionAgentClaudeCode:
		return r.evaluateAgentClaudeCodeOp(ctx, op)
	default:
		res := newOpResult(op)
		res.Reason = fmt.Sprintf("unknown position %q", op.Position)
		return res
	}
}

// newOpResult seeds an OpEvalResult with the configured op metadata. Matched
// defaults to false; per-position evaluators set it on success.
func newOpResult(op *SmartOp) OpEvalResult {
	return OpEvalResult{
		UUID:      op.UUID,
		Position:  string(op.Position),
		Operation: string(op.Operation),
		Value:     op.Value,
	}
}

// ValidateSmartRouting checks if the smart routing rule is valid
func ValidateSmartRouting(rule *SmartRouting) error {
	if rule.Description == "" {
		return fmt.Errorf("description cannot be empty")
	}
	if len(rule.Ops) == 0 {
		return fmt.Errorf("ops cannot be empty")
	}
	for i, op := range rule.Ops {
		if err := ValidateSmartOp(&op); err != nil {
			return fmt.Errorf("op[%d]: %w", i, err)
		}
	}
	if len(rule.Services) == 0 {
		return fmt.Errorf("services cannot be empty")
	}
	for i, svc := range rule.Services {
		if svc.Provider == "" {
			return fmt.Errorf("services[%d]: provider cannot be empty", i)
		}
		if svc.Model == "" {
			return fmt.Errorf("services[%d]: model cannot be empty", i)
		}
	}
	return nil
}

// ValidateSmartOp checks if the operation is valid for its position
func ValidateSmartOp(op *SmartOp) error {
	if !op.Position.IsValid() {
		return fmt.Errorf("invalid position: %s", op.Position)
	}
	if op.Operation == "" {
		return fmt.Errorf("operation cannot be empty")
	}
	if !isValidOperationForPosition(op.Position, op.Operation) {
		return fmt.Errorf("operation '%s' is not valid for position '%s'", op.Operation, op.Position)
	}
	return validateOpValueType(op)
}

func validateOpValueType(op *SmartOp) error {
	expectedType := ValueTypeString
	for _, validOp := range Operations {
		if validOp.Position == op.Position && validOp.Operation == op.Operation {
			if validOp.Meta.Type != "" {
				expectedType = validOp.Meta.Type
			}
			break
		}
	}
	if expectedType == ValueTypeString && op.Meta.Type == "" {
		return nil
	}
	switch expectedType {
	case ValueTypeString:
		return nil
	case ValueTypeInt:
		_, err := op.Int()
		return err
	case ValueTypeBool:
		_, err := op.Bool()
		return err
	default:
		return fmt.Errorf("unknown type: %s", expectedType)
	}
}

func isValidOperationForPosition(pos SmartOpPosition, op SmartOpOperation) bool {
	for _, validOp := range Operations {
		if validOp.Position == pos && validOp.Operation == op {
			return true
		}
	}
	return false
}

func (r *Router) evaluateModelOp(ctx *RequestContext, op *SmartOp) OpEvalResult {
	res := newOpResult(op)
	res.Actual = ctx.Model
	value, err := op.String()
	if err != nil {
		log.Printf("[smart_routing] invalid model value '%s': %v", op.Value, err)
		res.Reason = fmt.Sprintf("invalid value: %v", err)
		return res
	}
	switch op.Operation {
	case OpModelContains:
		res.Matched = strings.Contains(ctx.Model, value)
		res.Reason = fmt.Sprintf("model %q contains %q", ctx.Model, value)
	case OpModelGlob:
		g, err := glob.Compile(value)
		if err != nil {
			log.Printf("[smart_routing] invalid glob pattern '%s' in model operation: %v", value, err)
			res.Reason = fmt.Sprintf("invalid glob %q: %v", value, err)
			return res
		}
		res.Matched = g.Match(ctx.Model)
		res.Reason = fmt.Sprintf("model %q glob %q", ctx.Model, value)
	case OpModelEquals:
		res.Matched = ctx.Model == value
		res.Reason = fmt.Sprintf("model %q equals %q", ctx.Model, value)
	default:
		res.Reason = fmt.Sprintf("unsupported model op %q", op.Operation)
	}
	return res
}

func (r *Router) evaluateThinkingOp(ctx *RequestContext, op *SmartOp) OpEvalResult {
	res := newOpResult(op)
	res.Actual = boolStr(ctx.ThinkingEnabled)
	val, err := op.Bool()
	if err != nil && op.Value != "" {
		log.Printf("[smart_routing] invalid thinking value '%s': %v", op.Value, err)
		res.Reason = fmt.Sprintf("invalid bool %q: %v", op.Value, err)
		return res
	}
	switch op.Operation {
	case OpThinkingEnabled:
		if op.Value == "" || val {
			res.Matched = ctx.ThinkingEnabled
			res.Reason = "thinking enabled"
		} else {
			res.Reason = "value=false; check is no-op"
		}
	case OpThinkingDisabled:
		if op.Value == "" || val {
			res.Matched = !ctx.ThinkingEnabled
			res.Reason = "thinking disabled"
		} else {
			res.Reason = "value=false; check is no-op"
		}
	default:
		res.Reason = fmt.Sprintf("unsupported thinking op %q", op.Operation)
	}
	return res
}

func (r *Router) evaluateContextSystemOp(ctx *RequestContext, op *SmartOp) OpEvalResult {
	combined := ctx.CombineMessages(ctx.SystemMessages)
	return evaluateTextContains(op, combined, "system context",
		OpContextSystemContains, OpContextSystemRegex)
}

func (r *Router) evaluateContextUserOp(ctx *RequestContext, op *SmartOp) OpEvalResult {
	combined := ctx.CombineMessages(ctx.UserMessages)
	return evaluateTextContains(op, combined, "user context",
		OpContextUserContains, OpContextUserRegex)
}

// evaluateTextContains is the shared body for system/user contains+regex ops.
// containsOp/regexOp are typed so we don't depend on string equality of "contains".
func evaluateTextContains(op *SmartOp, text, label string, containsOp, regexOp SmartOpOperation) OpEvalResult {
	res := newOpResult(op)
	value, err := op.String()
	if err != nil {
		log.Printf("[smart_routing] invalid %s value '%s': %v", label, op.Value, err)
		res.Reason = fmt.Sprintf("invalid value: %v", err)
		return res
	}
	switch op.Operation {
	case containsOp:
		res.Matched = strings.Contains(text, value)
		res.Actual = snippetAround(text, value)
		res.Reason = fmt.Sprintf("%s contains %q", label, value)
	case regexOp:
		ok, err := stringsMatch(text, value, true)
		if err != nil {
			res.Actual = snippetHead(text, snippetHeadLen)
			res.Reason = fmt.Sprintf("regex error: %v", err)
			return res
		}
		res.Matched = ok
		res.Actual = snippetHead(text, snippetHeadLen)
		res.Reason = fmt.Sprintf("%s matches regex %q", label, value)
	default:
		res.Actual = snippetHead(text, snippetHeadLen)
		res.Reason = fmt.Sprintf("unsupported %s op %q", label, op.Operation)
	}
	return res
}

func (r *Router) evaluateLatestUserOp(ctx *RequestContext, op *SmartOp) OpEvalResult {
	res := newOpResult(op)
	value, err := op.String()
	if err != nil {
		log.Printf("[smart_routing] invalid latest user value '%s': %v", op.Value, err)
		res.Reason = fmt.Sprintf("invalid value: %v", err)
		return res
	}
	switch op.Operation {
	case OpLatestUserContains:
		latest := ctx.GetLatestUserMessage()
		if ctx.LatestRole != "user" {
			res.Actual = snippetHead(latest, snippetHeadLen)
			res.Reason = fmt.Sprintf("latest role is %q, not user", ctx.LatestRole)
			return res
		}
		if !ctx.LatestUserHasText {
			// Latest user-role message was a tool_result (no text). Matching
			// against GetLatestUserMessage() would return a stale previous text
			// and could produce a false positive — return no-match instead so
			// the request falls through to the default.
			res.Reason = "latest user message has no text content (tool_result)"
			return res
		}
		res.Matched = strings.Contains(latest, value)
		res.Actual = snippetAround(latest, value)
		res.Reason = fmt.Sprintf("latest user contains %q", value)
	case OpLatestUserRequestType:
		if ctx.LatestRole != "user" || !ctx.LatestUserHasText {
			res.Actual = ctx.LatestContentType
			res.Reason = fmt.Sprintf("latest role is %q, not user text", ctx.LatestRole)
			return res
		}
		res.Actual = ctx.LatestContentType
		res.Matched = ctx.LatestContentType == value
		res.Reason = fmt.Sprintf("latest content_type %q == %q", ctx.LatestContentType, value)
	default:
		res.Reason = fmt.Sprintf("unsupported latest_user op %q", op.Operation)
	}
	return res
}

func (r *Router) evaluateToolUseOp(ctx *RequestContext, op *SmartOp) OpEvalResult {
	res := newOpResult(op)
	res.Actual = strings.Join(ctx.ToolUses, ",")
	value, err := op.String()
	if err != nil {
		log.Printf("[smart_routing] invalid tool_use value '%s': %v", op.Value, err)
		res.Reason = fmt.Sprintf("invalid value: %v", err)
		return res
	}
	if op.Operation != OpToolUseEquals {
		res.Reason = fmt.Sprintf("unsupported tool_use op %q", op.Operation)
		return res
	}
	for _, t := range ctx.ToolUses {
		if t == value {
			res.Matched = true
			res.Reason = fmt.Sprintf("tool %q used", value)
			return res
		}
	}
	res.Reason = fmt.Sprintf("tool %q not in [%s]", value, strings.Join(ctx.ToolUses, ","))
	return res
}

func (r *Router) evaluateTokenOp(ctx *RequestContext, op *SmartOp) OpEvalResult {
	res := newOpResult(op)
	res.Actual = fmt.Sprintf("%d", ctx.EstimatedTokens)
	target, err := op.Int()
	if err != nil {
		log.Printf("[smart_routing] invalid token value '%s': %v", op.Value, err)
		res.Reason = fmt.Sprintf("invalid int: %v", err)
		return res
	}
	tokens := ctx.EstimatedTokens
	switch op.Operation {
	case OpTokenGe:
		res.Matched = tokens >= target
		res.Reason = fmt.Sprintf("tokens %d >= %d", tokens, target)
	case OpTokenGt:
		res.Matched = tokens > target
		res.Reason = fmt.Sprintf("tokens %d > %d", tokens, target)
	case OpTokenLe:
		res.Matched = tokens <= target
		res.Reason = fmt.Sprintf("tokens %d <= %d", tokens, target)
	case OpTokenLt:
		res.Matched = tokens < target
		res.Reason = fmt.Sprintf("tokens %d < %d", tokens, target)
	default:
		res.Reason = fmt.Sprintf("unsupported token op %q", op.Operation)
	}
	return res
}

// evaluateServiceTTFTOp operates on ctx.ServiceStats, which is pre-filtered
// per-rule by evaluateRule. Returns Matched=true (pass) when there is no data
// (cold-start friendliness).
func (r *Router) evaluateServiceTTFTOp(ctx *RequestContext, op *SmartOp) OpEvalResult {
	res := newOpResult(op)
	if len(ctx.ServiceStats) == 0 {
		res.Matched = true
		res.Reason = "no service stats; pass"
		return res
	}
	threshold, err := op.Int()
	if err != nil {
		log.Printf("[smart_routing] invalid service_ttft value '%s': %v", op.Value, err)
		res.Reason = fmt.Sprintf("invalid int: %v", err)
		return res
	}
	var values []float64
	for _, s := range ctx.ServiceStats {
		switch op.Operation {
		case OpServiceTTFTAvgLe, OpServiceTTFTAvgGe:
			if s.AvgTTFTMs > 0 {
				values = append(values, s.AvgTTFTMs)
			}
		case OpServiceTTFTMaxLe, OpServiceTTFTMaxGe:
			if s.P99TTFTMs > 0 {
				values = append(values, s.P99TTFTMs)
			}
		}
	}
	if len(values) == 0 {
		res.Matched = true
		res.Reason = "no TTFT samples; pass"
		return res
	}
	thresholdF := float64(threshold)
	switch op.Operation {
	case OpServiceTTFTAvgLe:
		actual := minFloat(values)
		res.Matched = actual <= thresholdF
		res.Actual = fmt.Sprintf("%.0f", actual)
		res.Reason = fmt.Sprintf("min(avg_ttft) %.0f <= %d", actual, threshold)
	case OpServiceTTFTAvgGe:
		actual := avgFloat(values)
		res.Matched = actual >= thresholdF
		res.Actual = fmt.Sprintf("%.0f", actual)
		res.Reason = fmt.Sprintf("avg(avg_ttft) %.0f >= %d", actual, threshold)
	case OpServiceTTFTMaxLe:
		actual := minFloat(values)
		res.Matched = actual <= thresholdF
		res.Actual = fmt.Sprintf("%.0f", actual)
		res.Reason = fmt.Sprintf("min(p99_ttft) %.0f <= %d", actual, threshold)
	case OpServiceTTFTMaxGe:
		actual := avgFloat(values)
		res.Matched = actual >= thresholdF
		res.Actual = fmt.Sprintf("%.0f", actual)
		res.Reason = fmt.Sprintf("avg(p99_ttft) %.0f >= %d", actual, threshold)
	default:
		res.Reason = fmt.Sprintf("unsupported service_ttft op %q", op.Operation)
	}
	return res
}

// evaluateServiceCapacityOp computes seat utilization = activeCount / capacity * 100
// per service, averaged across services with capacity configured. Services with
// capacity == 0 are unlimited and skipped. Returns Matched=true (pass) when no
// service has capacity configured.
func (r *Router) evaluateServiceCapacityOp(ctx *RequestContext, op *SmartOp) OpEvalResult {
	res := newOpResult(op)
	if len(ctx.ServiceCapacity) == 0 {
		res.Matched = true
		res.Reason = "no capacity info; pass"
		return res
	}
	threshold, err := op.Int()
	if err != nil {
		log.Printf("[smart_routing] invalid service_capacity value '%s': %v", op.Value, err)
		res.Reason = fmt.Sprintf("invalid int: %v", err)
		return res
	}
	var utilValues []float64
	for _, c := range ctx.ServiceCapacity {
		if c.Capacity <= 0 {
			continue
		}
		utilValues = append(utilValues, float64(c.ActiveCount)/float64(c.Capacity)*100)
	}
	if len(utilValues) == 0 {
		res.Matched = true
		res.Reason = "no capacity-configured services; pass"
		return res
	}
	avg := avgFloat(utilValues)
	thresholdF := float64(threshold)
	res.Actual = fmt.Sprintf("%.1f%%", avg)
	switch op.Operation {
	case OpServiceCapacityUtilLe:
		res.Matched = avg <= thresholdF
		res.Reason = fmt.Sprintf("avg util %.1f%% <= %d%%", avg, threshold)
	case OpServiceCapacityUtilGe:
		res.Matched = avg >= thresholdF
		res.Reason = fmt.Sprintf("avg util %.1f%% >= %d%%", avg, threshold)
	case OpServiceCapacityUtilLt:
		res.Matched = avg < thresholdF
		res.Reason = fmt.Sprintf("avg util %.1f%% < %d%%", avg, threshold)
	case OpServiceCapacityUtilGt:
		res.Matched = avg > thresholdF
		res.Reason = fmt.Sprintf("avg util %.1f%% > %d%%", avg, threshold)
	default:
		res.Reason = fmt.Sprintf("unsupported service_capacity op %q", op.Operation)
	}
	return res
}

// evaluateAgentClaudeCodeOp evaluates the agent.claude_code position.
// ctx.ClaudeCodeRequestKind is populated by SmartRoutingStage only when the
// request scenario is claude_code; for other scenarios it is empty and no
// value matches — which is the desired behavior.
func (r *Router) evaluateAgentClaudeCodeOp(ctx *RequestContext, op *SmartOp) OpEvalResult {
	res := newOpResult(op)
	res.Actual = ctx.ClaudeCodeRequestKind
	value, err := op.String()
	if err != nil {
		log.Printf("[smart_routing] invalid agent.claude_code value '%s': %v", op.Value, err)
		res.Reason = fmt.Sprintf("invalid value: %v", err)
		return res
	}
	if op.Operation != OpAgentClaudeCodeEquals {
		res.Reason = fmt.Sprintf("unsupported agent.claude_code op %q", op.Operation)
		return res
	}
	if ctx.ClaudeCodeRequestKind == "" {
		res.Reason = "request kind not set (scenario is not claude_code)"
		return res
	}
	res.Matched = ctx.ClaudeCodeRequestKind == value
	res.Reason = fmt.Sprintf("agent.claude_code %q == %q", ctx.ClaudeCodeRequestKind, value)
	return res
}

// stringsMatch provides basic regex matching support.
// For now, it provides simple pattern matching with support for:
// - Wildcards (*)
// - Character classes ([abc])
// - Alternatives (a|b)
func stringsMatch(text, pattern string, useRegex bool) (bool, error) {
	if !useRegex {
		return strings.Contains(text, pattern), nil
	}
	g, err := glob.Compile(pattern)
	if err != nil {
		log.Printf("[smart_routing] invalid glob/regex pattern '%s', falling back to contains: %v", pattern, err)
		return strings.Contains(text, pattern), nil
	}
	return g.Match(text), nil
}

// EstimateTokens estimates token count from text (rough approximation: 4 chars per token)
func EstimateTokens(text string) int {
	if text == "" {
		return 0
	}
	return len(text) / 4
}

// GetRules returns the router's rules
func (r *Router) GetRules() []SmartRouting { return r.rules }

func boolStr(b bool) string {
	if b {
		return "true"
	}
	return "false"
}

// collectRuleStats returns a snapshot of ServiceStats for each service in the rule.
func collectRuleStats(services []*loadbalance.Service) []loadbalance.ServiceStats {
	result := make([]loadbalance.ServiceStats, 0, len(services))
	for _, svc := range services {
		result = append(result, svc.Stats.GetStats())
	}
	return result
}

// filterCapacityForRule filters all capacity info down to services belonging to this rule.
func filterCapacityForRule(all []ServiceCapacityInfo, services []*loadbalance.Service) []ServiceCapacityInfo {
	if len(all) == 0 {
		return nil
	}
	ids := make(map[string]struct{}, len(services))
	for _, svc := range services {
		ids[svc.ServiceID()] = struct{}{}
	}
	var result []ServiceCapacityInfo
	for _, c := range all {
		if _, ok := ids[c.ServiceID]; ok {
			result = append(result, c)
		}
	}
	return result
}

func minFloat(vals []float64) float64 {
	m := vals[0]
	for _, v := range vals[1:] {
		if v < m {
			m = v
		}
	}
	return m
}

func avgFloat(vals []float64) float64 {
	var sum float64
	for _, v := range vals {
		sum += v
	}
	return sum / float64(len(vals))
}
