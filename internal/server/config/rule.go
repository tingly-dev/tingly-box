package config

import (
	"fmt"
	"time"

	"github.com/tingly-dev/tingly-box/internal/loadbalance"
	smartrouting "github.com/tingly-dev/tingly-box/internal/smart_routing"
	"github.com/tingly-dev/tingly-box/internal/typ"
)

// Wildcard rule names that match any model
const (
	WildcardRuleName = "*"
)

// GetEffectiveAffinity returns the effective affinity TTL for a rule.
// session_affinity is rule-only — there is no scenario-level inheritance.
// The built-in Claude Code / Claude Desktop / Codex rules seed 1800s by
// default (init + migration); every other rule is disabled unless it sets
// an explicit value. Returns 0 (disabled) when the rule's value is 0.
func (c *Config) GetEffectiveAffinity(rule *typ.Rule) time.Duration {
	if rule.Flags.SessionAffinity > 0 {
		return time.Duration(rule.Flags.SessionAffinity) * time.Second
	}
	return 0
}

// applyScenarioCreateDefaults seeds sensible per-scenario flag defaults on a
// rule at creation time. It lives here, in the AddRule choke point, so every
// creation path benefits — HTTP, CLI, TUI, agent, and import all funnel through
// AddRule. Loading an existing config never calls AddRule, so a user who later
// turns a default off is never re-seeded.
//
// Team: rules under the team scenario are almost always fronted by Claude Code
// clients pointed at /tingly/team, so they hit the same third-party-provider
// incompatibilities the built-in Claude Code rules already default around —
// mid-conversation system-role messages that strict Anthropic-compatible
// providers reject (ClaudeCodeCompat) and Claude Code's injected billing header
// that must not leak upstream (CleanHeader). Default both on so team rules work
// out of the box. Only seed when the caller sent no flags at all, so an explicit
// payload that sets any flag is left untouched.
func applyScenarioCreateDefaults(rule *typ.Rule) {
	if rule.Scenario.Is(typ.ScenarioTeam) && rule.Flags == (typ.RuleFlags{}) {
		rule.Flags.ClaudeCodeCompat = true
		rule.Flags.CleanHeader = true
	}
}

// AddRule updates the default Rule
func (c *Config) AddRule(rule typ.Rule) error {
	c.mu.Lock()
	defer c.mu.Unlock()

	ensureRuleUUID(&rule)
	applyScenarioCreateDefaults(&rule)

	// A brand-new rule has no grandfathered references — validate everything.
	if err := c.validateRuleServices(rule, nil); err != nil {
		return err
	}
	if err := validateSmartRoutingRules(rule); err != nil {
		return err
	}

	normalizeRuleServiceTiers(&rule)

	// Guard name unique within same scenario
	for _, rc := range c.Rules {
		if rc.RequestModel == rule.RequestModel && rc.Scenario == rule.Scenario {
			if rc.UUID != rule.UUID {
				return fmt.Errorf("rule with Name %s already exists in same scenario", rule.RequestModel)
			}
		}
	}

	for _, rc := range c.Rules {
		if rc.UUID == rule.UUID {
			return fmt.Errorf("rule with UUID %s already exists", rule.UUID)
		}
	}

	// If not found, append new config
	c.Rules = append(c.Rules, rule)
	c.DefaultRequestID = len(c.Rules) - 1
	return c.Save()
}

