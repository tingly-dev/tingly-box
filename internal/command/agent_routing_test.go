package command

import (
	"testing"

	"github.com/tingly-dev/tingly-box/internal/db"
)

// The (agent type → request model, scenario) routing-key mapping that
// apply / show / restore all use to look up routing rules now lives in
// internal/usecase.AgentUseCase.RoutingKey — see
// internal/usecase/agent_test.go:TestAgentUseCase_RoutingKey for its
// coverage. Moved here previously to lock down drift between this
// package's agentRoutingKey and tui/agent_mode.go's agentRequestModel;
// both call sites now call the single usecase-owned function instead.

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
