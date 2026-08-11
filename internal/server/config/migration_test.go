package config

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/tingly-dev/tingly-box/internal/loadbalance"
	"github.com/tingly-dev/tingly-box/internal/typ"
)

func TestNormalizeLegacyConfigBaseline_RuleBasics(t *testing.T) {
	c := &Config{
		Rules: []typ.Rule{
			{UUID: RuleUUIDBuiltinOpenAI},             // known legacy UUID → scenario repaired
			{UUID: "drop-me"},                         // unknown empty scenario → dropped
			{Scenario: typ.ScenarioClaudeCode},        // missing UUID + tactic repaired
			{UUID: "ok", Scenario: typ.ScenarioCodex}, // invalid zero tactic repaired
		},
	}

	normalizeLegacyConfigBaseline(c)

	if len(c.Rules) != 3 {
		t.Fatalf("rule count after baseline normalization = %d, want 3: %+v", len(c.Rules), c.Rules)
	}
	if c.Rules[0].Scenario != typ.ScenarioOpenAI {
		t.Errorf("legacy OpenAI rule scenario = %q, want %q", c.Rules[0].Scenario, typ.ScenarioOpenAI)
	}
	if c.Rules[1].UUID == "" {
		t.Error("missing rule UUID should be generated")
	}
	for _, r := range c.Rules {
		if r.LBTactic.Type != loadbalance.TacticRandom {
			t.Errorf("rule %q tactic type = %q, want %q", r.UUID, r.LBTactic.Type, loadbalance.TacticRandom)
		}
		if _, ok := typ.AsRandomParams(r.LBTactic.Params); !ok {
			t.Errorf("rule %q tactic params should be random: %+v", r.UUID, r.LBTactic.Params)
		}
	}
}

func TestNormalizeLegacyConfigBaseline_MultiTierRulesBecomeTierWithRandomWithinTier(t *testing.T) {
	c := &Config{
		Rules: []typ.Rule{
			{
				UUID:     "tier-rule",
				Scenario: typ.ScenarioClaudeCode,
				Services: []*loadbalance.Service{
					{Provider: "p0", Model: "high", Active: true, Tier: 0},
					{Provider: "p1", Model: "low", Active: true, Tier: 1},
				},
				LBTactic: typ.Tactic{
					Type:   loadbalance.TacticRandom,
					Params: typ.NewRandomParams(),
				},
			},
		},
	}

	normalizeLegacyConfigBaseline(c)

	got := c.Rules[0].LBTactic
	if got.Type != loadbalance.TacticTier {
		t.Fatalf("multi-tier rule tactic = %q, want %q", got.Type, loadbalance.TacticTier)
	}
	params, ok := got.Params.(*typ.TierParams)
	if !ok {
		t.Fatalf("multi-tier params = %T, want *typ.TierParams", got.Params)
	}
	if params.WithinTierTactic != loadbalance.TacticRandom {
		t.Fatalf("within tier tactic = %q, want random", params.WithinTierTactic)
	}
}

func TestEnsureCurrentBuiltinRules_SeedsDesktopRules(t *testing.T) {
	c := &Config{
		Rules: []typ.Rule{
			{UUID: RuleUUIDBuiltinCC, Scenario: typ.ScenarioClaudeCode, Services: []*loadbalance.Service{svc("p1")}},
			// One desktop rule already present — must be left untouched / not duplicated.
			{UUID: RuleUUIDBuiltinClaudeDesktopSonnet46, Scenario: typ.ScenarioClaudeDesktop},
		},
	}

	ensureCurrentBuiltinRules(c)

	for _, uuid := range []string{
		RuleUUIDBuiltinClaudeDesktopSonnet46,
		RuleUUIDBuiltinClaudeDesktopOpus46,
		RuleUUIDBuiltinClaudeDesktopOpus47,
	} {
		if c.findRuleByUUID(uuid) == nil {
			t.Errorf("expected desktop rule %q to exist", uuid)
		}
	}
	if n := countRules(c, RuleUUIDBuiltinClaudeDesktopSonnet46); n != 1 {
		t.Errorf("sonnet rule duplicated: count = %d", n)
	}
	opus := c.findRuleByUUID(RuleUUIDBuiltinClaudeDesktopOpus46)
	if opus == nil || len(opus.Services) != 1 || opus.Services[0].Provider != "p1" {
		t.Errorf("opus46 should inherit reference services, got %+v", opus)
	}
}

