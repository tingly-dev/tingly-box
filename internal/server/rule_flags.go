package server

import (
	"github.com/openai/openai-go/v3"
	"github.com/openai/openai-go/v3/packages/param"
	"github.com/tingly-dev/tingly-box/internal/typ"
)

// resolveRuleFlags returns a copy of the rule's flags, or the zero value when
// no rule is bound. Callers may always read fields without nil-checking.
func resolveRuleFlags(rule *typ.Rule) typ.RuleFlags {
	if rule == nil {
		return typ.RuleFlags{}
	}
	return rule.Flags
}

// applyMaxCompletionTokensRewrite moves the value of `max_tokens` into the
// newer `max_completion_tokens` field. OpenAI's o1/o3/gpt-5 families reject
// `max_tokens`; this rewrite lets callers opt in per rule.
func applyMaxCompletionTokensRewrite(req *openai.ChatCompletionNewParams) {
	if req == nil {
		return
	}
	if req.MaxTokens.Valid() {
		req.MaxCompletionTokens = param.NewOpt(req.MaxTokens.Value)
		req.MaxTokens = param.Opt[int64]{}
	}
}

// applyMaxTokensRewrite moves the value of `max_completion_tokens` back into
// the legacy `max_tokens` field. Some providers and older model endpoints
// reject the newer field name; this rewrite lets callers force the legacy
// field per rule.
func applyMaxTokensRewrite(req *openai.ChatCompletionNewParams) {
	if req == nil {
		return
	}
	if req.MaxCompletionTokens.Valid() {
		req.MaxTokens = param.NewOpt(req.MaxCompletionTokens.Value)
		req.MaxCompletionTokens = param.Opt[int64]{}
	}
}

// shouldStripUsage merges the cursor_compat and skip_usage hints carried in
// reqCtx.Extra. The dispatch layer ORs both together so a rule that only
// flips skip_usage still strips the usage block, and cursor_compat keeps its
// historical behavior of suppressing usage as a side effect.
//
// Extracted so the wiring is unit-testable independent of the surrounding
// transform/forward machinery.
func shouldStripUsage(extra map[string]interface{}) bool {
	if extra == nil {
		return false
	}
	if v, ok := extra["cursor_compat"]; ok {
		if b, _ := v.(bool); b {
			return true
		}
	}
	if v, ok := extra["skip_usage"]; ok {
		if b, _ := v.(bool); b {
			return true
		}
	}
	return false
}
