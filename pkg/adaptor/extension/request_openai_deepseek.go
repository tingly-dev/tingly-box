package extension

import (
	"encoding/json"

	"tingly-box/internal/typ"

	"github.com/openai/openai-go/v3"
)

// applyDeepSeekTransform converts x_thinking field to reasoning_content for DeepSeek
// This is required by DeepSeek's reasoning models (e.g., deepseek-reasoner)
// The base conversion preserves thinking content in "x_thinking" field
func applyDeepSeekTransform(req *openai.ChatCompletionNewParams, provider *typ.Provider, model string) *openai.ChatCompletionNewParams {
	for i := range req.Messages {
		if req.Messages[i].OfAssistant != nil {
			// Convert the message to map to check/modify fields
			msgMap := req.Messages[i].ExtraFields()
			if msgMap == nil {
				continue
			}

			// Extract x_thinking and convert to reasoning_content
			if thinking, hasThinking := msgMap["x_thinking"]; hasThinking {
				// Convert x_thinking to reasoning_content
				if thinkingStr, ok := thinking.(string); ok {
					msgMap["reasoning_content"] = thinkingStr
				}
				// Remove x_thinking field
				delete(msgMap, "x_thinking")
			} else {
				// Ensure reasoning_content field exists even if no thinking content
				if _, has := msgMap["reasoning_content"]; !has {
					msgMap["reasoning_content"] = ""
				}
			}

			// Convert back to message param
			msgBytes, _ := json.Marshal(msgMap)
			_ = json.Unmarshal(msgBytes, &req.Messages[i])
		}
	}
	return req
}

// MessageToMap converts a ChatCompletionMessageParamUnion to a map for modification
func MessageToMap(msg openai.ChatCompletionMessageParamUnion) (map[string]interface{}, error) {
	msgBytes, err := json.Marshal(msg)
	if err != nil {
		return nil, err
	}

	var result map[string]interface{}
	if err := json.Unmarshal(msgBytes, &result); err != nil {
		return nil, err
	}
	return result, nil
}
