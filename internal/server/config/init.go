package config

import (
	"github.com/tingly-dev/tingly-box/internal/loadbalance"
	"github.com/tingly-dev/tingly-box/internal/typ"
)

var DefaultRules []typ.Rule

// defaultSessionAffinitySeconds is the 30-minute session-affinity TTL seeded on
// the built-in Claude Code / Claude Desktop / Codex rules. session_affinity is
// rule-only (no scenario-level inheritance); these seeds + migrate20260610 are
// the sole source of the default. Other scenarios are off unless set per-rule.
const defaultSessionAffinitySeconds = 1800

func init() {
	DefaultRules = []typ.Rule{
		{
			UUID:          RuleUIDAnthropic,
			Scenario:      typ.ScenarioAnthropic,
			RequestModel:  "tingly-claude",
			ResponseModel: "",
			Description:   "Default proxy rule in tingly-box for general use with Anthropic",
			Services:      []*loadbalance.Service{},
			LBTactic: typ.Tactic{
				Type:   loadbalance.TacticAdaptive,
				Params: typ.DefaultAdaptiveParams(),
			},
			Active: true,
		},
		{
			UUID:          RuleUUIDAgent,
			Scenario:      typ.ScenarioAgent,
			RequestModel:  "tingly-agent",
			ResponseModel: "",
			Description:   "Default proxy rule in tingly-box for agent",
			Services:      []*loadbalance.Service{},
			LBTactic: typ.Tactic{
				Type:   loadbalance.TacticAdaptive,
				Params: typ.DefaultAdaptiveParams(),
			},
			Active: true,
		},
		{
			UUID:          RuleUUIDAgentClaw,
			Scenario:      typ.ScenarioAgent,
			RequestModel:  "tingly-claw",
			ResponseModel: "",
			Description:   "Built in model rule for agent - claw",
			Services:      []*loadbalance.Service{},
			LBTactic: typ.Tactic{
				Type:   loadbalance.TacticAdaptive,
				Params: typ.DefaultAdaptiveParams(),
			},
			Active: true,
		},
		{
			UUID:          RuleUUIDOpenAI,
			Scenario:      typ.ScenarioOpenAI,
			RequestModel:  "tingly-gpt",
			ResponseModel: "",
			Description:   "Default proxy rule in tingly-box for general use with OpenAI",
			Services:      []*loadbalance.Service{},
			LBTactic: typ.Tactic{
				Type:   loadbalance.TacticAdaptive,
				Params: typ.DefaultAdaptiveParams(),
			},
			Active: true,
		},
		{
			UUID:          RuleUUIDCodex,
			Scenario:      typ.ScenarioCodex,
			RequestModel:  "tingly-codex",
			ResponseModel: "",
			Description:   "Default proxy rule for Codex",
			Services:      []*loadbalance.Service{},
			LBTactic: typ.Tactic{
				Type:   loadbalance.TacticAdaptive,
				Params: typ.DefaultAdaptiveParams(),
			},
			// 30-min session affinity improves cache hit rate for Codex sessions.
			Flags:  typ.RuleFlags{SessionAffinity: defaultSessionAffinitySeconds},
			Active: true,
		},
		ccRule(RuleUUIDCC, "tingly/cc", "Default proxy rule for Claude Code"),
		ccRule(RuleUUIDCCHaiku, "tingly/cc-haiku", "Claude Code - Haiku mode The model to use for haiku , or background functionality"),
		ccRule(RuleUUIDCCSonnet, "tingly/cc-sonnet", "Claude Code - Sonnet model - model to use for sonnet , or for opusplan when Plan Mode is not active."),
		ccRule(RuleUUIDCCOpus, "tingly/cc-opus", "Claude Code - Opus model - to use for opus , or for opusplan when Plan Mode is active."),
		ccRule(RuleUUIDCCDefault, "tingly/cc-default", "Claude Code - Default model - for general task"),
		ccRule(RuleUUIDCCSubagent, "tingly/cc-subagent", "Claude Code - Subagent model - model to use for subagents"),
		{
			UUID:          RuleUUIDOpenCode,
			Scenario:      typ.ScenarioOpenCode,
			RequestModel:  "tingly-opencode",
			ResponseModel: "",
			Description:   "Default proxy rule for OpenCode - AI coding assistant",
			Services:      []*loadbalance.Service{},
			LBTactic: typ.Tactic{
				Type:   loadbalance.TacticAdaptive,
				Params: typ.DefaultAdaptiveParams(),
			},
			Active: true,
		},
		cdRule("builtin:claude_desktop:claude-sonnet-4-6", "claude-sonnet-4-6", "Claude Desktop - Sonnet 4.6 model for balanced performance"),
		cdRule("builtin:claude_desktop:claude-opus-4-6", "claude-opus-4-6", "Claude Desktop - Opus 4.6 model for complex tasks"),
		cdRule("builtin:claude_desktop:claude-opus-4-7", "claude-opus-4-7", "Claude Desktop - Opus 4.7 model for advanced reasoning"),
		cdRule("builtin:claude_desktop:claude-haiku-4-5", "claude-haiku-4-5", "Claude Desktop - Haiku 4.5 model for fast responses"),
	}
}

// cdRule builds a built-in Claude Desktop rule with the shared defaults: an empty
// service list, the default adaptive load-balancing tactic, Active, and the
// clean_header + claude_code_compat + session_affinity flags on. Claude Desktop
// injects x-anthropic-billing-header blocks into system messages (CleanHeader)
// and sends mid-conversation system-role messages that third-party
// Anthropic-compatible providers reject (ClaudeCodeCompat); the 30-min
// SessionAffinity improves cache hit rate across a desktop session.
func cdRule(uuid, requestModel, description string) typ.Rule {
	return typ.Rule{
		UUID:         uuid,
		Scenario:     typ.ScenarioClaudeDesktop,
		RequestModel: requestModel,
		Description:  description,
		Services:     []*loadbalance.Service{},
		LBTactic: typ.Tactic{
			Type:   loadbalance.TacticAdaptive,
			Params: typ.DefaultAdaptiveParams(),
		},
		Flags:  typ.RuleFlags{ClaudeCodeCompat: true, CleanHeader: true, SessionAffinity: defaultSessionAffinitySeconds},
		Active: true,
	}
}

// ccRule builds a built-in Claude Code rule with the shared defaults: an empty
// service list, the default adaptive load-balancing tactic, Active, and the
// claude_code_compat + clean_header + session_affinity flags on. Claude Code
// emits mid-conversation system-role messages that third-party
// Anthropic-compatible providers reject (ClaudeCodeCompat), and injects
// x-anthropic-billing-header blocks into system messages that should never leak
// to external providers (CleanHeader); the 30-min SessionAffinity improves
// cache hit rate across a coding session.
func ccRule(uuid, requestModel, description string) typ.Rule {
	return typ.Rule{
		UUID:         uuid,
		Scenario:     typ.ScenarioClaudeCode,
		RequestModel: requestModel,
		Description:  description,
		Services:     []*loadbalance.Service{},
		LBTactic: typ.Tactic{
			Type:   loadbalance.TacticAdaptive,
			Params: typ.DefaultAdaptiveParams(),
		},
		Flags:  typ.RuleFlags{ClaudeCodeCompat: true, CleanHeader: true, SessionAffinity: defaultSessionAffinitySeconds},
		Active: true,
	}
}
