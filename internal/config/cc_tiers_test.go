package config

import (
	"testing"

	"github.com/tingly-dev/tingly-box/internal/typ"
)

// CCTiers is the one place a tier is declared; everything else (seeds, profile
// rules, UUID sets, legacy aliases) is derived. These checks catch an entry
// that would silently fall out of one of those derivations.
func TestCCTiers_DerivedViewsAreConsistent(t *testing.T) {
	names, uuids, envKeys := map[string]bool{}, map[string]bool{}, map[string]bool{}
	for _, tier := range CCTiers {
		if tier.Name == "" || tier.RuleUUID == "" || tier.LegacyModel == "" || tier.Description == "" || tier.ProfileDescription == "" {
			t.Errorf("tier %+v has an empty required field", tier)
		}
		if names[tier.Name] || uuids[tier.RuleUUID] || (tier.EnvKey != "" && envKeys[tier.EnvKey]) {
			t.Errorf("tier %q duplicates a name, UUID or env key", tier.Name)
		}
		names[tier.Name], uuids[tier.RuleUUID], envKeys[tier.EnvKey] = true, true, true

		// seeded main-scenario rule: only the unified rule starts active
		seeded, ok := defaultRuleByUUID(tier.RuleUUID)
		if !ok || seeded.RequestModel != tier.Name || seeded.Scenario != typ.ScenarioClaudeCode {
			t.Errorf("tier %q is not seeded as a claude_code rule: %+v", tier.Name, seeded)
		}
		if seeded.Active != (tier.Name == CCTierUnified) {
			t.Errorf("tier %q seeded Active=%v", tier.Name, seeded.Active)
		}
		// UUID sets and legacy tables
		set := claudeCodeSeparateRuleUUIDs
		if tier.Name == CCTierUnified {
			set = claudeCodeUnifiedRuleUUIDs
		}
		if !set[tier.RuleUUID] || (tier.LegacyUUID != "" && !set[tier.LegacyUUID]) {
			t.Errorf("tier %q missing from its rule UUID set", tier.Name)
		}
		if tier.LegacyUUID != "" && legacyCCRuleUUIDs[tier.LegacyUUID] != tier.RuleUUID {
			t.Errorf("tier %q legacy UUID not mapped", tier.Name)
		}
		if canonicalCCRequestModel(tier.LegacyModel) != tier.Name || canonicalCCRequestModel(tier.Name+"[1m]") != tier.Name {
			t.Errorf("tier %q aliases do not canonicalize to its name", tier.Name)
		}
		if !ccProfileTiers[tier.Name] {
			t.Errorf("tier %q is not a recognized profile tier", tier.Name)
		}
	}

	// profile rules: the unified set or the separate-mode set, never mixed
	if got := len(newCCProfileRules("claude_code:p1", true)); got != 1 {
		t.Errorf("unified profile rules = %d, want 1", got)
	}
	if got, want := len(newCCProfileRules("claude_code:p1", false)), len(CCSlotTiers()); got != want {
		t.Errorf("separate profile rules = %d, want %d", got, want)
	}
}
