package ops

import (
	"encoding/json"
	"strings"

	"github.com/openai/openai-go/v3"
	"github.com/tingly-dev/tingly-box/internal/protocol"
)

// applyDeepSeekTransform converts x_thinking field to reasoning_content for DeepSeek/Moonshot
// This is required by DeepSeek's and Moonshot's reasoning models
// The base conversion preserves thinking content in "x_thinking" field
func applyDeepSeekTransform(req *openai.ChatCompletionNewParams, providerURL, model string, config *protocol.OpenAIConfig) *openai.ChatCompletionNewParams {
	if config.CursorCompat {
		normalizeDeepSeekCursorContent(req)
	}
	// if has thinking, we should confirm each assistant contains `reasoning_content`
	if config.HasThinking {
		for i := range req.Messages {
			if req.Messages[i].OfAssistant != nil {
				// Convert the message to map to check/modify fields
				msgMap := req.Messages[i].ExtraFields()
				if msgMap == nil {
					msgMap = map[string]any{}
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
					// Use a placeholder (empty pointer) instead of empty string to ensure it's included in JSON
					var emptyStr string
					msgMap["reasoning_content"] = &emptyStr
				}

				// Convert back to message param
				req.Messages[i].SetExtraFields(msgMap)
			}
		}
	}
	return req
}

func normalizeDeepSeekCursorContent(req *openai.ChatCompletionNewParams) {
	for i := range req.Messages {
		msgMap, err := MessageToMap(req.Messages[i])
		if err != nil {
			continue
		}
		content, ok := msgMap["content"]
		if !ok {
			continue
		}
		contentParts, ok := content.([]interface{})
		if !ok {
			continue
		}
		flattened, _ := flattenRichContent(contentParts)
		msgMap["content"] = flattened

		updatedBytes, err := json.Marshal(msgMap)
		if err != nil {
			continue
		}
		var updated openai.ChatCompletionMessageParamUnion
		if err := json.Unmarshal(updatedBytes, &updated); err != nil {
			continue
		}
		req.Messages[i] = updated
	}
}

func flattenRichContent(parts []interface{}) (string, bool) {
	var segments []string
	var dropped bool
	for _, part := range parts {
		switch value := part.(type) {
		case string:
			if strings.TrimSpace(value) != "" {
				segments = append(segments, value)
			}
		case map[string]interface{}:
			if textValue, ok := value["text"].(string); ok {
				if strings.TrimSpace(textValue) != "" {
					segments = append(segments, textValue)
				}
			} else if contentValue, ok := value["content"].(string); ok {
				if strings.TrimSpace(contentValue) != "" {
					segments = append(segments, contentValue)
				}
			} else {
				dropped = true
			}
		default:
			dropped = true
		}
	}
	if len(segments) == 0 && dropped {
		return "[non-text content omitted]", true
	}
	if dropped {
		segments = append(segments, "[non-text content omitted]")
	}
	return strings.Join(segments, "\n"), dropped
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
