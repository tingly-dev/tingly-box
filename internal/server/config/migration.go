package config

import (
	"time"

	"github.com/google/uuid"
	"github.com/sirupsen/logrus"

	"github.com/tingly-dev/tingly-box/internal/constant"
	"github.com/tingly-dev/tingly-box/internal/loadbalance"
	typ "github.com/tingly-dev/tingly-box/internal/typ"
)

// Built-in rule UUID constants
const (
	RuleUUIDTingly            = "tingly"
	RuleUUIDBuiltinOpenAI     = "built-in-openai"
	RuleUUIDBuiltinAnthropic  = "built-in-anthropic"
	RuleUUIDBuiltinCodex      = "built-in-codex"
	RuleUUIDBuiltinCC         = "built-in-cc"
	RuleUUIDClaudeCode        = "claude-code"
	RuleUUIDBuiltinCCHaiku    = "built-in-cc-haiku"
	RuleUUIDBuiltinCCSonnet   = "built-in-cc-sonnet"
	RuleUUIDBuiltinCCOpus     = "built-in-cc-opus"
	RuleUUIDBuiltinCCDefault  = "built-in-cc-default"
	RuleUUIDBuiltinCCSubagent = "built-in-cc-subagent"

	// Claude Desktop built-in rules (locked, using builtin: prefix)
	RuleUUIDBuiltinClaudeDesktopSonnet46 = "builtin:claude_desktop:claude-sonnet-4-6"
	RuleUUIDBuiltinClaudeDesktopOpus46   = "builtin:claude_desktop:claude-opus-4-6"
	RuleUUIDBuiltinClaudeDesktopOpus47   = "builtin:claude_desktop:claude-opus-4-7"
	RuleUUIDBuiltinClaudeDesktopHaiku45  = "builtin:claude_desktop:claude-haiku-4-5"
)

