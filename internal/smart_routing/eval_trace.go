package smartrouting

import (
	"fmt"
	"strings"

	"github.com/gobwas/glob"
)

// OpEvalResult captures the outcome of evaluating a single SmartOp against a
// request context. It is intended for diagnostic logging and surfaces both the
// configured operation and the runtime value that was compared.
type OpEvalResult struct {
	UUID      string `json:"uuid,omitempty"`
	Position  string `json:"position"`
	Operation string `json:"operation"`
	Value     string `json:"value,omitempty"`     // configured comparison value
	Actual    string `json:"actual,omitempty"`    // value extracted from request (may be truncated)
	Matched   bool   `json:"matched"`
	Reason    string `json:"reason,omitempty"`    // human-readable explanation
}

// RuleEvalResult captures the outcome of evaluating a single SmartRouting rule.
// Ops contains entries for every op evaluated; evaluation short-circuits at the
// first failed op (so the slice may end with a non-matching entry).
type RuleEvalResult struct {
	RuleIndex    int            `json:"rule_index"`
	Description  string         `json:"description,omitempty"`
	Matched      bool           `json:"matched"`
	OpsEvaluated int            `json:"ops_evaluated"`
	OpsTotal     int            `json:"ops_total"`
	Ops          []OpEvalResult `json:"ops,omitempty"`
}

// TraceEvaluation evaluates rules against a context while recording the
// per-rule, per-op outcomes. It does not return matched services — callers
// should still use EvaluateRequestWithIndex for actual selection. The trace
// contains an entry for every rule that was considered (in order, stopping at
// the first match).
func (r *Router) TraceEvaluation(ctx *RequestContext) []RuleEvalResult {
	trace := make([]RuleEvalResult, 0, len(r.rules))
	for i := range r.rules {
		rule := &r.rules[i]
		opResults := r.evaluateRuleVerbose(ctx, rule)

		ruleMatched := true
		for _, op := range opResults {
			if !op.Matched {
				ruleMatched = false
				break
			}
		}

		trace = append(trace, RuleEvalResult{
			RuleIndex:    i,
			Description:  rule.Description,
			Matched:      ruleMatched,
			OpsEvaluated: len(opResults),
			OpsTotal:     len(rule.Ops),
			Ops:          opResults,
		})

		if ruleMatched {
			break
		}
	}
	return trace
}

// ServiceMatch is a lightweight, log-friendly reference to a matched service.
type ServiceMatch struct {
	Provider string `json:"provider"`
	Model    string `json:"model"`
}

// evaluateRuleVerbose mirrors evaluateRule but records the outcome of each op.
// It still short-circuits at the first failed op for parity with the runtime
// evaluator (we don't want to do extra work or report misleading "matches" for
// ops that wouldn't have been reached).
func (r *Router) evaluateRuleVerbose(ctx *RequestContext, rule *SmartRouting) []OpEvalResult {
	origStats := ctx.ServiceStats
	origCap := ctx.ServiceCapacity
	ctx.ServiceStats = collectRuleStats(rule.Services)
	ctx.ServiceCapacity = filterCapacityForRule(ctx.ServiceCapacity, rule.Services)
	defer func() {
		ctx.ServiceStats = origStats
		ctx.ServiceCapacity = origCap
	}()

	results := make([]OpEvalResult, 0, len(rule.Ops))
	for i := range rule.Ops {
		op := &rule.Ops[i]
		res := r.evaluateOpVerbose(ctx, op)
		results = append(results, res)
		if !res.Matched {
			break
		}
	}
	return results
}

const traceMaxLen = 200

func truncForTrace(s string) string {
	if len(s) <= traceMaxLen {
		return s
	}
	return s[:traceMaxLen] + "…"
}

