package config

import (
	"fmt"

	"github.com/tingly-dev/tingly-box/internal/typ"
)

// ccSwitchKeys are the built-in rules a switch copies: the default rule and every slot rule.
var ccSwitchKeys = append([]string{"cc"}, CCSlots...)

// UseProfileAsDefault overwrites the default Claude Code routing with a copy of
// a profile's: the default rule and every slot take the profile's routing
// (services, tactic, smart routing, rule flags), and a slot the profile does not
// split follows the default again. The profile is never changed and shares
// nothing with the default afterwards. The default's previous routing is not
// kept — to keep it, copy it into a profile first (DuplicateProfile with
// ProfileMainID). profile is an ID ("p1") or a name. See
// .design/claude-code-default-switch.md.
func (c *Config) UseProfileAsDefault(profile string) error {
	c.mu.Lock()
	defer c.mu.Unlock()

	var meta *typ.ProfileMeta
	for i, p := range c.Profiles[string(typ.ScenarioClaudeCode)] {
		if p.ID == profile || p.Name == profile {
			meta = &c.Profiles[string(typ.ScenarioClaudeCode)][i]
			break
		}
	}
	if meta == nil {
		return fmt.Errorf("%w: %q in scenario %q", ErrProfileNotFound, profile, typ.ScenarioClaudeCode)
	}
	profiled := typ.ProfiledScenarioName(typ.ScenarioClaudeCode, meta.ID)

	// Check everything before touching anything.
	src := c.ccRuleLocked(profiled, "cc")
	if src == nil || (len(src.Services) == 0 && !src.SmartEnabled) {
		return fmt.Errorf("profile %q has no routing configured yet", meta.Name)
	}
	if c.ccRuleLocked(typ.ScenarioClaudeCode, "cc") == nil {
		return fmt.Errorf("scenario %q has no main Claude Code rule", typ.ScenarioClaudeCode)
	}

	// Keep the default's rules in memory so a failed save leaves it as it was.
	nRules := len(c.Rules)
	var before []typ.Rule
	for _, key := range ccSwitchKeys {
		if r := c.ccRuleLocked(typ.ScenarioClaudeCode, key); r != nil {
			var cp typ.Rule
			if err := deepCopyJSON(r, &cp); err != nil {
				return err
			}
			before = append(before, cp)
		}
	}
	undo := func() {
		c.Rules = c.Rules[:nRules]
		for _, orig := range before {
			if r := c.findRuleByUUID(orig.UUID); r != nil {
				*r = orig
			}
		}
	}

	for _, key := range ccSwitchKeys {
		from := c.ccRuleLocked(profiled, key)
		if key == "cc" {
			dst := c.ccRuleLocked(typ.ScenarioClaudeCode, key)
			if err := copyCCRoutingDeep(dst, from); err != nil {
				undo()
				return err
			}
			dst.Active = true
			continue
		}
		if from == nil || !from.Active {
			if _, err := c.setCCSlotLocked(typ.ScenarioClaudeCode, key, false); err != nil {
				undo()
				return err
			}
			continue
		}
		// The slot rule may be created here, which can move c.Rules: look
		// up the profile's rule again afterwards.
		dst, err := c.setCCSlotLocked(typ.ScenarioClaudeCode, key, true)
		if err == nil {
			err = copyCCRoutingDeep(dst, c.ccRuleLocked(profiled, key))
		}
		if err != nil {
			undo()
			return err
		}
	}

	if err := c.saveLocked(); err != nil {
		undo()
		return err
	}
	return nil
}

// copyCCRoutingDeep is copyCCRouting without sharing anything with src, so
// editing one rule's services never changes the other's.
func copyCCRoutingDeep(dst, src *typ.Rule) error {
	var clone typ.Rule
	if err := deepCopyJSON(src, &clone); err != nil {
		return err
	}
	dst.Services = clone.Services
	dst.Flags = clone.Flags
	dst.LBTactic = clone.LBTactic
	dst.SmartEnabled = clone.SmartEnabled
	dst.SmartRouting = clone.SmartRouting
	return nil
}
