package ops

import (
	"encoding/json"
	"strings"

	"github.com/openai/openai-go/v3"
	"github.com/tingly-dev/tingly-box/internal/protocol"
)

// ApplyProviderTransforms applies provider-specific transformations to an
// OpenAI Chat request. The dispatch is a flat strings.Contains chain — short,
// explicit, and parallel to the per-shape dispatch in VendorTransform.
//
// New providers are added as new cases here; aliases (e.g. multiple URLs that
// share a vendor's quirks) sit in the same case body.
func ApplyProviderTransforms(req *openai.ChatCompletionNewParams, providerURL, model string, config *protocol.OpenAIConfig) *openai.ChatCompletionNewParams {
	url := strings.ToLower(providerURL)
	modelLower := strings.ToLower(model)

	switch {
	case strings.Contains(url, "api.deepseek.com"),
		strings.Contains(url, "api.moonshot.cn"),
		strings.Contains(url, "api.moonshot.ai"),
		strings.Contains(url, "api.kimi.com/coding/v1"),
		strings.Contains(url, "opencode.ai/zen/go") && strings.Contains(modelLower, "deepseek"):
		return applyDeepSeekTransform(req, providerURL, model, config)

	case strings.Contains(url, "generativelanguage.googleapis.com") && strings.Contains(modelLower, "gemini"):
		return applyGeminiTransform(req, providerURL, model, config)

	case strings.Contains(url, "poe.com") && strings.Contains(modelLower, "gemini"):
		return applyGeminiPoeTransform(req, providerURL, model, config)
	}

	return applyDefaultTransform(req, config)
}

// ApplyCursorCompatContentNormalization flattens rich content in messages for
// Cursor compatibility. Applies to ALL providers when cursor_compat is enabled.
func ApplyCursorCompatContentNormalization(req *openai.ChatCompletionNewParams) {
	for i := range req.Messages {
		msgMap, err := messageToMap(req.Messages[i])
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

func messageToMap(msg openai.ChatCompletionMessageParamUnion) (map[string]interface{}, error) {
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

// applyDefaultTransform applies the standard OpenAI-compatible thinking
// fallback when no vendor-specific transform matched. Sets reasoning_effort
// from config, or falls back to a `thinking.type=enabled` extra field for
// providers that accept the Anthropic-style extension.
func applyDefaultTransform(req *openai.ChatCompletionNewParams, config *protocol.OpenAIConfig) *openai.ChatCompletionNewParams {
	if config.HasThinking && config.ReasoningEffort != "" {
		req.ReasoningEffort = config.ReasoningEffort
	} else if config.HasThinking {
		extra := req.ExtraFields()
		if extra == nil {
			extra = map[string]interface{}{
				"thinking": map[string]interface{}{"type": "enabled"},
			}
		} else {
			extra["thinking"] = map[string]interface{}{"type": "enabled"}
		}
		req.SetExtraFields(extra)
	}
	return req
}
