package agent

import (
	"testing"

	aiagent "github.com/tingly-dev/tingly-box/ai/agent"
	serverconfig "github.com/tingly-dev/tingly-box/internal/config"
)

// Every slot declared in CCTiers needs a typed ClaudeCodePrefs field, or the
// slot would be silently dropped from prefs and the settings file.
func TestModelSlotsAreTypedFields(t *testing.T) {
	var p ClaudeCodePrefs
	for _, tier := range serverconfig.CCSlotTiers() {
		if p.modelSlot(tier.EnvKey) == nil {
			t.Errorf("tier %q: no ClaudeCodePrefs field for slot %s", tier.Name, tier.EnvKey)
		}
	}
	if p.modelSlot("API_TIMEOUT_MS") != nil {
		t.Error("non-slot keys must not resolve to a slot field")
	}
	got := ClaudeCodePrefs{}.WithModelSlots(map[string]string{"ANTHROPIC_DEFAULT_FABLE_MODEL": "x", "NOT_A_SLOT": "y"})
	if got.AnthropicDefaultFableModel != "x" {
		t.Errorf("WithModelSlots did not set the fable slot: %+v", got)
	}
}

// ai/agent is a standalone library and keeps its own list of slots; this keeps
// it in step with the tier table.
func TestAIAgentWritesEverySlot(t *testing.T) {
	env := (&aiagent.ClaudeCodeParams{}).BuildEnv()
	for _, tier := range serverconfig.CCSlotTiers() {
		if _, ok := env[tier.EnvKey]; !ok {
			t.Errorf("ai/agent BuildEnv does not write %s (tier %q)", tier.EnvKey, tier.Name)
		}
	}
}
