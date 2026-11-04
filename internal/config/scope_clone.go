package config

import (
	"encoding/json"
	"fmt"
	"slices"
	"strings"

	"github.com/tingly-dev/tingly-box/internal/typ"
)

// cloneScopeLocked deep-copies a routing scope — every rule of src and src's
// own scenario config — onto the empty scope dst. A scope is a scenario name:
// a base ("claude_code", "team"), a profile ("claude_code:p1") or a team
// ("team:<id>"). Callers must hold c.mu, and call undo if the following save
// fails, otherwise the next unrelated save persists the half-made copy.
//
// Rules get fresh UUIDs, except built-in ones, which keep their
// builtin:<scope>:<slot> identity under dst (generated Claude Code settings
// and the slot logic look rules up by it).
//
// When src is dst's base (the main Claude Code config, the Default team), dst
// already inherits src's scenario config live (scenarioConfigLocked), so it is
// not snapshotted: the copy keeps following the base.
func (c *Config) cloneScopeLocked(src, dst typ.RuleScenario) (undo func(), err error) {
	if slices.ContainsFunc(c.Rules, func(r typ.Rule) bool { return r.Scenario == dst }) ||
		slices.ContainsFunc(c.Scenarios, func(sc typ.ScenarioConfig) bool { return sc.Scenario == dst }) {
		return nil, fmt.Errorf("scenario %q is not empty", dst)
	}

	var rules []typ.Rule
	for i := range c.Rules {
		r := &c.Rules[i]
		if r.Scenario != src {
			continue
		}
		var cp typ.Rule
		if err := deepCopyJSON(r, &cp); err != nil {
			return nil, err
		}
		cp.Scenario = dst
		cp.UUID = clonedRuleUUID(r.UUID, src, dst)
		rules = append(rules, cp)
	}

	nRules, nScenarios := len(c.Rules), len(c.Scenarios)
	c.Rules = append(c.Rules, rules...)
	undo = func() {
		c.Rules = c.Rules[:nRules]
		c.Scenarios = c.Scenarios[:nScenarios]
	}

	if dst.Base() != src {
		if i := slices.IndexFunc(c.Scenarios, func(sc typ.ScenarioConfig) bool { return sc.Scenario == src }); i >= 0 {
			var cp typ.ScenarioConfig
			if err := deepCopyJSON(c.Scenarios[i], &cp); err != nil {
				undo()
				return nil, err
			}
			cp.Scenario = dst
			c.Scenarios = append(c.Scenarios, cp)
		}
	}
	return undo, nil
}

// clonedRuleUUID is the UUID a copy of the rule gets under dst.
func clonedRuleUUID(uuid string, src, dst typ.RuleScenario) string {
	// A main Claude Code rule that was never renamed to the modern form.
	if modern, ok := legacyCCRuleUUIDs[uuid]; ok && src == typ.ScenarioClaudeCode {
		uuid = modern
	}
	if slot, ok := strings.CutPrefix(uuid, BuiltinRuleUUID(src, "")); ok {
		return BuiltinRuleUUID(dst, slot)
	}
	return GenerateUUID()
}

func deepCopyJSON(src, dst any) error {
	data, err := json.Marshal(src)
	if err != nil {
		return err
	}
	return json.Unmarshal(data, dst)
}

// removeScopeLocked drops every rule of scope and its own scenario config —
// what a deleted profile or team leaves behind. Callers must hold c.mu.
// Reports whether anything was removed.
func (c *Config) removeScopeLocked(scope typ.RuleScenario) bool {
	nRules, nScenarios := len(c.Rules), len(c.Scenarios)
	c.Rules = slices.DeleteFunc(c.Rules, func(r typ.Rule) bool { return r.Scenario == scope })
	c.Scenarios = slices.DeleteFunc(c.Scenarios, func(sc typ.ScenarioConfig) bool { return sc.Scenario == scope })
	return len(c.Rules) != nRules || len(c.Scenarios) != nScenarios
}
