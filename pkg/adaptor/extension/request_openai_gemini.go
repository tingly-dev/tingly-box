package extension

import (
	"strings"

	"tingly-box/internal/typ"

	"github.com/openai/openai-go/v3"
)

// Constants and configurations for Gemini API compatibility
// ref: https://ai.google.dev/api/caching?hl=zh-cn#FunctionDeclaration

// geminiSupportedSchemaFields are JSON Schema fields supported by Gemini
var geminiSupportedSchemaFields = map[string]bool{
	"type":             true,
	"format":           true,
	"title":            true,
	"description":      true,
	"nullable":         true,
	"enum":             true,
	"maxItems":         true,
	"minItems":         true,
	"properties":       true,
	"required":         true,
	"minProperties":    true,
	"maxProperties":    true,
	"minLength":        true,
	"maxLength":        true,
	"pattern":          true,
	"example":          true,
	"anyOf":            true,
	"propertyOrdering": true,
	"default":          true,
	"items":            true,
	"minimum":          true,
	"maximum":          true,
}

// geminiSchemaFieldTransforms defines schema field transformations for Gemini
// key: source field name
// value: target field name
var geminiSchemaFieldTransforms = map[string]string{
	"exclusiveMinimum": "minimum", // convert exclusiveMinimum to minimum
	"exclusiveMaximum": "maximum", // convert exclusiveMaximum to maximum
}

// ============================================================================
// Main Transform Entry Points
// ============================================================================

// applyGeminiTransform handles official Google Gemini API transformations.
// This includes:
//   - Thinking configuration mapping to extra_body.google.thinking_config
//   - Tool schema filtering to supported fields only
func applyGeminiTransform(req *openai.ChatCompletionNewParams, provider *typ.Provider, model string, config *OpenAIConfig) *openai.ChatCompletionNewParams {
	req = applyGeminiThinkingConfig(req, model, config)
	req = applyGeminiToolSchemaFilter(req)
	return req
}

// applyGeminiOpenRouterTransform handles Gemini via OpenRouter.
// This applies OpenRouter-specific subset conversion.
func applyGeminiOpenRouterTransform(req *openai.ChatCompletionNewParams, provider *typ.Provider, model string, config *OpenAIConfig) *openai.ChatCompletionNewParams {
	return applyGeminiSubsetTransform(req, model)
}

// applyGeminiPoeTransform handles Gemini via Poe.
// This applies Poe-specific subset conversion.
func applyGeminiPoeTransform(req *openai.ChatCompletionNewParams, _ *typ.Provider, _ string, _ *OpenAIConfig) *openai.ChatCompletionNewParams {
	res := applyGeminiToolSchemaFilter(req)
	return res
}

// ============================================================================
// Thinking Configuration
// ============================================================================

// applyGeminiSubsetTransform is the shared Gemini transformation logic for proxy providers.
// ref: https://ai.google.dev/gemini-api/docs/openai?hl=zh-cn
func applyGeminiSubsetTransform(req *openai.ChatCompletionNewParams, model string) *openai.ChatCompletionNewParams {
	req = applyGeminiThinkingConfig(req, model, nil)
	req = applyGeminiToolSchemaFilter(req)
	return req
}

// applyGeminiThinkingConfig converts Anthropic thinking to Gemini's thinking_config.
// ref: https://ai.google.dev/gemini-api/docs/openai?hl=zh-cn#thinking
//
// Mapping for thinking_level (Gemini 3):
//   - minimal -> minimal
//   - low -> low
//   - medium -> medium (not supported on Gemini 3 Pro)
//   - high -> high
//
// Mapping for thinking_budget (Gemini 2.5):
//   - minimal -> 1024
//   - low -> 1024
//   - medium -> 8192
//   - high -> 24576
func applyGeminiThinkingConfig(req *openai.ChatCompletionNewParams, model string, config *OpenAIConfig) *openai.ChatCompletionNewParams {
	extraFields := req.ExtraFields()

	thinkingConfig, hasThinking := extraFields["thinking"].(map[string]interface{})
	if !hasThinking || thinkingConfig == nil {
		return req
	}

	modelLower := strings.ToLower(model)
	googleConfig := buildGeminiThinkingConfig(modelLower)

	// Add include_thoughts if specified
	if includeThoughts, ok := thinkingConfig["include_thoughts"].(bool); ok && includeThoughts {
		if tc, ok := googleConfig["thinking_config"].(map[string]interface{}); ok {
			tc["include_thoughts"] = true
		}
	}

	// Set the extra_body with Google config and remove the original thinking field
	extraFields["extra_body"] = map[string]interface{}{"google": googleConfig}
	delete(extraFields, "thinking")

	req.SetExtraFields(extraFields)
	return req
}