func TestEnsureCurrentBuiltinRules_AddsHaikuFromTemplate(t *testing.T) {
	c := &Config{
		Rules: []typ.Rule{
			{UUID: RuleUUIDBuiltinClaudeDesktopSonnet46, Scenario: typ.ScenarioClaudeDesktop, Services: []*loadbalance.Service{svc("pd")}},
		},
	}

	ensureCurrentBuiltinRules(c)

	haiku := c.findRuleByUUID(RuleUUIDBuiltinClaudeDesktopHaiku45)
	if haiku == nil {
		t.Fatal("expected haiku-4-5 desktop rule to be added")
	}
	// Pulled from the init.go template, so it carries the built-in desktop flags.
	if !haiku.Flags.ClaudeCodeCompat || !haiku.Flags.CleanHeader {
		t.Errorf("haiku rule missing built-in flags: %+v", haiku.Flags)
	}
	// Services mirrored from the existing desktop rule.
	if len(haiku.Services) != 1 || haiku.Services[0].Provider != "pd" {
		t.Errorf("haiku rule should inherit desktop reference services, got %+v", haiku.Services)
	}

	// Idempotent: running again does not duplicate it.
	ensureCurrentBuiltinRules(c)
	if n := countRules(c, RuleUUIDBuiltinClaudeDesktopHaiku45); n != 1 {
		t.Errorf("haiku rule duplicated: count = %d", n)
	}
}

func TestMigrate20260606_DefaultsXcodeSkipUsage(t *testing.T) {
	// NewConfig runs Migrate, which applies migrate20260606.
	cfg, err := NewConfig(WithConfigDir(t.TempDir()))
	if err != nil {
		t.Fatalf("NewConfig error: %v", err)
	}

	if !cfg.hasMigrationCompleted("20260606") {
		t.Error("migration 20260606 should be marked completed")
	}
	xcode := cfg.GetScenarioConfig(typ.ScenarioXcode)
	if xcode == nil {
		t.Fatal("Xcode scenario config not found")
	}
	if !xcode.Flags.SkipUsage {
		t.Error("Xcode should default SkipUsage = true")
	}
}

func TestMigrate20260606_PreservesUserOverride(t *testing.T) {
	tmpDir := t.TempDir()

	// First boot applies the default.
	cfg, err := NewConfig(WithConfigDir(tmpDir))
	if err != nil {
		t.Fatalf("NewConfig error: %v", err)
	}
	if x := cfg.GetScenarioConfig(typ.ScenarioXcode); x == nil || !x.Flags.SkipUsage {
		t.Fatalf("expected SkipUsage defaulted on after first boot, got %+v", x)
	}

	// User explicitly disables it.
	for i := range cfg.Scenarios {
		if cfg.Scenarios[i].Scenario == typ.ScenarioXcode {
			cfg.Scenarios[i].Flags.SkipUsage = false
			cfg.Save()
			break
		}
	}

	// Second boot: migration is marker-gated, so the user's choice survives.
	cfg2, err := NewConfig(WithConfigDir(tmpDir))
	if err != nil {
		t.Fatalf("NewConfig error (reload): %v", err)
	}
	if !cfg2.hasMigrationCompleted("20260606") {
		t.Error("migration should still be marked completed after reload")
	}
	if x := cfg2.GetScenarioConfig(typ.ScenarioXcode); x == nil || x.Flags.SkipUsage {
		t.Errorf("user's SkipUsage=false should be preserved across reload, got %+v", x)
	}
}

func TestDefaultBuiltinRuleFlagsOnce_SeedsRuleFlags(t *testing.T) {
	c := &Config{
		Rules: []typ.Rule{
			{UUID: "built-in-cc", Scenario: typ.ScenarioClaudeCode},
			{UUID: "cc-profile", Scenario: typ.RuleScenario("claude_code:p1")},
			{UUID: "cc-compat-on", Scenario: typ.ScenarioClaudeCode, Flags: typ.RuleFlags{ClaudeCodeCompat: true}},
			{UUID: "cc-affinity-set", Scenario: typ.ScenarioClaudeCode, Flags: typ.RuleFlags{SessionAffinity: 900}},
			{UUID: "desktop", Scenario: typ.ScenarioClaudeDesktop},
			{UUID: "codex", Scenario: typ.ScenarioCodex},
			{UUID: "openai", Scenario: typ.ScenarioOpenAI},
		},
	}

	defaultBuiltinRuleFlagsOnce(c)

	if !c.hasMigrationCompleted("20260610") {
		t.Fatal("migration should be marked completed")
	}

	type want struct {
		compat   bool
		clean    bool
		affinity int
	}
	const aff = defaultSessionAffinitySeconds
	wants := map[string]want{
		"built-in-cc":     {compat: true, clean: true, affinity: aff},   // claude_code base → all defaulted on
		"cc-profile":      {compat: true, clean: true, affinity: aff},   // claude_code:<profile> → covered
		"cc-compat-on":    {compat: true, clean: true, affinity: aff},   // already-on compat unchanged, others seeded
		"cc-affinity-set": {compat: true, clean: true, affinity: 900},   // user affinity preserved
		"desktop":         {compat: true, clean: true, affinity: aff},   // claude_desktop → all defaulted on
		"codex":           {compat: false, clean: false, affinity: aff}, // codex → affinity only
		"openai":          {compat: false, clean: false, affinity: 0},   // out of scope → untouched
	}
	for _, r := range c.Rules {
		w := wants[r.UUID]
		if r.Flags.ClaudeCodeCompat != w.compat {
			t.Errorf("rule %q ClaudeCodeCompat = %v, want %v", r.UUID, r.Flags.ClaudeCodeCompat, w.compat)
		}
		if r.Flags.CleanHeader != w.clean {
			t.Errorf("rule %q CleanHeader = %v, want %v", r.UUID, r.Flags.CleanHeader, w.clean)
		}
		if r.Flags.SessionAffinity != w.affinity {
			t.Errorf("rule %q SessionAffinity = %d, want %d", r.UUID, r.Flags.SessionAffinity, w.affinity)
		}
	}
}

