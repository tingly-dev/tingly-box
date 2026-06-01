package ops

import (
	"encoding/json"
	"strings"

	"github.com/openai/openai-go/v3"
	"github.com/tingly-dev/tingly-box/internal/protocol"
)

// ProviderTransform applies provider-specific transformations to OpenAI requests
type ProviderTransform func(*openai.ChatCompletionNewParams, string, string, *protocol.OpenAIConfig) *openai.ChatCompletionNewParams

// providerConfig maps APIBase patterns to their transforms
type providerConfig struct {
	APIBasePattern string
	ModelPattern   string // Optional: if specified, model name must also match this pattern
	Transform      ProviderTransform
}

// ProviderConfigs holds all registered provider configurations
// Add new providers here with their APIBase domain patterns
var ProviderConfigs = []providerConfig{
	// DeepSeek - official API
	{
		APIBasePattern: "api.deepseek.com",
		ModelPattern:   "*", // No specific model pattern needed for DeepSeek official API
		Transform:      applyDeepSeekTransform,
	},

	// OpenCode Go - aggregator that may route to DeepSeek models
	// Only apply DeepSeek transform when the model name contains "deepseek"
	{
		APIBasePattern: "opencode.ai/zen/go",
		ModelPattern:   "deepseek",
		Transform:      applyDeepSeekTransform,
	},

	{
		APIBasePattern: "https://api.kimi.com/coding/v1",
		ModelPattern:   "*",
		Transform:      applyDeepSeekTransform,
	},

	// Moonshot - official API (CN)
	// Moonshot requires reasoning_content in assistant messages with tool_calls when thinking is enabled
	// Similar to DeepSeek, we reuse applyDeepSeekTransform to handle x_thinking -> reasoning_content conversion
	{
		APIBasePattern: "api.moonshot.cn",
		ModelPattern:   "*",
		Transform:      applyDeepSeekTransform,
	},

	// Moonshot - official API (International)
	{
		APIBasePattern: "api.moonshot.ai",
		ModelPattern:   "*",
		Transform:      applyDeepSeekTransform,
	},

	// Gemini - official Google API
	{
		APIBasePattern: "generativelanguage.googleapis.com",
		ModelPattern:   "gemini", // No specific model pattern needed for official Gemini API
		Transform:      applyGeminiTransform,
	},

	// Gemini - Poe (only for Gemini models)
	{
		APIBasePattern: "poe.com",
		ModelPattern:   "gemini", // Apply transform only if model name contains "gemini"
		Transform:      applyGeminiPoeTransform,
	},

	// Gemini - OpenRouter
	// {"openrouter.ai", applyGeminiOpenRouterTransform},
}

// GetProviderTransform identifies provider by APIBase URL string and returns its transform
// Returns nil if no specific transform is needed (fallback to default)
func GetProviderTransform(providerURL, model string) ProviderTransform {
	apiBase := strings.ToLower(providerURL)
	modelLower := strings.ToLower(model)

	// Match by APIBase domain and optional ModelPattern
	if apiBase != "" {
		for _, config := range ProviderConfigs {
			if strings.Contains(apiBase, config.APIBasePattern) {
				if config.ModelPattern == "*" || strings.Contains(modelLower, config.ModelPattern) {
					return config.Transform
				}
			}
		}
	}

	// No specific transform needed - use default
	return nil
}

// normalizeCursorContent flattens rich content in messages for Cursor compatibility.
// This should be applied for ALL providers when cursor_compat is enabled.
func normalizeCursorContent(req *openai.ChatCompletionNewParams) {
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

// ApplyCursorCompatContentNormalization is the exported version of normalizeCursorContent.
// It flattens rich content in messages for Cursor compatibility.
// This should be called for ALL providers when cursor_compat is enabled.
func ApplyCursorCompatContentNormalization(req *openai.ChatCompletionNewParams) {
	normalizeCursorContent(req)
}

// messageToMap converts a ChatCompletionMessageParamUnion to a map for modification
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

// applyDefaultTransform applies default transformations for OpenAI-compatible requests
// This handles standard fields like reasoning_effort that are widely supported
func applyDefaultTransform(req *openai.ChatCompletionNewParams, config *protocol.OpenAIConfig) *openai.ChatCompletionNewParams {
	if config.HasThinking && config.ReasoningEffort != "" {
		// Set reasoning_effort from config for OpenAI-compatible APIs
		// This is widely supported by many providers (OpenAI, Azure, etc.)
		req.ReasoningEffort = config.ReasoningEffort
	} else if config.HasThinking {
		extra := req.ExtraFields()
		if extra == nil {
			extra = map[string]interface{}{
				"thinking": map[string]interface{}{
					"type": "enabled",
				},
			}
		} else {
			extra["thinking"] = map[string]interface{}{
				"type": "enabled",
			}
		}
		req.SetExtraFields(extra)
	}
	return req
}

// ApplyProviderTransforms applies provider-specific transformations
// Falls back to default handling if no specific transform found
func ApplyProviderTransforms(req *openai.ChatCompletionNewParams, providerURL, model string, config *protocol.OpenAIConfig) *openai.ChatCompletionNewParams {
	if transform := GetProviderTransform(providerURL, model); transform != nil {
		return transform(req, providerURL, model, config)
	}
	// Default: apply standard OpenAI-compatible transformations
	return applyDefaultTransform(req, config)
}
