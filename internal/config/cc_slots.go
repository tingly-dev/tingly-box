package config

import (
	"fmt"
	"maps"
	"slices"
	"strings"

	"github.com/tingly-dev/tingly-box/internal/typ"
)

// Claude Code model slots: the env vars through which Claude Code picks the
// model id it sends. Each slot is bound to one rule of the scenario; the rule's
// request_model is what lands in the env (it stays the routing key, so routing
// itself knows nothing about slots). See .design/claude-code-slot-binding.md.
const (
	CCSlotDefault  = "default"
	CCSlotHaiku    = "haiku"
	CCSlotSonnet   = "sonnet"
	CCSlotOpus     = "opus"
	CCSlotFable    = "fable"
	CCSlotSubagent = "subagent"
)

// CCSlots lists the slots in display order.
var CCSlots = []string{CCSlotDefault, CCSlotHaiku, CCSlotSonnet, CCSlotOpus, CCSlotFable, CCSlotSubagent}

// CCSlotEnvKeys maps each slot to the Claude Code env var it fills.
var CCSlotEnvKeys = map[string]string{
	CCSlotDefault:  "ANTHROPIC_MODEL",
	CCSlotHaiku:    "ANTHROPIC_DEFAULT_HAIKU_MODEL",
	CCSlotSonnet:   "ANTHROPIC_DEFAULT_SONNET_MODEL",
	CCSlotOpus:     "ANTHROPIC_DEFAULT_OPUS_MODEL",
	CCSlotFable:    "ANTHROPIC_DEFAULT_FABLE_MODEL",
	CCSlotSubagent: "CLAUDE_CODE_SUBAGENT_MODEL",
}

// Slot presets: the two shapes the old unified/separate switch offered.
const (
	CCSlotPresetUnified  = "unified"
	CCSlotPresetSeparate = "separate"
)

// CCSlotResolution is what one slot resolves to.
type CCSlotResolution struct {
	Slot string `json:"slot"`
	// RuleUUID is the effective rule; empty when no active rule resolves and
	// RequestModel is the canonical fallback name.
	RuleUUID string `json:"rule_uuid"`
	// RequestModel is the model id Claude Code sends for this slot (without
	// the [1m] marker; Context1M carries that).
	RequestModel string `json:"request_model"`
	Context1M    bool   `json:"context_1m"`
	// Bound reports whether the slot has its own binding. An unbound slot
	// follows the default slot; an unbound default slot uses the main "cc" rule.
	Bound bool `json:"bound"`
}

// IsCCSlot reports whether slot is a known Claude Code model slot.
func IsCCSlot(slot string) bool {
	return slices.Contains(CCSlots, slot)
}

// ResolveClaudeCodeSlots returns, for every slot in CCSlots order, the rule
// Claude Code requests through it. scenario is "claude_code" or a profiled
// "claude_code:<id>".
func (c *Config) ResolveClaudeCodeSlots(scenario typ.RuleScenario) []CCSlotResolution {
	c.mu.RLock()
	defer c.mu.RUnlock()
	return c.resolveCCSlotsLocked(scenario, c.ccSlotBindingsLocked(scenario))
}

// ClaudeCodeSlotsUnified reports whether every slot requests the same model.
func ClaudeCodeSlotsUnified(slots []CCSlotResolution) bool {
	for _, s := range slots {
		if s.RequestModel != slots[0].RequestModel {
			return false
		}
	}
	return true
}

