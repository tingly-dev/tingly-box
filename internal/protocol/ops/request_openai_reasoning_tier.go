package ops

import (
	"github.com/openai/openai-go/v3"
	"github.com/openai/openai-go/v3/shared"
	"github.com/tingly-dev/tingly-box/internal/protocol"
	"github.com/tingly-dev/tingly-box/internal/protocol/thinking"
)

// applyLowHighMaxReasoningEffort forwards the resolved thinking-effort
// signal as reasoning_effort, collapsed onto the low/high/max tiers shared
// by DeepSeek's V4-Flash/V4-Pro and Moonshot/Kimi's K3 thinking modes.
// Earlier models in both families had no effort dial at all (thinking was
// an on/off switch baked into the model alias), so this value used to be
// dropped on the floor for every request to either vendor.
//
// No actionable signal leaves reasoning_effort unset — the vendor keeps its
// own default rather than being forced into thinking mode.
func applyLowHighMaxReasoningEffort(req *openai.ChatCompletionNewParams, config *protocol.OpenAIConfig) {
	effort := resolveReasoningEffort(req, config)
	if effort == "" {
		req.ReasoningEffort = ""
		return
	}
	req.ReasoningEffort = shared.ReasoningEffort(lowHighMaxTier(effort))
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
	if effort := string(req.ReasoningEffort); effort != "" && effort != "none" {
		return effort
	}
	if config != nil && config.HasThinking && config.ReasoningEffort != "" && config.ReasoningEffort != "none" {
		return string(config.ReasoningEffort)
	}
	return ""
}

// lowHighMaxTier collapses the canonical six-level thinking ladder onto the
// low/high/max reasoning_effort scheme DeepSeek V4 and Moonshot/Kimi K3 both
// use:
//   - minimal/low  -> "low"  (everyday, non-agentic use)
//   - medium/high  -> "high" (everyday agent tasks)
//   - xhigh/max    -> "max"  (harder, more complex tasks)
func lowHighMaxTier(effort string) string {
	switch effort {
	case thinking.LevelXHigh, thinking.LevelMax:
		return "max"
	case thinking.LevelMedium, thinking.LevelHigh:
		return "high"
	default: // minimal, low, or an unrecognized value
		return "low"
	}
}
