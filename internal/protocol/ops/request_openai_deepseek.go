package ops

import (
	"github.com/openai/openai-go/v3"
	"github.com/tingly-dev/tingly-box/internal/protocol"
)

// applyDeepSeekTransform applies DeepSeek's request shaping: the
// reasoning_content message conversion shared with Moonshot/Kimi (see
// convertThinkingToReasoningContent), plus DeepSeek's own reasoning_effort
// forwarding onto the low/high/max tiers its V4-Flash/V4-Pro thinking mode
// accepts (see applyLowHighMaxReasoningEffort).
func applyDeepSeekTransform(req *openai.ChatCompletionNewParams, providerURL, model string, config *protocol.OpenAIConfig) *openai.ChatCompletionNewParams {
	applyLowHighMaxReasoningEffort(req, config)
	convertThinkingToReasoningContent(req)
	return req
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