// evaluateOpVerbose returns an OpEvalResult while preserving the existing
// boolean evaluation logic. The reason / actual fields are populated to make
// the outcome diagnosable from the system log.
func (r *Router) evaluateOpVerbose(ctx *RequestContext, op *SmartOp) OpEvalResult {
	res := OpEvalResult{
		UUID:      op.UUID,
		Position:  string(op.Position),
		Operation: string(op.Operation),
		Value:     op.Value,
	}

	switch op.Position {
	case PositionModel:
		res.Actual = ctx.Model
		matched, reason := evalModel(ctx.Model, op)
		res.Matched = matched
		res.Reason = reason
	case PositionThinking:
		res.Actual = boolStr(ctx.ThinkingEnabled)
		matched, reason := evalThinking(ctx.ThinkingEnabled, op)
		res.Matched = matched
		res.Reason = reason
	case PositionContextSystem:
		combined := ctx.CombineMessages(ctx.SystemMessages)
		res.Actual = truncForTrace(combined)
		matched, reason := evalContains(combined, op, "system")
		res.Matched = matched
		res.Reason = reason
	case PositionContextUser:
		combined := ctx.CombineMessages(ctx.UserMessages)
		res.Actual = truncForTrace(combined)
		matched, reason := evalContains(combined, op, "user")
		res.Matched = matched
		res.Reason = reason
	case PositionLatestUser:
		matched, reason, actual := evalLatestUser(ctx, op)
		res.Actual = truncForTrace(actual)
		res.Matched = matched
		res.Reason = reason
	case PositionToolUse:
		res.Actual = strings.Join(ctx.ToolUses, ",")
		matched, reason := evalToolUse(ctx.ToolUses, op)
		res.Matched = matched
		res.Reason = reason
	case PositionToken:
		res.Actual = fmt.Sprintf("%d", ctx.EstimatedTokens)
		matched, reason := evalToken(ctx.EstimatedTokens, op)
		res.Matched = matched
		res.Reason = reason
	case PositionServiceTTFT:
		matched, reason, actual := evalServiceTTFT(ctx, op)
		res.Actual = actual
		res.Matched = matched
		res.Reason = reason
	case PositionServiceCapacity:
		matched, reason, actual := evalServiceCapacity(ctx, op)
		res.Actual = actual
		res.Matched = matched
		res.Reason = reason
	case PositionAgentClaudeCode:
		res.Actual = ctx.ClaudeCodeRequestKind
		matched, reason := evalAgentClaudeCode(ctx.ClaudeCodeRequestKind, op)
		res.Matched = matched
		res.Reason = reason
	default:
		res.Matched = false
		res.Reason = fmt.Sprintf("unknown position %q", op.Position)
	}
	return res
}

func boolStr(b bool) string {
	if b {
		return "true"
	}
	return "false"
}

// --- per-position evaluators (return matched, reason). They duplicate the core
//     logic in routing.go but produce diagnostic strings so the system log can
//     explain *why* each op did or didn't match. ---

func evalModel(model string, op *SmartOp) (bool, string) {
	value, err := op.String()
	if err != nil {
		return false, fmt.Sprintf("invalid value: %v", err)
	}
	switch op.Operation {
	case OpModelContains:
		ok := strings.Contains(model, value)
		return ok, fmt.Sprintf("model %q contains %q = %v", model, value, ok)
	case OpModelGlob:
		g, err := glob.Compile(value)
		if err != nil {
			return false, fmt.Sprintf("invalid glob %q: %v", value, err)
		}
		ok := g.Match(model)
		return ok, fmt.Sprintf("model %q matches glob %q = %v", model, value, ok)
	case OpModelEquals:
		ok := model == value
		return ok, fmt.Sprintf("model %q equals %q = %v", model, value, ok)
	}
	return false, fmt.Sprintf("unsupported model op %q", op.Operation)
}

