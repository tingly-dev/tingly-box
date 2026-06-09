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

// --- Shared migration helpers -------------------------------------------------
//
// Several migrations seed or repair built-in rules with the same moves: look a
// rule up by UUID, pull a template from DefaultRules, and copy a reference
// rule's upstream services onto a freshly added rule. These helpers centralize
// those moves so each migration reads as intent, not bookkeeping.

// findRuleByUUID returns a pointer to the rule with the given UUID, or nil.
func (c *Config) findRuleByUUID(uuid string) *typ.Rule {
	for i := range c.Rules {
		if c.Rules[i].UUID == uuid {
			return &c.Rules[i]
		}
	}
	return nil
}

// defaultRuleByUUID looks up a built-in rule template (from DefaultRules) by UUID.
func defaultRuleByUUID(uuid string) (typ.Rule, bool) {
	for _, r := range DefaultRules {
		if r.UUID == uuid {
			return r, true
		}
	}
	return typ.Rule{}, false
}

// cloneServices returns a shallow copy of a load-balancing services slice
// (nil for an empty input) so a seeded rule gets its own slice header rather
// than aliasing the source rule's.
func cloneServices(src []*loadbalance.Service) []*loadbalance.Service {
	if len(src) == 0 {
		return nil
	}
	dst := make([]*loadbalance.Service, len(src))
	copy(dst, src)
	return dst
}

// referenceServicesFor returns a copy of the services of the first rule whose
// scenario matches one of the given scenarios and that has a non-empty service
// list, or nil when none exists. Used to seed a new built-in rule with the same
// upstream services the user already configured for a sibling scenario.
func (c *Config) referenceServicesFor(scenarios ...typ.RuleScenario) []*loadbalance.Service {
	for i := range c.Rules {
		rule := &c.Rules[i]
		if len(rule.Services) == 0 {
			continue
		}
		for _, s := range scenarios {
			if rule.Scenario.Is(s) {
				return cloneServices(rule.Services)
			}
		}
	}
	return nil
}

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
	migrate20260517(c) // Rewrite 127.0.0.1 to localhost in tingly-owned agent configs
	migrate20260518(c) // Set OpenAIEndpointMode=responses on existing Codex OAuth providers
	migrate20260521(c) // Add Claude Desktop haiku-4-5 built-in rule
	migrate20260606(c) // Default SkipUsage on for the Xcode scenario
	migrate20260610(c) // Seed default rule flags (claude_code_compat / clean_header / session_affinity) for CC / Desktop / Codex rules
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

