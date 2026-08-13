package ops

import (
	"strings"

	"github.com/openai/openai-go/v3"
	"github.com/openai/openai-go/v3/shared"
	"github.com/tingly-dev/tingly-box/internal/protocol"
	"github.com/tingly-dev/tingly-box/internal/protocol/thinking"
)

// applyDeepSeekTransform converts x_thinking field to reasoning_content for DeepSeek/Moonshot
// This is required by DeepSeek's and Moonshot's reasoning models
// The base conversion preserves thinking content in "x_thinking" field
func applyDeepSeekTransform(req *openai.ChatCompletionNewParams, providerURL, model string, config *protocol.OpenAIConfig) *openai.ChatCompletionNewParams {
	if isDeepSeekVendor(providerURL, model) {
		applyDeepSeekReasoningEffort(req, config)
	}

	for i := range req.Messages {
		if req.Messages[i].OfAssistant != nil {
			// Read/write extra fields on OfAssistant (variant level) for consistency.
			msgMap := req.Messages[i].OfAssistant.ExtraFields()
			if msgMap == nil {
				msgMap = map[string]any{}
			}

			// Extract x_thinking and convert to reasoning_content
			if thinking, hasThinking := msgMap["x_thinking"]; hasThinking {
				if thinkingStr, ok := thinking.(string); ok {
					msgMap["reasoning_content"] = thinkingStr
				}
				delete(msgMap, "x_thinking")
			} else if _, hasReasoning := msgMap["reasoning_content"]; !hasReasoning {
				// DeepSeek requires reasoning_content on assistant messages, especially
				// those with tool_calls. Per DeepSeek docs: "For turns that do perform
				// tool calls, the reasoning_content must be fully passed back to the API
				// in all subsequent requests."
				msgMap["reasoning_content"] = ""
			}

			req.Messages[i].OfAssistant.SetExtraFields(msgMap)
		}
	}
	return req
}

// isDeepSeekVendor reports whether the request targets DeepSeek proper (as
// opposed to Moonshot/Kimi, which share the reasoning_content message shape
// but not DeepSeek's reasoning_effort ladder).
func isDeepSeekVendor(providerURL, model string) bool {
	return strings.Contains(strings.ToLower(providerURL), "deepseek.com") ||
		strings.Contains(strings.ToLower(model), "deepseek")
}

// applyDeepSeekReasoningEffort forwards the resolved thinking-effort signal
// to DeepSeek as reasoning_effort, collapsed onto the three tiers DeepSeek's
// V4-Flash/V4-Pro thinking mode actually accepts: low/high/max. Earlier
// DeepSeek models had no effort dial at all (thinking was an on/off switch
// baked into the model alias, deepseek-chat vs deepseek-reasoner), so this
// value was previously dropped on the floor for every DeepSeek request.
//
// No actionable signal leaves reasoning_effort unset — DeepSeek keeps its
// own default rather than being forced into thinking mode.
func applyDeepSeekReasoningEffort(req *openai.ChatCompletionNewParams, config *protocol.OpenAIConfig) {
	effort := resolveDeepSeekEffort(req, config)
	if effort == "" {
		req.ReasoningEffort = ""
		return
	}
	req.ReasoningEffort = shared.ReasoningEffort(deepSeekReasoningEffortTier(effort))
}

// resolveDeepSeekEffort picks the effort level driving the DeepSeek request,
// mirroring the Gemini resolution order (see resolveGeminiEffort):
//  1. req.ReasoningEffort — set by a forced thinking_effort rule flag, or by
//     an OpenAI-native client sending reasoning_effort directly.
//  2. config.ReasoningEffort — derived during Anthropic→OpenAI conversion
//     (explicit output_config.effort, or budget_tokens tiered onto the
//     ladder).
//
// "none" (explicit disable) and "" (no signal) both resolve to "" so thinking
// is left at DeepSeek's own default rather than forced on.
func resolveDeepSeekEffort(req *openai.ChatCompletionNewParams, config *protocol.OpenAIConfig) string {
	if effort := string(req.ReasoningEffort); effort != "" && effort != "none" {
		return effort
	}
	if config != nil && config.HasThinking && config.ReasoningEffort != "" && config.ReasoningEffort != "none" {
		return string(config.ReasoningEffort)
	}
	return ""
}

// deepSeekReasoningEffortTier collapses the canonical six-level thinking
// ladder onto DeepSeek's three reasoning_effort tiers:
//   - minimal/low  -> "low"  (everyday, non-agentic use)
//   - medium/high  -> "high" (everyday agent tasks)
//   - xhigh/max    -> "max"  (harder, more complex tasks)
func deepSeekReasoningEffortTier(effort string) string {
	switch effort {
	case thinking.LevelXHigh, thinking.LevelMax:
		return "max"
	case thinking.LevelMedium, thinking.LevelHigh:
		return "high"
	default: // minimal, low, or an unrecognized value
		return "low"
	}
}
