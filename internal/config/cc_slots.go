package config

import (
	"fmt"
	"slices"

	"github.com/tingly-dev/tingly-box/internal/typ"
)

// CCSlots are the Claude Code model slots that can get a rule of their own in
// unified mode. The default slot (ANTHROPIC_MODEL) is the main rule itself.
// See .design/claude-code-slots.md.
var CCSlots = []string{"haiku", "sonnet", "opus", "fable", "subagent"}

// ClaudeCodeSlots returns the slots that have a rule of their own in the given
// unified-mode Claude Code scenario (main or profile), in CCSlots order.
func (c *Config) ClaudeCodeSlots(scenario typ.RuleScenario) []string {
	c.mu.RLock()
	defer c.mu.RUnlock()
	return slices.Clone(c.ccSlotsLocked(scenario))
}

// ccSlotsLocked reads the slot list of exactly this scenario: the main
// scenario's record, or the profile itself — never one through the other.
func (c *Config) ccSlotsLocked(scenario typ.RuleScenario) []string {
	if p := c.ccProfileLocked(scenario); p != nil {
		return p.ClaudeCodeSlots
	}
	if scenario != typ.ScenarioClaudeCode {
		return nil
	}
	for i := range c.Scenarios {
		if c.Scenarios[i].Scenario == scenario {
			return c.Scenarios[i].ClaudeCodeSlots
		}
	}
	return nil
}

// ccProfileLocked returns the Claude Code profile a profiled scenario
// names, nil for the main scenario or an unknown profile.
func (c *Config) ccProfileLocked(scenario typ.RuleScenario) *typ.ProfileMeta {
	base, profileID := typ.ParseScenarioProfile(scenario)
	if base != typ.ScenarioClaudeCode || profileID == "" {
		return nil
	}
	profiles := c.Profiles[string(base)]
	for i := range profiles {
		if profiles[i].ID == profileID {
			return &profiles[i]
		}
	}
	return nil
}

// SetClaudeCodeSlot gives a slot of a unified-mode Claude Code scenario a rule
// of its own (enabled) or hands it back to the main rule. Enabling switches
// the slot's built-in rule on, creating it — or filling a seeded rule that
// has no routing yet — from the main rule, so the slot keeps routing as before
// until the user changes it; disabling switches the rule off and keeps its
// configuration for next time. Returns the slot's rule.
func (c *Config) SetClaudeCodeSlot(scenario typ.RuleScenario, slot string, enabled bool) (typ.Rule, error) {
	if !slices.Contains(CCSlots, slot) {
		return typ.Rule{}, fmt.Errorf("unknown Claude Code slot %q (one of %v)", slot, CCSlots)
	}
	c.mu.Lock()
	defer c.mu.Unlock()

	if err := c.checkCCSlotScenarioLocked(scenario); err != nil {
		return typ.Rule{}, err
	}
	main := c.ccRuleLocked(scenario, "cc")
	if main == nil {
		return typ.Rule{}, fmt.Errorf("scenario %q has no main Claude Code rule", scenario)
	}

	rule := c.ccRuleLocked(scenario, slot)
	if enabled {
		if rule == nil {
			rule = c.addCCRuleLocked(scenario, slot, main)
		} else if len(rule.Services) == 0 && !rule.SmartEnabled {
			copyCCRouting(rule, main)
		}
		rule.Active = true
	} else if rule != nil {
		rule.Active = false
	}

	slots := slices.DeleteFunc(slices.Clone(c.ccSlotsLocked(scenario)), func(s string) bool { return s == slot })
	if enabled {
		slots = append(slots, slot)
	}
	slices.SortFunc(slots, func(a, b string) int { return slices.Index(CCSlots, a) - slices.Index(CCSlots, b) })
	if p := c.ccProfileLocked(scenario); p != nil {
		p.ClaudeCodeSlots = slots
	} else {
		c.findOrCreateScenarioConfigLocked(scenario).ClaudeCodeSlots = slots
	}

	var out typ.Rule
	if rule != nil {
		out = *rule
	}
	return out, c.Save()
}

// checkCCSlotScenarioLocked accepts the main Claude Code scenario or an
// existing profile of it, in unified mode: separate mode already gives every
// slot its own rule.
func (c *Config) checkCCSlotScenarioLocked(scenario typ.RuleScenario) error {
	base, profileID := typ.ParseScenarioProfile(scenario)
	if base != typ.ScenarioClaudeCode {
		return fmt.Errorf("scenario %q has no Claude Code slots", scenario)
	}
	if profileID == "" {
		if sc := c.scenarioConfigLocked(scenario); sc != nil && sc.GetDefaultFlags().Separate {
			return fmt.Errorf("claude_code is in separate mode: every slot already has its own rule")
		}
		return nil
	}
	p := c.ccProfileLocked(scenario)
	if p == nil {
		return fmt.Errorf("profile %q not found", profileID)
	}
	if !p.Unified {
		return fmt.Errorf("profile %q is in separate mode: every slot already has its own rule", profileID)
	}
	return nil
}

// ClaudeCodeRuleUUID returns the UUID of a Claude Code scenario's built-in rule
// for slot ("cc" for the main rule; legacy UUIDs included), or "" if missing.
func (c *Config) ClaudeCodeRuleUUID(scenario typ.RuleScenario, slot string) string {
	c.mu.RLock()
	defer c.mu.RUnlock()
	if r := c.ccRuleLocked(scenario, slot); r != nil {
		return r.UUID
	}
	return ""
}

// ccRuleLocked returns the scenario's rule for slot (or "cc"), falling back to
// the pre-migration UUID in the main scenario.
func (c *Config) ccRuleLocked(scenario typ.RuleScenario, slot string) *typ.Rule {
	if r := c.findRuleByUUID(BuiltinRuleUUID(scenario, slot)); r != nil {
		return r
	}
	if scenario == typ.ScenarioClaudeCode {
		if legacy := LegacyCCRuleUUID(slot); legacy != "" {
			return c.findRuleByUUID(legacy)
		}
	}
	return nil
}

// addCCRuleLocked appends the built-in rule for slot with seed's routing and
// returns a pointer to it (valid until the next append to c.Rules).
func (c *Config) addCCRuleLocked(scenario typ.RuleScenario, slot string, seed *typ.Rule) *typ.Rule {
	var rule typ.Rule
	if tmpl, ok := defaultRuleByUUID(BuiltinRuleUUID(scenario, slot)); ok && scenario == typ.ScenarioClaudeCode {
		rule = tmpl
	} else {
		rule = newCCProfileRule(scenario, slot, "Claude Code profile - "+slot+" model")
	}
	rule.Scenario = scenario
	copyCCRouting(&rule, seed)
	c.Rules = append(c.Rules, rule)
	return &c.Rules[len(c.Rules)-1]
}

// copyCCRouting copies what decides where a rule's requests go.
func copyCCRouting(dst, src *typ.Rule) {
	dst.Services = cloneServices(src.Services)
	dst.Flags = src.Flags
	dst.LBTactic = src.LBTactic
	dst.SmartEnabled = src.SmartEnabled
	dst.SmartRouting = slices.Clone(src.SmartRouting)
}

// LegacyCCRuleUUID returns the pre-migration built-in-cc-* UUID of a main
// Claude Code rule ("cc" or a slot name), or "" when it never had one.
func LegacyCCRuleUUID(slot string) string {
	modern := BuiltinRuleUUID(typ.ScenarioClaudeCode, slot)
	for legacy, m := range legacyCCRuleUUIDs {
		if m == modern {
			return legacy
		}
	}
	return ""
}