// migrate20251225 clamps any legacy JSON-config provider timeout above the max
// to the cap. Runs before migrateProvidersToDB, so it only affects the one-time
// JSON→DB import; once providers live in SQLite the JSON list is empty and this
// is a no-op.
func migrate20251225(c *Config) {
	needsSave := false
	for _, p := range c.Providers {
		if p.Timeout > int64(constant.DefaultMaxTimeout) {
			p.Timeout = int64(constant.DefaultMaxTimeout)
			needsSave = true
		}
	}
	if needsSave {
		_ = c.Save()
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

	// Source rule (built-in-cc). If it's missing or has no services there's
	// nothing to copy — still mark completed so we don't retry every restart.
	fallbackRule := c.findRuleByUUID(RuleUUIDBuiltinCC)
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

	needsSave := false
	for _, targetUUID := range targetUUIDs {
		defaultRule, ok := defaultRuleByUUID(targetUUID)
		if !ok {
			continue
		}
		target := c.findRuleByUUID(targetUUID)
		if target == nil {
			continue
		}
		target.Description = defaultRule.Description

		// Only fill services when the target has none of its own.
		if len(target.Services) > 0 {
			continue
		}
		target.Services = cloneServices(fallbackRule.Services)
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

	// Without a haiku model, there's nothing to mirror.
	// Mark as completed so we don't retry every restart.
	haikuRule := c.findRuleByUUID(RuleUUIDBuiltinCCHaiku)
	if haikuRule == nil {
		c.markMigrationCompleted("20260210")
		return
	}

	needsSave := false

	// Ensure subagent rule exists (add the default template if missing).
	subagentRule := c.findRuleByUUID(RuleUUIDBuiltinCCSubagent)
	if subagentRule == nil {
		defaultRule, ok := defaultRuleByUUID(RuleUUIDBuiltinCCSubagent)
		if !ok {
			c.markMigrationCompleted("20260210")
			return
		}
		c.Rules = append(c.Rules, defaultRule)
		subagentRule = &c.Rules[len(c.Rules)-1]
		needsSave = true
	}

	// Keep subagent request model as-is; mirror haiku's services if it has none.
	if len(subagentRule.Services) == 0 && len(haikuRule.Services) > 0 {
		subagentRule.Services = cloneServices(haikuRule.Services)
		needsSave = true
	}

	c.markMigrationCompleted("20260210")
	if needsSave {
		_ = c.Save()
	}
}

// migrate20260306 adds the built-in Codex rule if it's missing.
func migrate20260306(c *Config) {
	if c.findRuleByUUID(RuleUUIDBuiltinCodex) != nil {
		return
	}
	if defaultRule, ok := defaultRuleByUUID(RuleUUIDBuiltinCodex); ok {
		c.Rules = append(c.Rules, defaultRule)
		_ = c.Save()
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

	// All three token fields are empty (guaranteed by the guard above), so seed
	// the defaults and enable multi-tenant — the main purpose of the migration.
	c.MultiTenantConfig.APITokenSecret = generateSecret()
	c.MultiTenantConfig.APITokenAlgorithm = "HS256"
	c.MultiTenantConfig.APITokenIssuer = "tingly-box"
	c.MultiTenantConfig.Enabled = true

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
	desktopUUIDs := []string{
		RuleUUIDBuiltinClaudeDesktopSonnet46,
		RuleUUIDBuiltinClaudeDesktopOpus46,
		RuleUUIDBuiltinClaudeDesktopOpus47,
	}

	// Seed new desktop rules with the services the user already uses for a
	// sibling agent scenario (prefer claude_code, then codex).
	refServices := c.referenceServicesFor(typ.ScenarioClaudeCode, typ.ScenarioCodex)

	needsSave := false
	for _, uuid := range desktopUUIDs {
		if c.findRuleByUUID(uuid) != nil {
			continue // Rule already exists, skip
		}
		newRule, ok := defaultRuleByUUID(uuid)
		if !ok {
			continue
		}
		if services := cloneServices(refServices); services != nil {
			newRule.Services = services
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

// migrate20260521 adds the claude-haiku-4-5 built-in rule for Claude Desktop.
func migrate20260521(c *Config) {
	if c.findRuleByUUID(RuleUUIDBuiltinClaudeDesktopHaiku45) != nil {
		return
	}

	// Use the init.go template so this rule's flags/tactic stay in sync with the
	// other built-in Claude Desktop rules instead of drifting from a local copy.
	newRule, ok := defaultRuleByUUID(RuleUUIDBuiltinClaudeDesktopHaiku45)
	if !ok {
		return
	}
	if services := c.referenceServicesFor(typ.ScenarioClaudeDesktop); services != nil {
		newRule.Services = services
	}

	c.Rules = append(c.Rules, newRule)
	_ = c.Save()
	logrus.Info("Migration 2026-05-21 completed: added Claude Desktop haiku-4-5 rule")
}

// migrate20260606 ensures the Xcode scenario defaults SkipUsage on (the Xcode
// client cannot handle usage in streaming chunks).
//
// This migration originally also seeded scenario-level session_affinity for the
// IDE/Agent scenarios. session_affinity has since been downgraded to a rule-only
// flag (seeded on the built-in Claude Code / Desktop / Codex rules via init +
// migrate20260610), so the affinity seeding has been removed; only the Xcode
// SkipUsage default remains.
func migrate20260606(c *Config) {
	if c.hasMigrationCompleted("20260606") {
		return
	}

	needsSave := false

	// Add or update the xcode scenario config with SkipUsage on.
	found := false
	for i := range c.Scenarios {
		if c.Scenarios[i].Scenario == typ.ScenarioXcode {
			// Set SkipUsage if user hasn't explicitly set it.
			// Note: false means "not set" due to omitempty, true means explicitly enabled.
			if !c.Scenarios[i].Flags.SkipUsage {
				c.Scenarios[i].Flags.SkipUsage = true
				needsSave = true
			}
			found = true
			break
		}
	}
	if !found {
		c.Scenarios = append(c.Scenarios, typ.ScenarioConfig{
			Scenario: typ.ScenarioXcode,
			Flags:    typ.ScenarioFlags{SkipUsage: true},
		})
		needsSave = true
	}

	c.markMigrationCompleted("20260606")
	if needsSave {
		_ = c.Save()
		logrus.Info("Migration 2026-06-06 completed: defaulted SkipUsage on for the Xcode scenario")
	}
}

// migrate20260610 seeds the default rule-level flags for the built-in agent
// scenarios in a single pass, consolidating the earlier per-flag/per-scenario
// migrations (claude_code_compat, clean_header, session_affinity) that were
// fragmented across 20260608*/20260609*. For each existing rule it mirrors what
// init.go seeds on the built-in rules:
//
//   - Claude Code / Claude Desktop rules (base scenario, profile-aware):
//       claude_code_compat on, clean_header on, session_affinity = 1800s
//   - Codex rules: session_affinity = 1800s
//
// Rationale per flag:
//   - claude_code_compat: CC / Desktop emit mid-conversation system-role
//     messages that third-party Anthropic-compatible providers reject.
//   - clean_header: CC / Desktop inject x-anthropic-billing-header blocks that
//     must never leak to external providers.
//   - session_affinity: 30-min session pinning improves cache hit rate.
//
// Each default is applied only when the rule hasn't set it yet (false / 0 = not
// set), and the whole pass is gated by one marker so a user who later turns any
// flag off (or affinity to 0) keeps it off across restarts. New installs get
// these straight from init.go (ccRule / cdRule / built-in-codex); this migration
// covers configs persisted before the defaults existed.
func migrate20260610(c *Config) {
	if c.hasMigrationCompleted("20260610") {
		return
	}

	needsSave := false
	for i := range c.Rules {
		base, _ := typ.ParseScenarioProfile(c.Rules[i].Scenario)
		switch base {
		case typ.ScenarioClaudeCode, typ.ScenarioClaudeDesktop:
			if !c.Rules[i].Flags.ClaudeCodeCompat {
				c.Rules[i].Flags.ClaudeCodeCompat = true
				needsSave = true
			}
			if !c.Rules[i].Flags.CleanHeader {
				c.Rules[i].Flags.CleanHeader = true
				needsSave = true
			}
			if c.Rules[i].Flags.SessionAffinity == 0 {
				c.Rules[i].Flags.SessionAffinity = defaultSessionAffinitySeconds
				needsSave = true
			}
		case typ.ScenarioCodex:
			if c.Rules[i].Flags.SessionAffinity == 0 {
				c.Rules[i].Flags.SessionAffinity = defaultSessionAffinitySeconds
				needsSave = true
			}
		}
	}

	c.markMigrationCompleted("20260610")
	if needsSave {
		_ = c.Save()
		logrus.Info("Migration 20260610 completed: seeded default rule flags (claude_code_compat / clean_header / session_affinity) for Claude Code, Claude Desktop, and Codex rules")
	}
}