func (c *Config) UpdateRule(uid string, rule typ.Rule) error {
	c.mu.Lock()
	defer c.mu.Unlock()

	// A payload without a UUID keeps the identity it is replacing.
	if rule.UUID == "" {
		rule.UUID = uid
	}

	// Incremental validation: references the persisted rule already carries
	// stay editable even if their provider was disabled/deleted since.
	if err := c.validateRuleServices(rule, c.findRuleByUUID(uid)); err != nil {
		return err
	}
	if err := validateSmartRoutingRules(rule); err != nil {
		return err
	}

	normalizeRuleServiceTiers(&rule)

	// Claude Desktop pulls its model picker from /v1/models, which lists rule
	// request models verbatim — the [1m] context-window advertisement must
	// therefore live on the rule name itself. Keep the name suffix in sync
	// with the context_1m flag so toggling the flag renames the rule.
	// Claude Code is intentionally excluded: its [1m] advertisement travels
	// via the launcher env, and the rule keeps its bare routing name.
	if rule.Scenario.Is(typ.ScenarioClaudeDesktop) {
		rule.RequestModel = syncContext1MSuffix(rule.RequestModel, rule.Flags.Context1M)
		rule.ResponseModel = syncContext1MSuffix(rule.ResponseModel, rule.Flags.Context1M)
	}

	// Guard name unique
	for _, rc := range c.Rules {
		if rc.RequestModel == rule.RequestModel && rc.GetScenario() == rule.Scenario {
			if rc.UUID != rule.UUID {
				return fmt.Errorf("rule with Name %s already exists in same scenario", rule.RequestModel)
			}
		}
	}

	// Find existing config with same request model
	for i, rc := range c.Rules {
		if rc.UUID == uid {
			c.Rules[i] = rule
			return c.Save()
		}
	}

	return nil
}

// AddRequestConfig adds a new Rule. If a rule with the same UUID already exists,
// it is rejected instead of adding a duplicate.
func (c *Config) AddRequestConfig(reqConfig typ.Rule) error {
	c.mu.Lock()
	defer c.mu.Unlock()

	ensureRuleUUID(&reqConfig)

	// Check if rule with same UUID already exists
	for _, rule := range c.Rules {
		if rule.UUID == reqConfig.UUID {
			return nil
		}
	}

	// No existing rule, append new one
	c.Rules = append(c.Rules, reqConfig)
	return c.Save()
}

// GetDefaultRequestConfig returns the default Rule
func (c *Config) GetDefaultRequestConfig() *typ.Rule {
	c.mu.RLock()
	defer c.mu.RUnlock()

	if c.DefaultRequestID >= 0 && c.DefaultRequestID < len(c.Rules) {
		return &c.Rules[c.DefaultRequestID]
	}
	return nil
}

// SetDefaultRequestID sets the index of the default Rule
func (c *Config) SetDefaultRequestID(id int) error {
	c.mu.Lock()
	defer c.mu.Unlock()

	c.DefaultRequestID = id
	return c.Save()
}

// GetRequestConfigs returns all Rules
func (c *Config) GetRequestConfigs() []typ.Rule {
	c.mu.RLock()
	defer c.mu.RUnlock()

	return c.Rules
}

// GetDefaultRequestID returns the index of the default Rule
func (c *Config) GetDefaultRequestID() int {
	c.mu.RLock()
	defer c.mu.RUnlock()

	return c.DefaultRequestID
}

// IsRequestModel checks if the given model name is a request model in any config
func (c *Config) IsRequestModel(modelName string) bool {
	c.mu.RLock()
	defer c.mu.RUnlock()

	for _, rc := range c.Rules {
		if rc.RequestModel == modelName {
			return true
		}
	}
	return false
}

// GetUUIDByRequestModel returns the UUID for the given request model name
func (c *Config) GetUUIDByRequestModel(requestModel string) string {
	c.mu.RLock()
	defer c.mu.RUnlock()

	for _, rule := range c.Rules {
		if rule.RequestModel == requestModel {
			return rule.UUID
		}
	}
	return ""
}

// GetRuleByUUID returns the Rule for the given request uuid
func (c *Config) GetRuleByUUID(UUID string) *typ.Rule {
	c.mu.RLock()
	defer c.mu.RUnlock()

	for _, rule := range c.Rules {
		if rule.UUID == UUID {
			return &rule
		}
	}
	return nil
}

// GetRuleByRequestModelAndScenario returns the Rule for the given request model and scenario
func (c *Config) GetRuleByRequestModelAndScenario(requestModel string, scenario typ.RuleScenario) *typ.Rule {
	c.mu.RLock()
	defer c.mu.RUnlock()

	for _, rule := range c.Rules {
		if rule.RequestModel == requestModel && rule.GetScenario() == scenario {
			return &rule
		}
	}
	return nil
}