// SetClaudeCodeSlot binds slot to ruleUUID (a rule of the same scenario), or
// with an empty ruleUUID unbinds it so it follows the default slot (the main
// "cc" rule, for the default slot itself). A bound rule is switched on: being
// requested through a slot is what it is for.
func (c *Config) SetClaudeCodeSlot(scenario typ.RuleScenario, slot, ruleUUID string) error {
	if err := validateCCSlot(scenario, slot); err != nil {
		return err
	}
	c.mu.Lock()
	defer c.mu.Unlock()

	bindings := c.ccSlotBindingsLocked(scenario)
	if ruleUUID == "" {
		if slot == CCSlotDefault {
			c.ensureCCMainRuleLocked(scenario, c.resolveCCSlotsLocked(scenario, bindings)[0].RuleUUID)
		}
		delete(bindings, slot)
	} else {
		rule := c.findRuleByUUID(ruleUUID)
		if rule == nil || rule.GetScenario() != scenario {
			return fmt.Errorf("rule %q not found in scenario %q", ruleUUID, scenario)
		}
		rule.Active = true
		bindings[slot] = rule.UUID
	}
	c.storeCCSlotBindingsLocked(scenario, bindings)
	return c.Save()
}

// CreateClaudeCodeSlotRule gives slot a rule of its own and binds it. The
// slot's built-in rule (builtin:<scenario>:<slot>) is reused when it exists,
// keeping whatever it was configured with before; otherwise a new one is
// seeded from the rule the slot resolves to now, so the slot keeps routing
// exactly as before until the user edits it.
func (c *Config) CreateClaudeCodeSlotRule(scenario typ.RuleScenario, slot string) (typ.Rule, error) {
	if err := validateCCSlot(scenario, slot); err != nil {
		return typ.Rule{}, err
	}
	c.mu.Lock()
	defer c.mu.Unlock()

	bindings := c.ccSlotBindingsLocked(scenario)
	current := c.resolveCCSlotsLocked(scenario, bindings)
	rule := c.ensureCCSlotRuleLocked(scenario, slot, current[slices.Index(CCSlots, slot)].RuleUUID)
	bindings[slot] = rule.UUID
	c.storeCCSlotBindingsLocked(scenario, bindings)
	return *rule, c.Save()
}

// ApplyClaudeCodeSlotPreset rebinds every slot: "unified" sends all of them
// through the main "cc" rule, "separate" gives each its own rule. Rules are
// never switched off, so a Claude Code still running on the previous env
// keeps routing until it is re-applied.
func (c *Config) ApplyClaudeCodeSlotPreset(scenario typ.RuleScenario, preset string) error {
	if !scenario.Is(typ.ScenarioClaudeCode) {
		return fmt.Errorf("scenario %q has no Claude Code slots", scenario)
	}
	c.mu.Lock()
	defer c.mu.Unlock()
	if err := c.applyCCSlotPresetLocked(scenario, preset); err != nil {
		return err
	}
	return c.Save()
}

func (c *Config) applyCCSlotPresetLocked(scenario typ.RuleScenario, preset string) error {
	bindings := c.ccSlotBindingsLocked(scenario)
	current := c.resolveCCSlotsLocked(scenario, bindings)
	switch preset {
	case CCSlotPresetUnified:
		c.ensureCCMainRuleLocked(scenario, current[0].RuleUUID)
		bindings = map[string]string{}
	case CCSlotPresetSeparate:
		for i, slot := range CCSlots {
			bindings[slot] = c.ensureCCSlotRuleLocked(scenario, slot, current[i].RuleUUID).UUID
		}
	default:
		return fmt.Errorf("unknown slot preset %q", preset)
	}
	c.storeCCSlotBindingsLocked(scenario, bindings)
	return nil
}

func validateCCSlot(scenario typ.RuleScenario, slot string) error {
	if !scenario.Is(typ.ScenarioClaudeCode) {
		return fmt.Errorf("scenario %q has no Claude Code slots", scenario)
	}
	if !IsCCSlot(slot) {
		return fmt.Errorf("unknown Claude Code slot %q", slot)
	}
	return nil
}

// ccSlotBindingsLocked returns a mutable copy of the scenario's slot bindings.
// A scenario that has never stored any gets the bindings equivalent to its
// legacy mode: none when unified, every slot on its own built-in rule when
// separate (skipping missing or inactive ones, which then follow the default
// slot, as the fable tier always has).
func (c *Config) ccSlotBindingsLocked(scenario typ.RuleScenario) map[string]string {
	for i := range c.Scenarios {
		if c.Scenarios[i].Scenario == scenario && c.Scenarios[i].ClaudeCodeSlots != nil {
			return maps.Clone(c.Scenarios[i].ClaudeCodeSlots)
		}
	}
	bindings := map[string]string{}
	if !c.ccLegacySeparateLocked(scenario) {
		return bindings
	}
	for _, slot := range CCSlots {
		if rule := c.ccSlotBuiltinRuleLocked(scenario, slot); rule != nil && rule.Active {
			bindings[slot] = rule.UUID
		}
	}
	return bindings
}

