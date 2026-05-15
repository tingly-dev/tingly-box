package server

import (
	"github.com/tingly-dev/tingly-box/internal/protocol/transform"
	"github.com/tingly-dev/tingly-box/internal/typ"
)

// ruleExtraTransforms builds the per-rule list of post-base transforms to
// append to the protocol chain. Returns nil when no rule-level flag requires
// a chain stage so callers can pass the result straight to a variadic
// `extraTransforms ...transform.Transform` parameter.
//
// Takes already-resolved flags so callers that need other fields off
// RuleFlags (CustomUserAgent, SkipUsage) can resolve once and share.
func ruleExtraTransforms(flags typ.RuleFlags) []transform.Transform {
	var extras []transform.Transform
	if flags.UseMaxCompletionTokens || flags.UseMaxTokens {
		extras = append(extras, NewOpenAIMaxTokensRewriteTransform(
			flags.UseMaxCompletionTokens,
			flags.UseMaxTokens,
		))
	}
	return extras
}

// resolveRuleFlags returns a copy of the rule's flags, or the zero value when
// no rule is bound. Callers may always read fields without nil-checking.
func resolveRuleFlags(rule *typ.Rule) typ.RuleFlags {
	if rule == nil {
		return typ.RuleFlags{}
	}
	return rule.Flags
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
