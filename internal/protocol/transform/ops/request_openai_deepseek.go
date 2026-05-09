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
	// if has thinking, we should confirm each assistant contains `reasoning_content`
	// deepseek do not regard to config's has thinking field.
	//if config.HasThinking {
	for i := range req.Messages {
		if req.Messages[i].OfAssistant != nil {
			// Anthropic -> OpenAI conversion stores x_thinking on the union as
			// internal metadata, but ChatCompletionMessageParamUnion marshals
			// only its concrete OfAssistant variant when present.
			unionExtra := req.Messages[i].ExtraFields()
			assistantExtra := req.Messages[i].OfAssistant.ExtraFields()

			// Preserve existing assistant-level extras; these are the fields
			// that actually reach the provider in the final JSON payload.
			msgMap := map[string]any{}
			for k, v := range assistantExtra {
				msgMap[k] = v
			}

			thinking, hasThinking := unionExtra["x_thinking"]
			if !hasThinking {
				thinking, hasThinking = assistantExtra["x_thinking"]
			}

			// Extract x_thinking and convert to reasoning_content
			if hasThinking {
				// Convert x_thinking to reasoning_content
				if thinkingStr, ok := thinking.(string); ok {
					msgMap["reasoning_content"] = thinkingStr
				}
				// Remove x_thinking field
				delete(msgMap, "x_thinking")
			} else if _, hasReasoning := msgMap["reasoning_content"]; !hasReasoning {
				// Ensure reasoning_content field exists even if no thinking content
				// Use a placeholder (empty pointer) instead of empty string to ensure it's included in JSON
				var emptyStr string
				msgMap["reasoning_content"] = &emptyStr
			}

			// ChatCompletionMessageParamUnion marshals the concrete assistant variant,
			// so provider-specific extras must live on OfAssistant to reach the wire.
			req.Messages[i].OfAssistant.SetExtraFields(msgMap)
		}
	}
	//}
	return req
}
