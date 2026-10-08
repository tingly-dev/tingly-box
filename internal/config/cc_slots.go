package config

import (
	"fmt"
	"slices"

	"github.com/tingly-dev/tingly-box/internal/typ"
)

// CCSlots are Claude Code's model slots. A slot whose rule
// (builtin:<scenario>:<slot>) is active requests that rule; any other slot
// falls back to the main "cc" rule. All on is what used to be separate mode,
// none is unified. See .design/claude-code-slot-binding.md.
var CCSlots = []string{"default", "haiku", "sonnet", "opus", "fable", "subagent"}

// SetClaudeCodeSlot gives a slot its own rule (enabled) or hands it back to
// the main rule. Enabling switches the slot's rule on, creating it from the
// main rule's routing when it doesn't exist yet; disabling only switches it
// off, so turning the slot on again restores what it had.
func (c *Config) SetClaudeCodeSlot(scenario typ.RuleScenario, slot string, enabled bool) (typ.Rule, error) {
	if !scenario.Is(typ.ScenarioClaudeCode) {
		return typ.Rule{}, fmt.Errorf("scenario %q has no Claude Code slots", scenario)
	}
	if !slices.Contains(CCSlots, slot) {
		return typ.Rule{}, fmt.Errorf("unknown Claude Code slot %q", slot)
	}
	c.mu.Lock()
	defer c.mu.Unlock()

	main := c.ccRuleLocked(scenario, "cc")
	rule := c.ccRuleLocked(scenario, slot)
	if enabled && rule == nil {
		rule = c.addCCRuleLocked(scenario, slot, main)
		main = c.ccRuleLocked(scenario, "cc") // the append may have moved it
	}
	if rule == nil {
		return typ.Rule{}, nil // off and absent: already following the main rule
	}
	rule.Active = enabled
	out := *rule
	if !enabled {
		// The slot now relies on the main rule. A profile created with
		// separate rules has none yet: seed it from the rule being switched off.
		if main == nil {
			main = c.addCCRuleLocked(scenario, "cc", c.ccRuleLocked(scenario, slot))
		}
		main.Active = true
	}
	c.syncClaudeCodeModeFlagsLocked(scenario)
	return out, c.Save()
}

// ccRuleLocked returns the scenario's rule for slot (or "cc"), falling back to
// the pre-migration UUID in the main scenario.
func (c *Config) ccRuleLocked(scenario typ.RuleScenario, slot string) *typ.Rule {
	if r := c.findRuleByUUID(BuiltinRuleUUID(scenario, slot)); r != nil {
		return r
	}
	if scenario == typ.ScenarioClaudeCode {
		return c.findRuleByUUID(LegacyCCRuleUUID(slot))
	}
	return nil
}

// addCCRuleLocked appends the built-in rule for slot (or "cc"), copying the
// routing (services, flags, tactic, smart routing) of seed when given.
func (c *Config) addCCRuleLocked(scenario typ.RuleScenario, slot string, seed *typ.Rule) *typ.Rule {
	var rule typ.Rule
	if tmpl, ok := defaultRuleByUUID(BuiltinRuleUUID(scenario, slot)); ok && scenario == typ.ScenarioClaudeCode {
		rule = tmpl
	} else {
		model := slot
		if scenario == typ.ScenarioClaudeCode {
			model = "tingly/cc-" + slot
		}
		rule = newCCProfileRule(scenario, model, "Claude Code - "+slot+" model")
		rule.UUID = BuiltinRuleUUID(scenario, slot)
	}
	rule.Scenario = scenario
	if seed != nil {
		rule.Services = cloneServices(seed.Services)
		rule.Flags = seed.Flags
		rule.LBTactic = seed.LBTactic
		rule.SmartEnabled = seed.SmartEnabled
		rule.SmartRouting = slices.Clone(seed.SmartRouting)
	}
	c.Rules = append(c.Rules, rule)
	return &c.Rules[len(c.Rules)-1]
}

// syncClaudeCodeModeFlagsLocked keeps the legacy unified/separate flags (and
// a profile's Unified) as a reading of the slots, for code that still asks:
// separate means every slot has its own rule.
func (c *Config) syncClaudeCodeModeFlagsLocked(scenario typ.RuleScenario) {
	separate := true
	for _, slot := range CCSlots {
		if r := c.ccRuleLocked(scenario, slot); r == nil || !r.Active {
			separate = false
			break
		}
	}
	base, profileID := typ.ParseScenarioProfile(scenario)
	if profileID == "" {
		sc := c.findOrCreateScenarioConfigLocked(scenario)
		sc.Flags.Unified, sc.Flags.Separate = !separate, separate
		return
	}
	for i, p := range c.Profiles[string(base)] {
		if p.ID == profileID {
			c.Profiles[string(base)][i].Unified = !separate
		}
	}
}