// GetUUIDByRequestModelAndScenario returns the UUID for the given request model and scenario
func (c *Config) GetUUIDByRequestModelAndScenario(requestModel string, scenario typ.RuleScenario) string {
	c.mu.RLock()
	defer c.mu.RUnlock()

	for _, rule := range c.Rules {
		if rule.RequestModel == requestModel && rule.GetScenario() == scenario {
			return rule.UUID
		}
	}
	return ""
}

// IsRequestModelInScenario checks if the given model name is a request model in the given scenario
func (c *Config) IsRequestModelInScenario(modelName string, scenario typ.RuleScenario) bool {
	c.mu.RLock()
	defer c.mu.RUnlock()

	for _, rc := range c.Rules {
		if rc.RequestModel == modelName && rc.GetScenario() == scenario {
			return true
		}
	}
	return false
}

// IsWildcardRuleName checks if the given rule name is a wildcard that matches any model.
// This function is thread-safe as it only performs constant string comparisons
// and does not access any shared state. It can be called without holding Config.mu.
func IsWildcardRuleName(name string) bool {
	return name == WildcardRuleName
}

// MatchRuleByModelAndScenario finds a rule by model name with wildcard support
// Priority: exact match > wildcard match
// Returns nil if no rule matches
func (c *Config) MatchRuleByModelAndScenario(requestModel string, scenario typ.RuleScenario) *typ.Rule {
	c.mu.RLock()
	defer c.mu.RUnlock()

	// First, try exact match
	for _, rule := range c.Rules {
		if rule.RequestModel == requestModel && rule.GetScenario() == scenario {
			return &rule
		}
	}

	// Claude scenarios advertise the 1M context window via a "[1m]"
	// model-name suffix that may be present on either side independently:
	// Claude Code strips it from outgoing requests while the env carries it,
	// Claude Desktop picks names verbatim from /v1/models where renamed
	// rules list it, and a stale client config may still send a suffix the
	// rule no longer has. Normalize both sides before comparing.
	if base := scenario.Base(); base == typ.ScenarioClaudeCode || base == typ.ScenarioClaudeDesktop {
		want := TrimContext1M(requestModel)
		for _, rule := range c.Rules {
			if TrimContext1M(rule.RequestModel) == want && rule.GetScenario() == scenario {
				return &rule
			}
		}
	}

	// Then, try wildcard match
	for _, rule := range c.Rules {
		if IsWildcardRuleName(rule.RequestModel) && rule.GetScenario() == scenario {
			return &rule
		}
	}

	return nil
}

// SetRequestConfigs updates all Rules
func (c *Config) SetRequestConfigs(requestConfigs []typ.Rule) error {
	c.mu.Lock()
	defer c.mu.Unlock()

	ensureRuleUUIDs(requestConfigs)
	c.Rules = requestConfigs

	return c.Save()
}

// UpdateRequestConfigAt updates the Rule at the given index
func (c *Config) UpdateRequestConfigAt(index int, reqConfig typ.Rule) error {
	c.mu.Lock()
	defer c.mu.Unlock()

	if index < 0 || index >= len(c.Rules) {
		return fmt.Errorf("index %d is out of bounds for Rules (length %d)", index, len(c.Rules))
	}

	// A payload without a UUID keeps the identity it is replacing.
	if reqConfig.UUID == "" {
		reqConfig.UUID = c.Rules[index].UUID
	}
	ensureRuleUUID(&reqConfig)

	c.Rules[index] = reqConfig
	return c.Save()
}

// UpdateRequestConfigByRequestModel updates a Rule by its request model name
func (c *Config) UpdateRequestConfigByRequestModel(requestModel string, reqConfig typ.Rule) error {
	c.mu.Lock()
	defer c.mu.Unlock()

	for i, rule := range c.Rules {
		if rule.RequestModel == requestModel {
			// A payload without a UUID keeps the identity it is replacing.
			if reqConfig.UUID == "" {
				reqConfig.UUID = rule.UUID
			}
			ensureRuleUUID(&reqConfig)
			c.Rules[i] = reqConfig
			return c.Save()
		}
	}

	return fmt.Errorf("rule with request model '%s' not found", requestModel)
}

