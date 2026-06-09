package config

import (
	"testing"
	"time"

	"github.com/tingly-dev/tingly-box/internal/typ"
)

// TestConfig_GetEffectiveAffinity verifies session_affinity is rule-only:
// the rule's own value is used directly, and there is no scenario-level
// inheritance.
func TestConfig_GetEffectiveAffinity(t *testing.T) {
	tests := []struct {
		name      string
		ruleFlags typ.RuleFlags
		wantTTL   time.Duration
	}{
		{
			name:      "rule explicit affinity is used",
			ruleFlags: typ.RuleFlags{SessionAffinity: 1800},
			wantTTL:   1800 * time.Second,
		},
		{
			name:      "rule custom affinity is used",
			ruleFlags: typ.RuleFlags{SessionAffinity: 900},
			wantTTL:   900 * time.Second,
		},
		{
			name:      "zero rule affinity is disabled",
			ruleFlags: typ.RuleFlags{SessionAffinity: 0},
			wantTTL:   0,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			cfg, err := NewConfig(WithConfigDir(t.TempDir()))
			if err != nil {
				t.Fatalf("NewConfig error: %v", err)
			}

			rule := &typ.Rule{
				UUID:     "test-rule-uuid",
				Scenario: typ.ScenarioClaudeCode,
				Flags:    tt.ruleFlags,
			}

			got := cfg.GetEffectiveAffinity(rule)
			if got != tt.wantTTL {
				t.Errorf("GetEffectiveAffinity() = %v, want %v", got, tt.wantTTL)
			}
		})
	}
}

// TestScenarioAffinityDoesNotLeakIntoRules verifies that session_affinity is no
// longer inherited from any scenario-level config: a rule with affinity 0 stays
// disabled regardless of its scenario.
func TestScenarioAffinityDoesNotLeakIntoRules(t *testing.T) {
	cfg, err := NewConfig(WithConfigDir(t.TempDir()))
	if err != nil {
		t.Fatalf("NewConfig error: %v", err)
	}

	for _, scenario := range []typ.RuleScenario{
		typ.ScenarioClaudeCode,
		typ.ScenarioClaudeDesktop,
		typ.ScenarioCodex,
		typ.ScenarioVSCode,
		typ.ScenarioOpenAI,
	} {
		rule := &typ.Rule{UUID: "r", Scenario: scenario, Flags: typ.RuleFlags{SessionAffinity: 0}}
		if got := cfg.GetEffectiveAffinity(rule); got != 0 {
			t.Errorf("scenario %s: rule with affinity 0 should be disabled, got %v", scenario, got)
		}
	}
}

// TestBuiltInRulesSessionAffinity verifies the built-in Claude Code / Desktop /
// Codex rules seed session_affinity to the 30-min default, while other built-in
// rules leave it disabled.
func TestBuiltInRulesSessionAffinity(t *testing.T) {
	wantOn := map[typ.RuleScenario]bool{
		typ.ScenarioClaudeCode:    true,
		typ.ScenarioClaudeDesktop: true,
		typ.ScenarioCodex:         true,
	}

	sawScenario := map[typ.RuleScenario]bool{}
	for _, rule := range DefaultRules {
		sawScenario[rule.Scenario] = true
		if wantOn[rule.Scenario] {
			if rule.Flags.SessionAffinity != defaultSessionAffinitySeconds {
				t.Errorf("built-in rule %q (%s): SessionAffinity = %d, want %d",
					rule.UUID, rule.Scenario, rule.Flags.SessionAffinity, defaultSessionAffinitySeconds)
			}
		} else if rule.Flags.SessionAffinity != 0 {
			t.Errorf("built-in rule %q (%s): SessionAffinity = %d, want 0",
				rule.UUID, rule.Scenario, rule.Flags.SessionAffinity)
		}
	}

	for scenario := range wantOn {
		if !sawScenario[scenario] {
			t.Errorf("expected at least one built-in rule for scenario %s", scenario)
		}
	}
}
