package config

import typ "github.com/tingly-dev/tingly-box/internal/typ"

// Built-in rule UUID constants
const (
	// Very old / pre-scenario-split legacy UUIDs. Still present in configs
	// written before the scenario model was introduced.
	RuleUUIDTingly     = "tingly"
	RuleUUIDClaudeCode = "claude-code"

	// Legacy built-in UUIDs (hyphenated "built-in-*" form). Live configs are
	// renamed to the modern RuleUUID* values by migrate20260612; these
	// constants remain so older migrations and compatibility fallbacks can
	// still address pre-rename configs.
	RuleUUIDBuiltinOpenAI    = "built-in-openai"
	RuleUUIDBuiltinAnthropic = "built-in-anthropic"
	RuleUUIDBuiltinCodex     = "built-in-codex"
	RuleUUIDBuiltinOpenCode  = "built-in-opencode"
	RuleUUIDBuiltinAgent     = "built-in-agent"
	RuleUUIDBuiltinAgentClaw = "built-in-agent-claw"
	RuleUUIDBuiltinTeam      = "built-in-team"

	// Legacy Claude Code built-in UUIDs. Live configs are renamed to the
	// modern RuleUUIDCC* values by migrate20260611; these constants remain so
	// older migrations (which run before the rename) and compatibility
	// fallbacks can still address pre-rename configs.
	RuleUUIDBuiltinCC         = "built-in-cc"
	RuleUUIDBuiltinCCHaiku    = "built-in-cc-haiku"
	RuleUUIDBuiltinCCSonnet   = "built-in-cc-sonnet"
	RuleUUIDBuiltinCCOpus     = "built-in-cc-opus"
	RuleUUIDBuiltinCCDefault  = "built-in-cc-default"
	RuleUUIDBuiltinCCSubagent = "built-in-cc-subagent"

	// Modern built-in rules — "builtin:<scenario>:<tier>" form.
	// These are the target UUIDs after migrate20260612 renames the legacy ones.
	RuleUUIDOpenAI    = "builtin:openai:default"
	RuleUIDAnthropic  = "builtin:anthropic:default"
	RuleUUIDCodex     = "builtin:codex:default"
	RuleUUIDOpenCode  = "builtin:opencode:default"
	RuleUUIDAgent     = "builtin:agent:default"
	RuleUUIDAgentClaw = "builtin:agent:claw"

	// Claude Code built-in rules (modern "builtin:<scenario>:<tier>" form)
	RuleUUIDCC         = "builtin:claude_code:cc"
	RuleUUIDCCDefault  = "builtin:claude_code:default"
	RuleUUIDCCHaiku    = "builtin:claude_code:haiku"
	RuleUUIDCCSonnet   = "builtin:claude_code:sonnet"
	RuleUUIDCCOpus     = "builtin:claude_code:opus"
	RuleUUIDCCSubagent = "builtin:claude_code:subagent"

	// Claude Desktop built-in rules (locked, using builtin: prefix)
	RuleUUIDBuiltinClaudeDesktopSonnet46 = "builtin:claude_desktop:claude-sonnet-4-6"
	RuleUUIDBuiltinClaudeDesktopOpus46   = "builtin:claude_desktop:claude-opus-4-6"
	RuleUUIDBuiltinClaudeDesktopOpus47   = "builtin:claude_desktop:claude-opus-4-7"
	RuleUUIDBuiltinClaudeDesktopHaiku45  = "builtin:claude_desktop:claude-haiku-4-5"
)

// legacyCCRuleUUIDs maps the legacy Claude Code built-in UUIDs to their modern
// counterparts. Used by migrate20260611 to rename live configs and by
// defaultRuleByUUID to keep older migrations (written against the legacy
// names) able to pull templates from the modern DefaultRules.
var legacyCCRuleUUIDs = map[string]string{
	RuleUUIDBuiltinCC:         RuleUUIDCC,
	RuleUUIDBuiltinCCDefault:  RuleUUIDCCDefault,
	RuleUUIDBuiltinCCHaiku:    RuleUUIDCCHaiku,
	RuleUUIDBuiltinCCSonnet:   RuleUUIDCCSonnet,
	RuleUUIDBuiltinCCOpus:     RuleUUIDCCOpus,
	RuleUUIDBuiltinCCSubagent: RuleUUIDCCSubagent,
}

// legacySimpleRuleUUIDs maps the remaining legacy built-in UUIDs (all
// non-CC single-rule scenarios) to their modern "builtin:<scenario>:<model>"
// counterparts. Used by migrate20260612 and defaultRuleByUUID.
var legacySimpleRuleUUIDs = map[string]string{
	RuleUUIDTingly:           RuleUUIDOpenAI, // very old "tingly" rule preceded the openai scenario
	RuleUUIDBuiltinOpenAI:    RuleUUIDOpenAI,
	RuleUUIDBuiltinAnthropic: RuleUIDAnthropic,
	RuleUUIDBuiltinCodex:     RuleUUIDCodex,
	RuleUUIDBuiltinOpenCode:  RuleUUIDOpenCode,
	RuleUUIDBuiltinAgent:     RuleUUIDAgent,
	RuleUUIDBuiltinAgentClaw: RuleUUIDAgentClaw,
}

// BuiltinRuleUUID builds a built-in rule UUID in the modern
// "builtin:<scenario>:<model>" form — the target convention all built-in rules
// will eventually converge on (the legacy "built-in-*" constants above predate
// it). Works for profiled scenarios too, since the scenario name carries the
// profile suffix: BuiltinRuleUUID("claude_code:p1", "haiku") =
// "builtin:claude_code:p1:haiku".
func BuiltinRuleUUID(scenario typ.RuleScenario, model string) string {
	return "builtin:" + string(scenario) + ":" + model
}

// ccProfileTiers is the set of request models a system-seeded Claude Code
// profile rule routes on (unified "cc", or the five separate-mode tiers).
// Profile rules with any other request model are user-customized and keep
// whatever UUID they have.
var ccProfileTiers = map[string]bool{
	"cc":       true,
	"default":  true,
	"haiku":    true,
	"sonnet":   true,
	"opus":     true,
	"subagent": true,
}

var claudeCodeUnifiedRuleUUIDs = map[string]bool{
	RuleUUIDCC:        true,
	RuleUUIDBuiltinCC: true,
}

var claudeCodeSeparateRuleUUIDs = map[string]bool{
	RuleUUIDCCDefault:         true,
	RuleUUIDCCHaiku:           true,
	RuleUUIDCCSonnet:          true,
	RuleUUIDCCOpus:            true,
	RuleUUIDCCSubagent:        true,
	RuleUUIDBuiltinCCDefault:  true,
	RuleUUIDBuiltinCCHaiku:    true,
	RuleUUIDBuiltinCCSonnet:   true,
	RuleUUIDBuiltinCCOpus:     true,
	RuleUUIDBuiltinCCSubagent: true,
}
