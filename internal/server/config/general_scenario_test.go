package config

import (
	"testing"

	"github.com/tingly-dev/tingly-box/internal/typ"
)

func TestDefaultRules_KeepBuiltinScenariosSpecialized(t *testing.T) {
	for _, rule := range DefaultRules {
		switch rule.UUID {
		case RuleUUIDTingly, RuleUUIDBuiltinOpenAI:
			if rule.Scenario != typ.ScenarioOpenAI {
				t.Fatalf("expected %s to stay openai, got %q", rule.UUID, rule.Scenario)
			}
		case RuleUUIDBuiltinAnthropic:
			if rule.Scenario != typ.ScenarioAnthropic {
				t.Fatalf("expected %s to stay anthropic, got %q", rule.UUID, rule.Scenario)
			}
		}
	}
}

func TestMatchRuleByModelAndScenario_FallsBackToGeneralForOpenAIAndAnthropic(t *testing.T) {
	cfg, err := NewConfigWithDir(t.TempDir())
	if err != nil {
		t.Fatalf("NewConfigWithDir() error = %v", err)
	}

	rule := typ.Rule{
		UUID:         "general-rule",
		Scenario:     typ.ScenarioGeneral,
		RequestModel: "shared-model",
		Active:       true,
	}
	if err := cfg.AddRule(rule); err != nil {
		t.Fatalf("AddRule() error = %v", err)
	}

	if got := cfg.MatchRuleByModelAndScenario("shared-model", typ.ScenarioOpenAI); got == nil || got.UUID != rule.UUID {
		t.Fatalf("expected general fallback for openai, got %#v", got)
	}
	if got := cfg.MatchRuleByModelAndScenario("shared-model", typ.ScenarioAnthropic); got == nil || got.UUID != rule.UUID {
		t.Fatalf("expected general fallback for anthropic, got %#v", got)
	}
}

func TestMatchRuleByModelAndScenario_PrefersExactScenarioOverGeneral(t *testing.T) {
	cfg, err := NewConfigWithDir(t.TempDir())
	if err != nil {
		t.Fatalf("NewConfigWithDir() error = %v", err)
	}

	generalRule := typ.Rule{
		UUID:         "general-rule",
		Scenario:     typ.ScenarioGeneral,
		RequestModel: "shared-model",
		Active:       true,
	}
	openAIRule := typ.Rule{
		UUID:         "openai-rule",
		Scenario:     typ.ScenarioOpenAI,
		RequestModel: "shared-model",
		Active:       true,
	}
	if err := cfg.AddRule(generalRule); err != nil {
		t.Fatalf("AddRule(general) error = %v", err)
	}
	if err := cfg.AddRule(openAIRule); err != nil {
		t.Fatalf("AddRule(openai) error = %v", err)
	}

	got := cfg.MatchRuleByModelAndScenario("shared-model", typ.ScenarioOpenAI)
	if got == nil || got.UUID != openAIRule.UUID {
		t.Fatalf("expected exact openai rule, got %#v", got)
	}
}

func TestMatchRuleByModelAndScenario_DoesNotFallbackForSpecializedScenario(t *testing.T) {
	cfg, err := NewConfigWithDir(t.TempDir())
	if err != nil {
		t.Fatalf("NewConfigWithDir() error = %v", err)
	}

	rule := typ.Rule{
		UUID:         "general-rule",
		Scenario:     typ.ScenarioGeneral,
		RequestModel: "shared-model",
		Active:       true,
	}
	if err := cfg.AddRule(rule); err != nil {
		t.Fatalf("AddRule() error = %v", err)
	}

	if got := cfg.MatchRuleByModelAndScenario("shared-model", typ.ScenarioClaudeCode); got != nil {
		t.Fatalf("expected no fallback for claude_code, got %#v", got)
	}
}