func TestNormalizeBuiltinRuleIdentity_NormalizesProfileRuleUUIDs(t *testing.T) {
	p1 := typ.RuleScenario("claude_code:p1")
	p2 := typ.RuleScenario("claude_code:p2")
	c := &Config{
		Rules: []typ.Rule{
			// Legacy random UUIDs → renamed to canonical.
			{UUID: "5f1c2a9e-0000-0000-0000-000000000001", Scenario: p1, RequestModel: "haiku"},
			{UUID: "5f1c2a9e-0000-0000-0000-000000000002", Scenario: p1, RequestModel: "sonnet"},
			// Unified-mode profile rule.
			{UUID: "5f1c2a9e-0000-0000-0000-000000000003", Scenario: p2, RequestModel: "cc"},
			// Already canonical → untouched.
			{UUID: "builtin:claude_code:p1:opus", Scenario: p1, RequestModel: "opus"},
			// Custom request model with no built-in counterpart → untouched.
			{UUID: "5f1c2a9e-0000-0000-0000-000000000004", Scenario: p1, RequestModel: "my-custom"},
			// Main-scenario legacy built-in → renamed by exact UUID match,
			// even with a user-renamed request model.
			{UUID: RuleUUIDBuiltinCCHaiku, Scenario: typ.ScenarioClaudeCode, RequestModel: "vendor/fast"},
			{UUID: RuleUUIDBuiltinCC, Scenario: typ.ScenarioClaudeCode, RequestModel: "tingly/cc"},
			// Main-scenario user rule with a random UUID → untouched.
			{UUID: "5f1c2a9e-0000-0000-0000-000000000005", Scenario: typ.ScenarioClaudeCode, RequestModel: "haiku"},
			// Tier rule renamed for 1M context → tier identified after
			// stripping the [1m] suffix; request model kept as-is.
			{UUID: "5f1c2a9e-0000-0000-0000-000000000006", Scenario: p2, RequestModel: "haiku[1m]"},
		},
	}

	normalizeBuiltinRuleIdentity(c)

	wants := map[int]string{
		0: "builtin:claude_code:p1:haiku",
		1: "builtin:claude_code:p1:sonnet",
		2: "builtin:claude_code:p2:cc",
		3: "builtin:claude_code:p1:opus",
		4: "5f1c2a9e-0000-0000-0000-000000000004",
		5: RuleUUIDCCHaiku,
		6: RuleUUIDCC,
		7: "5f1c2a9e-0000-0000-0000-000000000005",
		8: "builtin:claude_code:p2:haiku",
	}
	for i, want := range wants {
		if got := c.Rules[i].UUID; got != want {
			t.Errorf("rule %d UUID = %q, want %q", i, got, want)
		}
	}

	// Idempotent: a second pass changes nothing.
	normalizeBuiltinRuleIdentity(c)
	for i, want := range wants {
		if got := c.Rules[i].UUID; got != want {
			t.Errorf("second pass: rule %d UUID = %q, want %q", i, got, want)
		}
	}
}

