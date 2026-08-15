package config

import (
	"time"

	"github.com/sirupsen/logrus"
	"github.com/tingly-dev/tingly-box/ai"

	"github.com/tingly-dev/tingly-box/internal/constant"
	"github.com/tingly-dev/tingly-box/internal/loadbalance"
	typ "github.com/tingly-dev/tingly-box/internal/typ"
)

// --- Migration pipeline --------------------------------------------------
//
// Migrations are organized by the invariants they protect rather than by every
// historical patch date. The current supported baseline is the scenario/provider
// config model; very old config shapes are repaired through normalizeLegacyConfigBaseline.
//
// Every step is classified by migrationKind so a reader can tell its lifecycle
// without reading the body, and Migrate saves once at the end instead of each
// step saving individually. See .design/config-migration.md for the full
// policy on when a dated step is expected to be folded into a baseline
// normalizer, and how legacy UUID lookup tables are retired alongside it.

// migrationKind classifies a migration step by its lifecycle.
type migrationKind int

const (
	// kindBaseline repairs structural invariants and runs on every boot,
	// forever. It is the target state itself, so it never gets folded away.
	kindBaseline migrationKind = iota
	// kindDated is a one-off data repair tied to a specific change. It runs
	// unconditionally on every boot (idempotent via its own internal guard)
	// until it is folded into a baseline normalizer once no active config
	// can predate it — see .design/config-migration.md.
	kindDated
	// kindOnce is gated by a MigrationsCompleted marker: it runs exactly once
	// per config, so it can change a value a user might since have overridden
	// without re-clobbering that override on every boot.
	kindOnce
)

// migrationStep is one entry in the migration pipeline. fn reports whether it
// changed Config state that needs persisting; Migrate saves once at the end
// if any step reported dirty.
type migrationStep struct {
	name    string
	kind    migrationKind
	addedAt string // when the step was introduced; empty for baseline steps
	fn      func(*Config) bool
}

var migrationSteps = []migrationStep{
	{"normalize-legacy-config-baseline", kindBaseline, "", normalizeLegacyConfigBaseline},
	{"normalize-builtin-rule-identity", kindBaseline, "", normalizeBuiltinRuleIdentity},
	{"agent-scenario-to-custom", kindBaseline, "", migrateAgentScenarioToCustom},
	{"ensure-current-builtin-rules", kindBaseline, "", ensureCurrentBuiltinRules},
	{"20260712-drop-unsupported-smart-routing", kindDated, "2026-07-12", migrate20260712},
	{"20260606-xcode-skip-usage", kindOnce, "2026-06-06", defaultXcodeSkipUsageOnce},
	{"20260610-builtin-rule-flags", kindOnce, "2026-06-10", defaultBuiltinRuleFlagsOnce},
}

// Migrate runs every registered migration step in order and persists the
// config once at the end if any step reported a change, instead of each step
// saving independently.
func Migrate(c *Config) error {
	dirty := false
	for _, step := range migrationSteps {
		if step.fn(c) {
			dirty = true
		}
	}
	if dirty {
		c.saveMigration()
	}
	return nil
}

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
		if c.usageStore != nil {
			if err := c.usageStore.RenameRuleUUID(oldUUID, newUUID); err != nil {
				logrus.WithError(err).Warnf("Migration %s: failed to rename usage records %s -> %s", migrationID, oldUUID, newUUID)
			}
		}
	}
}

// migrateAgentScenarioToCustom renames every stored Rule and ScenarioConfig
// still on the deprecated "agent" scenario (OpenClaw) to "custom". Runs every
// boot, un-gated: it is cheap, and — unlike a "once" marker — self-heals a
// config restored from an old backup instead of leaving it stuck on the
// deprecated name forever.
//
// This only rewrites stored data. New requests that still hit the literal
// "/tingly/agent" URL keep working indefinitely via
// legacyScenarioAliasMiddleware, which resolves "agent" -> "custom" at the
// routing layer (typ.ResolveScenarioAlias) — independent of whether any rule
// migrated here.
func migrateAgentScenarioToCustom(c *Config) bool {
	needsSave := false

	for i := range c.Rules {
		if c.Rules[i].Scenario != typ.ScenarioAgent {
			continue
		}
		c.Rules[i].Scenario = typ.ScenarioCustom
		needsSave = true
	}

	for i := range c.Scenarios {
		if c.Scenarios[i].Scenario != typ.ScenarioAgent {
			continue
		}
		c.Scenarios[i].Scenario = typ.ScenarioCustom
		needsSave = true
	}

	if needsSave {
		logrus.Info("Migration agent-to-custom completed: renamed \"agent\" scenario rules/config to \"custom\"")
	}
	return needsSave
}

