package protocoltest

import (
	"encoding/json"
	"testing"

	serverconfig "github.com/tingly-dev/tingly-box/internal/config"
)

// A request's model name has to reach the Claude Code built-in rule whichever
// spelling the client uses: new installs seed the short names ("cc"), older
// installs keep "tingly/cc", and settings files written for either end up
// pointed at the other. This drives the real gateway pipeline over HTTP.
func TestClaudeCodeGateway_ShortAndLegacyNamesRoute(t *testing.T) {
	env, err := NewAgentTestEnv(AgentTypeClaudeCode)
	if err != nil {
		t.Fatalf("NewAgentTestEnv: %v", err)
	}
	defer env.Close(false)
	if err := env.SetupAgent(AgentTypeClaudeCode, "virtual-claude", "claude-sonnet-4-5"); err != nil {
		t.Fatalf("SetupAgent: %v", err)
	}

	status := func(model string) int {
		t.Helper()
		body, _ := json.Marshal(map[string]any{
			"model":      model,
			"max_tokens": 64,
			"stream":     false,
			"messages":   []map[string]any{{"role": "user", "content": "What is the capital of France?"}},
		})
		res, err := env.ReplayFixture(AgentTypeClaudeCode, body, false)
		if err != nil {
			t.Fatalf("ReplayFixture(%q): %v", model, err)
		}
		return res.HTTPStatus
	}
	expect := func(model string, ok bool) {
		t.Helper()
		if got := status(model); (got == 200) != ok {
			t.Errorf("model %q: HTTP %d, want success=%v", model, got, ok)
		}
	}

	t.Run("new install seeded with the short name", func(t *testing.T) {
		expect("cc", true)
		expect("cc[1m]", true)
		expect("tingly/cc", true) // settings file written by an older version
		expect("tingly/cc[1m]", true)
		expect("not-a-rule", false) // control: the checks above are not vacuous
	})

	t.Run("older install keeping the prefixed name", func(t *testing.T) {
		cfg := env.appConfig.GetGlobalConfig()
		rule := cfg.GetRuleByUUID(serverconfig.RuleUUIDCC)
		if rule == nil {
			t.Fatal("unified built-in rule missing")
		}
		legacy := *rule
		legacy.RequestModel = serverconfig.CCTierByName(serverconfig.CCTierUnified).LegacyModel
		if err := cfg.UpdateRule(legacy.UUID, legacy); err != nil {
			t.Fatalf("UpdateRule: %v", err)
		}

		expect("tingly/cc", true)
		expect("cc", true) // settings file written by a newer version
		expect("cc[1m]", true)
		expect("not-a-rule", false)
	})
}
