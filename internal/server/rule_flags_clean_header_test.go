package server

import (
	"testing"

	"github.com/tingly-dev/tingly-box/internal/protocol"
	"github.com/tingly-dev/tingly-box/internal/typ"
)

// TestIsBillingHeaderScenario verifies the detection of billing header scenarios
func TestIsBillingHeaderScenario(t *testing.T) {
	tests := []struct {
		name     string
		scenario typ.RuleScenario
		want     bool
	}{
		{
			name:     "Claude Code scenario",
			scenario: typ.ScenarioClaudeCode,
			want:     true,
		},
		{
			name:     "Claude Desktop scenario",
			scenario: typ.ScenarioClaudeDesktop,
			want:     true,
		},
		{
			name:     "OpenAI scenario",
			scenario: typ.ScenarioOpenAI,
			want:     false,
		},
		{
			name:     "Anthropic scenario",
			scenario: typ.ScenarioAnthropic,
			want:     false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := isBillingHeaderScenario(tt.scenario)
			if got != tt.want {
				t.Errorf("isBillingHeaderScenario() = %v, want %v", got, tt.want)
			}
		})
	}
}

// TestAutoSetCleanHeaderFlag verifies the flag auto-setting logic
func TestAutoSetCleanHeaderFlag(t *testing.T) {
	tests := []struct {
		name            string
		flags           typ.RuleFlags
		sourceAPI      protocol.APIType
		targetAPI      protocol.APIType
		scenario       typ.RuleScenario
		wantCleanHeader bool
	}{
		{
			name:            "Auto-set for Claude Code transformation",
			flags:           typ.RuleFlags{CleanHeader: false},
			sourceAPI:       protocol.TypeAnthropicV1,
			targetAPI:       protocol.TypeOpenAIChat,
			scenario:        typ.ScenarioClaudeCode,
			wantCleanHeader: true,
		},
		{
			name:            "Manual CleanHeader=true preserved",
			flags:           typ.RuleFlags{CleanHeader: true},
			sourceAPI:       protocol.TypeAnthropicV1,
			targetAPI:       protocol.TypeAnthropicV1,
			scenario:        typ.ScenarioClaudeCode,
			wantCleanHeader: true, // Manual flag preserved
		},
		{
			name:            "No transformation, not set",
			flags:           typ.RuleFlags{CleanHeader: false},
			sourceAPI:       protocol.TypeAnthropicV1,
			targetAPI:       protocol.TypeAnthropicV1,
			scenario:        typ.ScenarioClaudeCode,
			wantCleanHeader: false,
		},
		{
			name:            "Non-billing scenario, not set",
			flags:           typ.RuleFlags{CleanHeader: false},
			sourceAPI:       protocol.TypeAnthropicV1,
			targetAPI:       protocol.TypeOpenAIChat,
			scenario:        typ.ScenarioOpenAI,
			wantCleanHeader: false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := autoSetCleanHeaderFlag(tt.flags, tt.sourceAPI, tt.targetAPI, tt.scenario)
			if result.CleanHeader != tt.wantCleanHeader {
				t.Errorf("autoSetCleanHeaderFlag() CleanHeader = %v, want %v", result.CleanHeader, tt.wantCleanHeader)
			}
		})
	}
}

// TestRulePreBaseTransformsWithCleanHeader verifies the transform building
func TestRulePreBaseTransformsWithCleanHeader(t *testing.T) {
	tests := []struct {
		name            string
		flags           typ.RuleFlags
		wantCleanCount int
	}{
		{
			name:            "CleanHeader flag adds transform",
			flags:           typ.RuleFlags{CleanHeader: true},
			wantCleanCount: 1,
		},
		{
			name:            "No CleanHeader flag, no transform",
			flags:           typ.RuleFlags{CleanHeader: false},
			wantCleanCount: 0,
		},
		{
			name:            "CursorCompat + CleanHeader both added",
			flags:           typ.RuleFlags{CursorCompat: true, CleanHeader: true},
			wantCleanCount: 1, // CleanHeader count (CursorCompat also added)
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			transforms := rulePreBaseTransforms(tt.flags)

			cleanCount := 0
			for _, transform := range transforms {
				if transform.Name() == "clean_header" {
					cleanCount++
				}
			}

			if cleanCount != tt.wantCleanCount {
				t.Errorf("CleanHeader count = %v, want %v", cleanCount, tt.wantCleanCount)
			}
		})
	}
}
