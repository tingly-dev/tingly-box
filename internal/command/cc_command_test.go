package command

import (
	"os"
	"testing"

	appconfig "github.com/tingly-dev/tingly-box/internal/config"
	"github.com/tingly-dev/tingly-box/internal/server/config"
	"github.com/tingly-dev/tingly-box/internal/typ"
)

func newTestConfig(t *testing.T) *config.Config {
	t.Helper()
	tempDir, err := os.MkdirTemp("", "tingly-cc-slots-*")
	if err != nil {
		t.Fatalf("MkdirTemp: %v", err)
	}

	ac, err := appconfig.NewAppConfig(appconfig.WithConfigDir(tempDir))
	if err != nil {
		t.Fatalf("NewAppConfig: %v", err)
	}
	return ac.GetGlobalConfig()
}

// setContext1M flips the Context1M flag on the rule with the given UUID.
func setContext1M(t *testing.T, cfg *config.Config, uuid string, on bool) {
	t.Helper()
	for _, r := range cfg.GetRequestConfigs() {
		if r.UUID == uuid {
			r.Context1M = on
			if err := cfg.UpdateRule(uuid, r); err != nil {
				t.Fatalf("UpdateRule(%s): %v", uuid, err)
			}
			return
		}
	}
	t.Fatalf("rule %s not found", uuid)
}

func TestResolveCCModelSlots_DefaultSeparate(t *testing.T) {
	cfg := newTestConfig(t)

	slots := resolveCCModelSlots(cfg, "", false)
	if got := slots["ANTHROPIC_DEFAULT_SONNET_MODEL"]; got != "tingly/cc-sonnet" {
		t.Fatalf("sonnet slot = %q, want tingly/cc-sonnet", got)
	}

	// Toggling Context1M on the sonnet rule appends [1m] to that slot only —
	// the rule's request_model itself stays clean.
	setContext1M(t, cfg, config.RuleUUIDBuiltinCCSonnet, true)
	for _, r := range cfg.GetRequestConfigs() {
		if r.UUID == config.RuleUUIDBuiltinCCSonnet {
			if r.RequestModel != "tingly/cc-sonnet" {
				t.Fatalf("rule.request_model dirtied after 1M toggle: %q", r.RequestModel)
			}
		}
	}
	slots = resolveCCModelSlots(cfg, "", false)
	if got := slots["ANTHROPIC_DEFAULT_SONNET_MODEL"]; got != "tingly/cc-sonnet[1m]" {
		t.Fatalf("sonnet slot after 1M = %q, want tingly/cc-sonnet[1m]", got)
	}
	if got := slots["ANTHROPIC_DEFAULT_HAIKU_MODEL"]; got != "tingly/cc-haiku" {
		t.Fatalf("haiku slot leaked 1M = %q", got)
	}
}

func TestResolveCCModelSlots_DefaultUnified(t *testing.T) {
	cfg := newTestConfig(t)

	setContext1M(t, cfg, config.RuleUUIDBuiltinCC, true)
	slots := resolveCCModelSlots(cfg, "", true)
	for _, k := range []string{
		"ANTHROPIC_MODEL", "ANTHROPIC_DEFAULT_HAIKU_MODEL", "ANTHROPIC_DEFAULT_SONNET_MODEL",
		"ANTHROPIC_DEFAULT_OPUS_MODEL", "CLAUDE_CODE_SUBAGENT_MODEL",
	} {
		if got := slots[k]; got != "tingly/cc[1m]" {
			t.Fatalf("unified slot %s = %q, want tingly/cc[1m]", k, got)
		}
	}
}

func TestResolveCCModelSlots_Profile(t *testing.T) {
	cfg := newTestConfig(t)

	meta, err := cfg.CreateProfile(typ.ScenarioClaudeCode, "test", false)
	if err != nil {
		t.Fatalf("CreateProfile: %v", err)
	}

	slots := resolveCCModelSlots(cfg, meta.ID, false)
	if got := slots["ANTHROPIC_DEFAULT_SONNET_MODEL"]; got != "sonnet" {
		t.Fatalf("profile sonnet slot = %q, want sonnet", got)
	}

	// Flip Context1M on the profile's sonnet rule (matched by short base name).
	profiledScenario := typ.ProfiledScenarioName(typ.ScenarioClaudeCode, meta.ID)
	for _, r := range cfg.GetRequestConfigs() {
		if r.GetScenario() == profiledScenario && r.RequestModel == "sonnet" {
			setContext1M(t, cfg, r.UUID, true)
			break
		}
	}
	slots = resolveCCModelSlots(cfg, meta.ID, false)
	if got := slots["ANTHROPIC_DEFAULT_SONNET_MODEL"]; got != "sonnet[1m]" {
		t.Fatalf("profile sonnet slot after 1M = %q, want sonnet[1m]", got)
	}
	if got := slots["ANTHROPIC_DEFAULT_HAIKU_MODEL"]; got != "haiku" {
		t.Fatalf("profile haiku slot leaked 1M = %q", got)
	}
}

func TestMatchRuleByModelAndScenario_StripsContext1M(t *testing.T) {
	cfg := newTestConfig(t)

	// Default built-in sonnet rule has request_model = "tingly/cc-sonnet".
	// Client with 1M enabled will send "tingly/cc-sonnet[1m]" on the wire.
	rule := cfg.MatchRuleByModelAndScenario("tingly/cc-sonnet[1m]", typ.ScenarioClaudeCode)
	if rule == nil {
		t.Fatal("expected to find rule by stripping [1m]")
	}
	if rule.UUID != config.RuleUUIDBuiltinCCSonnet {
		t.Fatalf("matched wrong rule: %s", rule.UUID)
	}
	if rule.RequestModel != "tingly/cc-sonnet" {
		t.Fatalf("rule.request_model = %q, want clean tingly/cc-sonnet", rule.RequestModel)
	}

	// Exact-match still works for legacy rules that hand-carry [1m]
	// (defense in depth; canonical form is the flag).
	rule = cfg.MatchRuleByModelAndScenario("tingly/cc-sonnet", typ.ScenarioClaudeCode)
	if rule == nil || rule.UUID != config.RuleUUIDBuiltinCCSonnet {
		t.Fatal("clean exact match broken")
	}
}
