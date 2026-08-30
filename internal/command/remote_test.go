package command

import (
	"testing"

	"github.com/tingly-dev/tingly-box/internal/db"
)

// TestStandaloneBotSettingPreservesClaudeProfileSelection covers
// standaloneBotSetting (remote.go): a bot's DefaultAgent/Scenarios carry the
// specific profile the operator selected (e.g. "claude_code:p1"), and
// standaloneBotSetting must not collapse that back down to the bare agent
// type when building the settings passed to runStandaloneBot.
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
