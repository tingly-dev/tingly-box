package config

import typ "github.com/tingly-dev/tingly-box/internal/typ"

// Claude Code built-in tier names. A tier is both the key of its built-in rule
// and the request model new installs seed it with (profile rules use the same
// names), so adding a tier is one entry in CCTiers.
const (
	CCTierUnified  = "cc"
	CCTierDefault  = "default"
	CCTierOpus     = "opus"
	CCTierSonnet   = "sonnet"
	CCTierHaiku    = "haiku"
	CCTierFable    = "fable"
	CCTierSubagent = "subagent"
)

// CCTier describes one Claude Code built-in model tier: its rule identity, the
// settings.json env slot it fills, and how it is seeded and labelled. It is the
// single source the rule seeds, profile rules, env generation, prefs defaults,
// alias lookup and drift checks are derived from.
type CCTier struct {
	Name        string // tier name; also the request model new installs seed
	RuleUUID    string // modern "builtin:claude_code:<tier>" UUID
	LegacyUUID  string // pre-"builtin:" UUID, "" when the tier never had one
	LegacyModel string // prefixed request model older installs seeded
	EnvKey      string // env slot the tier fills; "" for the unified tier, which fills every slot
	// Alias is true for tiers Claude Code addresses by a --model alias
	// (opus/sonnet/haiku/fable); the others are the default and subagent slots.
	Alias bool
	// FollowsDefault tiers arrived after separate mode shipped, so their rule
	// may be missing or switched off; the slot then follows the default tier.
	FollowsDefault bool

	Description        string // seeded rule description
	ProfileDescription string // description of the profile's rule
}

// Label is the tier's name in listings ("unified" for the unified tier).
func (t CCTier) Label() string {
	if t.Name == CCTierUnified {
		return "unified"
	}
	return t.Name
}

// CCTiers lists the tiers in seeding order: the unified tier, then the
// separate-mode tiers (default first, since FollowsDefault tiers fall back to it).
var CCTiers = []CCTier{
	{
		Name: CCTierUnified, RuleUUID: RuleUUIDCC, LegacyUUID: RuleUUIDBuiltinCC, LegacyModel: "tingly/cc",
		Description:        "Default proxy rule for Claude Code",
		ProfileDescription: "Claude Code profile - unified mode",
	},
	{
		Name: CCTierDefault, RuleUUID: RuleUUIDCCDefault, LegacyUUID: RuleUUIDBuiltinCCDefault, LegacyModel: "tingly/cc-default",
		EnvKey:             "ANTHROPIC_MODEL",
		Description:        "Claude Code - Default model - for general task",
		ProfileDescription: "Claude Code profile - default model",
	},
	{
		Name: CCTierOpus, RuleUUID: RuleUUIDCCOpus, LegacyUUID: RuleUUIDBuiltinCCOpus, LegacyModel: "tingly/cc-opus",
		EnvKey: "ANTHROPIC_DEFAULT_OPUS_MODEL", Alias: true,
		Description:        "Claude Code - Opus model - to use for opus , or for opusplan when Plan Mode is active.",
		ProfileDescription: "Claude Code profile - opus model",
	},
	{
		Name: CCTierSonnet, RuleUUID: RuleUUIDCCSonnet, LegacyUUID: RuleUUIDBuiltinCCSonnet, LegacyModel: "tingly/cc-sonnet",
		EnvKey: "ANTHROPIC_DEFAULT_SONNET_MODEL", Alias: true,
		Description:        "Claude Code - Sonnet model - model to use for sonnet , or for opusplan when Plan Mode is not active.",
		ProfileDescription: "Claude Code profile - sonnet model",
	},
	{
		Name: CCTierHaiku, RuleUUID: RuleUUIDCCHaiku, LegacyUUID: RuleUUIDBuiltinCCHaiku, LegacyModel: "tingly/cc-haiku",
		EnvKey: "ANTHROPIC_DEFAULT_HAIKU_MODEL", Alias: true,
		Description:        "Claude Code - Haiku mode The model to use for haiku , or background functionality",
		ProfileDescription: "Claude Code profile - haiku model",
	},
	{
		Name: CCTierFable, RuleUUID: RuleUUIDCCFable, LegacyModel: "tingly/cc-fable",
		EnvKey: "ANTHROPIC_DEFAULT_FABLE_MODEL", Alias: true, FollowsDefault: true,
		Description:        "Claude Code - Fable model - model to use for the fable alias",
		ProfileDescription: "Claude Code profile - fable model",
	},
	{
		Name: CCTierSubagent, RuleUUID: RuleUUIDCCSubagent, LegacyUUID: RuleUUIDBuiltinCCSubagent, LegacyModel: "tingly/cc-subagent",
		EnvKey:             "CLAUDE_CODE_SUBAGENT_MODEL",
		Description:        "Claude Code - Subagent model - model to use for subagents",
		ProfileDescription: "Claude Code profile - subagent model",
	},
}

