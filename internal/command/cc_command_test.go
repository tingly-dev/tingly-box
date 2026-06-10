package command

import (
	"testing"

	"github.com/tingly-dev/tingly-box/internal/server/config"
	"github.com/tingly-dev/tingly-box/internal/typ"
)

func TestGenerateCCEnv_ProfileSeparate_ResolvesModelsFromCanonicalRules(t *testing.T) {
	cfg := &config.Config{Rules: []typ.Rule{
		// Renamed request model — env must follow the rule, not the seeded name.
		{UUID: "builtin:claude_code:p1:haiku", Scenario: "claude_code:p1", RequestModel: "my-fast", Active: true},
		// Inactive rule → fall back to the seeded tier name.
		{UUID: "builtin:claude_code:p1:opus", Scenario: "claude_code:p1", RequestModel: "my-smart", Active: false},
		{UUID: "builtin:claude_code:p1:sonnet", Scenario: "claude_code:p1", RequestModel: "sonnet", Active: true},
		// default / subagent rules absent → fall back to the seeded tier name.
	}}

	env := generateCCEnv(cfg, "http://localhost:12580", "tok", "claude_code:p1", false, true)

	wants := map[string]string{
		"ANTHROPIC_BASE_URL":             "http://localhost:12580/tingly/claude_code:p1",
		"ANTHROPIC_MODEL":                "default",
		"ANTHROPIC_DEFAULT_HAIKU_MODEL":  "my-fast",
		"ANTHROPIC_DEFAULT_OPUS_MODEL":   "opus",
		"ANTHROPIC_DEFAULT_SONNET_MODEL": "sonnet",
		"CLAUDE_CODE_SUBAGENT_MODEL":     "subagent",
	}
	for k, want := range wants {
		if env[k] != want {
			t.Errorf("env[%q] = %q, want %q", k, env[k], want)
		}
	}
}

func TestGenerateCCEnv_Profile_Context1MSuffix(t *testing.T) {
	cfg := &config.Config{Rules: []typ.Rule{
		// Flag set → env model advertises [1m] to Claude Code.
		{UUID: "builtin:claude_code:p1:haiku", Scenario: "claude_code:p1", RequestModel: "haiku",
			Flags: typ.RuleFlags{Context1M: true}, Active: true},
		// Already suffixed (e.g. user renamed it) → no double suffix.
		{UUID: "builtin:claude_code:p1:opus", Scenario: "claude_code:p1", RequestModel: "opus[1m]",
			Flags: typ.RuleFlags{Context1M: true}, Active: true},
		// Flag off → untouched.
		{UUID: "builtin:claude_code:p1:sonnet", Scenario: "claude_code:p1", RequestModel: "sonnet", Active: true},
	}}

	env := generateCCEnv(cfg, "http://localhost:12580", "tok", "claude_code:p1", false, true)

	if got := env["ANTHROPIC_DEFAULT_HAIKU_MODEL"]; got != "haiku[1m]" {
		t.Errorf("haiku model = %q, want %q", got, "haiku[1m]")
	}
	if got := env["ANTHROPIC_DEFAULT_OPUS_MODEL"]; got != "opus[1m]" {
		t.Errorf("opus model = %q, want %q (no double suffix)", got, "opus[1m]")
	}
	if got := env["ANTHROPIC_DEFAULT_SONNET_MODEL"]; got != "sonnet" {
		t.Errorf("sonnet model = %q, want %q", got, "sonnet")
	}
}

func TestGenerateCCEnv_ProfileUnified_ResolvesCCRule(t *testing.T) {
	cfg := &config.Config{Rules: []typ.Rule{
		{UUID: "builtin:claude_code:p2:cc", Scenario: "claude_code:p2", RequestModel: "renamed-cc", Active: true},
	}}

	env := generateCCEnv(cfg, "http://localhost:12580", "tok", "claude_code:p2", true, true)

	for _, k := range []string{
		"ANTHROPIC_MODEL",
		"ANTHROPIC_DEFAULT_HAIKU_MODEL",
		"ANTHROPIC_DEFAULT_OPUS_MODEL",
		"ANTHROPIC_DEFAULT_SONNET_MODEL",
		"CLAUDE_CODE_SUBAGENT_MODEL",
	} {
		if env[k] != "renamed-cc" {
			t.Errorf("env[%q] = %q, want %q", k, env[k], "renamed-cc")
		}
	}
}

func TestGenerateCCEnv_MainScenario_ResolvesModernBuiltins(t *testing.T) {
	cfg := &config.Config{Rules: []typ.Rule{
		{UUID: config.RuleUUIDCCHaiku, Scenario: typ.ScenarioClaudeCode, RequestModel: "vendor/fast", Active: true},
	}}

	env := generateCCEnv(cfg, "http://localhost:12580", "tok", "claude_code", false, false)

	if got := env["ANTHROPIC_DEFAULT_HAIKU_MODEL"]; got != "vendor/fast" {
		t.Errorf("haiku model = %q, want %q", got, "vendor/fast")
	}
}

func TestGenerateCCEnv_MainScenario_ResolvesLegacyBuiltins(t *testing.T) {
	// Pre-migration configs still carry the legacy built-in-cc-* UUIDs.
	cfg := &config.Config{Rules: []typ.Rule{
		{UUID: config.RuleUUIDBuiltinCCHaiku, Scenario: typ.ScenarioClaudeCode, RequestModel: "vendor/fast", Active: true},
	}}

	env := generateCCEnv(cfg, "http://localhost:12580", "tok", "claude_code", false, false)

	if got := env["ANTHROPIC_DEFAULT_HAIKU_MODEL"]; got != "vendor/fast" {
		t.Errorf("haiku model = %q, want %q", got, "vendor/fast")
	}
	// Missing rules keep the canonical tingly/* fallbacks.
	if got := env["ANTHROPIC_MODEL"]; got != "tingly/cc-default" {
		t.Errorf("default model = %q, want %q", got, "tingly/cc-default")
	}
}
