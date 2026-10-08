package config

import (
	"fmt"
	"slices"

	"github.com/tingly-dev/tingly-box/internal/typ"
)

// CCSlots are the Claude Code model slots that can have a rule of their own.
// The default slot is the main rule itself and is always there; a slot whose
// rule (builtin:<scenario>:<slot>) is active requests that rule, any other
// slot follows the default. All on is what used to be separate mode, none is
// unified. See .design/claude-code-slot-binding.md.
var CCSlots = []string{"haiku", "sonnet", "opus", "fable", "subagent"}

// SetClaudeCodeSlot gives a slot its own rule (enabled) or hands it back to
// the default. Enabling switches the slot's rule on, creating it from the
// main rule's routing when it doesn't exist yet; disabling only switches it
// off, so turning the slot on again restores what it had.
func (c *Config) SetClaudeCodeSlot(scenario typ.RuleScenario, slot string, enabled bool) (typ.Rule, error) {
	if !scenario.Is(typ.ScenarioClaudeCode) {
		return typ.Rule{}, fmt.Errorf("scenario %q has no Claude Code slots", scenario)
	}
	if !slices.Contains(CCSlots, slot) {
		return typ.Rule{}, fmt.Errorf("slot %q can't be switched: the default slot is the main rule, the others are %v", slot, CCSlots)
	}
	c.mu.Lock()
	defer c.mu.Unlock()

	rule := c.ccRuleLocked(scenario, slot)
	if enabled && rule == nil {
		rule = c.addCCRuleLocked(scenario, slot, c.ccMainRuleLocked(scenario))
	}
	if rule == nil {
		return typ.Rule{}, nil // off and absent: already following the default
	}
	rule.Active = enabled
	out := *rule
	if !enabled {
		// The slot now relies on the main rule; make sure one is on.
		if main := c.ccMainRuleLocked(scenario); main != nil {
			main.Active = true
		} else {
			c.addCCRuleLocked(scenario, "cc", c.ccRuleLocked(scenario, slot))
		}
	}
	c.syncClaudeCodeModeFlagsLocked(scenario)
	return out, c.Save()
}

// ccMainRuleLocked returns the rule the default slot requests: the "cc" rule,
// or the "default" rule of a profile (or legacy separate config) that has no
// active cc rule. Active rules win over inactive ones; nil when neither exists.
func (c *Config) ccMainRuleLocked(scenario typ.RuleScenario) *typ.Rule {
	cc, def := c.ccRuleLocked(scenario, "cc"), c.ccRuleLocked(scenario, "default")
	for _, r := range []*typ.Rule{cc, def} {
		if r != nil && r.Active {
			return r
		}
	}
	if cc != nil {
		return cc
	}
	return def
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
