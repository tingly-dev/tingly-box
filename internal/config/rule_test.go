package config

import (
	"fmt"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/tingly-dev/tingly-box/internal/typ"
)

func TestMatchRule_Context1MNormalized(t *testing.T) {
	c := &Config{
		Rules: []typ.Rule{
			// Desktop rule renamed by the 1M toggle.
			{UUID: "d1", Scenario: typ.ScenarioClaudeDesktop, RequestModel: "claude-sonnet-4-6[1m]"},
			// Bare CC profile rule; client env may advertise "haiku[1m]".
			{UUID: "p1", Scenario: typ.RuleScenario("claude_code:p1"), RequestModel: "haiku"},
			// Non-Claude scenario must NOT be normalized.
			{UUID: "o1", Scenario: typ.ScenarioOpenAI, RequestModel: "gpt-4o"},
		},
	}

	// Stale Desktop config sends the bare name → matches the renamed rule.
	if r := c.MatchRuleByModelAndScenario("claude-sonnet-4-6", typ.ScenarioClaudeDesktop); r == nil || r.UUID != "d1" {
		t.Errorf("bare name should match [1m]-renamed desktop rule, got %+v", r)
	}
	// Suffixed pick from /v1/models matches exactly (fast path).
	if r := c.MatchRuleByModelAndScenario("claude-sonnet-4-6[1m]", typ.ScenarioClaudeDesktop); r == nil || r.UUID != "d1" {
		t.Errorf("suffixed name should match renamed desktop rule, got %+v", r)
	}
	// Suffixed request against a bare profile rule (profiled scenario → base
	// scenario is claude_code, so normalization applies).
	if r := c.MatchRuleByModelAndScenario("haiku[1m]", typ.RuleScenario("claude_code:p1")); r == nil || r.UUID != "p1" {
		t.Errorf("suffixed request should match bare profile rule, got %+v", r)
	}
	// Non-Claude scenarios keep strict matching.
	if r := c.MatchRuleByModelAndScenario("gpt-4o[1m]", typ.ScenarioOpenAI); r != nil {
		t.Errorf("openai scenario must not 1m-normalize, got %+v", r)
	}
}

func TestUpdateRule_DesktopSyncsContext1MName(t *testing.T) {
	c := &Config{
		ConfigFile: filepath.Join(t.TempDir(), "config.json"),
		Rules: []typ.Rule{
			{UUID: "d1", Scenario: typ.ScenarioClaudeDesktop, RequestModel: "claude-sonnet-4-6", Active: true},
			{UUID: "cc1", Scenario: typ.ScenarioClaudeCode, RequestModel: "tingly/cc-haiku", Active: true},
		},
	}

	// Enabling the flag renames the desktop rule so /v1/models lists [1m].
	r := c.Rules[0]
	r.Flags.Context1M = true
	if err := c.UpdateRule("d1", r); err != nil {
		t.Fatalf("UpdateRule error: %v", err)
	}
	if got := c.Rules[0].RequestModel; got != "claude-sonnet-4-6[1m]" {
		t.Errorf("desktop rule should be renamed with [1m], got %q", got)
	}

	// Disabling the flag strips the suffix again.
	r = c.Rules[0]
	r.Flags.Context1M = false
	if err := c.UpdateRule("d1", r); err != nil {
		t.Fatalf("UpdateRule error: %v", err)
	}
	if got := c.Rules[0].RequestModel; got != "claude-sonnet-4-6" {
		t.Errorf("desktop rule should be stripped back, got %q", got)
	}

	// Claude Code rules are not renamed — their [1m] travels via the env.
	r = c.Rules[1]
	r.Flags.Context1M = true
	if err := c.UpdateRule("cc1", r); err != nil {
		t.Fatalf("UpdateRule error: %v", err)
	}
	if got := c.Rules[1].RequestModel; got != "tingly/cc-haiku" {
		t.Errorf("claude_code rule must keep its bare name, got %q", got)
	}
}

func TestAddRule_DuplicateNameSameScenario(t *testing.T) {
	cfg, err := NewConfig(WithConfigDir(t.TempDir()))
	if err != nil {
		t.Fatalf("NewConfig error: %v", err)
	}

	rule1 := typ.Rule{
		UUID:         "uuid-1",
		Scenario:     "openai",
		RequestModel: "gpt-4",
	}
	if err := cfg.AddRule(rule1); err != nil {
		t.Fatalf("first AddRule failed: %v", err)
	}

	rule2 := typ.Rule{
		UUID:         "uuid-2",
		Scenario:     "openai",
		RequestModel: "gpt-4",
	}
	err = cfg.AddRule(rule2)
	if err == nil {
		t.Fatal("expected error for duplicate name in same scenario, got nil")
	}
	if !strings.Contains(err.Error(), "already exists") {
		t.Errorf("unexpected error message: %v", err)
	}
}