func Migrate(c *Config) error {
	migrate20251220(c)
	migrate20251221(c)
	migrate20251225(c)
	migrate20260103(c)
	migrate20260110(c)
	migrate20260114(c)
	migrate20260210(c)
	migrate20260306(c)
	migrate20260416(c) // Enable multi-tenant by default
	migrate20260421(c) // Migrate profile unified model from "*" to "cc"
	migrate20260502(c) // Remove wildcard (*) rules for smart_guide scenario
	migrate20260513(c) // Add Claude Desktop built-in rules
	migrate20260521(c) // Add Claude Desktop haiku-4-5 built-in rule
	migrate20260517(c) // Rewrite 127.0.0.1 to localhost in tingly-owned agent configs
	migrate20260518(c) // Set OpenAIEndpointMode=responses on existing Codex OAuth providers
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
				Type:   loadbalance.TacticAdaptive,
				Params: typ.DefaultAdaptiveParams(),
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
		RuleUUIDBuiltinCodex:      typ.ScenarioCodex,
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

// migrate20260110 copies services from built-in-cc to built-in-cc-* rules if they are empty.
// This migration runs only once; subsequent restarts skip it so that a user who intentionally
// clears a rule's services does not have them silently refilled.
func migrate20260110(c *Config) {
	if c.hasMigrationCompleted("20260110") {
		return
	}

	// Find the source rule (built-in-cc)
	var fallbackRule *typ.Rule
	for i := range c.Rules {
		if c.Rules[i].UUID == RuleUUIDBuiltinCC {
			fallbackRule = &c.Rules[i]
			break
		}
	}

	// If source rule doesn't exist or has no services, nothing to copy.
	// Still mark as completed so we don't keep retrying on every restart.
	if fallbackRule == nil || len(fallbackRule.Services) == 0 {
		c.markMigrationCompleted("20260110")
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

	needsSave := false
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

	c.markMigrationCompleted("20260110")
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
// This migration runs only once so that a user who intentionally clears the subagent
// rule's services does not have them silently restored on the next restart.
func migrate20260210(c *Config) {
	if c.hasMigrationCompleted("20260210") {
		return
	}

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
	// Mark as completed so we don't retry every restart.
	if haikuRule == nil {
		c.markMigrationCompleted("20260210")
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
			c.markMigrationCompleted("20260210")
			if needsSave {
				_ = c.Save()
			}
			return
		}
	}

	// Keep subagent request model as-is; only mirror services.
	// Mirror haiku's services if subagent has none.
	if len(subagentRule.Services) == 0 && len(haikuRule.Services) > 0 {
		subagentRule.Services = make([]*loadbalance.Service, len(haikuRule.Services))
		copy(subagentRule.Services, haikuRule.Services)
		needsSave = true
	}

	c.markMigrationCompleted("20260210")
	if needsSave {
		_ = c.Save()
	}
}

func migrate20260306(c *Config) {
	for _, rule := range c.Rules {
		if rule.UUID == RuleUUIDBuiltinCodex {
			return
		}
	}

	for _, defaultRule := range DefaultRules {
		if defaultRule.UUID == RuleUUIDBuiltinCodex {
			c.Rules = append(c.Rules, defaultRule)
			_ = c.Save()
			return
		}
	}
}

// migrate20260416 enables multi-tenant by default for existing configurations
func migrate20260416(c *Config) {
	// Skip migration if multi-tenant config has any values set
	// This means the user has explicitly configured multi-tenant settings
	if c.MultiTenantConfig.APITokenSecret != "" ||
		c.MultiTenantConfig.APITokenAlgorithm != "" ||
		c.MultiTenantConfig.APITokenIssuer != "" {
		return
	}

	// Enable multi-tenant with default settings
	// Only set values that are not already configured
	if c.MultiTenantConfig.APITokenSecret == "" {
		c.MultiTenantConfig.APITokenSecret = generateSecret()
	}
	if c.MultiTenantConfig.APITokenAlgorithm == "" {
		c.MultiTenantConfig.APITokenAlgorithm = "HS256"
	}
	if c.MultiTenantConfig.APITokenIssuer == "" {
		c.MultiTenantConfig.APITokenIssuer = "tingly-box"
	}
	// Always enable multi-tenant (this is the main purpose of the migration)
	c.MultiTenantConfig.Enabled = true
	// Keep global token enabled for backward compatibility

	_ = c.Save()
}

// migrate20260421 migrates profile unified model name from "*" to "cc"
// This ensures consistency with the new naming convention where profile
// rules use simplified names: "cc" (unified), "default", "haiku", etc. (separate)
// Only applies to claude-code scenario profiles.
func migrate20260421(c *Config) {
	needsSave := false

	for i := range c.Rules {
		rule := &c.Rules[i]

		// Only migrate claude-code profile rules
		// Profile rules have scenario like "claude-code:profileID"
		if !typ.IsProfiledScenario(rule.Scenario) {
			continue
		}
		// Check if base scenario is claude-code
		baseScenario, _ := typ.ParseScenarioProfile(rule.Scenario)
		if baseScenario != typ.ScenarioClaudeCode {
			continue
		}

		// Migrate "*" to "cc" for unified mode
		if rule.RequestModel == "*" {
			rule.RequestModel = "cc"
			needsSave = true
		}
	}

	if needsSave {
		_ = c.Save()
	}
}

// migrate20260502 removes wildcard (*) rules for smart_guide scenario
// This cleans up legacy wildcard rules that are no longer needed
// as SmartGuide now uses bot-specific rules with UUID pattern: _internal_smart_guide_{botUUID}
func migrate20260502(c *Config) {
	needsSave := false

	// Filter out smart_guide rules with wildcard RequestModel
	var filteredRules []typ.Rule
	for _, rule := range c.Rules {
		// Skip smart_guide rules with wildcard RequestModel
		if rule.Scenario == typ.ScenarioSmartGuide && rule.RequestModel == "*" {
			logrus.WithFields(logrus.Fields{
				"rule_uuid":      rule.UUID,
				"request_model":  rule.RequestModel,
				"response_model": rule.ResponseModel,
			}).Info("Removing smart_guide wildcard rule")
			needsSave = true
			continue
		}
		filteredRules = append(filteredRules, rule)
	}

	if needsSave {
		c.Rules = filteredRules
		_ = c.Save()
		logrus.Info("Migration 2026-05-02 completed: removed smart_guide wildcard rules")
	}
}

// migrate20260513 adds Claude Desktop built-in rules
// Claude Desktop has 3 locked rules with builtin: UUID format:
// - builtin:claude_desktop:claude-sonnet-4-6
// - builtin:claude_desktop:claude-opus-4-6
// - builtin:claude_desktop:claude-opus-4-7
func migrate20260513(c *Config) {
	needsSave := false

	// Check if Claude Desktop rules already exist
	existingRules := map[string]bool{
		RuleUUIDBuiltinClaudeDesktopSonnet46: false,
		RuleUUIDBuiltinClaudeDesktopOpus46:   false,
		RuleUUIDBuiltinClaudeDesktopOpus47:   false,
	}

	for _, rule := range c.Rules {
		if _, exists := existingRules[rule.UUID]; exists {
			existingRules[rule.UUID] = true
		}
	}

	// Find a reference rule to copy services from (prefer claude_code or codex)
	var referenceRule *typ.Rule
	for i := range c.Rules {
		rule := &c.Rules[i]
		if rule.Scenario == typ.ScenarioClaudeCode || rule.Scenario == typ.ScenarioCodex {
			if len(rule.Services) > 0 {
				referenceRule = rule
				break
			}
		}
	}

	// Add missing Claude Desktop rules
	for _, defaultRule := range DefaultRules {
		uuid := defaultRule.UUID
		alreadyExists, exists := existingRules[uuid]
		if !exists {
			continue // Not a Claude Desktop rule
		}

		if alreadyExists {
			continue // Rule already exists, skip
		}

		// Add the rule
		newRule := defaultRule
		// Copy services from reference rule if available
		if referenceRule != nil && len(referenceRule.Services) > 0 {
			newRule.Services = make([]*loadbalance.Service, len(referenceRule.Services))
			copy(newRule.Services, referenceRule.Services)
		}

		c.Rules = append(c.Rules, newRule)
		needsSave = true

		logrus.WithFields(logrus.Fields{
			"rule_uuid":      newRule.UUID,
			"request_model":  newRule.RequestModel,
			"response_model": newRule.ResponseModel,
		}).Info("Added Claude Desktop built-in rule")
	}

	if needsSave {
		_ = c.Save()
		logrus.Info("Migration 2026-05-13 completed: added Claude Desktop built-in rules")
	}
}

// migrate20260521 adds the claude-haiku-4-5 built-in rule for Claude Desktop
func migrate20260521(c *Config) {
	for _, rule := range c.Rules {
		if rule.UUID == RuleUUIDBuiltinClaudeDesktopHaiku45 {
			return
		}
	}

	var referenceRule *typ.Rule
	for i := range c.Rules {
		rule := &c.Rules[i]
		if rule.Scenario == typ.ScenarioClaudeDesktop && len(rule.Services) > 0 {
			referenceRule = rule
			break
		}
	}

	newRule := typ.Rule{
		UUID:         RuleUUIDBuiltinClaudeDesktopHaiku45,
		Scenario:     typ.ScenarioClaudeDesktop,
		RequestModel: "claude-haiku-4-5",
		Description:  "Claude Desktop - Haiku 4.5 model for fast responses",
		Services:     []*loadbalance.Service{},
		LBTactic: typ.Tactic{
			Type:   loadbalance.TacticAdaptive,
			Params: typ.DefaultAdaptiveParams(),
		},
		Active: true,
	}
	if referenceRule != nil && len(referenceRule.Services) > 0 {
		newRule.Services = make([]*loadbalance.Service, len(referenceRule.Services))
		copy(newRule.Services, referenceRule.Services)
	}

	c.Rules = append(c.Rules, newRule)
	_ = c.Save()
	logrus.Info("Migration 2026-05-21 completed: added Claude Desktop haiku-4-5 rule")
}