// ccLegacySeparateLocked reports the pre-slot mode: the profile's Unified
// flag for a profile, the scenario's unified/separate flags otherwise.
func (c *Config) ccLegacySeparateLocked(scenario typ.RuleScenario) bool {
	base, profileID := typ.ParseScenarioProfile(scenario)
	if profileID != "" {
		for _, p := range c.Profiles[string(base)] {
			if p.ID == profileID {
				return !p.Unified
			}
		}
		return false
	}
	for i := range c.Scenarios {
		if c.Scenarios[i].Scenario == scenario {
			return c.Scenarios[i].GetDefaultFlags().Separate
		}
	}
	return false
}

func (c *Config) resolveCCSlotsLocked(scenario typ.RuleScenario, bindings map[string]string) []CCSlotResolution {
	activeRule := func(uuid string) *typ.Rule {
		if uuid == "" {
			return nil
		}
		r := c.findRuleByUUID(uuid)
		if r == nil || !r.Active || r.GetScenario() != scenario || strings.TrimSpace(r.RequestModel) == "" {
			return nil
		}
		return r
	}
	from := func(slot string, r *typ.Rule, bound bool) CCSlotResolution {
		return CCSlotResolution{
			Slot:         slot,
			RuleUUID:     r.UUID,
			RequestModel: TrimContext1M(strings.TrimSpace(r.RequestModel)),
			Context1M:    r.Flags.Context1M || strings.HasSuffix(r.RequestModel, Context1MSuffix),
			Bound:        bound,
		}
	}

	out := make([]CCSlotResolution, 0, len(CCSlots))
	var def CCSlotResolution
	if r := activeRule(bindings[CCSlotDefault]); r != nil {
		def = from(CCSlotDefault, r, true)
	} else if r := activeRule(c.ccMainRuleUUIDLocked(scenario)); r != nil {
		def = from(CCSlotDefault, r, false)
	} else {
		def = CCSlotResolution{Slot: CCSlotDefault, RequestModel: ccSlotRequestModel(scenario, "cc")}
	}
	out = append(out, def)
	for _, slot := range CCSlots[1:] {
		if r := activeRule(bindings[slot]); r != nil {
			out = append(out, from(slot, r, true))
			continue
		}
		follow := def
		follow.Slot, follow.Bound = slot, false
		out = append(out, follow)
	}
	return out
}

// storeCCSlotBindingsLocked persists bindings and keeps the derived legacy
// mode (scenario flags, profile Unified) in step for readers that still
// only know unified vs separate.
func (c *Config) storeCCSlotBindingsLocked(scenario typ.RuleScenario, bindings map[string]string) {
	unified := ClaudeCodeSlotsUnified(c.resolveCCSlotsLocked(scenario, bindings))
	sc := c.findOrCreateScenarioConfigLocked(scenario)
	sc.ClaudeCodeSlots = bindings
	base, profileID := typ.ParseScenarioProfile(scenario)
	if profileID == "" {
		sc.Flags.Unified, sc.Flags.Separate = unified, !unified
		return
	}
	profiles := c.Profiles[string(base)]
	for i := range profiles {
		if profiles[i].ID == profileID {
			profiles[i].Unified = unified
		}
	}
}

// ccMainRuleUUIDLocked returns the UUID of the scenario's main "cc" rule.
func (c *Config) ccMainRuleUUIDLocked(scenario typ.RuleScenario) string {
	uuid := BuiltinRuleUUID(scenario, "cc")
	if scenario == typ.ScenarioClaudeCode && c.findRuleByUUID(uuid) == nil && c.findRuleByUUID(RuleUUIDBuiltinCC) != nil {
		return RuleUUIDBuiltinCC
	}
	return uuid
}

