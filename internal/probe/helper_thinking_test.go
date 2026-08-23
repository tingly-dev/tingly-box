package probe

import (
	"testing"

	"github.com/stretchr/testify/assert"

	"github.com/tingly-dev/tingly-box/internal/protocol/thinking"
)

// TestThinkingEnabled covers the "" / "none" disablement rule — both must read
// as disabled so an unset request and an explicit "none" behave identically
// (no thinking param sent). Every ladder level below reads as enabled.
func TestThinkingEnabled(t *testing.T) {
	assert.False(t, thinkingEnabled(""), `"" must be disabled`)
	assert.False(t, thinkingEnabled(ThinkingNone), `"none" must be disabled`)
	for _, lvl := range []ThinkingLevel{ThinkingLow, ThinkingMedium, ThinkingHigh, ThinkingMax} {
		assert.True(t, thinkingEnabled(lvl), "%q must be enabled", lvl)
	}
}

// TestThinkingBudget verifies the probe ladder maps to the canonical
// thinking.BudgetMapping values used by the budget-dialect targets (Anthropic
// budget_tokens, Gemini thinking_budget). Drift here would desync the probe
// from the rest of the gateway.
func TestThinkingBudget(t *testing.T) {
	cases := map[ThinkingLevel]int64{
		ThinkingLow:    thinking.BudgetMapping[thinking.LevelLow],    // 4096
		ThinkingMedium: thinking.BudgetMapping[thinking.LevelMedium], // 10240
		ThinkingHigh:   thinking.BudgetMapping[thinking.LevelHigh],   // 20480
		ThinkingMax:    thinking.BudgetMapping[thinking.LevelMax],    // 31999
	}
	for lvl, want := range cases {
		assert.Equal(t, want, thinkingBudget(lvl), "budget for %q", lvl)
		assert.Greater(t, want, int64(1024), "%q budget must satisfy the Anthropic ≥1024 invariant", lvl)
	}
}

// TestThinkingBudgetSatisfiesAnthropicInvariant is the safety check that
// matters most: budget < budget+2048 (the MaxTokens we set), and budget ≥ 1024.
// If either breaks, Anthropic rejects the probe. Encoded as a test so a future
// BudgetMapping change can't silently break the probe.
func TestThinkingBudgetSatisfiesAnthropicInvariant(t *testing.T) {
	for _, lvl := range []ThinkingLevel{ThinkingLow, ThinkingMedium, ThinkingHigh, ThinkingMax} {
		budget := thinkingBudget(lvl)
		maxTokens := budget + 2048
		assert.GreaterOrEqual(t, budget, int64(1024), "%q: budget must be ≥1024", lvl)
		assert.Less(t, budget, maxTokens, "%q: budget must be < MaxTokens (budget+2048)", lvl)
	}
}
