package config

import (
	"time"

	"github.com/google/uuid"

	"tingly-box/internal/constant"
	"tingly-box/internal/loadbalance"
	typ "tingly-box/internal/typ"
)

// Built-in rule UUID constants
const (
	RuleUUIDTingly           = "tingly"
	RuleUUIDBuiltinOpenAI    = "built-in-openai"
	RuleUUIDBuiltinAnthropic = "built-in-anthropic"
	RuleUUIDBuiltinCC        = "built-in-cc"
	RuleUUIDClaudeCode       = "claude-code"
	RuleUUIDBuiltinCCHaiku   = "built-in-cc-haiku"
	RuleUUIDBuiltinCCSonnet  = "built-in-cc-sonnet"
	RuleUUIDBuiltinCCOpus    = "built-in-cc-opus"
	RuleUUIDBuiltinCCDefault = "built-in-cc-default"
)

func Migrate(c *Config) error {
	migrate20251220(c)
	migrate20251221(c)
	migrate20251225(c)
	migrate20260103(c)
	migrate20260110(c)
	migrate20260114(c)
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
		RuleUUIDTingly:           typ.ScenarioOpenAI,
		RuleUUIDBuiltinOpenAI:    typ.ScenarioOpenAI,
		RuleUUIDBuiltinAnthropic: typ.ScenarioAnthropic,
		RuleUUIDBuiltinCC:        typ.ScenarioClaudeCode,
		RuleUUIDClaudeCode:       typ.ScenarioClaudeCode,
		RuleUUIDBuiltinCCHaiku:   typ.ScenarioClaudeCode,
		RuleUUIDBuiltinCCSonnet:  typ.ScenarioClaudeCode,
		RuleUUIDBuiltinCCOpus:    typ.ScenarioClaudeCode,
		RuleUUIDBuiltinCCDefault: typ.ScenarioClaudeCode,
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
	var sourceRule *typ.Rule
	for i := range c.Rules {
		if c.Rules[i].UUID == RuleUUIDBuiltinCC {
			sourceRule = &c.Rules[i]
			break
		}
	}

	// If source rule doesn't exist or has no services, skip migration
	if sourceRule == nil || len(sourceRule.Services) == 0 {
		return
	}

	// built-in-cc-* rule UUIDs that should inherit from built-in-cc
	targetRules := []string{
		RuleUUIDBuiltinCCHaiku,
		RuleUUIDBuiltinCCSonnet,
		RuleUUIDBuiltinCCOpus,
		RuleUUIDBuiltinCCDefault,
	}

	for i := range c.Rules {
		rule := &c.Rules[i]

		// Check if this is a target rule
		isTarget := false
		for _, targetUUID := range targetRules {
			if rule.UUID == targetUUID {
				isTarget = true
				break
			}
		}

		if !isTarget {
			continue
		}

		rule.Description = sourceRule.Description

		// If services is not empty, skip
		if len(rule.Services) > 0 {
			continue
		}

		// Copy services from source rule
		rule.Services = make([]loadbalance.Service, len(sourceRule.Services))
		copy(rule.Services, sourceRule.Services)
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