// TestNormalizeBuiltinRuleIdentity_UpgradeFromLegacyConfig exercises the real startup path:
// a config.json persisted by an older build (legacy main-scenario UUIDs,
// random-UUID profile rules) is loaded via NewConfig, which runs baseline
// normalization plus InsertDefaultRule.
func TestNormalizeBuiltinRuleIdentity_UpgradeFromLegacyConfig(t *testing.T) {
	tmpDir := t.TempDir()
	legacy := `{
		"rules": [
			{"uuid": "built-in-cc", "scenario": "claude_code", "request_model": "tingly/cc", "active": true},
			{"uuid": "built-in-cc-haiku", "scenario": "claude_code", "request_model": "vendor/fast", "active": true},
			{"uuid": "11111111-2222-3333-4444-555555555555", "scenario": "claude_code:p1", "request_model": "haiku", "active": true}
		],
		"profiles": {"claude_code": [{"id": "p1", "name": "Test", "unified": false}]}
	}`
	if err := os.WriteFile(filepath.Join(tmpDir, "config.json"), []byte(legacy), 0644); err != nil {
		t.Fatalf("write legacy config: %v", err)
	}

	cfg, err := NewConfig(WithConfigDir(tmpDir))
	if err != nil {
		t.Fatalf("NewConfig error: %v", err)
	}

	// Legacy main rules renamed, request models (incl. user-customized) preserved.
	if r := cfg.GetRuleByUUID(RuleUUIDCC); r == nil || r.RequestModel != "tingly/cc" {
		t.Errorf("built-in-cc not renamed to %s: %+v", RuleUUIDCC, r)
	}
	if r := cfg.GetRuleByUUID(RuleUUIDCCHaiku); r == nil || r.RequestModel != "vendor/fast" {
		t.Errorf("built-in-cc-haiku not renamed with custom model preserved: %+v", r)
	}
	// Profile rule renamed by tier.
	if r := cfg.GetRuleByUUID("builtin:claude_code:p1:haiku"); r == nil || r.RequestModel != "haiku" {
		t.Errorf("profile haiku rule not renamed: %+v", r)
	}
	// No legacy UUIDs left, and InsertDefaultRule must not have duplicated
	// renamed rules or resurrected legacy ones.
	counts := map[string]int{}
	for _, r := range cfg.Rules {
		counts[r.UUID]++
	}
	for _, legacyUUID := range []string{RuleUUIDBuiltinCC, RuleUUIDBuiltinCCHaiku, RuleUUIDBuiltinCCDefault} {
		if counts[legacyUUID] != 0 {
			t.Errorf("legacy UUID %q still present after upgrade", legacyUUID)
		}
	}
	for uuid, n := range counts {
		if n > 1 {
			t.Errorf("rule UUID %q duplicated %d times after upgrade", uuid, n)
		}
	}
	// Missing built-ins (sonnet/opus/...) seeded by InsertDefaultRule with modern UUIDs.
	if r := cfg.GetRuleByUUID(RuleUUIDCCOpus); r == nil {
		t.Error("missing opus built-in should be seeded with the modern UUID")
	}

	// Second boot: stable, still no duplicates.
	cfg2, err := NewConfig(WithConfigDir(tmpDir))
	if err != nil {
		t.Fatalf("NewConfig (reload) error: %v", err)
	}
	counts2 := map[string]int{}
	for _, r := range cfg2.Rules {
		counts2[r.UUID]++
	}
	for uuid, n := range counts2 {
		if n > 1 {
			t.Errorf("reload: rule UUID %q duplicated %d times", uuid, n)
		}
	}
	if r := cfg2.GetRuleByUUID(RuleUUIDCCHaiku); r == nil || r.RequestModel != "vendor/fast" {
		t.Errorf("reload: custom haiku model lost: %+v", r)
	}
}

func TestNormalizeBuiltinRuleIdentity_SkipsOnCanonicalCollision(t *testing.T) {
	p1 := typ.RuleScenario("claude_code:p1")
	c := &Config{
		Rules: []typ.Rule{
			// Canonical identity already held by another rule.
			{UUID: "builtin:claude_code:p1:haiku", Scenario: p1, RequestModel: "haiku"},
			// Duplicate tier rule with a random UUID — must not steal the identity.
			{UUID: "5f1c2a9e-0000-0000-0000-00000000000a", Scenario: p1, RequestModel: "haiku"},
		},
	}

	normalizeBuiltinRuleIdentity(c)

	if c.Rules[0].UUID != "builtin:claude_code:p1:haiku" {
		t.Errorf("canonical rule UUID changed: %q", c.Rules[0].UUID)
	}
	if c.Rules[1].UUID != "5f1c2a9e-0000-0000-0000-00000000000a" {
		t.Errorf("duplicate tier rule should keep its UUID on collision, got %q", c.Rules[1].UUID)
	}
}