// UpdateRequestConfigByUUID updates a Rule by its UUID
func (c *Config) UpdateRequestConfigByUUID(uuid string, reqConfig typ.Rule) error {
	c.mu.Lock()
	defer c.mu.Unlock()

	for i, rule := range c.Rules {
		if rule.UUID == uuid {
			// A payload without a UUID keeps the identity it is replacing.
			if reqConfig.UUID == "" {
				reqConfig.UUID = uuid
			}
			c.Rules[i] = reqConfig
			return c.Save()
		}
	}

	return fmt.Errorf("rule with UUID '%s' not found", uuid)
}

// AddOrUpdateRequestConfigByRequestModel adds a new Rule or updates an existing one by request model name
func (c *Config) AddOrUpdateRequestConfigByRequestModel(reqConfig typ.Rule) error {
	c.mu.Lock()
	defer c.mu.Unlock()

	for i, rule := range c.Rules {
		if rule.RequestModel == reqConfig.RequestModel {
			// A payload without a UUID keeps the identity it is replacing.
			if reqConfig.UUID == "" {
				reqConfig.UUID = rule.UUID
			}
			ensureRuleUUID(&reqConfig)
			c.Rules[i] = reqConfig
			return c.Save()
		}
	}

	// Rule not found, add new one
	ensureRuleUUID(&reqConfig)
	c.Rules = append(c.Rules, reqConfig)
	return c.Save()
}

// RemoveRequestConfig removes the Rule at the given index
func (c *Config) RemoveRequestConfig(index int) error {
	c.mu.Lock()
	defer c.mu.Unlock()

	if index < 0 || index >= len(c.Rules) {
		return fmt.Errorf("index %d is out of bounds for Rules (length %d)", index, len(c.Rules))
	}

	c.Rules = append(c.Rules[:index], c.Rules[index+1:]...)

	// Adjust DefaultRequestID after removal
	if len(c.Rules) == 0 {
		c.DefaultRequestID = -1
	} else if c.DefaultRequestID >= len(c.Rules) {
		c.DefaultRequestID = len(c.Rules) - 1
	}

	return c.Save()
}

// GetRequestModel returns the request model from the default Rule
func (c *Config) GetRequestModel() string {
	c.mu.RLock()
	defer c.mu.RUnlock()

	if c.DefaultRequestID >= 0 && c.DefaultRequestID < len(c.Rules) {
		return c.Rules[c.DefaultRequestID].RequestModel
	}
	return ""
}

// GetResponseModel returns the response model from the default Rule
func (c *Config) GetResponseModel() string {
	c.mu.RLock()
	defer c.mu.RUnlock()

	if c.DefaultRequestID >= 0 && c.DefaultRequestID < len(c.Rules) {
		return c.Rules[c.DefaultRequestID].ResponseModel
	}
	return ""
}

// GetDefaults returns all default values from the default Rule
func (c *Config) GetDefaults() (requestModel, responseModel string) {
	c.mu.RLock()
	defer c.mu.RUnlock()

	if c.DefaultRequestID >= 0 && c.DefaultRequestID < len(c.Rules) {
		rc := c.Rules[c.DefaultRequestID]
		return rc.RequestModel, rc.ResponseModel
	}
	return "", ""
}

// IsTacticValid checks if the tactic params are valid (not zero values)
func IsTacticValid(tactic *typ.Tactic) bool {
	if tactic.Params == nil {
		return false
	}

	// Check for invalid zero values in params
	switch p := tactic.Params.(type) {
	case *typ.RandomParams:
		// Random params has no fields, always valid if not nil
		return true
	case typ.RandomParams:
		return true
	case *typ.TierParams:
		return tactic.Type == loadbalance.TacticTier && p.WithinTierTactic != loadbalance.TacticTier
	case typ.TierParams:
		return tactic.Type == loadbalance.TacticTier && p.WithinTierTactic != loadbalance.TacticTier
	default:
		// Unknown params type, treat as invalid
		return false
	}
}

func (c *Config) DeleteRule(ruleUUID string) error {
	c.mu.Lock()
	defer c.mu.Unlock()

	var found = false
	var index = 0
	for i := range c.Rules {
		if c.Rules[i].UUID == ruleUUID {
			index = i
			found = true
		}
	}

	if !found {
		// Rule not found - return an error
		return fmt.Errorf("rule with UUID %s not found", ruleUUID)
	}

	c.Rules = append(c.Rules[:index], c.Rules[index+1:]...)
	return c.Save()
}

