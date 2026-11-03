package config

import (
	"encoding/json"
	"fmt"
	"slices"

	"github.com/tingly-dev/tingly-box/internal/typ"
)

// ClaudeCodeDefaultSwitch is the persisted state of "default Claude Code
// switched to a profile": which profile the default now mirrors, and the
// default's own rules from before the first switch, so they can be restored.
// See .design/claude-code-default-switch.md.
type ClaudeCodeDefaultSwitch struct {
	ProfileID   string     `json:"profile_id"`
	ProfileName string     `json:"profile_name"`
	Original    []typ.Rule `json:"original"`
}

// ClaudeCodeDefaultSwitchStatus is the read view of the switch state.
type ClaudeCodeDefaultSwitchStatus struct {
	// Switched reports whether the default was switched to a profile and can be restored.
	Switched    bool   `json:"switched"`
	ProfileID   string `json:"profile_id,omitempty"`
	ProfileName string `json:"profile_name,omitempty"`
	// ProfileExists is false once the profile it was copied from is deleted.
	ProfileExists bool `json:"profile_exists"`
	// InSync is true while the default's routing still equals the profile's;
	// the default was edited (or the profile changed) since the switch otherwise.
	InSync bool `json:"in_sync"`
}

// ccSwitchKeys are the built-in rules a switch copies: the default rule and every slot rule.
var ccSwitchKeys = append([]string{"cc"}, CCSlots...)

// SwitchClaudeCodeDefaultToProfile makes the default Claude Code routing the
// same as profile's: the default rule and every slot take the profile's
// routing (providers, models, tactic, smart routing, rule flags), and a slot
// the profile does not split follows the default again. It copies — the
// profile is never modified and stays independent of the default afterwards —
// and keeps the default's rules from before the first switch for
// RestoreClaudeCodeDefault. profile is an ID ("p1") or a name.
func (c *Config) SwitchClaudeCodeDefaultToProfile(profile string) (ClaudeCodeDefaultSwitchStatus, error) {
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
		return ClaudeCodeDefaultSwitchStatus{}, fmt.Errorf("profile %q not found", profile)
	}
	profiled := typ.ProfiledScenarioName(typ.ScenarioClaudeCode, meta.ID)

	// Check everything before touching anything.
	src := c.ccRuleLocked(profiled, "cc")
	if src == nil || (len(src.Services) == 0 && !src.SmartEnabled) {
		return ClaudeCodeDefaultSwitchStatus{}, fmt.Errorf("profile %q has no routing configured yet", meta.Name)
	}
	if c.ccRuleLocked(typ.ScenarioClaudeCode, "cc") == nil {
		return ClaudeCodeDefaultSwitchStatus{}, fmt.Errorf("scenario %q has no main Claude Code rule", typ.ScenarioClaudeCode)
	}

	state := c.ClaudeCodeDefaultSwitch
	if state == nil {
		state = &ClaudeCodeDefaultSwitch{}
		for _, key := range ccSwitchKeys {
			if r := c.ccRuleLocked(typ.ScenarioClaudeCode, key); r != nil {
				state.Original = append(state.Original, cloneRuleDeep(*r))
			}
		}
	}
	state.ProfileID, state.ProfileName = meta.ID, meta.Name

	for _, key := range ccSwitchKeys {
		from := c.ccRuleLocked(profiled, key)
		if key == "cc" {
			dst := c.ccRuleLocked(typ.ScenarioClaudeCode, key)
			copyCCRoutingDeep(dst, from)
			dst.Active = true
			continue
		}
		if from == nil || !from.Active {
			if _, err := c.setCCSlotLocked(typ.ScenarioClaudeCode, key, false); err != nil {
				return ClaudeCodeDefaultSwitchStatus{}, err
			}
			continue
		}
		// The slot rule may be created here, which can move c.Rules: look
		// up the profile's rule again afterwards.
		dst, err := c.setCCSlotLocked(typ.ScenarioClaudeCode, key, true)
		if err != nil {
			return ClaudeCodeDefaultSwitchStatus{}, err
		}
		copyCCRoutingDeep(dst, c.ccRuleLocked(profiled, key))
	}

	c.ClaudeCodeDefaultSwitch = state
	if err := c.saveLocked(); err != nil {
		return ClaudeCodeDefaultSwitchStatus{}, err
	}
	return c.claudeCodeDefaultSwitchStatusLocked(), nil
}

