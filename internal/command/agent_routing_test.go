package command

import (
	"testing"

	"github.com/tingly-dev/tingly-box/internal/agent"
	"github.com/tingly-dev/tingly-box/internal/data/db"
	"github.com/tingly-dev/tingly-box/internal/typ"
)

// TestAgentRoutingKey locks down the (agent type → request model, scenario)
// mapping that apply / show / restore all use to look up routing rules. The
// strings must match the canonical request models registered by the rule
// system (internal/agent/rule.go) — drift previously caused `agent show
// opencode` to always report "No routing rule configured" even after apply.
func TestAgentRoutingKey(t *testing.T) {
	cases := []struct {
		agentType    agent.AgentType
		wantModel    string
		wantScenario typ.RuleScenario
	}{
		{agent.AgentTypeClaudeCode, "tingly/cc", typ.ScenarioClaudeCode},
		{agent.AgentTypeOpenCode, "tingly-opencode", typ.ScenarioOpenCode},
		{agent.AgentTypeCodex, "tingly-codex", typ.ScenarioCodex},
	}

	for _, tc := range cases {
		t.Run(string(tc.agentType), func(t *testing.T) {
			gotModel, gotScenario, err := agentRoutingKey(tc.agentType)
			if err != nil {
				t.Fatalf("agentRoutingKey(%q) returned error: %v", tc.agentType, err)
			}
			if gotModel != tc.wantModel {
				t.Errorf("request model: got %q, want %q", gotModel, tc.wantModel)
			}
			if gotScenario != tc.wantScenario {
				t.Errorf("scenario: got %q, want %q", gotScenario, tc.wantScenario)
			}
		})
	}
}

func TestAgentRoutingKeyUnknown(t *testing.T) {
	_, _, err := agentRoutingKey(agent.AgentType("not-a-real-agent"))
	if err == nil {
		t.Fatal("expected error for unknown agent type, got nil")
	}
}

func TestStandaloneBotSettingPreservesClaudeProfileSelection(t *testing.T) {
	got := standaloneBotSetting(db.Settings{
		UUID:         "bot-1",
		Auth:         map[string]string{"token": "secret"},
		DefaultAgent: "claude_code:p1",
		Scenarios:    `[{"scenario":"claude_code:p1"}]`,
	}, "provider-1", "model-1")

	if got.DefaultAgent != "claude_code:p1" {
		t.Fatalf("DefaultAgent = %q, want claude_code:p1", got.DefaultAgent)
	}
	if got.Scenarios != `[{"scenario":"claude_code:p1"}]` {
		t.Fatalf("Scenarios = %q", got.Scenarios)
	}
}
