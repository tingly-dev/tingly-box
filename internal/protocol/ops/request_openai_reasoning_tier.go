package ops

import (
	"github.com/openai/openai-go/v3"
	"github.com/openai/openai-go/v3/shared"
	"github.com/tingly-dev/tingly-box/internal/protocol"
	"github.com/tingly-dev/tingly-box/internal/protocol/thinking"
)

// reasoningEffortTierMap collapses the canonical six-level thinking-effort
// ladder (internal/protocol/thinking) onto a vendor's own reasoning_effort
// enum. Each vendor family that needs this keeps its own map (see
// deepSeekEffortTiers, kimiEffortTiers) even where the values happen to
// match today — a future vendor, or a future divergence between existing
// ones (e.g. Kimi's rollout only accepting a subset at first), is then just
// a different map, not a new hand-rolled transform.
type reasoningEffortTierMap map[string]string

// lowHighMaxEffortTiers is the low/high/max scheme DeepSeek's
// V4-Flash/V4-Pro and Moonshot/Kimi's K3 both document today:
//   - minimal/low  -> "low"  (everyday, non-agentic use)
//   - medium/high  -> "high" (everyday agent tasks)
//   - xhigh/max    -> "max"  (harder, more complex tasks)
func lowHighMaxEffortTiers() reasoningEffortTierMap {
	return reasoningEffortTierMap{
		thinking.LevelMinimal: "low",
		thinking.LevelLow:     "low",
		thinking.LevelMedium:  "high",
		thinking.LevelHigh:    "high",
		thinking.LevelXHigh:   "max",
		thinking.LevelMax:     "max",
	}
}

// genericEffortTiers is the safe fallback for any OpenAI-compatible vendor
// not confirmed (via supportsExplicitPromptCache) to accept the full
// six-level ladder: collapses onto the low/medium/high range every
// OpenAI-compatible clone has historically documented, rather than
// forwarding "minimal"/"xhigh"/"max" verbatim to a vendor that may 400 on
// the unrecognized enum member. All six levels are listed explicitly, even
// where the value is unchanged, so the map doesn't rely on
// applyReasoningEffortTier's passthrough-on-miss behavior to be complete.
// See .design/model-data.md.
func genericEffortTiers() reasoningEffortTierMap {
	return reasoningEffortTierMap{
		thinking.LevelMinimal: "low",
		thinking.LevelLow:     "low",
		thinking.LevelMedium:  "medium",
		thinking.LevelHigh:    "high",
		thinking.LevelXHigh:   "high",
		thinking.LevelMax:     "high",
	}
}

// applyReasoningEffortTier forwards the resolved thinking-effort signal as
// reasoning_effort, collapsed through the given tier map. Earlier models in
// the DeepSeek/Kimi family had no effort dial at all (thinking was an on/off
// switch baked into the model alias), so this value used to be dropped on
// the floor for every request to either vendor.
//
// No actionable signal leaves reasoning_effort unset — the vendor keeps its
// own default rather than being forced into thinking mode. A signal with no
// entry in tiers (an already vendor-native value, or an unrecognized one)
// passes through unchanged rather than being silently dropped.
func applyReasoningEffortTier(req *openai.ChatCompletionNewParams, config *protocol.OpenAIConfig, tiers reasoningEffortTierMap) {
	effort := resolveReasoningEffort(req, config)
	if effort == "" {
		req.ReasoningEffort = ""
		return
	}
	if mapped, ok := tiers[effort]; ok {
		effort = mapped
	}
	req.ReasoningEffort = shared.ReasoningEffort(effort)
}

// resolveReasoningEffort picks the effort level driving the request,
// mirroring the Gemini resolution order (see resolveGeminiEffort):
//  1. req.ReasoningEffort — set by a forced thinking_effort rule flag, or by
//     an OpenAI-native client sending reasoning_effort directly.
//  2. config.ReasoningEffort — derived during Anthropic→OpenAI conversion
//     (explicit output_config.effort, or budget_tokens tiered onto the
//     ladder).
//
// "none" (explicit disable) and "" (no signal) both resolve to "" so
// thinking is left at the vendor's own default rather than forced on.
func resolveReasoningEffort(req *openai.ChatCompletionNewParams, config *protocol.OpenAIConfig) string {
	if effort := string(req.ReasoningEffort); effort != "" {
		if effort == "none" {
			return ""
		}
		return effort
	}
	if config != nil && config.HasThinking && config.ReasoningEffort != "" && config.ReasoningEffort != "none" {
		return string(config.ReasoningEffort)
	}
	return ""
}
