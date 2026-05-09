package smartrouting

import "strings"

// OpEvalResult captures the outcome of evaluating a single SmartOp against a
// request context. It is the unified return type of the evaluator: the
// fast-path (EvaluateRequest etc.) reads only Matched, while diagnostic
// callers also read Reason and Actual.
type OpEvalResult struct {
	UUID      string `json:"uuid,omitempty"`
	Position  string `json:"position"`
	Operation string `json:"operation"`
	Value     string `json:"value,omitempty"`  // configured comparison value
	Actual    string `json:"actual,omitempty"` // value extracted from request (may be truncated)
	Matched   bool   `json:"matched"`
	Reason    string `json:"reason,omitempty"` // human-readable explanation
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

// ServiceMatch is a lightweight, log-friendly reference to a matched service.
type ServiceMatch struct {
	Provider string `json:"provider"`
	Model    string `json:"model"`
}

// TraceEvaluation evaluates rules against a context and returns the per-rule
// trace. Callers that also need the matched services should use
// (*Router).Evaluate to avoid iterating twice.
func (r *Router) TraceEvaluation(ctx *RequestContext) []RuleEvalResult {
	_, _, _, trace := r.Evaluate(ctx)
	return trace
}

// snippetMatchCtx is how many characters of leading/trailing context are kept
// around a matched substring. Tuned to read like ~one short line on either
// side (enough for context, but tiny relative to typical chat history).
const snippetMatchCtx = 80

// snippetHeadLen is the maximum head length kept when there's nothing more
// specific to point at (e.g. a regex op against a long body). Smaller than
// snippetAround windows because it carries less diagnostic value.
const snippetHeadLen = 120

// snippetAround returns a window around the first occurrence of needle inside
// text, decorated with ellipses on whichever side was trimmed. When needle is
// empty or not present, falls back to snippetHead so the trace still shows
// some of the input the rule was evaluated against.
func snippetAround(text, needle string) string {
	if text == "" {
		return ""
	}
	if needle == "" {
		return snippetHead(text, snippetHeadLen)
	}
	idx := strings.Index(text, needle)
	if idx < 0 {
		return snippetHead(text, snippetHeadLen)
	}
	start := idx - snippetMatchCtx
	if start < 0 {
		start = 0
	}
	end := idx + len(needle) + snippetMatchCtx
	if end > len(text) {
		end = len(text)
	}
	prefix := ""
	if start > 0 {
		prefix = "…"
	}
	suffix := ""
	if end < len(text) {
		suffix = "…"
	}
	return prefix + text[start:end] + suffix
}

// snippetHead returns the first n chars of s with an ellipsis if truncated.
func snippetHead(s string, n int) string {
	if len(s) <= n {
		return s
	}
	return s[:n] + "…"
}
