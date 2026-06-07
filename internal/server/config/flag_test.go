package config

import (
	"testing"
	"time"

	"github.com/tingly-dev/tingly-box/internal/typ"
)

func TestScenarioFlags_SessionAffinity(t *testing.T) {
	tests := []struct {
		name     string
		flags    typ.ScenarioFlags
		expected int
	}{
		{
			name:     "zero value",
			flags:    typ.ScenarioFlags{},
			expected: 0,
		},
		{
			name: "session affinity set",
			flags: typ.ScenarioFlags{
				SessionAffinity: 1800,
			},
			expected: 1800,
		},
		{
			name: "session affinity with other flags",
			flags: typ.ScenarioFlags{
				Unified:          true,
				SessionAffinity:  7200,
				SkipUsage: true,
			},
			expected: 7200,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if tt.flags.SessionAffinity != tt.expected {
				t.Errorf("SessionAffinity = %v, want %v", tt.flags.SessionAffinity, tt.expected)
			}
		})
	}
}

func TestConfig_GetEffectiveAffinity(t *testing.T) {
	tests := []struct {
		name               string
		ruleFlags          typ.RuleFlags
		scenarioAffinity   int
		wantTTL            time.Duration
		useNoAffinityScenario bool
		explicitlySetScenario bool
	}{
		{
			name:               "rule explicit affinity takes precedence",
			ruleFlags:          typ.RuleFlags{SessionAffinity: 1800},
			scenarioAffinity:   1800,
			wantTTL:            1800 * time.Second,
			useNoAffinityScenario: false,
		},
		{
			name:               "scenario affinity when rule has none",
			ruleFlags:          typ.RuleFlags{SessionAffinity: 0},
			scenarioAffinity:   1800,
			wantTTL:            1800 * time.Second,
			useNoAffinityScenario: false,
		},
		{
			name:               "disabled when neither rule nor scenario has affinity",
			ruleFlags:          typ.RuleFlags{SessionAffinity: 0},
			scenarioAffinity:   0,
			wantTTL:            0,
			useNoAffinityScenario: true,
		},
		{
			name:               "scenario default affinity can be overridden",
			ruleFlags:          typ.RuleFlags{SessionAffinity: 900},
			scenarioAffinity:   1800,
			wantTTL:            900 * time.Second,
			useNoAffinityScenario: false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			cfg, err := NewConfig(WithConfigDir(t.TempDir()))
			if err != nil {
				t.Fatalf("NewConfig error: %v", err)
			}

			// Choose scenario based on test
			scenario := typ.ScenarioClaudeCode
			if tt.useNoAffinityScenario {
				scenario = typ.ScenarioOpenAI // This scenario has no default affinity
			}

			// Setup scenario config
			if tt.scenarioAffinity > 0 || tt.explicitlySetScenario {
				// Find and update existing scenario config, or add new one
				found := false
				for i := range cfg.Scenarios {
					if cfg.Scenarios[i].Scenario == scenario {
						cfg.Scenarios[i].Flags.SessionAffinity = tt.scenarioAffinity
						found = true
						break
					}
				}
				if !found {
					cfg.Scenarios = append(cfg.Scenarios, typ.ScenarioConfig{
						Scenario: scenario,
						Flags: typ.ScenarioFlags{
							SessionAffinity: tt.scenarioAffinity,
						},
					})
				}
			}

			// Create rule
			rule := &typ.Rule{
				UUID:     "test-rule-uuid",
				Scenario: scenario,
				Flags:    tt.ruleFlags,
			}

			got := cfg.GetEffectiveAffinity(rule)
			if got != tt.wantTTL {
				t.Errorf("GetEffectiveAffinity() = %v, want %v", got, tt.wantTTL)
			}
		})
	}
}

func TestScenarioDefaultAffinity(t *testing.T) {
	cfg, err := NewConfig(WithConfigDir(t.TempDir()))
	if err != nil {
		t.Fatalf("NewConfig error: %v", err)
	}

	// Test that default scenarios have affinity set
	expectedAffinityScenarios := []typ.RuleScenario{
		typ.ScenarioClaudeCode,
		typ.ScenarioClaudeDesktop,
		typ.ScenarioVSCode,
		typ.ScenarioAgent,
		typ.ScenarioCodex,
		typ.ScenarioOpenCode,
	}

	for _, scenario := range expectedAffinityScenarios {
		t.Run(string(scenario), func(t *testing.T) {
			scenarioConfig := cfg.GetScenarioConfig(scenario)
			if scenarioConfig == nil {
				t.Errorf("Scenario config not found for %s", scenario)
				return
			}
			if scenarioConfig.Flags.SessionAffinity != 1800 {
				t.Errorf("Scenario %s: expected SessionAffinity = 1800, got %d",
					scenario, scenarioConfig.Flags.SessionAffinity)
			}
		})
	}

	// Test that API scenarios don't have affinity by default
	noAffinityScenarios := []typ.RuleScenario{
		typ.ScenarioOpenAI,
		typ.ScenarioAnthropic,
		typ.ScenarioEmbed,
	}

	for _, scenario := range noAffinityScenarios {
		t.Run(string(scenario)+"_no_affinity", func(t *testing.T) {
			scenarioConfig := cfg.GetScenarioConfig(scenario)
			// Scenario might not exist, that's OK
			if scenarioConfig != nil && scenarioConfig.Flags.SessionAffinity != 0 {
				t.Errorf("Scenario %s: expected SessionAffinity = 0, got %d",
					scenario, scenarioConfig.Flags.SessionAffinity)
			}
		})
	}
}

func TestRuleWithInheritedAffinity(t *testing.T) {
	cfg, err := NewConfig(WithConfigDir(t.TempDir()))
	if err != nil {
		t.Fatalf("NewConfig error: %v", err)
	}

	// Setup scenario with affinity
	scenario := typ.ScenarioClaudeCode
	cfg.Scenarios = append(cfg.Scenarios, typ.ScenarioConfig{
		Scenario: scenario,
		Flags: typ.ScenarioFlags{
			SessionAffinity: 1800,
		},
	})

	// Create rule without explicit affinity
	rule := &typ.Rule{
		UUID:     "test-rule-uuid",
		Scenario: scenario,
		Flags:    typ.RuleFlags{SessionAffinity: 0},
	}

	// Get effective affinity
	got := cfg.GetEffectiveAffinity(rule)
	expected := 1800 * time.Second

	if got != expected {
		t.Errorf("GetEffectiveAffinity() = %v, want %v", got, expected)
	}

	// Now set rule-level affinity
	rule.Flags.SessionAffinity = 1800
	got = cfg.GetEffectiveAffinity(rule)
	expected = 1800 * time.Second

	if got != expected {
		t.Errorf("GetEffectiveAffinity() with rule override = %v, want %v", got, expected)
	}
}