func evalThinking(enabled bool, op *SmartOp) (bool, string) {
	val, err := op.Bool()
	if err != nil && op.Value != "" {
		return false, fmt.Sprintf("invalid bool %q: %v", op.Value, err)
	}
	switch op.Operation {
	case OpThinkingEnabled:
		if op.Value == "" || val {
			return enabled, fmt.Sprintf("thinking enabled = %v", enabled)
		}
		return false, "value=false; check is no-op"
	case OpThinkingDisabled:
		if op.Value == "" || val {
			return !enabled, fmt.Sprintf("thinking disabled = %v", !enabled)
		}
		return false, "value=false; check is no-op"
	}
	return false, fmt.Sprintf("unsupported thinking op %q", op.Operation)
}

func evalContains(combined string, op *SmartOp, label string) (bool, string) {
	value, err := op.String()
	if err != nil {
		return false, fmt.Sprintf("invalid value: %v", err)
	}
	// OpContextSystemContains and OpContextUserContains are distinguished by
	// position (handled by the caller), but share the same operation string
	// "contains". Same for "regex". Match by string value here.
	switch string(op.Operation) {
	case "contains":
		ok := strings.Contains(combined, value)
		return ok, fmt.Sprintf("%s context contains %q = %v", label, value, ok)
	case "regex":
		ok, err := stringsMatch(combined, value, true)
		if err != nil {
			return false, fmt.Sprintf("regex error: %v", err)
		}
		return ok, fmt.Sprintf("%s context matches %q = %v", label, value, ok)
	}
	return false, fmt.Sprintf("unsupported %s op %q", label, op.Operation)
}

func evalLatestUser(ctx *RequestContext, op *SmartOp) (bool, string, string) {
	value, err := op.String()
	if err != nil {
		return false, fmt.Sprintf("invalid value: %v", err), ""
	}
	switch op.Operation {
	case OpLatestUserContains:
		if ctx.LatestRole != "user" {
			return false, fmt.Sprintf("latest role is %q, not user", ctx.LatestRole), ctx.GetLatestUserMessage()
		}
		latest := ctx.GetLatestUserMessage()
		ok := strings.Contains(latest, value)
		return ok, fmt.Sprintf("latest user contains %q = %v", value, ok), latest
	case OpLatestUserRequestType:
		ok := ctx.LatestContentType == value
		return ok, fmt.Sprintf("latest content_type %q == %q = %v", ctx.LatestContentType, value, ok), ctx.LatestContentType
	}
	return false, fmt.Sprintf("unsupported latest_user op %q", op.Operation), ""
}

func evalToolUse(tools []string, op *SmartOp) (bool, string) {
	value, err := op.String()
	if err != nil {
		return false, fmt.Sprintf("invalid value: %v", err)
	}
	if op.Operation != OpToolUseEquals {
		return false, fmt.Sprintf("unsupported tool_use op %q", op.Operation)
	}
	for _, t := range tools {
		if t == value {
			return true, fmt.Sprintf("tool %q used", value)
		}
	}
	return false, fmt.Sprintf("tool %q not in [%s]", value, strings.Join(tools, ","))
}

func evalToken(tokens int, op *SmartOp) (bool, string) {
	target, err := op.Int()
	if err != nil {
		return false, fmt.Sprintf("invalid int: %v", err)
	}
	switch op.Operation {
	case OpTokenGe:
		ok := tokens >= target
		return ok, fmt.Sprintf("tokens %d >= %d = %v", tokens, target, ok)
	case OpTokenGt:
		ok := tokens > target
		return ok, fmt.Sprintf("tokens %d > %d = %v", tokens, target, ok)
	case OpTokenLe:
		ok := tokens <= target
		return ok, fmt.Sprintf("tokens %d <= %d = %v", tokens, target, ok)
	case OpTokenLt:
		ok := tokens < target
		return ok, fmt.Sprintf("tokens %d < %d = %v", tokens, target, ok)
	}
	return false, fmt.Sprintf("unsupported token op %q", op.Operation)
}

