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
	t.Cleanup(func() { _ = os.RemoveAll(tempDir) })

	ac, err := appconfig.NewAppConfig(appconfig.WithConfigDir(tempDir))
	if err != nil {
		t.Fatalf("NewAppConfig: %v", err)
	}
	return ac.GetGlobalConfig()
}

// setRequestModel updates the request_model of the rule with the given UUID.
func setRequestModel(t *testing.T, cfg *config.Config, uuid, model string) {
	t.Helper()
	for _, r := range cfg.GetRequestConfigs() {
		if r.UUID == uuid {
			r.RequestModel = model
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
	// Built-in defaults flow through unchanged.
	if got := slots["ANTHROPIC_DEFAULT_SONNET_MODEL"]; got != "tingly/cc-sonnet" {
		t.Fatalf("sonnet slot = %q, want tingly/cc-sonnet", got)
	}

	// Toggling 1M on the sonnet rule flows the [1m] suffix into that slot only.
	setRequestModel(t, cfg, config.RuleUUIDBuiltinCCSonnet, "tingly/cc-sonnet"+typ.ContextWindow1MTag)
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

	setRequestModel(t, cfg, config.RuleUUIDBuiltinCC, "tingly/cc"+typ.ContextWindow1MTag)
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

	// Baseline: profile separate mode uses short names.
	slots := resolveCCModelSlots(cfg, meta.ID, false)
	if got := slots["ANTHROPIC_DEFAULT_SONNET_MODEL"]; got != "sonnet" {
		t.Fatalf("profile sonnet slot = %q, want sonnet", got)
	}

	// Toggle 1M on the profile's sonnet rule (matched by short base name).
	profiledScenario := typ.ProfiledScenarioName(typ.ScenarioClaudeCode, meta.ID)
	for _, r := range cfg.GetRequestConfigs() {
		if r.GetScenario() == profiledScenario && r.RequestModel == "sonnet" {
			setRequestModel(t, cfg, r.UUID, "sonnet"+typ.ContextWindow1MTag)
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