func TestNormalizeBuiltinRuleIdentity_RenamesSimpleLegacyUUIDs(t *testing.T) {
	c := &Config{
		Rules: []typ.Rule{
			{UUID: RuleUUIDTingly, Scenario: typ.ScenarioOpenAI, RequestModel: "legacy-openai"},
			{UUID: RuleUUIDBuiltinAnthropic, Scenario: typ.ScenarioAnthropic, RequestModel: "claude"},
			{UUID: RuleUUIDBuiltinCodex, Scenario: typ.ScenarioCodex, RequestModel: "codex"},
			{UUID: "custom-rule", Scenario: typ.ScenarioOpenAI, RequestModel: "custom"},
		},
	}

	normalizeBuiltinRuleIdentity(c)

	wants := []string{
		RuleUUIDOpenAI,
		RuleUIDAnthropic,
		RuleUUIDCodex,
		"custom-rule",
	}
	for i, want := range wants {
		if got := c.Rules[i].UUID; got != want {
			t.Errorf("rule %d UUID = %q, want %q", i, got, want)
		}
	}
	if c.Rules[0].RequestModel != "legacy-openai" || c.Rules[1].Scenario != typ.ScenarioAnthropic {
		t.Errorf("migration should preserve rule fields, got %+v", c.Rules[:2])
	}

	normalizeBuiltinRuleIdentity(c)
	for i, want := range wants {
		if got := c.Rules[i].UUID; got != want {
			t.Errorf("second pass: rule %d UUID = %q, want %q", i, got, want)
		}
	}
}

func TestNormalizeBuiltinRuleIdentity_SkipsSimpleUUIDCollision(t *testing.T) {
	c := &Config{
		Rules: []typ.Rule{
			{UUID: RuleUUIDOpenAI, Scenario: typ.ScenarioOpenAI, RequestModel: "modern"},
			{UUID: RuleUUIDBuiltinOpenAI, Scenario: typ.ScenarioOpenAI, RequestModel: "legacy"},
		},
	}

	normalizeBuiltinRuleIdentity(c)

	if c.Rules[0].UUID != RuleUUIDOpenAI {
		t.Errorf("canonical rule UUID changed: %q", c.Rules[0].UUID)
	}
	if c.Rules[1].UUID != RuleUUIDBuiltinOpenAI {
		t.Errorf("legacy rule should keep UUID on collision, got %q", c.Rules[1].UUID)
	}
}

func TestMigrateAgentScenarioToCustom_RenamesRulesAndScenarioConfig(t *testing.T) {
	c := &Config{
		Rules: []typ.Rule{
			{UUID: RuleUUIDAgent, Scenario: typ.ScenarioAgent, RequestModel: "tingly-agent"},
			{UUID: "user-agent-rule", Scenario: typ.ScenarioAgent, RequestModel: "my-model"},
			{UUID: "unrelated", Scenario: typ.ScenarioOpenAI, RequestModel: "gpt"},
		},
		Scenarios: []typ.ScenarioConfig{
			{Scenario: typ.ScenarioAgent, Flags: typ.ScenarioFlags{SkipUsage: true}},
			{Scenario: typ.ScenarioOpenAI},
		},
	}

	migrateAgentScenarioToCustom(c)

	if c.Rules[0].Scenario != typ.ScenarioCustom || c.Rules[1].Scenario != typ.ScenarioCustom {
		t.Errorf("agent-scenario rules should be renamed to custom, got %+v", c.Rules[:2])
	}
	if c.Rules[2].Scenario != typ.ScenarioOpenAI {
		t.Errorf("unrelated rule should be untouched, got %+v", c.Rules[2])
	}
	if c.Rules[0].RequestModel != "tingly-agent" || c.Rules[1].RequestModel != "my-model" {
		t.Errorf("migration should preserve rule fields, got %+v", c.Rules[:2])
	}
	if c.Scenarios[0].Scenario != typ.ScenarioCustom || !c.Scenarios[0].Flags.SkipUsage {
		t.Errorf("agent ScenarioConfig should be renamed to custom, preserving flags, got %+v", c.Scenarios[0])
	}
	if c.Scenarios[1].Scenario != typ.ScenarioOpenAI {
		t.Errorf("unrelated ScenarioConfig should be untouched, got %+v", c.Scenarios[1])
	}

	// Idempotent: a second pass is a no-op.
	migrateAgentScenarioToCustom(c)
	if c.Rules[0].Scenario != typ.ScenarioCustom {
		t.Errorf("second pass should leave already-migrated rules alone, got %+v", c.Rules[0])
	}
}

