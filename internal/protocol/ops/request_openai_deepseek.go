package ops

import (
	"github.com/openai/openai-go/v3"
	"github.com/openai/openai-go/v3/shared"
	"github.com/tingly-dev/tingly-box/internal/protocol"
	"github.com/tingly-dev/tingly-box/internal/protocol/thinking"
)

// deepSeekEffortTiers is DeepSeek's own reasoning_effort tier map, built
// fresh so it can diverge from kimiEffortTiers without touching the shared
// transform (they hold equal but independent maps — not the same map value,
// which sharing a Go map reference would make them, silently coupling any
// future edit to one into the other).
var deepSeekEffortTiers = lowHighMaxEffortTiers()

// deepSeekDefaultEffort is the level DeepSeek documents for thinking mode
// when reasoning_effort is not given: "Thinking mode is enabled by default,
// with the default effort being high"
// (https://api-docs.deepseek.com/guides/thinking_mode).
const deepSeekDefaultEffort = thinking.LevelHigh

// applyDeepSeekTransform applies DeepSeek's request shaping: the
// reasoning_content message conversion shared with Moonshot/Kimi (see
// convertThinkingToReasoningContent), plus DeepSeek's own reasoning_effort
// forwarding through deepSeekEffortTiers.
//
// A client that switches thinking on with DeepSeek's own toggle
// (extra_body {"thinking": {"type": "enabled"}}) but gives no
// reasoning_effort gets DeepSeek's documented default written out, so the
// level the request runs at is visible on the wire rather than implied.
func applyDeepSeekTransform(req *openai.ChatCompletionNewParams, providerURL, model string, config *protocol.OpenAIConfig) *openai.ChatCompletionNewParams {
	clientEffort := req.ReasoningEffort
	applyReasoningEffortTier(req, config, deepSeekEffortTiers)
	if clientEffort == "" && req.ReasoningEffort == "" && deepSeekThinkingEnabled(req) {
		req.ReasoningEffort = shared.ReasoningEffort(deepSeekEffortTiers[deepSeekDefaultEffort])
	}
	convertThinkingToReasoningContent(req)
	return req
}

// deepSeekThinkingEnabled reports whether the request carries DeepSeek's
// thinking toggle switched on: {"thinking": {"type": "enabled"}}.
func deepSeekThinkingEnabled(req *openai.ChatCompletionNewParams) bool {
	toggle, ok := req.ExtraFields()["thinking"].(map[string]any)
	if !ok {
		return false
	}
	kind, _ := toggle["type"].(string)
	return kind == "enabled"
}

// convertThinkingToReasoningContent converts the x_thinking field to
// reasoning_content on assistant messages. Required by both DeepSeek's and
// Moonshot/Kimi's reasoning models, so it's shared by applyDeepSeekTransform
// and applyKimiTransform.
func convertThinkingToReasoningContent(req *openai.ChatCompletionNewParams) {
	for i := range req.Messages {
		if req.Messages[i].OfAssistant != nil {
			// Read/write extra fields on OfAssistant (variant level) for consistency.
			msgMap := req.Messages[i].OfAssistant.ExtraFields()
			if msgMap == nil {
				msgMap = map[string]any{}
			}

			// Extract x_thinking and convert to reasoning_content
			if val, hasThinking := msgMap["x_thinking"]; hasThinking {
				if thinkingStr, ok := val.(string); ok {
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
}