// migrate20260712 drops smart-routing partitions whose ops reference a
// position the current build no longer recognizes (tool_use was removed;
// this also covers any future removal). Without this, one stale op fails
// ValidateSmartRouting for the WHOLE rule: NewRouter rejects it at request
// time — disabling every sibling partition — and AddRule/UpdateRule reject
// any edit, leaving the rule both broken and unrepairable from the UI.
// Dropping the partition preserves observed behavior: a tool_use partition
// never matched real traffic. Runs every boot (idempotent) so configs
// restored from backups heal too.
func migrate20260712(c *Config) bool {
	needsSave := false

	for i := range c.Rules {
		rule := &c.Rules[i]
		if len(rule.SmartRouting) == 0 {
			continue
		}
		kept := rule.SmartRouting[:0]
		for _, sr := range rule.SmartRouting {
			invalid := ""
			for _, op := range sr.Ops {
				if !op.Position.IsValid() {
					invalid = string(op.Position)
					break
				}
			}
			if invalid == "" {
				kept = append(kept, sr)
				continue
			}
			needsSave = true
			logrus.WithFields(logrus.Fields{
				"rule_uuid":     rule.UUID,
				"request_model": rule.RequestModel,
				"partition":     sr.Description,
				"position":      invalid,
			}).Warn("Removing smart-routing partition: position is no longer supported")
		}
		rule.SmartRouting = kept
		if len(rule.SmartRouting) == 0 {
			rule.SmartEnabled = false
		}
	}

	if needsSave {
		logrus.Info("Migration 2026-07-12 completed: removed smart-routing partitions with unsupported positions")
	}
	return needsSave
}

// normalizeLegacyConfigBaseline folds config repair migrations that are old
// enough no active config can predate them into one baseline normalizer. It
// keeps old configs usable without keeping every historical date migration as
// a permanent startup phase — see .design/config-migration.md for the fold
// policy. Originally covered everything pre-2026-04; the 2026-04/05 batch
// (multi-tenant defaults, profile unified model naming, smart_guide wildcard
// cleanup, Codex endpoint mode) was folded in on top of that.
func normalizeLegacyConfigBaseline(c *Config) bool {
	needsSave := false

	if normalizeLegacyProviders(c) {
		needsSave = true
	}
	if normalizeRuleBasics(c) {
		needsSave = true
	}
	if normalizeMultiTenantDefaults(c) {
		needsSave = true
	}
	if normalizeClaudeCodeProfileUnifiedModel(c) {
		needsSave = true
	}
	if dropSmartGuideWildcardRules(c) {
		needsSave = true
	}
	if normalizeCodexEndpointMode(c) {
		needsSave = true
	}

	if needsSave {
		logrus.Info("Migration baseline normalization completed: repaired legacy provider/rule config")
	}
	return needsSave
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

		normalizedTactic := normalizeRuleTactic(rule)
		if rule.LBTactic.Type != normalizedTactic.Type || !IsTacticValid(&rule.LBTactic) {
			rule.LBTactic = normalizedTactic
			needsSave = true
		}
		// Compact tier numbering (contiguous from 0) for configs written
		// before saves started normalizing tiers.
		if normalizeRuleServiceTiers(&rule) {
			needsSave = true
		}
		valid = append(valid, rule)
	}
	if len(valid) != len(c.Rules) {
		needsSave = true
	}
	c.Rules = valid
	if ensureRuleUUIDs(c.Rules) {
		needsSave = true
	}
	return needsSave
}

// ensureRuleUUIDs makes every rule carry a unique, non-empty UUID, the
// invariant the rule store keys on. Empty UUIDs (very old configs) get a
// fresh one; duplicates (hand-edited copy-pasted rule blocks) keep the first
// occurrence and reassign the rest — both were routable under the file-based
// loader, so both must survive. This is the single place UUID repair policy
// lives: normalizeRuleBasics applies it every startup and the one-time legacy
// import (hydrateRulesFromStore) applies it before first persisting to the
// store. Returns whether anything changed.
func ensureRuleUUIDs(rules []typ.Rule) bool {
	changed := false
	seen := make(map[string]bool, len(rules))
	for i := range rules {
		rule := &rules[i]
		if rule.UUID == "" || seen[rule.UUID] {
			if rule.UUID != "" {
				logrus.Warnf("Rule %q (scenario %s) duplicates UUID %s; assigning a new one",
					rule.RequestModel, rule.Scenario, rule.UUID)
			}
			rule.UUID = GenerateUUID()
			changed = true
		}
		seen[rule.UUID] = true
	}
	return changed
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
func normalizeBuiltinRuleIdentity(c *Config) bool {
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
		return false
	}
	c.rekeyRuleUUIDState("builtin-rule-identity", renames)
	logrus.Infof("Migration builtin-rule-identity completed: normalized %d built-in rule UUID(s)", len(renames))
	return true
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

func ensureCurrentBuiltinRules(c *Config) bool {
	needsSave := false

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
		logrus.Info("Migration current-builtin-rules completed: ensured current built-in rules")
	}
	return needsSave
}