// CCSlotTiers returns the tiers that fill an env slot (everything except the
// unified tier), in CCTiers order.
func CCSlotTiers() []CCTier {
	out := make([]CCTier, 0, len(CCTiers)-1)
	for _, t := range CCTiers {
		if t.EnvKey != "" {
			out = append(out, t)
		}
	}
	return out
}

// CCTierByName returns the tier with the given name (the zero CCTier if none).
func CCTierByName(name string) CCTier {
	for _, t := range CCTiers {
		if t.Name == name {
			return t
		}
	}
	return CCTier{}
}

// CCRequestModels returns the request model of every tier, unified first.
func CCRequestModels() []string {
	out := make([]string, 0, len(CCTiers))
	for _, t := range CCTiers {
		out = append(out, t.Name)
	}
	return out
}

// ccBuiltinRule builds the seeded rule of a tier in the main scenario.
func ccBuiltinRule(t CCTier) typ.Rule {
	return ccRule(t.RuleUUID, t.Name, t.Description, t.Name == CCTierUnified)
}

// ccRuleUUIDs returns every UUID a tier's rule has gone by (modern, legacy).
func ccRuleUUIDs(t CCTier) []string {
	if t.LegacyUUID == "" {
		return []string{t.RuleUUID}
	}
	return []string{t.RuleUUID, t.LegacyUUID}
}

// Lookup tables derived from CCTiers. They are package-level initializers (not
// init() functions) so they exist before DefaultRules and any other init uses them.
var (
	// ccProfileTiers is the set of request models a system-seeded Claude Code
	// profile rule routes on. Profile rules with any other request model are
	// user-customized and keep whatever UUID they have.
	ccProfileTiers = func() map[string]bool {
		m := map[string]bool{}
		for _, t := range CCTiers {
			m[t.Name] = true
		}
		return m
	}()

	// claudeCodeUnifiedRuleUUIDs / claudeCodeSeparateRuleUUIDs hold every UUID
	// (modern and legacy) of the unified rule and of the separate-mode rules.
	claudeCodeUnifiedRuleUUIDs  = ccRuleUUIDSet(true)
	claudeCodeSeparateRuleUUIDs = ccRuleUUIDSet(false)

	// legacyCCRuleUUIDs maps the legacy Claude Code built-in UUIDs to their
	// modern counterparts. Used by normalizeBuiltinRuleIdentity to rename live
	// configs and by defaultRuleByUUID to keep older migrations (written against
	// the legacy names) able to pull templates from the modern DefaultRules.
	//
	// Retirement of individual entries (or the table as a whole) follows the
	// policy in .design/config-migration.md — do not delete entries ad hoc.
	legacyCCRuleUUIDs = func() map[string]string {
		m := map[string]string{}
		for _, t := range CCTiers {
			if t.LegacyUUID != "" {
				m[t.LegacyUUID] = t.RuleUUID
			}
		}
		return m
	}()

	// legacyCCRequestModels maps the prefixed request models older installs
	// seeded for the built-in rules to the short names new installs use. The two
	// spellings are treated as one name when a request or an "apply" is matched
	// to a rule, so a config written for either keeps working against the other.
	// Only this fixed group is aliased: it is deliberately not a generic
	// "tingly/" prefix strip.
	legacyCCRequestModels = func() map[string]string {
		m := map[string]string{}
		for _, t := range CCTiers {
			m[t.LegacyModel] = t.Name
		}
		return m
	}()
)

func ccRuleUUIDSet(unified bool) map[string]bool {
	m := map[string]bool{}
	for _, t := range CCTiers {
		if (t.EnvKey == "") != unified {
			continue
		}
		for _, uuid := range ccRuleUUIDs(t) {
			m[uuid] = true
		}
	}
	return m
}

// canonicalCCRequestModel returns the short spelling of a Claude Code built-in
// request model, dropping the [1m] marker; other names are returned trimmed.
func canonicalCCRequestModel(model string) string {
	model = TrimContext1M(model)
	if short, ok := legacyCCRequestModels[model]; ok {
		return short
	}
	return model
}
