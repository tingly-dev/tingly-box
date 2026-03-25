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
		{
			UUID:          "built-in-cc",
			Scenario:      typ.ScenarioClaudeCode,
			RequestModel:  "tingly/cc",
			ResponseModel: "",
			Description:   "Default proxy rule for Claude Code",
			Services:      []*loadbalance.Service{}, // Empty services initially
			LBTactic: typ.Tactic{ // Initialize with default adaptive tactic
				Type:   loadbalance.TacticAdaptive,
				Params: typ.DefaultAdaptiveParams(),
			},
			Active: true,
		},
		{
			UUID:          "built-in-cc-haiku",
			Scenario:      typ.ScenarioClaudeCode,
			RequestModel:  "tingly/cc-haiku",
			ResponseModel: "",
			Description:   "Claude Code - Haiku mode The model to use for haiku , or background functionality",
			Services:      []*loadbalance.Service{},
			LBTactic: typ.Tactic{
				Type:   loadbalance.TacticAdaptive,
				Params: typ.DefaultAdaptiveParams(),
			},
			Active: true,
		},
		{
			UUID:          "built-in-cc-sonnet",
			Scenario:      typ.ScenarioClaudeCode,
			RequestModel:  "tingly/cc-sonnet",
			ResponseModel: "",
			Description:   "Claude Code - Sonnet model - model to use for sonnet , or for opusplan when Plan Mode is not active.",
			Services:      []*loadbalance.Service{},
			LBTactic: typ.Tactic{
				Type:   loadbalance.TacticAdaptive,
				Params: typ.DefaultAdaptiveParams(),
			},
			Active: true,
		},
		{
			UUID:          "built-in-cc-opus",
			Scenario:      typ.ScenarioClaudeCode,
			RequestModel:  "tingly/cc-opus",
			ResponseModel: "",
			Description:   "Claude Code - Opus model - to use for opus , or for opusplan when Plan Mode is active.",
			Services:      []*loadbalance.Service{},
			LBTactic: typ.Tactic{
				Type:   loadbalance.TacticAdaptive,
				Params: typ.DefaultAdaptiveParams(),
			},
			Active: true,
		},
		{
			UUID:          "built-in-cc-default",
			Scenario:      typ.ScenarioClaudeCode,
			RequestModel:  "tingly/cc-default",
			ResponseModel: "",
			Description:   "Claude Code - Default model - for general task",
			Services:      []*loadbalance.Service{},
			LBTactic: typ.Tactic{
				Type:   loadbalance.TacticAdaptive,
				Params: typ.DefaultAdaptiveParams(),
			},
			Active: true,
		},
		{
			UUID:          "built-in-cc-subagent",
			Scenario:      typ.ScenarioClaudeCode,
			RequestModel:  "tingly/cc-subagent",
			ResponseModel: "",
			Description:   "Claude Code - Subagent model - model to use for subagents",
			Services:      []*loadbalance.Service{},
			LBTactic: typ.Tactic{
				Type:   loadbalance.TacticAdaptive,
				Params: typ.DefaultAdaptiveParams(),
			},
			Active: true,
		},
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
	}
}