func TestAddRule_DuplicateNameDifferentScenario(t *testing.T) {
	cfg, err := NewConfig(WithConfigDir(t.TempDir()))
	if err != nil {
		t.Fatalf("NewConfig error: %v", err)
	}

	rule1 := typ.Rule{
		UUID:         "uuid-1",
		Scenario:     "openai",
		RequestModel: "gpt-4",
	}
	if err := cfg.AddRule(rule1); err != nil {
		t.Fatalf("first AddRule failed: %v", err)
	}

	// Same request_model but different scenario — must succeed
	rule2 := typ.Rule{
		UUID:         "uuid-2",
		Scenario:     "anthropic",
		RequestModel: "gpt-4",
	}
	if err := cfg.AddRule(rule2); err != nil {
		t.Errorf("AddRule with same name in different scenario should succeed, got: %v", err)
	}
}

func TestAddRule_TeamSeedsCreateDefaults(t *testing.T) {
	cfg, err := NewConfig(WithConfigDir(t.TempDir()))
	if err != nil {
		t.Fatalf("NewConfig error: %v", err)
	}

	// A bare team rule (no flags) — as any non-HTTP path (CLI/TUI/import) would
	// build it — must come out with the team creation defaults seeded on.
	if err := cfg.AddRule(typ.Rule{UUID: "team-1", Scenario: typ.ScenarioTeam, RequestModel: "m"}); err != nil {
		t.Fatalf("AddRule failed: %v", err)
	}
	seeded := cfg.GetRuleByUUID("team-1")
	if seeded == nil {
		t.Fatal("team rule not found after AddRule")
	}
	if !seeded.Flags.ClaudeCodeCompat || !seeded.Flags.CleanHeader {
		t.Errorf("expected team defaults seeded, got %+v", seeded.Flags)
	}
	if seeded.Flags.SessionAffinity != 1800 || seeded.AffinityTTL() != 30*time.Minute {
		t.Errorf("expected team rule affinity seeded to 30 minutes, got %d", seeded.Flags.SessionAffinity)
	}

	// An explicit flag set is left untouched — the defaults are not layered on.
	if err := cfg.AddRule(typ.Rule{UUID: "team-2", Scenario: typ.ScenarioTeam, RequestModel: "m2", Flags: typ.RuleFlags{SkipUsage: true}}); err != nil {
		t.Fatalf("AddRule failed: %v", err)
	}
	explicit := cfg.GetRuleByUUID("team-2")
	if explicit == nil {
		t.Fatal("explicit team rule not found after AddRule")
	}
	if !explicit.Flags.SkipUsage || explicit.Flags.ClaudeCodeCompat || explicit.Flags.CleanHeader || explicit.Flags.SessionAffinity != 0 {
		t.Errorf("explicit flags must not be overridden, got %+v", explicit.Flags)
	}

	// A non-team rule keeps its empty flag set.
	if err := cfg.AddRule(typ.Rule{UUID: "oai-1", Scenario: typ.ScenarioOpenAI, RequestModel: "m3"}); err != nil {
		t.Fatalf("AddRule failed: %v", err)
	}
	other := cfg.GetRuleByUUID("oai-1")
	if other == nil {
		t.Fatal("openai rule not found after AddRule")
	}
	if !other.Flags.IsZero() {
		t.Errorf("non-team rule should keep empty flags, got %+v", other.Flags)
	}
}

func TestAddRule_TeamScopesSeedAffinity(t *testing.T) {
	cfg, err := NewConfig(WithConfigDir(t.TempDir()))
	if err != nil {
		t.Fatalf("NewConfig error: %v", err)
	}

	// Non-default teams live under "team:<id>"; they get the same default.
	scoped := typ.RuleScenario("team:t-123")
	if err := cfg.AddRule(typ.Rule{UUID: "team-scoped", Scenario: scoped, RequestModel: "m"}); err != nil {
		t.Fatalf("AddRule failed: %v", err)
	}
	if got := cfg.GetRuleByUUID("team-scoped"); got == nil || got.Flags.SessionAffinity != 1800 {
		t.Errorf("team:<id> rule should default to 1800s affinity, got %+v", got)
	}

	// Other scenarios stay off, including ones that seed other defaults.
	for i, sc := range []typ.RuleScenario{typ.ScenarioOpenAI, typ.ScenarioAnthropic, typ.ScenarioCursor, typ.ScenarioCustom} {
		id := fmt.Sprintf("other-%d", i)
		if err := cfg.AddRule(typ.Rule{UUID: id, Scenario: sc, RequestModel: id}); err != nil {
			t.Fatalf("AddRule(%s) failed: %v", sc, err)
		}
		if got := cfg.GetRuleByUUID(id); got == nil || got.Flags.SessionAffinity != 0 || got.AffinityEnabled() {
			t.Errorf("scenario %s must not get affinity by default, got %+v", sc, got)
		}
	}
}

