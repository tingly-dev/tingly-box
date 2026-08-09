// Package thinking holds the canonical thinking-effort ladder shared across
// the gateway. It is a leaf package (no internal deps) so both internal/typ
// and the protocol conversion layer can import it without cycles.
//
// Effort is the primary axis across providers: OpenAI reasoning_effort
// (none/minimal/low/medium/high/xhigh/max), Anthropic output_config.effort
// (low/medium/high/xhigh/max, Claude 4.5+), and Gemini 3 thinking_level
// (minimal/low/medium/high) are all effort-based. Token budgets remain only
// as the fallback dialect for budget-based targets (Claude ≤ 4.5
// thinking.budget_tokens, Gemini 2.5 thinking_budget) via BudgetMapping /
// EffortFromBudget.
package thinking

// Level represents a thinking effort level for extended thinking.
type Level = string

const (
	// LevelDefault is the "by client" sentinel: pass the client's thinking
	// config through unchanged. Empty string so omitempty hides it.
	LevelDefault Level = ""
	// LevelOff is the "explicitly disabled" sentinel: strip thinking from the
	// outbound request regardless of what the client sent. (OpenAI's "none"
	// level maps here, not to a ladder entry.)
	LevelOff     Level = "off"
	LevelMinimal Level = "minimal"
	LevelLow     Level = "low"
	LevelMedium  Level = "medium"
	LevelHigh    Level = "high"
	LevelXHigh   Level = "xhigh"
	LevelMax     Level = "max"
)

// BudgetMapping defines fallback budget_tokens for each effort level, used
// only when the target speaks budgets instead of effort levels.
// "off" / "" are intentionally absent — they signal disabled / pass-through,
// not a budget value, and are handled out-of-band by the transform layer.
var BudgetMapping = map[Level]int64{
	LevelMinimal: 1024,  // ~1K tokens - Anthropic's minimum for extended thinking
	LevelLow:     4096,  // ~4K tokens - light reasoning (Claude Code "think")
	LevelMedium:  10240, // ~10K tokens - balanced (Claude Code "megathink")
	LevelHigh:    20480, // ~20K tokens - deep reasoning
	LevelXHigh:   24576, // ~24K tokens - deeper reasoning
	LevelMax:     31999, // ~32K tokens - maximum (Claude Code "ultrathink")
}

// EffortFromBudget is the inverse of BudgetMapping: it tiers an explicit
// budget_tokens value back onto the effort ladder. This is the single
// canonical budget→effort conversion for effort-based targets fed by
// budget-based clients. Thresholds are inclusive of each tier's own budget so
// the two mappings round-trip.
func EffortFromBudget(budget int64) Level {
	switch {
	case budget <= BudgetMapping[LevelMinimal]:
		return LevelMinimal
	case budget <= BudgetMapping[LevelLow]:
		return LevelLow
	case budget <= BudgetMapping[LevelMedium]:
		return LevelMedium
	case budget <= BudgetMapping[LevelHigh]:
		return LevelHigh
	case budget <= BudgetMapping[LevelXHigh]:
		return LevelXHigh
	default:
		return LevelMax
	}
}
