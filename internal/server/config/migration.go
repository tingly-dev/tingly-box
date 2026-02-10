package config

import (
	"time"

	"github.com/google/uuid"

	"github.com/tingly-dev/tingly-box/internal/constant"
	"github.com/tingly-dev/tingly-box/internal/loadbalance"
	typ "github.com/tingly-dev/tingly-box/internal/typ"
)

// Built-in rule UUID constants
const (
	RuleUUIDTingly            = "tingly"
	RuleUUIDBuiltinOpenAI     = "built-in-openai"
	RuleUUIDBuiltinAnthropic  = "built-in-anthropic"
	RuleUUIDBuiltinCC         = "built-in-cc"
	RuleUUIDClaudeCode        = "claude-code"
	RuleUUIDBuiltinCCHaiku    = "built-in-cc-haiku"
	RuleUUIDBuiltinCCSonnet   = "built-in-cc-sonnet"
	RuleUUIDBuiltinCCOpus     = "built-in-cc-opus"
	RuleUUIDBuiltinCCDefault  = "built-in-cc-default"
	RuleUUIDBuiltinCCSubagent = "built-in-cc-subagent"
)

func Migrate(c *Config) error {
	migrate20251220(c)
	migrate20251221(c)
	migrate20251225(c)
	migrate20260103(c)
	migrate20260110(c)
	migrate20260114(c)
	migrate20260210(c)
	return nil
}

// migrate20251220 ensures all rules have proper UUID and LBTactic set
func migrate20251220(c *Config) {
	needsSave := false
	for i := range c.Rules {
		// Ensure UUID exists
		if c.Rules[i].UUID == "" {
			uid, err := uuid.NewUUID()
			if err != nil {
				continue
			}
			c.Rules[i].UUID = uid.String()
			needsSave = true
		}

		// Ensure LBTactic is properly initialized
		// Check if params are nil or have invalid zero values
		if !IsTacticValid(&c.Rules[i].LBTactic) {
			// Set default tactic if params are invalid
			c.Rules[i].LBTactic = typ.Tactic{
				Type:   loadbalance.TacticRoundRobin,
				Params: typ.DefaultRoundRobinParams(),
			}
			needsSave = true
		}
	}

	// Save if any rules were updated
	if needsSave {
		_ = c.Save()
	}
}

// migrate20251221 migrates provider configurations from v1 to v2 format
func migrate20251221(c *Config) {
	needsSave := false

	// Ensure all providers have a valid timeout (set to default if zero)
	for _, p := range c.Providers {
		if p.Timeout == 0 {
			p.Timeout = int64(constant.DefaultRequestTimeout)
			needsSave = true
		}
	}

	// Skip migration if Providers is already populated
	if len(c.Providers) > 0 {
		if needsSave {
			_ = c.Save()
		}
		return
	}

	// Check if there are v1 providers to migrate
	if len(c.ProvidersV1) == 0 {
		return
	}

	// Initialize Providers slice
	c.Providers = make([]*typ.Provider, 0, len(c.Providers))

	// Migrate each v1 provider to v2
	for _, pv1 := range c.ProvidersV1 {
		providerV2 := &typ.Provider{
			UUID:        pv1.UUID,
			Name:        pv1.Name,
			APIBase:     pv1.APIBase,
			APIStyle:    pv1.APIStyle,
			Token:       pv1.Token,
			Enabled:     pv1.Enabled,
			ProxyURL:    pv1.ProxyURL,
			Timeout:     int64(constant.DefaultRequestTimeout), // Default timeout from constants
			Tags:        []string{},                            // Empty tags
			Models:      []string{},                            // Empty models initially
			LastUpdated: time.Now().Format(time.RFC3339),
		}

		// Generate UUID if not present in v1
		if providerV2.UUID == "" {
			providerV2.UUID = GenerateUUID()
		}

		c.Providers = append(c.Providers, providerV2)
	}

	// Only mark for save if migration actually occurred
	if len(c.Providers) > 0 {
		needsSave = true
	}

	for i, rule := range c.Rules {
		for j := range rule.Services {
			for _, p := range c.Providers {
				if rule.Services[j].Provider == p.Name {
					rule.Services[j].Provider = p.UUID
				}
			}
		}
		c.Rules[i] = rule
	}

	// Save if migration occurred
	if needsSave {
		_ = c.Save()
	}
}

func migrate20251225(c *Config) {
	for _, p := range c.Providers {
		// second
		if p.Timeout >= 30*60 {
			p.Timeout = int64(constant.DefaultMaxTimeout)
		}
	}
}