// TestMigrateAgentScenarioToCustom_ClawRuleUpgradesFromOldestLegacyForm boots a
// real config from the oldest legacy shape (hyphenated "built-in-agent-claw"
// UUID, "agent" scenario) — the second half of the old agent/OpenClaw default
// pair alongside "tingly-agent" — and confirms the full Migrate() pipeline
// (normalizeBuiltinRuleIdentity's UUID rename + migrateAgentScenarioToCustom's
// scenario rename) lands it on the modern "custom" identity without touching
// the request_model a client may already be pointed at.
func TestMigrateAgentScenarioToCustom_ClawRuleUpgradesFromOldestLegacyForm(t *testing.T) {
	tmpDir := t.TempDir()
	legacy := `{
		"rules": [
			{"uuid": "built-in-agent-claw", "scenario": "agent", "request_model": "tingly-claw", "active": true}
		]
	}`
	if err := os.WriteFile(filepath.Join(tmpDir, "config.json"), []byte(legacy), 0644); err != nil {
		t.Fatalf("write legacy config: %v", err)
	}

	cfg, err := NewConfig(WithConfigDir(tmpDir))
	if err != nil {
		t.Fatalf("NewConfig error: %v", err)
	}

	r := cfg.GetRuleByUUID(RuleUUIDCustomClaw)
	if r == nil {
		t.Fatal("expected the claw rule to be found under the modern builtin:custom:claw UUID")
	}
	if r.Scenario != typ.ScenarioCustom {
		t.Errorf("claw rule scenario = %q, want %q", r.Scenario, typ.ScenarioCustom)
	}
	if r.RequestModel != "tingly-claw" {
		t.Errorf("claw rule request_model = %q, want unchanged %q", r.RequestModel, "tingly-claw")
	}
	if r := cfg.GetRuleByUUID(RuleUUIDBuiltinAgentClaw); r != nil {
		t.Errorf("legacy built-in-agent-claw UUID should be gone after upgrade, still found: %+v", r)
	}

	// The routing layer must still resolve it via both the modern path and
	// the permanently-aliased legacy one.
	if !typ.CanUseScenarioInPath(typ.ScenarioCustom) {
		t.Error("custom scenario should be usable directly in the request path")
	}
	canonical, ok := typ.ResolveScenarioAlias(typ.ScenarioAgent)
	if !ok || canonical != typ.ScenarioCustom {
		t.Errorf("ResolveScenarioAlias(agent) = (%q, %v), want (%q, true)", canonical, ok, typ.ScenarioCustom)
	}

	// Second boot: stable, no duplicate/resurrected legacy rule.
	cfg2, err := NewConfig(WithConfigDir(tmpDir))
	if err != nil {
		t.Fatalf("NewConfig (reload) error: %v", err)
	}
	count := 0
	for _, rule := range cfg2.Rules {
		if rule.UUID == RuleUUIDCustomClaw {
			count++
		}
	}
	if count != 1 {
		t.Errorf("reload: expected exactly 1 rule with UUID %q, found %d", RuleUUIDCustomClaw, count)
	}
}

func TestNewCCProfileRules_CanonicalUUIDs(t *testing.T) {
	separate := newCCProfileRules(typ.RuleScenario("claude_code:p3"), false)
	wantSeparate := map[string]string{
		"default":  "builtin:claude_code:p3:default",
		"haiku":    "builtin:claude_code:p3:haiku",
		"sonnet":   "builtin:claude_code:p3:sonnet",
		"opus":     "builtin:claude_code:p3:opus",
		"subagent": "builtin:claude_code:p3:subagent",
	}
	if len(separate) != len(wantSeparate) {
		t.Fatalf("separate mode rule count = %d, want %d", len(separate), len(wantSeparate))
	}
	for _, r := range separate {
		if want := wantSeparate[r.RequestModel]; r.UUID != want {
			t.Errorf("separate rule %q UUID = %q, want %q", r.RequestModel, r.UUID, want)
		}
	}

	unified := newCCProfileRules(typ.RuleScenario("claude_code:p3"), true)
	if len(unified) != 1 || unified[0].UUID != "builtin:claude_code:p3:cc" {
		t.Errorf("unified rule UUID = %+v, want builtin:claude_code:p3:cc", unified)
	}
}