// RestoreClaudeCodeDefault puts the default Claude Code rules back as they
// were before the first switch and forgets the switch. Edits made to the
// default since the switch are discarded with the switch.
func (c *Config) RestoreClaudeCodeDefault() error {
	c.mu.Lock()
	defer c.mu.Unlock()

	state := c.ClaudeCodeDefaultSwitch
	if state == nil {
		return fmt.Errorf("the default Claude Code configuration has not been switched to a profile")
	}

	kept := make(map[string]bool, len(state.Original))
	for _, r := range state.Original {
		kept[r.UUID] = true
	}
	// A rule the switch created did not exist before: drop it again.
	c.Rules = slices.DeleteFunc(c.Rules, func(r typ.Rule) bool {
		if r.Scenario != typ.ScenarioClaudeCode || kept[r.UUID] {
			return false
		}
		for _, key := range ccSwitchKeys {
			if r.UUID == BuiltinRuleUUID(typ.ScenarioClaudeCode, key) || r.UUID == LegacyCCRuleUUID(key) {
				return true
			}
		}
		return false
	})
	for _, orig := range state.Original {
		restored := cloneRuleDeep(orig)
		if r := c.findRuleByUUID(orig.UUID); r != nil {
			*r = restored
		} else {
			c.Rules = append(c.Rules, restored)
		}
	}

	c.ClaudeCodeDefaultSwitch = nil
	return c.saveLocked()
}

// GetClaudeCodeDefaultSwitch reports the default's switch state.
func (c *Config) GetClaudeCodeDefaultSwitch() ClaudeCodeDefaultSwitchStatus {
	c.mu.RLock()
	defer c.mu.RUnlock()
	return c.claudeCodeDefaultSwitchStatusLocked()
}

func (c *Config) claudeCodeDefaultSwitchStatusLocked() ClaudeCodeDefaultSwitchStatus {
	state := c.ClaudeCodeDefaultSwitch
	if state == nil {
		return ClaudeCodeDefaultSwitchStatus{}
	}
	status := ClaudeCodeDefaultSwitchStatus{Switched: true, ProfileID: state.ProfileID, ProfileName: state.ProfileName}
	for _, p := range c.Profiles[string(typ.ScenarioClaudeCode)] {
		if p.ID == state.ProfileID {
			status.ProfileExists, status.ProfileName = true, p.Name
			status.InSync = c.ccRoutingMatchesLocked(typ.ProfiledScenarioName(typ.ScenarioClaudeCode, p.ID))
			break
		}
	}
	return status
}

// ccRoutingMatchesLocked reports whether the main Claude Code routing equals
// profiled's: the same slots split, and the same routing in every active rule.
func (c *Config) ccRoutingMatchesLocked(profiled typ.RuleScenario) bool {
	for _, key := range ccSwitchKeys {
		a, b := c.ccRuleLocked(typ.ScenarioClaudeCode, key), c.ccRuleLocked(profiled, key)
		aOn, bOn := a != nil && a.Active, b != nil && b.Active
		if key == "cc" {
			aOn, bOn = true, true
		}
		if aOn != bOn {
			return false
		}
		if aOn && ccRoutingJSON(a) != ccRoutingJSON(b) {
			return false
		}
	}
	return true
}

// ccRoutingJSON is the comparable form of what decides where a rule routes;
// runtime stats are not part of it.
func ccRoutingJSON(r *typ.Rule) string {
	b, _ := json.Marshal(struct {
		Services     any
		Flags        any
		LBTactic     any
		SmartEnabled bool
		SmartRouting any
	}{r.Services, r.Flags, r.LBTactic, r.SmartEnabled, r.SmartRouting})
	return string(b)
}

// copyCCRoutingDeep is copyCCRouting without sharing anything with src, so
// editing one rule's services never changes the other's.
func copyCCRoutingDeep(dst, src *typ.Rule) {
	clone := cloneRuleDeep(*src)
	dst.Services = clone.Services
	dst.Flags = clone.Flags
	dst.LBTactic = clone.LBTactic
	dst.SmartEnabled = clone.SmartEnabled
	dst.SmartRouting = clone.SmartRouting
}

// cloneRuleDeep copies a rule through its persisted (JSON) form, which is
// everything that survives a save; per-service runtime stats start empty.
func cloneRuleDeep(r typ.Rule) typ.Rule {
	var out typ.Rule
	b, err := json.Marshal(r)
	if err == nil {
		err = json.Unmarshal(b, &out)
	}
	if err != nil {
		return r
	}
	return out
}