// buildGeminiThinkingConfig builds the thinking_config based on model version.
func buildGeminiThinkingConfig(modelLower string) map[string]interface{} {
	// Check if it's Gemini 2.5 (use thinking_budget)
	if strings.Contains(modelLower, "2.5") || strings.Contains(modelLower, "gemini-2") {
		return map[string]interface{}{
			"thinking_config": map[string]interface{}{
				"thinking_budget": getThinkingBudget(modelLower),
			},
		}
	}

	// Gemini 3 uses thinking_level
	return map[string]interface{}{
		"thinking_config": map[string]interface{}{
			"thinking_level": getThinkingLevel(modelLower),
		},
	}
}

// getThinkingLevel returns the appropriate thinking_level for Gemini 3 models.
// Default is "low" - can be extended to parse from request parameters if needed.
func getThinkingLevel(model string) string {
	return "low"
}

// getThinkingBudget returns the appropriate thinking_budget for Gemini 2.5 models.
// Default is 1024 (low) - can be extended to parse from request parameters if needed.
func getThinkingBudget(model string) int {
	return 1024
}

// ============================================================================
// Tool Schema Filtering
// ============================================================================

// applyGeminiToolSchemaFilter filters tool schemas to only include supported fields.
// This removes unsupported JSON Schema fields like exclusiveMinimum/exclusiveMaximum.
func applyGeminiToolSchemaFilter(req *openai.ChatCompletionNewParams) *openai.ChatCompletionNewParams {
	if len(req.Tools) == 0 {
		return req
	}

	for i, toolUnion := range req.Tools {
		if toolUnion.OfFunction != nil {
			fn := toolUnion.OfFunction.Function
			if len(fn.Parameters) > 0 {
				req.Tools[i].OfFunction.Function.Parameters = filterGeminiSchema(fn.Parameters)
			}
		}
	}

	return req
}

// filterGeminiSchema recursively filters and transforms a JSON Schema for Gemini compatibility.
// This handles:
//  1. Field transformation (e.g., exclusiveMinimum -> minimum)
//  2. Field filtering (removing unsupported fields)
//  3. Recursive filtering of nested schemas (properties, items, anyOf)
func filterGeminiSchema(schema map[string]interface{}) map[string]interface{} {
	result := make(map[string]interface{})

	for key, value := range schema {
		// Check if this field needs to be transformed
		if targetKey, needsTransform := geminiSchemaFieldTransforms[key]; needsTransform {
			result[targetKey] = value
			continue
		}

		// // Only include supported fields
		// if !geminiSupportedSchemaFields[key] {
		// 	continue
		// }

		// Handle special recursive fields
		switch key {
		case "properties":
			if props, ok := value.(map[string]interface{}); ok {
				result[key] = filterGeminiProperties(props)
			} else {
				result[key] = value
			}
		case "items":
			if itemSchema, ok := value.(map[string]interface{}); ok {
				result[key] = filterGeminiSchema(itemSchema)
			} else {
				result[key] = value
			}
		case "anyOf":
			if anyOfSchemas, ok := value.([]interface{}); ok {
				filtered := make([]interface{}, 0, len(anyOfSchemas))
				for _, schemaRef := range anyOfSchemas {
					if schemaMap, ok := schemaRef.(map[string]interface{}); ok {
						filtered = append(filtered, filterGeminiSchema(schemaMap))
					} else {
						filtered = append(filtered, schemaRef)
					}
				}
				result[key] = filtered
			} else {
				result[key] = value
			}
		default:
			result[key] = value
		}
	}

	return result
}

// filterGeminiProperties filters all property schemas in a properties object.
func filterGeminiProperties(props map[string]interface{}) map[string]interface{} {
	result := make(map[string]interface{})

	for key, value := range props {
		if propSchema, ok := value.(map[string]interface{}); ok {
			result[key] = filterGeminiSchema(propSchema)
		} else {
			result[key] = value
		}
	}

	return result
}