// ccSlotBuiltinRuleLocked returns the slot's built-in rule, if any.
func (c *Config) ccSlotBuiltinRuleLocked(scenario typ.RuleScenario, slot string) *typ.Rule {
	if r := c.findRuleByUUID(BuiltinRuleUUID(scenario, slot)); r != nil {
		return r
	}
	if scenario == typ.ScenarioClaudeCode {
		for legacy, modern := range legacyCCRuleUUIDs {
			if modern == BuiltinRuleUUID(scenario, slot) {
				if r := c.findRuleByUUID(legacy); r != nil {
					return r
				}
			}
		}
	}
	return nil
}

// ccSlotRequestModel is the canonical request model of a slot's (or, for
// "cc", the main) rule: "tingly/cc-<slot>" in the main scenario, the bare
// name in a profile.
func ccSlotRequestModel(scenario typ.RuleScenario, slot string) string {
	if _, profileID := typ.ParseScenarioProfile(scenario); profileID != "" {
		return slot
	}
	if slot == "cc" {
		return "tingly/cc"
	}
	return "tingly/cc-" + slot
}

// ensureCCMainRuleLocked makes sure the main "cc" rule exists and is on,
// seeding a missing one (a profile created in separate mode has none) from
// seedUUID's rule.
func (c *Config) ensureCCMainRuleLocked(scenario typ.RuleScenario, seedUUID string) *typ.Rule {
	if r := c.findRuleByUUID(c.ccMainRuleUUIDLocked(scenario)); r != nil {
		r.Active = true
		return r
	}
	return c.addCCRuleLocked(scenario, "cc", seedUUID)
}

// ensureCCSlotRuleLocked returns the slot's own rule, switched on: its
// built-in rule, or a rule of the scenario already answering to its
// canonical request model, or a new one seeded from seedUUID's rule.
func (c *Config) ensureCCSlotRuleLocked(scenario typ.RuleScenario, slot, seedUUID string) *typ.Rule {
	rule := c.ccSlotBuiltinRuleLocked(scenario, slot)
	if rule == nil {
		model := ccSlotRequestModel(scenario, slot)
		for i := range c.Rules {
			if c.Rules[i].GetScenario() == scenario && TrimContext1M(c.Rules[i].RequestModel) == model {
				rule = &c.Rules[i]
				break
			}
		}
	}
	if rule == nil {
		return c.addCCRuleLocked(scenario, slot, seedUUID)
	}
	rule.Active = true
	return rule
}

// addCCRuleLocked appends a new built-in rule for slot (or "cc") whose
// routing (services, flags, tactic, smart routing) is copied from seedUUID's
// rule when there is one, and returns a pointer to it.
func (c *Config) addCCRuleLocked(scenario typ.RuleScenario, slot, seedUUID string) *typ.Rule {
	var rule typ.Rule
	if _, profileID := typ.ParseScenarioProfile(scenario); profileID != "" {
		rule = newCCProfileRule(scenario, slot, "Claude Code profile - "+slot+" model")
	} else if tmpl, ok := defaultRuleByUUID(BuiltinRuleUUID(scenario, slot)); ok {
		rule = tmpl
	} else {
		rule = newCCProfileRule(scenario, ccSlotRequestModel(scenario, slot), "Claude Code - "+slot+" model")
		rule.UUID = BuiltinRuleUUID(scenario, slot)
	}
	rule.Scenario = scenario
	rule.Active = true
	if seed := c.findRuleByUUID(seedUUID); seedUUID != "" && seed != nil {
		rule.Services = cloneServices(seed.Services)
		rule.Flags = seed.Flags
		rule.LBTactic = seed.LBTactic
		rule.SmartEnabled = seed.SmartEnabled
		rule.SmartRouting = slices.Clone(seed.SmartRouting)
	}
	c.Rules = append(c.Rules, rule)
	return &c.Rules[len(c.Rules)-1]
}