func TestDefaultBuiltinRuleFlagsOnce_PreservesUserOff(t *testing.T) {
	c := &Config{
		Rules: []typ.Rule{
			{UUID: "built-in-cc", Scenario: typ.ScenarioClaudeCode},
			{UUID: "codex", Scenario: typ.ScenarioCodex},
		},
	}

	defaultBuiltinRuleFlagsOnce(c)
	if !c.Rules[0].Flags.ClaudeCodeCompat || !c.Rules[0].Flags.CleanHeader ||
		c.Rules[0].Flags.SessionAffinity != defaultSessionAffinitySeconds {
		t.Fatal("first run should seed the default flags on the CC rule")
	}
	if c.Rules[1].Flags.SessionAffinity != defaultSessionAffinitySeconds {
		t.Fatal("first run should seed affinity on the Codex rule")
	}

	// User turns everything off; a later boot must not re-enable any of it.
	c.Rules[0].Flags.ClaudeCodeCompat = false
	c.Rules[0].Flags.CleanHeader = false
	c.Rules[0].Flags.SessionAffinity = 0
	c.Rules[1].Flags.SessionAffinity = 0
	defaultBuiltinRuleFlagsOnce(c)
	if c.Rules[0].Flags.ClaudeCodeCompat || c.Rules[0].Flags.CleanHeader ||
		c.Rules[0].Flags.SessionAffinity != 0 || c.Rules[1].Flags.SessionAffinity != 0 {
		t.Error("migration re-enabled user-disabled flags; one-time gate is broken")
	}
}

func TestNormalizeLegacyConfigBaseline_CompactsTierGaps(t *testing.T) {
	c := &Config{
		Rules: []typ.Rule{
			{
				UUID:     "gappy",
				Scenario: typ.ScenarioOpenAI,
				Services: []*loadbalance.Service{
					{Provider: "p0", Model: "a", Active: true, Tier: 1},
					{Provider: "p1", Model: "b", Active: true, Tier: 3},
				},
			},
		},
	}

	normalizeLegacyConfigBaseline(c)

	got := c.Rules[0].Services
	if got[0].Tier != 0 || got[1].Tier != 1 {
		t.Fatalf("tiers = [%d, %d], want [0, 1]", got[0].Tier, got[1].Tier)
	}
}

func TestUpdateRule_NormalizesTiers(t *testing.T) {
	cfg, err := NewConfig(WithConfigDir(t.TempDir()))
	if err != nil {
		t.Fatalf("NewConfig error: %v", err)
	}

	prov := &typ.Provider{
		UUID:     "tier-norm-prov",
		Name:     "tier-norm-prov",
		APIBase:  "https://example.com/v1",
		APIStyle: "openai",
		Enabled:  true,
	}
	if err := cfg.AddProvider(prov); err != nil {
		t.Fatalf("AddProvider error: %v", err)
	}

	rule := typ.Rule{
		UUID:         "tier-norm",
		Scenario:     typ.ScenarioOpenAI,
		RequestModel: "tier-norm-model",
		Active:       true,
		Services: []*loadbalance.Service{
			// The T0 service was "deleted": remaining tiers must promote.
			{Provider: prov.UUID, Model: "a", Active: true, Tier: 1},
			{Provider: prov.UUID, Model: "b", Active: true, Tier: 2},
		},
	}
	if err := cfg.AddRule(rule); err != nil {
		t.Fatalf("AddRule error: %v", err)
	}
	saved := cfg.GetRuleByUUID("tier-norm")
	if saved == nil {
		t.Fatal("rule not found after AddRule")
	}
	if saved.Services[0].Tier != 0 || saved.Services[1].Tier != 1 {
		t.Fatalf("AddRule tiers = [%d, %d], want [0, 1]", saved.Services[0].Tier, saved.Services[1].Tier)
	}

	rule.Services = []*loadbalance.Service{
		{Provider: prov.UUID, Model: "a", Active: true, Tier: 2},
		{Provider: prov.UUID, Model: "b", Active: true, Tier: 5},
	}
	if err := cfg.UpdateRule("tier-norm", rule); err != nil {
		t.Fatalf("UpdateRule error: %v", err)
	}
	saved = cfg.GetRuleByUUID("tier-norm")
	if saved.Services[0].Tier != 0 || saved.Services[1].Tier != 1 {
		t.Fatalf("UpdateRule tiers = [%d, %d], want [0, 1]", saved.Services[0].Tier, saved.Services[1].Tier)
	}
}

