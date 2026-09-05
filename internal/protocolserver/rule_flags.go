package protocolserver

import (
	"strings"

	"github.com/gin-gonic/gin"
	"github.com/tingly-dev/tingly-box/internal/protocol"
	"github.com/tingly-dev/tingly-box/internal/protocol/transform"
	servertransform "github.com/tingly-dev/tingly-box/internal/protocolserver/transform"
	"github.com/tingly-dev/tingly-box/internal/typ"
)

// RulePreBaseTransforms builds the per-rule list of pre-Base transforms for the
// chain's preBase slot. Pre-Base transforms act on the *inbound* request shape —
// they run before BaseTransform's protocol conversion, so the type-switch inside
// each transform sees what the client actually sent.
//
// Returns nil when no rule-level flag requires a pre-Base stage so callers can
// pass the result straight to BuildTransformChain's preBase parameter.
func RulePreBaseTransforms(flags typ.RuleFlags) []transform.Transform {
	var pre []transform.Transform
	if flags.CursorCompat {
		pre = append(pre, transform.NewOpenAICursorCompatTransform())
	}
	if flags.CleanHeader {
		pre = append(pre, servertransform.NewCleanHeaderTransform())
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
	for part := range strings.SplitSeq(raw, ",") {
		if name := strings.TrimSpace(part); name != "" {
			names = append(names, name)
		}
	}
	return names
}

// RulePreVendorTransforms builds the per-rule list of pre-Vendor transforms for
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
func RulePreVendorTransforms(flags typ.RuleFlags) []transform.Transform {
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

// ResolveRuleFlags returns the effective flags for this request: a copy of
// the rule's persisted flags, with:
// - cursor_compat_auto folded into cursor_compat when the inbound request carries Cursor headers
// Returns the zero value when no rule is bound.
//
// All flag folding/injection happens here (not at each handler call site) so that
// RulePreBaseTransforms and downstream consumers read the same merged value.
func ResolveRuleFlags(c *gin.Context, rule *typ.Rule) typ.RuleFlags {
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

// ResolveRuleFlagsWithScenario extends resolveRuleFlags to also inject scenario-level
// flags and auto-apply CleanHeader for protocol transformation scenarios.
//
// This is the main entry point that merges:
//  1. Rule-level flags (from the rule definition)
//  2. Scenario flags (from the scenario configuration)
//  3. Auto-applied flags (like CleanHeader for protocol transformation)
//  4. Provider-driven suppressions (CleanHeader is cleared for Claude OAuth providers;
//     the billing header must reach Anthropic's billing backend unchanged).
//
// Side effect: it attaches the whole resolved flag set to the request context
// (applyRuleFlags) plus the inbound client UA fallback (applyClientUserAgent),
// so no handler repeats either.
func ResolveRuleFlagsWithScenario(
	c *gin.Context,
	rule *typ.Rule,
	scenarioType typ.RuleScenario,
	scenarioConfig *typ.ScenarioConfig,
	sourceAPI, targetAPI protocol.APIType,
	provider *typ.Provider,
) typ.RuleFlags {
	flags := ResolveRuleFlags(c, rule)

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

	// Recording: the rule's capture-point selection overrides the scenario's
	// recording_v2 default (override inheritance, like thinking_effort). Both
	// sides accept legacy enum values; the resolved value is normalized to a
	// canonical point set so downstream consumers never re-parse legacy forms.
	if m := typ.ParseRecordingMode(flags.Recording); m != typ.RecordingModeDisabled {
		flags.Recording = string(m)
	} else if scenarioConfig != nil {
		flags.Recording = string(typ.ParseRecordingMode(string(scenarioConfig.Flags.RecordingV2)))
	} else {
		flags.Recording = ""
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

	// Attach the whole resolved flag set once, at the single merge point, so
	// every downstream consumer — ruleFlagTransport (custom_user_agent,
	// extra_headers), the Anthropic client's Beta/Messages methods
	// (context_1m), NewClaudeClient (claude_org_id) — reads the same value
	// via typ.GetRuleFlags. This is the one Type-2 (context-passed) injection
	// point; no handler applies anything itself.
	applyRuleFlags(c, flags)

	// The inbound client UA is attached unconditionally; ruleFlagTransport is
	// the sole arbiter of the UA precedence (custom_user_agent > client UA >
	// SDK default), so no precedence judgment is duplicated here.
	applyClientUserAgent(c)

	return flags
}

// applyRuleFlags attaches the resolved RuleFlags to the request context for
// the outbound client layer (typ.GetRuleFlags).
func applyRuleFlags(c *gin.Context, flags typ.RuleFlags) {
	if c == nil || c.Request == nil {
		return
	}
	c.Request = c.Request.WithContext(typ.WithRuleFlags(c.Request.Context(), flags))
}

// applyClientUserAgent attaches the inbound client's own User-Agent header to
// the request context so the generic outbound transport (ruleFlagTransport)
// can forward it upstream. Only the generic pass-through clients wire that
// transport; vendor-specialized paths never read it, keeping their pinned UA
// decisive. No-op when the client sent no User-Agent (SDK default stands).
func applyClientUserAgent(c *gin.Context) {
	if c == nil || c.Request == nil {
		return
	}
	ua := c.GetHeader("User-Agent")
	if ua == "" {
		return
	}
	c.Request = c.Request.WithContext(typ.WithClientUserAgent(c.Request.Context(), ua))
}

// ShouldStripUsage merges the cursor_compat and skip_usage hints carried in
// reqCtx.Extra. The dispatch layer ORs both together so a rule that only
// flips skip_usage still strips the usage block, and cursor_compat keeps its
// historical behavior of suppressing usage as a side effect.
//
// Extracted so the wiring is unit-testable independent of the surrounding
// transform/forward machinery.
func ShouldStripUsage(extra map[string]interface{}) bool {
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