// validateSmartRoutingRules rejects invalid configured predicates before saving a rule.
// Empty smart-routing blocks are editor drafts: the frontend creates one before
// the user adds conditions and services, so full rule validation remains in
// Router construction when the rule can actually be evaluated.
func validateSmartRoutingRules(rule typ.Rule) error {
	if !rule.SmartEnabled {
		return nil
	}
	for i := range rule.SmartRouting {
		for j := range rule.SmartRouting[i].Ops {
			if err := smartrouting.ValidateSmartOp(&rule.SmartRouting[i].Ops[j]); err != nil {
				return fmt.Errorf("invalid smart routing rule[%d] op[%d]: %w", i, j, err)
			}
		}
	}
	return nil
}

// normalizeRuleServiceTiers keeps every service pool's tier numbering
// contiguous from 0 (T0 always exists, no gaps): deleting or moving the last
// T0 service automatically promotes the tiers below. The rule's default pool
// and each smart-routing partition are independent pools, normalized
// separately. Returns true when any tier was rewritten.
func normalizeRuleServiceTiers(rule *typ.Rule) bool {
	changed := loadbalance.NormalizeServiceTiers(rule.Services)
	for i := range rule.SmartRouting {
		if loadbalance.NormalizeServiceTiers(rule.SmartRouting[i].Services) {
			changed = true
		}
	}
	return changed
}

// validateRuleServices checks provider references incrementally: only
// references *newly introduced* by this save (absent from the persisted rule,
// when one exists) must point at an existing, enabled provider — those are
// genuine input errors (typo, stale UUID, picking a disabled provider outside
// the normal UI flow). References the rule already carried are always allowed
// through, even when their provider has since been disabled or deleted:
// disabling a provider is a temporary, reversible state the runtime already
// tolerates (the selector skips such services at dispatch time), and blocking
// every edit of the rule — tier moves, renames, even removing the dead
// reference itself — would make the rule read-only from an unrelated surface.
func (c *Config) validateRuleServices(rule typ.Rule, existing *typ.Rule) error {
	if c.providerStore == nil {
		return nil // Skip validation if provider store is not initialized
	}

	// Grandfathering is per exact (provider, model) reference, not per
	// provider UUID: a provider-level pass would also waive validation for
	// brand-new services that happen to reuse an already-referenced (but
	// disabled/deleted) provider, letting a rule accumulate fresh dangling
	// references. Pair granularity keeps the promised edits working — tier
	// moves, renames, removing the dead reference — while any new reference
	// still has to point at an existing, enabled provider.
	var grandfathered map[string]struct{}
	if existing != nil {
		grandfathered = make(map[string]struct{})
		addRefs := func(services []*loadbalance.Service) {
			for _, svc := range services {
				if svc != nil {
					grandfathered[svc.ServiceID()] = struct{}{}
				}
			}
		}
		addRefs(existing.Services)
		for _, sr := range existing.SmartRouting {
			addRefs(sr.Services)
		}
	}

	check := func(svc *loadbalance.Service, context string) error {
		if _, ok := grandfathered[svc.ServiceID()]; ok {
			return nil
		}
		provider, err := c.providerStore.GetByUUID(svc.Provider)
		if err != nil {
			return fmt.Errorf("%s references non-existent provider '%s': %w", context, svc.Provider, err)
		}
		if provider == nil {
			return fmt.Errorf("%s references non-existent provider '%s'", context, svc.Provider)
		}
		if !provider.Enabled {
			return fmt.Errorf("%s references disabled provider '%s'", context, svc.Provider)
		}
		return nil
	}

	for _, svc := range rule.Services {
		if svc == nil {
			continue
		}
		if err := check(svc, "service"); err != nil {
			return err
		}
	}

	for _, sr := range rule.SmartRouting {
		for _, svc := range sr.Services {
			if svc == nil {
				continue
			}
			if err := check(svc, "smart routing service"); err != nil {
				return err
			}
		}
	}

	return nil
}