func evalServiceTTFT(ctx *RequestContext, op *SmartOp) (bool, string, string) {
	if len(ctx.ServiceStats) == 0 {
		return true, "no service stats; pass", ""
	}
	threshold, err := op.Int()
	if err != nil {
		return false, fmt.Sprintf("invalid int: %v", err), ""
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
		return true, "no TTFT samples; pass", ""
	}
	thresholdF := float64(threshold)
	switch op.Operation {
	case OpServiceTTFTAvgLe:
		actual := minFloat(values)
		ok := actual <= thresholdF
		return ok, fmt.Sprintf("min(avg_ttft) %.0f <= %d = %v", actual, threshold, ok), fmt.Sprintf("%.0f", actual)
	case OpServiceTTFTAvgGe:
		actual := avgFloat(values)
		ok := actual >= thresholdF
		return ok, fmt.Sprintf("avg(avg_ttft) %.0f >= %d = %v", actual, threshold, ok), fmt.Sprintf("%.0f", actual)
	case OpServiceTTFTMaxLe:
		actual := minFloat(values)
		ok := actual <= thresholdF
		return ok, fmt.Sprintf("min(p99_ttft) %.0f <= %d = %v", actual, threshold, ok), fmt.Sprintf("%.0f", actual)
	case OpServiceTTFTMaxGe:
		actual := avgFloat(values)
		ok := actual >= thresholdF
		return ok, fmt.Sprintf("avg(p99_ttft) %.0f >= %d = %v", actual, threshold, ok), fmt.Sprintf("%.0f", actual)
	}
	return false, fmt.Sprintf("unsupported service_ttft op %q", op.Operation), ""
}

func evalAgentClaudeCode(kind string, op *SmartOp) (bool, string) {
	value, err := op.String()
	if err != nil {
		return false, fmt.Sprintf("invalid value: %v", err)
	}
	if op.Operation != OpAgentClaudeCodeEquals {
		return false, fmt.Sprintf("unsupported agent.claude_code op %q", op.Operation)
	}
	if kind == "" {
		return false, "request kind not set (scenario is not claude_code)"
	}
	ok := kind == value
	return ok, fmt.Sprintf("agent.claude_code %q == %q = %v", kind, value, ok)
}

func evalServiceCapacity(ctx *RequestContext, op *SmartOp) (bool, string, string) {
	if len(ctx.ServiceCapacity) == 0 {
		return true, "no capacity info; pass", ""
	}
	threshold, err := op.Int()
	if err != nil {
		return false, fmt.Sprintf("invalid int: %v", err), ""
	}
	var utilValues []float64
	for _, c := range ctx.ServiceCapacity {
		if c.Capacity <= 0 {
			continue
		}
		util := float64(c.ActiveCount) / float64(c.Capacity) * 100
		utilValues = append(utilValues, util)
	}
	if len(utilValues) == 0 {
		return true, "no capacity-configured services; pass", ""
	}
	avg := avgFloat(utilValues)
	thresholdF := float64(threshold)
	actual := fmt.Sprintf("%.1f%%", avg)
	switch op.Operation {
	case OpServiceCapacityUtilLe:
		ok := avg <= thresholdF
		return ok, fmt.Sprintf("avg util %.1f%% <= %d%% = %v", avg, threshold, ok), actual
	case OpServiceCapacityUtilGe:
		ok := avg >= thresholdF
		return ok, fmt.Sprintf("avg util %.1f%% >= %d%% = %v", avg, threshold, ok), actual
	case OpServiceCapacityUtilLt:
		ok := avg < thresholdF
		return ok, fmt.Sprintf("avg util %.1f%% < %d%% = %v", avg, threshold, ok), actual
	case OpServiceCapacityUtilGt:
		ok := avg > thresholdF
		return ok, fmt.Sprintf("avg util %.1f%% > %d%% = %v", avg, threshold, ok), actual
	}
	return false, fmt.Sprintf("unsupported service_capacity op %q", op.Operation), actual
}
