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

// TestProfileScenarioFlagWriteInheritsBaseConfig verifies that writing a flag
// to a profiled scenario (e.g. "claude_code:p1") for the first time seeds the
// new profile-scoped ScenarioConfig from the resolved base scenario config,
// rather than a blank one. Before the fix, SetScenarioFlag/SetScenarioStringFlag/
// SetScenarioIntFlag/SetScenarioExtensions did an exact-match-only lookup and
// created an empty entry on miss, silently orphaning the profile from every
// flag it previously inherited from the base scenario via scenarioConfigLocked's
// fallback (used by the Get* counterparts). See .claude/plans -
// "Fix: Claude Code profile page top flag panel doesn't read/write flags
// correctly".
func TestProfileScenarioFlagWriteInheritsBaseConfig(t *testing.T) {
	cfg, err := NewConfig(WithConfigDir(t.TempDir()))
	if err != nil {
		t.Fatalf("NewConfig error: %v", err)
	}

	base := typ.ScenarioClaudeCode
	profile := typ.RuleScenario("claude_code:p1")

	// Base scenario is switched to separate mode.
	if err := cfg.SetScenarioFlag(base, FlagSeparate, true); err != nil {
		t.Fatalf("SetScenarioFlag(base, separate) error: %v", err)
	}

	// Sanity: before any profile-local write, the profile reads through to
	// base via scenarioConfigLocked's fallback.
	if v := cfg.GetScenarioFlag(profile, FlagSeparate); !v {
		t.Fatalf("expected profile to inherit base separate=true via fallback before any profile write, got false")
	}

	// Writing an unrelated flag on the profile (e.g. toggling Smart Compact
	// from the profile page's plugin panel) must not orphan the profile from
	// the base scenario's already-set flags.
	if err := cfg.SetScenarioFlag(profile, FlagSmartCompact, true); err != nil {
		t.Fatalf("SetScenarioFlag(profile, smart_compact) error: %v", err)
	}

	if v := cfg.GetScenarioFlag(profile, FlagSeparate); !v {
		t.Errorf("profile lost inherited separate=true after an unrelated profile-local flag write")
	}
	if v := cfg.GetScenarioFlag(profile, FlagSmartCompact); !v {
		t.Errorf("profile-local smart_compact write did not persist")
	}
	// Base scenario itself must be unaffected by the profile write.
	if v := cfg.GetScenarioFlag(base, FlagSmartCompact); v {
		t.Errorf("smart_compact leaked into the base scenario config")
	}

	// Same guarantee for the string-flag setter (e.g. thinking_effort set on
	// base, then a different flag toggled on the profile).
	profile2 := typ.RuleScenario("claude_code:p2")
	if err := cfg.SetScenarioStringFlag(base, FlagThinkingEffort, "high"); err != nil {
		t.Fatalf("SetScenarioStringFlag(base, thinking_effort) error: %v", err)
	}
	if err := cfg.SetScenarioStringFlag(profile2, FlagRecordingV2, string(typ.RecordingModeRequestOnly)); err != nil {
		t.Fatalf("SetScenarioStringFlag(profile2, recording_v2) error: %v", err)
	}
	if got := cfg.GetScenarioStringFlag(profile2, FlagThinkingEffort); got != "high" {
		t.Errorf("profile2 lost inherited thinking_effort=high after an unrelated profile-local string flag write, got %q", got)
	}

	// Same guarantee for SetScenarioExtensions.
	profile3 := typ.RuleScenario("claude_code:p3")
	if err := cfg.SetScenarioFlag(base, FlagSkipUsage, true); err != nil {
		t.Fatalf("SetScenarioFlag(base, skip_usage) error: %v", err)
	}
	if err := cfg.SetScenarioExtensions(profile3, map[string]interface{}{"some_extension": "x"}); err != nil {
		t.Fatalf("SetScenarioExtensions(profile3) error: %v", err)
	}
	if v := cfg.GetScenarioFlag(profile3, FlagSkipUsage); !v {
		t.Errorf("profile3 lost inherited skip_usage=true after an unrelated SetScenarioExtensions write")
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