// normalizeMultiTenantDefaults enables multi-tenant by default for configs
// written before multi-tenant existed. New installs already seed
// MultiTenantConfig in CreateDefaultConfig; this only backfills older ones.
// Folded from the dated migrate20260416 migration (2026-04-16).
func normalizeMultiTenantDefaults(c *Config) bool {
	// Skip if multi-tenant config has any values set — that means either a
	// prior run of this normalizer already seeded it, or the user has
	// explicitly configured multi-tenant settings.
	if c.MultiTenantConfig.APITokenSecret != "" ||
		c.MultiTenantConfig.APITokenAlgorithm != "" ||
		c.MultiTenantConfig.APITokenIssuer != "" {
		return false
	}

	// All three token fields are empty (guaranteed by the guard above), so seed
	// the defaults and enable multi-tenant.
	c.MultiTenantConfig.APITokenSecret = generateSecret()
	c.MultiTenantConfig.APITokenAlgorithm = "HS256"
	c.MultiTenantConfig.APITokenIssuer = "tingly-box"
	c.MultiTenantConfig.Enabled = true

	return true
}

// normalizeClaudeCodeProfileUnifiedModel migrates profile unified model name
// from "*" to "cc". This ensures consistency with the naming convention where
// profile rules use simplified names: "cc" (unified), "default", "haiku",
// etc. (separate). Only applies to claude-code scenario profiles.
// Folded from the dated migrate20260421 migration (2026-04-21).
func normalizeClaudeCodeProfileUnifiedModel(c *Config) bool {
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

	return needsSave
}

// dropSmartGuideWildcardRules removes wildcard (*) rules for the smart_guide
// scenario. This cleans up legacy wildcard rules that are no longer needed
// as SmartGuide now uses bot-specific rules with UUID pattern:
// _internal_smart_guide_{botUUID}.
// Folded from the dated migrate20260502 migration (2026-05-02).
func dropSmartGuideWildcardRules(c *Config) bool {
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
		logrus.Info("Removed smart_guide wildcard rules")
	}
	return needsSave
}

// normalizeCodexEndpointMode sets OpenAIEndpointMode=responses on existing
// Codex OAuth providers. Codex's API only exposes /responses (no
// /chat/completions); the mode is declared on Codex providers at OAuth
// instantiation, but providers created before that don't carry it. Without
// the backfill, the resolver's default-Chat semantics would silently send
// /chat/completions requests to Codex and fail.
//
// Idempotent: only flips the mode when issuer is Codex and the mode is unset.
// Unlike the other baseline sub-steps, this never reports dirty: providers
// live in SQLite (db.ProviderStore) and are backfilled directly there, so
// there is no Config JSON state for the caller to persist.
// Folded from the dated migrate20260518 migration (2026-05-18).
func normalizeCodexEndpointMode(c *Config) bool {
	// Providers live in SQLite (the JSON c.Providers slice is legacy backup).
	// Backfill the DB-stored ones directly so the resolver sees the right mode.
	if c.providerStore != nil {
		if oauthProviders, err := c.providerStore.ListOAuth(); err == nil {
			for _, p := range oauthProviders {
				if p.OAuthDetail == nil || p.OAuthDetail.GetIssuer() != ai.IssuerCodex {
					continue
				}
				if p.OpenAIEndpointMode == ai.EndpointModeResponses {
					continue
				}
				p.OpenAIEndpointMode = ai.EndpointModeResponses
				if err := c.providerStore.Save(p); err != nil {
					logrus.WithError(err).WithField("provider_uuid", p.UUID).Warn("Failed to backfill openai_endpoint_mode on Codex provider")
					continue
				}
				logrus.WithField("provider_uuid", p.UUID).Info("Backfilled openai_endpoint_mode=responses on Codex provider (db)")
			}
		} else {
			logrus.WithError(err).Warn("Failed to list OAuth providers for openai_endpoint_mode backfill")
		}
	}
	return false
}

// defaultXcodeSkipUsageOnce ensures the Xcode scenario defaults SkipUsage on
// (the Xcode client cannot handle usage in streaming chunks). The marker is
// retained so a user who later turns it off keeps that choice across restarts.
func defaultXcodeSkipUsageOnce(c *Config) bool {
	if c.hasMigrationCompleted("20260606") {
		return false
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
		logrus.Info("Migration 2026-06-06 completed: defaulted SkipUsage on for the Xcode scenario")
	}
	// The marker itself just changed c.MigrationsCompleted, so this run needs
	// persisting even when needsSave (the substantive Scenarios change) is
	// false — e.g. a config that already had SkipUsage=true before this
	// migration shipped.
	return true
}

// defaultBuiltinRuleFlagsOnce seeds rule-level defaults for built-in agent
// scenarios. The marker is retained so user-disabled defaults stay disabled.
func defaultBuiltinRuleFlagsOnce(c *Config) bool {
	if c.hasMigrationCompleted("20260610") {
		return false
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
		logrus.Info("Migration 20260610 completed: seeded default rule flags (claude_code_compat / clean_header / session_affinity) for Claude Code, Claude Desktop, and Codex rules")
	}
	// Same reasoning as defaultXcodeSkipUsageOnce: the marker append alone
	// needs persisting even when no rule flag actually changed.
	return true
}
