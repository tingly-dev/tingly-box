package ops

import (
	"github.com/openai/openai-go/v3"
	"github.com/tingly-dev/tingly-box/internal/protocol"
)

// applyDeepSeekTransform converts x_thinking field to reasoning_content for DeepSeek/Moonshot
// This is required by DeepSeek's and Moonshot's reasoning models
// The base conversion preserves thinking content in "x_thinking" field
func applyDeepSeekTransform(req *openai.ChatCompletionNewParams, providerURL, model string, config *protocol.OpenAIConfig) *openai.ChatCompletionNewParams {
	if config.CursorCompat {
		normalizeCursorContent(req)
	}
	for i := range req.Messages {
		if req.Messages[i].OfAssistant != nil {
			// Read/write extra fields on OfAssistant (variant level), not on union level.
			// MarshalUnion only serializes the active variant — union-level ExtraFields are dropped.
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