func TestAddRule_CursorSeedsCreateDefaults(t *testing.T) {
	cfg, err := NewConfig(WithConfigDir(t.TempDir()))
	if err != nil {
		t.Fatalf("NewConfig error: %v", err)
	}

	// A bare cursor rule (no flags) must come out with cursor_compat seeded on —
	// every request through the dedicated /tingly/cursor scenario is from
	// Cursor, so there's no need to rely on the cursor_compat_auto header sniff.
	if err := cfg.AddRule(typ.Rule{UUID: "cursor-1", Scenario: typ.ScenarioCursor, RequestModel: "m"}); err != nil {
		t.Fatalf("AddRule failed: %v", err)
	}
	seeded := cfg.GetRuleByUUID("cursor-1")
	if seeded == nil {
		t.Fatal("cursor rule not found after AddRule")
	}
	if !seeded.Flags.CursorCompat {
		t.Errorf("expected cursor_compat seeded on, got %+v", seeded.Flags)
	}

	// An explicit flag set is left untouched — the default is not layered on.
	if err := cfg.AddRule(typ.Rule{UUID: "cursor-2", Scenario: typ.ScenarioCursor, RequestModel: "m2", Flags: typ.RuleFlags{SkipUsage: true}}); err != nil {
		t.Fatalf("AddRule failed: %v", err)
	}
	explicit := cfg.GetRuleByUUID("cursor-2")
	if explicit == nil {
		t.Fatal("explicit cursor rule not found after AddRule")
	}
	if !explicit.Flags.SkipUsage || explicit.Flags.CursorCompat {
		t.Errorf("explicit flags must not be overridden, got %+v", explicit.Flags)
	}
}

func TestAddRule_DuplicateUUID(t *testing.T) {
	cfg, err := NewConfig(WithConfigDir(t.TempDir()))
	if err != nil {
		t.Fatalf("NewConfig error: %v", err)
	}

	rule1 := typ.Rule{
		UUID:         "uuid-1",
		Scenario:     "openai",
		RequestModel: "gpt-4",
	}
	if err := cfg.AddRule(rule1); err != nil {
		t.Fatalf("first AddRule failed: %v", err)
	}

	rule2 := typ.Rule{
		UUID:         "uuid-1", // same UUID, different model
		Scenario:     "openai",
		RequestModel: "gpt-3.5-turbo",
	}
	if err := cfg.AddRule(rule2); err == nil {
		t.Fatal("expected error for duplicate UUID, got nil")
	}
}

// TestAddRule_ExtraHeadersValidation: the AddRule/UpdateRule choke points
// reject structurally invalid headers and canonicalize accepted ones. No
// denylist — extra headers are user-driven config.
func TestAddRule_ExtraHeadersValidation(t *testing.T) {
	cfg, err := NewConfig(WithConfigDir(t.TempDir()))
	if err != nil {
		t.Fatalf("NewConfig error: %v", err)
	}

	malformed := typ.Rule{
		UUID:         "uuid-hdr-1",
		Scenario:     "openai",
		RequestModel: "hdr-model",
		Flags:        typ.RuleFlags{ExtraHeaders: map[string]string{"X Title": "bad name"}},
	}
	if err := cfg.AddRule(malformed); err == nil {
		t.Fatal("expected malformed header name to be rejected, got nil")
	}

	ok := typ.Rule{
		UUID:         "uuid-hdr-2",
		Scenario:     "openai",
		RequestModel: "hdr-model-2",
		Flags:        typ.RuleFlags{ExtraHeaders: map[string]string{"x-title": "tingly"}},
	}
	if err := cfg.AddRule(ok); err != nil {
		t.Fatalf("valid header rejected: %v", err)
	}
	saved := cfg.GetRuleByUUID("uuid-hdr-2")
	if saved == nil {
		t.Fatal("rule not saved")
	}
	if _, exists := saved.Flags.ExtraHeaders["X-Title"]; !exists {
		t.Errorf("header name not canonicalized on save: %v", saved.Flags.ExtraHeaders)
	}

	// UpdateRule shares the gate.
	bad := *saved
	bad.Flags.ExtraHeaders = map[string]string{"X-Dup": "1", "x-dup": "2"}
	if err := cfg.UpdateRule(bad.UUID, bad); err == nil {
		t.Fatal("expected UpdateRule to reject case-insensitive duplicate, got nil")
	}
}

