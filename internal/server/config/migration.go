package config

import (
	"time"

	"github.com/google/uuid"
	"github.com/sirupsen/logrus"

	"github.com/tingly-dev/tingly-box/internal/constant"
	"github.com/tingly-dev/tingly-box/internal/loadbalance"
	typ "github.com/tingly-dev/tingly-box/internal/typ"
)

// --- Shared migration helpers -------------------------------------------------
//
// Migrations are organized by the invariants they protect rather than by every
// historical patch date. The current supported baseline is the scenario/provider
// config model; very old config shapes are repaired through normalizeLegacyConfigBaseline.

// findRuleByUUID returns a pointer to the rule with the given UUID, or nil.
// Legacy simple-rule UUIDs (openai, anthropic, codex, …) are resolved to
// their modern "builtin:<scenario>:<model>" aliases so older callers keep
// finding the rule after normalizeBuiltinRuleIdentity has renamed it.
func (c *Config) findRuleByUUID(uuid string) *typ.Rule {
	if modern, ok := legacySimpleRuleUUIDs[uuid]; ok {
		uuid = modern
	}
	for i := range c.Rules {
		if c.Rules[i].UUID == uuid {
			return &c.Rules[i]
		}
	}
	return nil
}

// defaultRuleByUUID looks up a built-in rule template (from DefaultRules) by
// UUID. Legacy UUIDs (CC and simple scenarios) are resolved to their modern
// aliases so migrations written before the rename keep finding their templates.
func defaultRuleByUUID(uuid string) (typ.Rule, bool) {
	if modern, ok := legacyCCRuleUUIDs[uuid]; ok {
		uuid = modern
	} else if modern, ok := legacySimpleRuleUUIDs[uuid]; ok {
		uuid = modern
	}
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

// referenceServicesFor returns a copy of the services of the first requested
// scenario that has any matching rule with a non-empty service list, or nil when
// none exists. Scenario argument order is the precedence order; within a given
// scenario, the first matching rule in config order wins. Used to seed a new
// built-in rule with the same upstream services the user already configured for
// a sibling scenario.
func (c *Config) referenceServicesFor(scenarios ...typ.RuleScenario) []*loadbalance.Service {
	for _, s := range scenarios {
		for i := range c.Rules {
			rule := &c.Rules[i]
			if len(rule.Services) == 0 {
				continue
			}
			if rule.Scenario.Is(s) {
				return cloneServices(rule.Services)
			}
		}
	}
	return nil
}

func (c *Config) seedBuiltinRuleIfMissing(uuid string, services []*loadbalance.Service) (typ.Rule, bool) {
	if c.findRuleByUUID(uuid) != nil {
		return typ.Rule{}, false
	}
	newRule, ok := defaultRuleByUUID(uuid)
	if !ok {
		return typ.Rule{}, false
	}
	if cloned := cloneServices(services); cloned != nil {
		newRule.Services = cloned
	}
	c.Rules = append(c.Rules, newRule)
	return newRule, true
}

func (c *Config) saveMigration() {
	if err := c.Save(); err != nil {
		logrus.WithError(err).Warn("Failed to save config during migration")
	}
}

func (c *Config) rekeyRuleUUIDState(migrationID string, renames map[string]string) {
	for oldUUID, newUUID := range renames {
		if c.ruleStateStore != nil {
			if err := c.ruleStateStore.RenameRuleUUID(oldUUID, newUUID); err != nil {
				logrus.WithError(err).Warnf("Migration %s: failed to rename rule state %s -> %s", migrationID, oldUUID, newUUID)
			}
		}
		if c.usageStore != nil {
			if err := c.usageStore.RenameRuleUUID(oldUUID, newUUID); err != nil {
				logrus.WithError(err).Warnf("Migration %s: failed to rename usage records %s -> %s", migrationID, oldUUID, newUUID)
			}
		}
	}
}

func Migrate(c *Config) error {
	normalizeLegacyConfigBaseline(c)
	normalizeBuiltinRuleIdentity(c)
	ensureCurrentBuiltinRules(c)
	migrate20260416(c) // Enable multi-tenant by default
	migrate20260421(c) // Migrate profile unified model from "*" to "cc"
	migrate20260502(c) // Remove wildcard (*) rules for smart_guide scenario
	migrate20260518(c) // Set OpenAIEndpointMode=responses on existing Codex OAuth providers
	normalizeRuleDefaultsOnce(c)
	return nil
}

// normalizeLegacyConfigBaseline folds the pre-2026-04 config repair migrations
// into one baseline normalizer. It keeps very old configs usable without keeping
// every historical date migration as a permanent startup phase.
func normalizeLegacyConfigBaseline(c *Config) {
	needsSave := false

	if normalizeLegacyProviders(c) {
		needsSave = true
	}
	if normalizeRuleBasics(c) {
		needsSave = true
	}

	if needsSave {
		c.saveMigration()
		logrus.Info("Migration baseline normalization completed: repaired legacy provider/rule config")
	}
}

func normalizeLegacyProviders(c *Config) bool {
	needsSave := false

	for _, p := range c.Providers {
		if p.Timeout == 0 {
			p.Timeout = int64(constant.DefaultRequestTimeout)
			needsSave = true
		}
		if p.Timeout > int64(constant.DefaultMaxTimeout) {
			p.Timeout = int64(constant.DefaultMaxTimeout)
			needsSave = true
		}
	}

	if len(c.Providers) > 0 || len(c.ProvidersV1) == 0 {
		return needsSave
	}

	c.Providers = make([]*typ.Provider, 0, len(c.ProvidersV1))
	for _, pv1 := range c.ProvidersV1 {
		provider := &typ.Provider{
			UUID:        pv1.UUID,
			Name:        pv1.Name,
			APIBase:     pv1.APIBase,
			APIStyle:    pv1.APIStyle,
			Token:       pv1.Token,
			Enabled:     pv1.Enabled,
			ProxyURL:    pv1.ProxyURL,
			Timeout:     int64(constant.DefaultRequestTimeout),
			Tags:        []string{},
			Models:      []string{},
			LastUpdated: time.Now().Format(time.RFC3339),
		}
		if provider.UUID == "" {
			provider.UUID = GenerateUUID()
		}
		c.Providers = append(c.Providers, provider)
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
	return true
}

func normalizeRuleBasics(c *Config) bool {
	needsSave := false
	valid := make([]typ.Rule, 0, len(c.Rules))
	for i := range c.Rules {
		rule := c.Rules[i]

		if rule.Scenario == "" {
			if scenario, ok := legacyRuleScenario(rule.UUID); ok {
				rule.Scenario = scenario
				needsSave = true
			} else {
				needsSave = true
				continue
			}
		}

		if rule.UUID == "" {
			uid, err := uuid.NewUUID()
			if err == nil {
				rule.UUID = uid.String()
				needsSave = true
			}
		}
		normalizedTactic := normalizeRuleTactic(rule)
		if rule.LBTactic.Type != normalizedTactic.Type || !IsTacticValid(&rule.LBTactic) {
			rule.LBTactic = normalizedTactic
			needsSave = true
		}
		valid = append(valid, rule)
	}
	if len(valid) != len(c.Rules) {
		needsSave = true
	}
	c.Rules = valid
	return needsSave
}

func normalizeRuleTactic(rule typ.Rule) typ.Tactic {
	if hasMultipleServiceTiers(rule.Services) {
		return typ.Tactic{
			Type:   loadbalance.TacticTier,
			Params: typ.DefaultTierParams(),
		}
	}
	if rule.LBTactic.Type == loadbalance.TacticTier && IsTacticValid(&rule.LBTactic) {
		return rule.LBTactic
	}
	return typ.Tactic{
		Type:   loadbalance.TacticRandom,
		Params: typ.DefaultRandomParams(),
	}
}

func hasMultipleServiceTiers(services []*loadbalance.Service) bool {
	seen := make(map[int]struct{})
	for _, svc := range services {
		if svc == nil || !svc.Active {
			continue
		}
		seen[svc.Tier] = struct{}{}
		if len(seen) > 1 {
			return true
		}
	}
	return false
}

func legacyRuleScenario(uuid string) (typ.RuleScenario, bool) {
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
	scenario, ok := scenarioMap[uuid]
	return scenario, ok
}

// normalizeBuiltinRuleIdentity keeps built-in rule UUIDs on the canonical
// "builtin:<scenario>:<tier/model>" form. It is intentionally not marker-gated:
// the pass is idempotent and self-healing for configs written by older builds.
func normalizeBuiltinRuleIdentity(c *Config) {
	renames := map[string]string{}
	for i := range c.Rules {
		rule := &c.Rules[i]
		canonical, ok := canonicalRuleUUID(rule)
		if !ok || rule.UUID == canonical {
			continue
		}
		if c.findRuleByUUID(canonical) != nil {
			logrus.WithFields(logrus.Fields{
				"rule_uuid":     rule.UUID,
				"canonical":     canonical,
				"request_model": rule.RequestModel,
			}).Warn("Migration builtin-rule-identity: canonical rule UUID already taken, skipping rename")
			continue
		}
		renames[rule.UUID] = canonical
		rule.UUID = canonical
	}

	if len(renames) == 0 {
		return
	}
	c.rekeyRuleUUIDState("builtin-rule-identity", renames)
	c.saveMigration()
	logrus.Infof("Migration builtin-rule-identity completed: normalized %d built-in rule UUID(s)", len(renames))
}

func canonicalRuleUUID(rule *typ.Rule) (string, bool) {
	if canonical, ok := legacySimpleRuleUUIDs[rule.UUID]; ok {
		return canonical, true
	}
	base, profileID := typ.ParseScenarioProfile(rule.Scenario)
	if base != typ.ScenarioClaudeCode {
		return "", false
	}
	if profileID == "" {
		canonical, ok := legacyCCRuleUUIDs[rule.UUID]
		return canonical, ok
	}
	tier := TrimContext1M(rule.RequestModel)
	if !ccProfileTiers[tier] {
		return "", false
	}
	return BuiltinRuleUUID(rule.Scenario, tier), true
}

func ensureCurrentBuiltinRules(c *Config) {
	needsSave := false

	if _, ok := c.seedBuiltinRuleIfMissing(RuleUUIDBuiltinCodex, nil); ok {
		needsSave = true
	}

	desktopRefServices := c.referenceServicesFor(typ.ScenarioClaudeCode, typ.ScenarioCodex)
	for _, uuid := range []string{
		RuleUUIDBuiltinClaudeDesktopSonnet46,
		RuleUUIDBuiltinClaudeDesktopOpus46,
		RuleUUIDBuiltinClaudeDesktopOpus47,
	} {
		newRule, ok := c.seedBuiltinRuleIfMissing(uuid, desktopRefServices)
		if !ok {
			continue
		}
		needsSave = true
		logrus.WithFields(logrus.Fields{
			"rule_uuid":      newRule.UUID,
			"request_model":  newRule.RequestModel,
			"response_model": newRule.ResponseModel,
		}).Info("Added Claude Desktop built-in rule")
	}

	if _, ok := c.seedBuiltinRuleIfMissing(RuleUUIDBuiltinClaudeDesktopHaiku45, c.referenceServicesFor(typ.ScenarioClaudeDesktop)); ok {
		needsSave = true
		logrus.Info("Added Claude Desktop haiku-4-5 built-in rule")
	}

	if needsSave {
		c.saveMigration()
		logrus.Info("Migration current-builtin-rules completed: ensured current built-in rules")
	}
}

// migrate20260416 enables multi-tenant by default for existing configurations.
func migrate20260416(c *Config) {
	// Skip migration if multi-tenant config has any values set.
	// This means the user has explicitly configured multi-tenant settings.
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

	c.saveMigration()
}

// migrate20260421 migrates profile unified model name from "*" to "cc".
// This ensures consistency with the new naming convention where profile
// rules use simplified names: "cc" (unified), "default", "haiku", etc. (separate).
// Only applies to claude-code scenario profiles.
func migrate20260421(c *Config) {
	needsSave := false

	for i := range c.Rules {
		rule := &c.Rules[i]

		// Only migrate claude-code profile rules.
		// Profile rules have scenario like "claude-code:profileID".
		if !typ.IsProfiledScenario(rule.Scenario) {
			continue
		}
		// Check if base scenario is claude-code.
		baseScenario, _ := typ.ParseScenarioProfile(rule.Scenario)
		if baseScenario != typ.ScenarioClaudeCode {
			continue
		}

		// Migrate "*" to "cc" for unified mode.
		if rule.RequestModel == "*" {
			rule.RequestModel = "cc"
			needsSave = true
		}
	}

	if needsSave {
		c.saveMigration()
	}
}

// migrate20260502 removes wildcard (*) rules for smart_guide scenario.
// This cleans up legacy wildcard rules that are no longer needed
// as SmartGuide now uses bot-specific rules with UUID pattern: _internal_smart_guide_{botUUID}.
func migrate20260502(c *Config) {
	needsSave := false

	// Filter out smart_guide rules with wildcard RequestModel.
	var filteredRules []typ.Rule
	for _, rule := range c.Rules {
		// Skip smart_guide rules with wildcard RequestModel.
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
		c.saveMigration()
		logrus.Info("Migration 2026-05-02 completed: removed smart_guide wildcard rules")
	}
}

func normalizeRuleDefaultsOnce(c *Config) {
	defaultXcodeSkipUsageOnce(c)
	defaultBuiltinRuleFlagsOnce(c)
}

// defaultXcodeSkipUsageOnce ensures the Xcode scenario defaults SkipUsage on
// (the Xcode client cannot handle usage in streaming chunks). The marker is
// retained so a user who later turns it off keeps that choice across restarts.
func defaultXcodeSkipUsageOnce(c *Config) {
	if c.hasMigrationCompleted("20260606") {
		return
	}

	needsSave := false
	found := false
	for i := range c.Scenarios {
		if c.Scenarios[i].Scenario == typ.ScenarioXcode {
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
		c.saveMigration()
		logrus.Info("Migration 2026-06-06 completed: defaulted SkipUsage on for the Xcode scenario")
	}
}

// defaultBuiltinRuleFlagsOnce seeds rule-level defaults for built-in agent
// scenarios. The marker is retained so user-disabled defaults stay disabled.
func defaultBuiltinRuleFlagsOnce(c *Config) {
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
		c.saveMigration()
		logrus.Info("Migration 20260610 completed: seeded default rule flags (claude_code_compat / clean_header / session_affinity) for Claude Code, Claude Desktop, and Codex rules")
	}
}
