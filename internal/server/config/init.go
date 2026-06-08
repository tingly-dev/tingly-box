package config

import (
	"github.com/tingly-dev/tingly-box/internal/loadbalance"
	"github.com/tingly-dev/tingly-box/internal/typ"
)

var DefaultRules []typ.Rule

func init() {
	DefaultRules = []typ.Rule{
		{
			UUID:          "built-in-anthropic",
			Scenario:      typ.ScenarioAnthropic,
			RequestModel:  "tingly-claude",
			ResponseModel: "",
			Description:   "Default proxy rule in tingly-box for general use with Anthropic",
			Services:      []*loadbalance.Service{}, // Empty services initially
			LBTactic: typ.Tactic{ // Initialize with default adaptive tactic
				Type:   loadbalance.TacticAdaptive,
				Params: typ.DefaultAdaptiveParams(),
			},
			Active: true,
		},
		{
			UUID:          "built-in-agent",
			Scenario:      typ.ScenarioAgent,
			RequestModel:  "tingly-agent",
			ResponseModel: "",
			Description:   "Default proxy rule in tingly-box for agent",
			Services:      []*loadbalance.Service{}, // Empty services initially
			LBTactic: typ.Tactic{ // Initialize with default adaptive tactic
				Type:   loadbalance.TacticAdaptive,
				Params: typ.DefaultAdaptiveParams(),
			},
			Active: true,
		},
		{
			UUID:          "built-in-agent-claw",
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
			UUID:          "built-in-openai",
			Scenario:      typ.ScenarioOpenAI,
			RequestModel:  "tingly-gpt",
			ResponseModel: "",
			Description:   "Default proxy rule in tingly-box for general use with OpenAI",
			Services:      []*loadbalance.Service{}, // Empty services initially
			LBTactic: typ.Tactic{ // Initialize with default adaptive tactic
				Type:   loadbalance.TacticAdaptive,
				Params: typ.DefaultAdaptiveParams(),
			},
			Active: true,
		},
		{
			UUID:          "built-in-codex",
			Scenario:      typ.ScenarioCodex,
			RequestModel:  "tingly-codex",
			ResponseModel: "",
			Description:   "Default proxy rule for Codex",
			Services:      []*loadbalance.Service{},
			LBTactic: typ.Tactic{
				Type:   loadbalance.TacticAdaptive,
				Params: typ.DefaultAdaptiveParams(),
			},
			Active: true,
		},
		ccRule("built-in-cc", "tingly/cc", "Default proxy rule for Claude Code"),
		ccRule("built-in-cc-haiku", "tingly/cc-haiku", "Claude Code - Haiku mode The model to use for haiku , or background functionality"),
		ccRule("built-in-cc-sonnet", "tingly/cc-sonnet", "Claude Code - Sonnet model - model to use for sonnet , or for opusplan when Plan Mode is not active."),
		ccRule("built-in-cc-opus", "tingly/cc-opus", "Claude Code - Opus model - to use for opus , or for opusplan when Plan Mode is active."),
		ccRule("built-in-cc-default", "tingly/cc-default", "Claude Code - Default model - for general task"),
		ccRule("built-in-cc-subagent", "tingly/cc-subagent", "Claude Code - Subagent model - model to use for subagents"),
		{
			UUID:          "built-in-opencode",
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
// clean_header + claude_code_compat flags on. Claude Desktop injects
// x-anthropic-billing-header blocks into system messages (CleanHeader) and sends
// mid-conversation system-role messages that third-party Anthropic-compatible
// providers reject (ClaudeCodeCompat).
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
		Flags:  typ.RuleFlags{ClaudeCodeCompat: true, CleanHeader: true},
		Active: true,
	}
}

// ccRule builds a built-in Claude Code rule with the shared defaults: an empty
// service list, the default adaptive load-balancing tactic, Active, and the
// claude_code_compat + clean_header flags on. Claude Code emits mid-conversation
// system-role messages that third-party Anthropic-compatible providers reject
// (ClaudeCodeCompat), and injects x-anthropic-billing-header blocks into system
// messages that should never leak to external providers (CleanHeader).
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
		Flags:  typ.RuleFlags{ClaudeCodeCompat: true, CleanHeader: true},
		Active: true,
	}
}
