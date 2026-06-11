package server

import (
	"strings"

	"github.com/gin-gonic/gin"
	"github.com/tingly-dev/tingly-box/internal/protocol"
	"github.com/tingly-dev/tingly-box/internal/protocol/transform"
	"github.com/tingly-dev/tingly-box/internal/typ"
)

// rulePreBaseTransforms builds the per-rule list of pre-Base transforms for the
// chain's preBase slot. Pre-Base transforms act on the *inbound* request shape —
// they run before BaseTransform's protocol conversion, so the type-switch inside
// each transform sees what the client actually sent.
//
// Returns nil when no rule-level flag requires a pre-Base stage so callers can
// pass the result straight to BuildTransformChain's preBase parameter.
func rulePreBaseTransforms(flags typ.RuleFlags) []transform.Transform {
	var pre []transform.Transform
	if flags.CursorCompat {
		pre = append(pre, transform.NewOpenAICursorCompatTransform())
	}
	if flags.CleanHeader {
		pre = append(pre, NewCleanHeaderTransform())
	}
	if names := parseBlockTools(flags.BlockTools); len(names) > 0 {
		pre = append(pre, transform.NewToolBlockTransform(names))
	}
	if flags.ClaudeCodeCompat {
		pre = append(pre, transform.NewClaudeCodeCompatTransform())
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

// rulePreVendorTransforms builds the per-rule list of pre-Vendor transforms for
// the chain's preVendor slot (after Consistency, before Vendor). These act on
// the *target* request shape — they run after BaseTransform's protocol
// conversion, so the type-switch inside each transform matches the
// upstream-bound form, but still before Vendor finalizes the request.
//
// Returns nil when no rule-level flag requires a chain stage so callers can
// pass the result straight to a `preVendorTransforms []transform.Transform`
// parameter.
//
// Takes already-resolved flags so callers that need other fields off
// RuleFlags (CustomUserAgent, SkipUsage) can resolve once and share.
func rulePreVendorTransforms(flags typ.RuleFlags) []transform.Transform {
	var preVendor []transform.Transform
	if flags.UseMaxCompletionTokens || flags.UseMaxTokens {
		preVendor = append(preVendor, transform.NewOpenAIMaxTokensRewriteTransform(
			flags.UseMaxCompletionTokens,
			flags.UseMaxTokens,
		))
	}
	if flags.ThinkingEffort != typ.ThinkingEffortDefault {
		preVendor = append(preVendor, transform.NewRuleThinkingTransform(flags.ThinkingEffort))
	}
	return preVendor
}

// resolveRuleFlags returns the effective flags for this request: a copy of
// the rule's persisted flags, with:
// - cursor_compat_auto folded into cursor_compat when the inbound request carries Cursor headers
// Returns the zero value when no rule is bound.
//
// All flag folding/injection happens here (not at each handler call site) so that
// rulePreBaseTransforms and downstream consumers read the same merged value.
func resolveRuleFlags(c *gin.Context, rule *typ.Rule) typ.RuleFlags {
	if rule == nil {
		return typ.RuleFlags{}
	}
	flags := rule.Flags

	// Auto-detect Cursor requests
	if flags.CursorCompatAuto && isCursorRequest(c) {
		flags.CursorCompat = true
	}

	return flags
}

// resolveRuleFlagsWithScenario extends resolveRuleFlags to also inject scenario-level
// flags and auto-apply CleanHeader for protocol transformation scenarios.
//
// This is the main entry point that merges:
//  1. Rule-level flags (from the rule definition)
//  2. Scenario flags (from the scenario configuration)
//  3. Auto-applied flags (like CleanHeader for protocol transformation)
//  4. Provider-driven suppressions (CleanHeader is cleared for Claude OAuth providers;
//     the billing header must reach Anthropic's billing backend unchanged).
//
// Side effect: it also attaches the resolved CustomUserAgent to the request
// context (applyCustomUserAgent) so callers don't have to repeat that at each
// handler. The User-Agent is the one rule flag that has to reach a deep
// component (the outbound transport) via ctx, so this central merge point is
// where it gets applied.
func resolveRuleFlagsWithScenario(
	c *gin.Context,
	rule *typ.Rule,
	scenarioType typ.RuleScenario,
	scenarioConfig *typ.ScenarioConfig,
	sourceAPI, targetAPI protocol.APIType,
	provider *typ.Provider,
) typ.RuleFlags {
	flags := resolveRuleFlags(c, rule)

	if scenarioConfig != nil {
		// Only inject scenario-level ThinkingEffort if rule hasn't set it explicitly
		if flags.ThinkingEffort == typ.ThinkingEffortDefault && scenarioConfig.Flags.ThinkingEffort != typ.ThinkingEffortDefault {
			flags.ThinkingEffort = scenarioConfig.Flags.ThinkingEffort
		}

		// Inject scenario-level ClaudeCodeCompat if not already set at rule level
		flags.ClaudeCodeCompat = flags.ClaudeCodeCompat || scenarioConfig.Flags.ClaudeCodeCompat

		// Inject scenario-level SkipUsage if not already set at rule level
		flags.SkipUsage = flags.SkipUsage || scenarioConfig.Flags.SkipUsage

		// Inject scenario-level CustomUserAgent if rule hasn't set one explicitly.
		// Rule value wins so a single rule can retarget UA without disturbing the
		// scenario-wide default.
		if flags.CustomUserAgent == "" && scenarioConfig.Flags.CustomUserAgent != "" {
			flags.CustomUserAgent = scenarioConfig.Flags.CustomUserAgent
		}

		// SessionAffinity is rule-only — no scenario-level inheritance. The
		// built-in Claude Code / Desktop / Codex rules seed it directly (init +
		// migrate20260610), so there is nothing to inject here.
	}

	// Auto-apply CleanHeader for protocol transformation in billing scenarios
	flags = autoSetCleanHeaderFlag(flags, sourceAPI, targetAPI, scenarioType)

	// Suppress CleanHeader when the provider is Claude OAuth (native Anthropic
	// subscription). The x-anthropic-billing-header injected by Claude Code is
	// consumed by Anthropic's billing backend; stripping it would break billing
	// for OAuth subscribers even though it must be stripped for every other
	// provider type (third-party Anthropic-compatible, OpenAI, etc.).
	if flags.CleanHeader && provider.IsClaudeCodeProvider() {
		flags.CleanHeader = false
	}

	// Attach the resolved User-Agent override to the request context here, at the
	// single merge point, so the chat / v1 / beta handlers don't each repeat it.
	applyCustomUserAgent(c, flags)

	// Attach the 1M-context hint the same way: the outbound Anthropic transport
	// (context1mBetaTransport) appends the context-1m beta flag at RoundTrip
	// time.
	applyContext1M(c, flags)

	return flags
}

// applyContext1M attaches the 1M-context hint to the request context so the
// outbound Anthropic transport injects the context-1m beta flag upstream.
// Without this, the flag would only ever be advertised to clients ([1m] model
// names) and never reach providers whose clients don't send it themselves.
func applyContext1M(c *gin.Context, flags typ.RuleFlags) {
	if !flags.Context1M || c == nil || c.Request == nil {
		return
	}
	c.Request = c.Request.WithContext(typ.WithContext1M(c.Request.Context()))
}

// applyCustomUserAgent attaches the effective custom User-Agent (already merged
// across rule + scenario) to the request context, so the outbound transport
// (customUserAgentTransport) can read it at RoundTrip time. This is the Type-2
// (context-passed hint) injection point: the dispatch path forwards
// c.Request.Context() down to the SDK call, where the transport applies the
// override. No-op when no override is configured, so the vendor/provider
// User-Agent is left untouched.
//
// Called from resolveRuleFlagsWithScenario, which every handler now routes
// through — so no handler needs to apply the User-Agent itself.
func applyCustomUserAgent(c *gin.Context, flags typ.RuleFlags) {
	if flags.CustomUserAgent == "" || c == nil || c.Request == nil {
		return
	}
	c.Request = c.Request.WithContext(typ.WithCustomUserAgent(c.Request.Context(), flags.CustomUserAgent))
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

// isBillingHeaderScenario returns true if the scenario is known to inject billing headers
// into system messages. These scenarios require the CleanHeader transform when doing
// protocol transformation (e.g., Anthropic → OpenAI).
func isBillingHeaderScenario(scenario typ.RuleScenario) bool {
	switch scenario.Base() {
	case typ.ScenarioClaudeCode, typ.ScenarioClaudeDesktop:
		return true
	default:
		return false
	}
}

// autoSetCleanHeaderFlag automatically sets the CleanHeader flag when protocol
// transformation is detected for billing scenarios (claude_code, claude_desktop).
// Returns the potentially modified flags.
func autoSetCleanHeaderFlag(
	flags typ.RuleFlags,
	sourceAPI, targetAPI protocol.APIType,
	scenario typ.RuleScenario,
) typ.RuleFlags {
	// Skip if manual flag is already set
	if flags.CleanHeader {
		return flags
	}

	// Auto-set for protocol transformation in billing scenarios
	if sourceAPI != targetAPI && isBillingHeaderScenario(scenario) {
		flags.CleanHeader = true
	}

	return flags
}