func TestReorderRules_BasicPinAndTolerantAppend(t *testing.T) {
	c := &Config{
		ConfigFile: filepath.Join(t.TempDir(), "config.json"),
		Rules: []typ.Rule{
			{UUID: "a", Scenario: typ.ScenarioOpenAI, RequestModel: "m-a"},
			{UUID: "b", Scenario: typ.ScenarioOpenAI, RequestModel: "m-b"},
			{UUID: "c", Scenario: typ.ScenarioOpenAI, RequestModel: "m-c"},
		},
	}

	// Pin: naming only the target moves it to the front; the unnamed rules
	// keep their relative order behind it.
	got, err := c.ReorderRules(typ.ScenarioOpenAI, []string{"c"})
	if err != nil {
		t.Fatalf("ReorderRules error: %v", err)
	}
	if len(got) != 3 || got[0].UUID != "c" || got[1].UUID != "a" || got[2].UUID != "b" {
		t.Errorf("pin should reorder to [c a b], got %v", uuids(got))
	}
	if uuids(c.Rules) != "c,a,b" {
		t.Errorf("in-memory list not reordered: %v", uuids(c.Rules))
	}

	// Full permutation.
	if _, err := c.ReorderRules(typ.ScenarioOpenAI, []string{"b", "a", "c"}); err != nil {
		t.Fatalf("ReorderRules error: %v", err)
	}
	if uuids(c.Rules) != "b,a,c" {
		t.Errorf("full permutation not applied: %v", uuids(c.Rules))
	}
}

func TestReorderRules_OtherScenarioRulesKeepPositions(t *testing.T) {
	c := &Config{
		ConfigFile: filepath.Join(t.TempDir(), "config.json"),
		Rules: []typ.Rule{
			{UUID: "o1", Scenario: typ.ScenarioOpenAI, RequestModel: "m-o1"},
			{UUID: "x1", Scenario: typ.ScenarioCursor, RequestModel: "m-x1"},
			{UUID: "o2", Scenario: typ.ScenarioOpenAI, RequestModel: "m-o2"},
			{UUID: "x2", Scenario: typ.ScenarioCursor, RequestModel: "m-x2"},
		},
	}

	if _, err := c.ReorderRules(typ.ScenarioOpenAI, []string{"o2", "o1"}); err != nil {
		t.Fatalf("ReorderRules error: %v", err)
	}
	// The openai rules swap among their own slots; the cursor rules keep
	// their exact positions.
	if uuids(c.Rules) != "o2,x1,o1,x2" {
		t.Errorf("foreign scenario rules must not move: %v", uuids(c.Rules))
	}
}

func TestReorderRules_Rejections(t *testing.T) {
	c := &Config{
		ConfigFile: filepath.Join(t.TempDir(), "config.json"),
		Rules: []typ.Rule{
			{UUID: "a", Scenario: typ.ScenarioOpenAI, RequestModel: "m-a"},
			{UUID: "x", Scenario: typ.ScenarioCursor, RequestModel: "m-x"},
		},
	}
	before := uuids(c.Rules)

	for name, tc := range map[string]struct {
		scenario typ.RuleScenario
		uuids    []string
	}{
		"empty":          {typ.ScenarioOpenAI, nil},
		"unknown uuid":   {typ.ScenarioOpenAI, []string{"nope"}},
		"foreign uuid":   {typ.ScenarioOpenAI, []string{"x"}},
		"duplicate":      {typ.ScenarioOpenAI, []string{"a", "a"}},
		"empty scenario": {"", []string{"a"}},
	} {
		if _, err := c.ReorderRules(tc.scenario, tc.uuids); err == nil {
			t.Errorf("%s: expected rejection, got nil", name)
		}
	}
	if uuids(c.Rules) != before {
		t.Errorf("rejected calls must not mutate: %v", uuids(c.Rules))
	}
}

// uuids renders a rule list's UUIDs for compact assertions.
func uuids(rules []typ.Rule) string {
	parts := make([]string, len(rules))
	for i := range rules {
		parts[i] = rules[i].UUID
	}
	return strings.Join(parts, ",")
}
