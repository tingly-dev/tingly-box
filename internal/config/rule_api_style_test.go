package config

import (
	"strings"
	"testing"

	"github.com/tingly-dev/tingly-box/internal/loadbalance"
	"github.com/tingly-dev/tingly-box/internal/protocol"
	"github.com/tingly-dev/tingly-box/internal/typ"
)

// seedStyledProvider saves an enabled provider with a fixed api_style.
func seedStyledProvider(t *testing.T, cfg *Config, uuid string, style protocol.APIStyle) {
	t.Helper()
	seedProviderFull(t, cfg, uuid, style, "")
}

// seedProviderFull saves an enabled provider with a fixed api_style and an
// optional decision fork URL.
func seedProviderFull(t *testing.T, cfg *Config, uuid string, style protocol.APIStyle, decisionBase string) {
	t.Helper()
	if err := cfg.StoreManager().Provider().Save(&typ.Provider{
		UUID:            uuid,
		Name:            uuid,
		APIBase:         "https://example.invalid/v1",
		APIStyle:        style,
		APIBaseDecision: decisionBase,
		Enabled:         true,
	}); err != nil {
		t.Fatalf("failed to seed provider %s: %v", uuid, err)
	}
}

func styledRule(uuid string, scenario typ.RuleScenario, providerUUID string) typ.Rule {
	return typ.Rule{
		UUID:          uuid,
		Scenario:      scenario,
		RequestModel:  uuid + "-model",
		ResponseModel: "resp-model",
		Active:        true,
		Services: []*loadbalance.Service{
			{Provider: providerUUID, Model: "m", Active: true},
		},
		LBTactic: typ.Tactic{Type: loadbalance.TacticRandom, Params: typ.DefaultRandomParams()},
	}
}

// TestRuleRejectsIncompatibleProviderAPIStyle pins the provider × scenario
// binding matrix (.design/decision-protocol.md §4): an incompatible pairing is
// refused at save time — where it is created — instead of failing on the first
// request. Decision is capability-gated: any chat-style provider with a
// decision fork URL serves it (users never pick a provider by protocol),
// while a provider without the fork is refused. Jev-native providers cannot
// serve chat scenarios in return.
func TestRuleRejectsIncompatibleProviderAPIStyle(t *testing.T) {
	dir := t.TempDir()
	cfg, err := NewConfigWithDir(dir)
	if err != nil {
		t.Fatalf("NewConfigWithDir failed: %v", err)
	}
	defer cfg.CloseStores()

	seedStyledProvider(t, cfg, "prov-openai", protocol.APIStyleOpenAI)
	seedProviderFull(t, cfg, "prov-openai-decision", protocol.APIStyleOpenAI, "https://decision.example.invalid/api/v1")
	seedStyledProvider(t, cfg, "prov-decision", protocol.APIStyleDecision)

	t.Run("decision rule rejects provider without a decision endpoint", func(t *testing.T) {
		err := cfg.AddRule(styledRule("rej-1", typ.ScenarioDecision, "prov-openai"))
		if err == nil {
			t.Fatal("expected AddRule to reject a provider without a decision endpoint under the decision scenario")
		}
		if !strings.Contains(err.Error(), "decision endpoint") {
			t.Errorf("error should name the missing decision endpoint: %v", err)
		}
	})

	t.Run("decision rule accepts chat provider with a decision fork", func(t *testing.T) {
		if err := cfg.AddRule(styledRule("acc-1", typ.ScenarioDecision, "prov-openai-decision")); err != nil {
			t.Fatalf("AddRule failed: %v", err)
		}
	})

	t.Run("decision rule accepts Jev-native provider", func(t *testing.T) {
		if err := cfg.AddRule(styledRule("acc-2", typ.ScenarioDecision, "prov-decision")); err != nil {
			t.Fatalf("AddRule failed: %v", err)
		}
	})

	t.Run("chat scenario rejects Jev-native provider", func(t *testing.T) {
		if err := cfg.AddRule(styledRule("rej-2", typ.ScenarioOpenAI, "prov-decision")); err == nil {
			t.Fatal("expected AddRule to reject a Jev-native provider under the openai scenario")
		}
	})
}