// Grandfathered provider references: a rule that already references a
// provider stays fully editable after that provider is disabled or deleted —
// only *newly introduced* references are validated strictly.
func TestUpdateRule_GrandfathersExistingProviderRefs(t *testing.T) {
	cfg, err := NewConfig(WithConfigDir(t.TempDir()))
	if err != nil {
		t.Fatalf("NewConfig error: %v", err)
	}

	prov := &typ.Provider{
		UUID:     "gf-prov",
		Name:     "gf-prov",
		APIBase:  "https://example.com/v1",
		APIStyle: "openai",
		Enabled:  true,
	}
	if err := cfg.AddProvider(prov); err != nil {
		t.Fatalf("AddProvider error: %v", err)
	}

	rule := typ.Rule{
		UUID:         "gf-rule",
		Scenario:     typ.ScenarioOpenAI,
		RequestModel: "gf-model",
		Active:       true,
		Services: []*loadbalance.Service{
			{Provider: prov.UUID, Model: "a", Active: true, Tier: 0},
			{Provider: prov.UUID, Model: "b", Active: true, Tier: 1},
		},
	}
	if err := cfg.AddRule(rule); err != nil {
		t.Fatalf("AddRule error: %v", err)
	}

	// Disable the provider the rule references.
	prov.Enabled = false
	if err := cfg.UpdateProvider(prov.UUID, prov); err != nil {
		t.Fatalf("disable provider: %v", err)
	}

	// Editing the rule (e.g. a tier move) must still succeed.
	rule.Services[0].Tier = 1
	rule.Services[1].Tier = 0
	if err := cfg.UpdateRule("gf-rule", rule); err != nil {
		t.Fatalf("UpdateRule with grandfathered disabled provider: %v", err)
	}

	// A NEW reference to a non-existent provider is still rejected.
	withNew := rule
	withNew.Services = append([]*loadbalance.Service{}, rule.Services...)
	withNew.Services = append(withNew.Services, &loadbalance.Service{Provider: "does-not-exist", Model: "c", Active: true})
	if err := cfg.UpdateRule("gf-rule", withNew); err == nil {
		t.Fatal("expected new non-existent provider reference to be rejected")
	}

	// Grandfathering is per (provider, model) pair: a NEW model on the
	// already-referenced disabled provider is a new reference — rejected.
	withNewModel := rule
	withNewModel.Services = append([]*loadbalance.Service{}, rule.Services...)
	withNewModel.Services = append(withNewModel.Services, &loadbalance.Service{Provider: prov.UUID, Model: "c", Active: true})
	if err := cfg.UpdateRule("gf-rule", withNewModel); err == nil {
		t.Fatal("expected new model on disabled provider to be rejected")
	}

	// A brand-new rule referencing the disabled provider is also rejected.
	fresh := typ.Rule{
		UUID:         "gf-rule-2",
		Scenario:     typ.ScenarioOpenAI,
		RequestModel: "gf-model-2",
		Active:       true,
		Services: []*loadbalance.Service{
			{Provider: prov.UUID, Model: "a", Active: true},
		},
	}
	if err := cfg.AddRule(fresh); err == nil {
		t.Fatal("expected new rule referencing disabled provider to be rejected")
	}
}

// Deleting a provider cascades service removal without going through
// UpdateRule; the cascade must re-compact tiers so it can't persist a gap.
func TestDeleteProviderCascade_CompactsTiers(t *testing.T) {
	cfg, err := NewConfig(WithConfigDir(t.TempDir()))
	if err != nil {
		t.Fatalf("NewConfig error: %v", err)
	}

	keep := &typ.Provider{UUID: "cascade-keep", Name: "cascade-keep", APIBase: "https://example.com/v1", APIStyle: "openai", Enabled: true}
	gone := &typ.Provider{UUID: "cascade-gone", Name: "cascade-gone", APIBase: "https://example.com/v1", APIStyle: "openai", Enabled: true}
	for _, p := range []*typ.Provider{keep, gone} {
		if err := cfg.AddProvider(p); err != nil {
			t.Fatalf("AddProvider error: %v", err)
		}
	}

	rule := typ.Rule{
		UUID:         "cascade-rule",
		Scenario:     typ.ScenarioOpenAI,
		RequestModel: "cascade-model",
		Active:       true,
		Services: []*loadbalance.Service{
			{Provider: gone.UUID, Model: "a", Active: true, Tier: 0}, // sole T0
			{Provider: keep.UUID, Model: "b", Active: true, Tier: 1},
			{Provider: keep.UUID, Model: "c", Active: true, Tier: 2},
		},
	}
	if err := cfg.AddRule(rule); err != nil {
		t.Fatalf("AddRule error: %v", err)
	}

	if err := cfg.DeleteProvider(gone.UUID); err != nil {
		t.Fatalf("DeleteProvider error: %v", err)
	}

	saved := cfg.GetRuleByUUID("cascade-rule")
	if saved == nil {
		t.Fatal("rule not found after cascade")
	}
	if len(saved.Services) != 2 || saved.Services[0].Tier != 0 || saved.Services[1].Tier != 1 {
		tiers := make([]int, 0, len(saved.Services))
		for _, s := range saved.Services {
			tiers = append(tiers, s.Tier)
		}
		t.Fatalf("post-cascade tiers = %v, want [0, 1]", tiers)
	}
}
