package server

import (
	"strings"

	"github.com/gin-gonic/gin"
	"github.com/tingly-dev/tingly-box/internal/protocol/transform"
	"github.com/tingly-dev/tingly-box/internal/typ"
)

// rulePreBaseTransforms builds the per-rule list of pre-Base transforms to
// prepend onto the protocol chain. Pre-Base transforms act on the *inbound*
// request shape — they run before BaseTransform's protocol conversion, so the
// type-switch inside each transform sees what the client actually sent.
//
// Returns nil when no rule-level flag requires a pre-Base stage so callers
// can pass the result straight to prependPreBaseTransforms.
func rulePreBaseTransforms(flags typ.RuleFlags) []transform.Transform {
	var pre []transform.Transform
	if flags.CursorCompat {
		pre = append(pre, transform.NewOpenAICursorCompatTransform())
	}
	if names := parseBlockTools(flags.BlockTools); len(names) > 0 {
		pre = append(pre, transform.NewToolBlockTransform(names))
	}
	return pre
}

// parseBlockTools splits the comma-separated block_tools flag into a list of
// trimmed, non-empty tool names. Returns nil when the flag is empty so callers
// can skip adding the transform entirely.
func parseBlockTools(raw string) []string {
	if strings.TrimSpace(raw) == "" {
		return nil
	}
	var names []string
	for _, part := range strings.Split(raw, ",") {
		if name := strings.TrimSpace(part); name != "" {
			names = append(names, name)
		}
	}
	return names
}

// ruleExtraTransforms builds the per-rule list of post-Base transforms to
// append to the protocol chain. Post-Base transforms act on the *target*
// request shape — they run after BaseTransform's protocol conversion, so the
// type-switch inside each transform matches the upstream-bound form.
//
// Returns nil when no rule-level flag requires a chain stage so callers can
// pass the result straight to a variadic `extraTransforms ...transform.Transform`
// parameter.
//
// Takes already-resolved flags so callers that need other fields off
// RuleFlags (CustomUserAgent, SkipUsage) can resolve once and share.
func ruleExtraTransforms(flags typ.RuleFlags) []transform.Transform {
	var extras []transform.Transform
	if flags.UseMaxCompletionTokens || flags.UseMaxTokens {
		extras = append(extras, transform.NewOpenAIMaxTokensRewriteTransform(
			flags.UseMaxCompletionTokens,
			flags.UseMaxTokens,
		))
	}
	return extras
}

// resolveRuleFlags returns the effective flags for this request: a copy of
// the rule's persisted flags, with cursor_compat_auto folded into
// cursor_compat when the inbound request carries Cursor headers. Returns the
// zero value when no rule is bound.
//
// Folding happens here (not at each handler call site) so that
// rulePreBaseTransforms and downstream consumers like reqCtx.Extra both read
// the same merged value with no duplication.
func resolveRuleFlags(c *gin.Context, rule *typ.Rule) typ.RuleFlags {
	if rule == nil {
		return typ.RuleFlags{}
	}
	flags := rule.Flags
	if flags.CursorCompatAuto && isCursorRequest(c) {
		flags.CursorCompat = true
	}
	return flags
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