func migrate20260103(c *Config) {
	needsSave := false

	// Map of default rule UUIDs to their scenarios
	scenarioMap := map[string]typ.RuleScenario{
		RuleUUIDTingly:            typ.ScenarioOpenAI,
		RuleUUIDBuiltinOpenAI:     typ.ScenarioOpenAI,
		RuleUUIDBuiltinAnthropic:  typ.ScenarioAnthropic,
		RuleUUIDBuiltinCC:         typ.ScenarioClaudeCode,
		RuleUUIDClaudeCode:        typ.ScenarioClaudeCode,
		RuleUUIDBuiltinCCHaiku:    typ.ScenarioClaudeCode,
		RuleUUIDBuiltinCCSonnet:   typ.ScenarioClaudeCode,
		RuleUUIDBuiltinCCOpus:     typ.ScenarioClaudeCode,
		RuleUUIDBuiltinCCDefault:  typ.ScenarioClaudeCode,
		RuleUUIDBuiltinCCSubagent: typ.ScenarioClaudeCode,
	}

	for i := range c.Rules {
		rule := &c.Rules[i]

		// If scenario is already set, skip
		if rule.Scenario != "" {
			continue
		}

		// Check if this is a default rule and set its scenario
		if scenario, ok := scenarioMap[rule.UUID]; ok {
			rule.Scenario = scenario
			needsSave = true
		}
	}

	if needsSave {
		_ = c.Save()
	}
}

// migrate20260110 copies services from built-in-cc to built-in-cc-* rules if they are empty
func migrate20260110(c *Config) {
	needsSave := false

	// Find the source rule (built-in-cc)
	var fallbackRule *typ.Rule
	for i := range c.Rules {
		if c.Rules[i].UUID == RuleUUIDBuiltinCC {
			fallbackRule = &c.Rules[i]
			break
		}
	}

	// If source rule doesn't exist or has no services, skip migration
	if fallbackRule == nil || len(fallbackRule.Services) == 0 {
		return
	}

	// built-in-cc-* rule UUIDs that should inherit from built-in-cc
	targetUUIDs := []string{
		RuleUUIDBuiltinCCHaiku,
		RuleUUIDBuiltinCCSonnet,
		RuleUUIDBuiltinCCOpus,
		RuleUUIDBuiltinCCDefault,
		RuleUUIDBuiltinCCSubagent,
	}

	defaultMap := map[string]typ.Rule{}

	for _, targetUUID := range targetUUIDs {
		for _, defaultRule := range DefaultRules {
			if targetUUID == defaultRule.UUID {
				defaultMap[targetUUID] = defaultRule
			}
		}
	}

	for i := range c.Rules {
		rule := &c.Rules[i]

		// Check if this is a target rule
		var defaultRule typ.Rule
		var ok bool
		if defaultRule, ok = defaultMap[rule.UUID]; !ok {
			continue
		}

		rule.Description = defaultRule.Description

		// If services is not empty, skip
		if len(rule.Services) > 0 {
			continue
		}

		// Copy services from fallback rule
		rule.Services = make([]*loadbalance.Service, len(fallbackRule.Services))
		copy(rule.Services, fallbackRule.Services)
		needsSave = true
	}

	if needsSave {
		_ = c.Save()
	}
}

// migrate20260114 for bugfix - bug which cause scenario empty
func migrate20260114(c *Config) {
	var valid []typ.Rule
	for _, r := range c.Rules {
		if r.Scenario == "" {
			continue
		}
		valid = append(valid, r)
	}

	if len(valid) != len(c.Rules) {
		c.Rules = valid
		c.Save()
	}
}

// migrate20260210 ensures the subagent rule exists and mirrors the haiku model.
func migrate20260210(c *Config) {
	// Find required rules.
	var haikuRule *typ.Rule
	var subagentRule *typ.Rule
	for i := range c.Rules {
		rule := &c.Rules[i]
		if rule.UUID == RuleUUIDBuiltinCCHaiku {
			haikuRule = rule
			continue
		}
		if rule.UUID == RuleUUIDBuiltinCCSubagent {
			subagentRule = rule
		}
	}

	// Without a haiku model, there's nothing to mirror.
	if haikuRule == nil {
		return
	}

	needsSave := false

	// Ensure subagent rule exists (add default if missing).
	if subagentRule == nil {
		for _, defaultRule := range DefaultRules {
			if defaultRule.UUID == RuleUUIDBuiltinCCSubagent {
				c.Rules = append(c.Rules, defaultRule)
				subagentRule = &c.Rules[len(c.Rules)-1]
				needsSave = true
				break
			}
		}
		if subagentRule == nil {
			if needsSave {
				_ = c.Save()
			}
			return
		}
	}

	// Mirror haiku's request model onto subagent.
	if haikuRule.RequestModel != "" && subagentRule.RequestModel != haikuRule.RequestModel {
		subagentRule.RequestModel = haikuRule.RequestModel
		needsSave = true
	}

	// Mirror haiku's services if subagent has none.
	if len(subagentRule.Services) == 0 && len(haikuRule.Services) > 0 {
		subagentRule.Services = make([]*loadbalance.Service, len(haikuRule.Services))
		copy(subagentRule.Services, haikuRule.Services)
		needsSave = true
	}

	if needsSave {
		_ = c.Save()
	}
}
