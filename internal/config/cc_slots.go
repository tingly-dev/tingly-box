package config

import (
	"fmt"
	"slices"

	"github.com/sirupsen/logrus"

	"github.com/tingly-dev/tingly-box/internal/typ"
)

// Claude Code routing is a main rule plus optional slot rules, with no mode:
//
//   - the main rule builtin:<scenario>:cc is the default slot. It always
//     exists and is always on;
//   - each of CCSlots requests its own rule builtin:<scenario>:<slot> while
//     that rule is on, and follows the main rule otherwise.
//
// Every slot on is what "separate" used to mean, none is "unified".
// See .design/claude-code-slot-binding.md.

// CCSlots are the Claude Code model slots that can have a rule of their own.
var CCSlots = []string{"haiku", "sonnet", "opus", "fable", "subagent"}

// SetClaudeCodeSlot gives a slot its own rule (enabled) or hands it back to
// the main rule. Enabling switches the slot's rule on, creating it from the
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
	var out typ.Rule
	if rule := c.setClaudeCodeSlotLocked(scenario, slot, enabled); rule != nil {
		out = *rule
	}
	return out, c.Save()
}

// setAllClaudeCodeSlotsLocked switches every slot on or off: what the legacy
// separate / unified mode writes mean now.
func (c *Config) setAllClaudeCodeSlotsLocked(scenario typ.RuleScenario, enabled bool) {
	for _, slot := range CCSlots {
		c.setClaudeCodeSlotLocked(scenario, slot, enabled)
	}
}

// setClaudeCodeSlotLocked returns the slot's rule, nil when it is off and
// was never created.
func (c *Config) setClaudeCodeSlotLocked(scenario typ.RuleScenario, slot string, enabled bool) *typ.Rule {
	// The main rule is where every slot without its own rule goes.
	main := c.ensureCCMainRuleLocked(scenario)
	rule := c.ccRuleLocked(scenario, slot)
	if enabled && rule == nil {
		rule = c.addCCRuleLocked(scenario, slot, main)
	}
	if rule != nil {
		rule.Active = enabled
	}
	return rule
}

// ensureCCMainRuleLocked returns the scenario's main rule, switched on,
// creating an empty one if it is missing. The returned pointer is only valid
// until the next append to c.Rules.
func (c *Config) ensureCCMainRuleLocked(scenario typ.RuleScenario) *typ.Rule {
	main := c.ccRuleLocked(scenario, "cc")
	if main == nil {
		main = c.addCCRuleLocked(scenario, "cc", nil)
	}
	main.Active = true
	return main
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
		rule = newCCProfileRule(scenario, slot, "Claude Code profile - "+slot+" model")
	}
	rule.Scenario = scenario
	rule.Active = true
	if seed != nil {
		copyCCRouting(&rule, seed)
	}
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

// claudeCodeScenariosLocked lists the main Claude Code scenario and every
// Claude Code profile scenario.
func (c *Config) claudeCodeScenariosLocked() []typ.RuleScenario {
	out := []typ.RuleScenario{typ.ScenarioClaudeCode}
	for _, p := range c.Profiles[string(typ.ScenarioClaudeCode)] {
		out = append(out, typ.ProfiledScenarioName(typ.ScenarioClaudeCode, p.ID))
	}
	return out
}

func collapseClaudeCodeMainRuleOnce(c *Config) bool {
	const marker = "20261008-claude-code-slots"
	if c.hasMigrationCompleted(marker) {
		return false
	}
	for _, scenario := range c.claudeCodeScenariosLocked() {
		c.collapseClaudeCodeMainRule(scenario)
	}
	c.markMigrationCompleted(marker)
	return true // the marker itself changed the config
}

// collapseClaudeCodeMainRule turns a scenario from the unified/separate era
// into main rule + slot rules: the "default" rule is merged into "cc", taking
// its request_model along when it was the one in use (separate mode), so the
// env already written to settings.json keeps routing exactly as before. The
// mode flags are dropped; the slot rules' on/off state already says it all.
func (c *Config) collapseClaudeCodeMainRule(scenario typ.RuleScenario) {
	cc := c.ccRuleLocked(scenario, "cc")
	def := c.ccRuleLocked(scenario, "default")
	if def != nil && def.Active && (cc == nil || !cc.Active) {
		// Separate mode: ANTHROPIC_MODEL points at the default rule.
		if cc == nil {
			cc = c.addCCRuleLocked(scenario, "cc", nil)
			def = c.ccRuleLocked(scenario, "default") // the append may have moved it
		}
		copyCCRouting(cc, def)
		cc.RequestModel = def.RequestModel
		cc.ResponseModel = def.ResponseModel
		logrus.WithField("scenario", scenario).Info("Merged the Claude Code default rule into the main rule")
	}
	if def != nil {
		defUUID := def.UUID
		c.Rules = slices.DeleteFunc(c.Rules, func(r typ.Rule) bool { return r.UUID == defUUID })
	}
	if cc := c.ccRuleLocked(scenario, "cc"); cc != nil {
		cc.Active = true
	}
	for i := range c.Scenarios {
		if c.Scenarios[i].Scenario == scenario {
			c.Scenarios[i].Flags.Unified, c.Scenarios[i].Flags.Separate = false, false
		}
	}
	base, profileID := typ.ParseScenarioProfile(scenario)
	for i, p := range c.Profiles[string(base)] {
		if p.ID == profileID {
			c.Profiles[string(base)][i].Unified = false
		}
	}
}
