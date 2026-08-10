package ops

import (
	"strings"

	"github.com/openai/openai-go/v3"
	"github.com/tingly-dev/tingly-box/internal/protocol"
	"github.com/tingly-dev/tingly-box/internal/protocol/thinking"
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
func applyGeminiTransform(req *openai.ChatCompletionNewParams, providerURL, model string, config *protocol.OpenAIConfig) *openai.ChatCompletionNewParams {
	req = applyGeminiThinkingConfig(req, model, config)
	req = applyGeminiToolSchemaFilter(req)
	return req
}

// applyGeminiOpenRouterTransform handles Gemini via OpenRouter.
// This applies OpenRouter-specific subset conversion.
func applyGeminiOpenRouterTransform(req *openai.ChatCompletionNewParams, providerURL, model string, config *protocol.OpenAIConfig) *openai.ChatCompletionNewParams {
	return applyGeminiSubsetTransform(req, model)
}

// applyGeminiPoeTransform handles Gemini via Poe.
// This applies Poe-specific subset conversion.
func applyGeminiPoeTransform(req *openai.ChatCompletionNewParams, providerURL, model string, config *protocol.OpenAIConfig) *openai.ChatCompletionNewParams {
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

// applyGeminiThinkingConfig converts the request's thinking signal to Gemini's
// thinking_config, driven by the canonical effort ladder (internal/protocol/thinking).
// ref: https://ai.google.dev/gemini-api/docs/openai?hl=zh-cn#thinking
//
// The effort level is resolved in priority order:
//  1. req.ReasoningEffort — set by a forced thinking_effort rule flag or by an
//     OpenAI-native client directly.
//  2. config.ReasoningEffort — derived during Anthropic→OpenAI conversion
//     (explicit output_config.effort, or budget_tokens tiered onto the ladder).
//  3. The Anthropic-style `thinking` extra blob — explicit budget_tokens
//     tiered via thinking.EffortFromBudget, a level carried in `type`, or
//     "low" for a bare enabled blob.
//
// The resolved level then becomes thinking_level for Gemini 3 or
// thinking_budget (via thinking.BudgetMapping) for Gemini 2.5.
func applyGeminiThinkingConfig(req *openai.ChatCompletionNewParams, model string, config *protocol.OpenAIConfig) *openai.ChatCompletionNewParams {
	extraFields := req.ExtraFields()
	thinkingBlob, hasBlob := extraFields["thinking"].(map[string]interface{})

	effort := resolveGeminiEffort(req, config, thinkingBlob, hasBlob)
	if effort == "" {
		// No actionable thinking signal. Still consume a stray blob (e.g.
		// type=disabled) so the non-standard field never reaches Google.
		if hasBlob {
			delete(extraFields, "thinking")
			req.SetExtraFields(extraFields)
		}
		return req
	}

	modelLower := strings.ToLower(model)
	googleConfig := buildGeminiThinkingConfig(modelLower, effort)

	// Add include_thoughts if specified
	if hasBlob {
		if includeThoughts, ok := thinkingBlob["include_thoughts"].(bool); ok && includeThoughts {
			if tc, ok := googleConfig["thinking_config"].(map[string]interface{}); ok {
				tc["include_thoughts"] = true
			}
		}
	}

	// Set the extra_body with Google config and remove the original thinking field
	if extraFields == nil {
		extraFields = map[string]interface{}{}
	}
	extraFields["extra_body"] = map[string]interface{}{"google": googleConfig}
	delete(extraFields, "thinking")

	req.SetExtraFields(extraFields)
	// The effort now lives in thinking_config; don't also send reasoning_effort.
	req.ReasoningEffort = ""
	return req
}

// resolveGeminiEffort resolves the effort level driving the Gemini thinking
// config. Returns "" when the request carries no actionable thinking signal
// ("none" passes through untouched — Google's OpenAI-compat layer handles it
// natively, and disabling is model-dependent on Gemini).
func resolveGeminiEffort(req *openai.ChatCompletionNewParams, config *protocol.OpenAIConfig, blob map[string]interface{}, hasBlob bool) string {
	if effort := string(req.ReasoningEffort); effort != "" {
		if effort == "none" {
			return ""
		}
		return effort
	}
	if config != nil && config.HasThinking && config.ReasoningEffort != "" && config.ReasoningEffort != "none" {
		return string(config.ReasoningEffort)
	}
	if !hasBlob || blob == nil {
		return ""
	}
	if t, _ := blob["type"].(string); t == "disabled" {
		return ""
	}
	if budget, ok := blob["budget_tokens"].(float64); ok && budget > 0 {
		return thinking.EffortFromBudget(int64(budget))
	}
	if t, ok := blob["type"].(string); ok {
		if _, known := thinking.BudgetMapping[t]; known {
			return t
		}
	}
	// Bare enabled blob with no level or budget: minimal-cost default.
	return thinking.LevelLow
}

// buildGeminiThinkingConfig builds the thinking_config based on model version.
func buildGeminiThinkingConfig(modelLower, effort string) map[string]interface{} {
	// Check if it's Gemini 2.5 (use thinking_budget)
	if strings.Contains(modelLower, "2.5") || strings.Contains(modelLower, "gemini-2") {
		return map[string]interface{}{
			"thinking_config": map[string]interface{}{
				"thinking_budget": getThinkingBudget(modelLower, effort),
			},
		}
	}

	// Gemini 3 uses thinking_level
	return map[string]interface{}{
		"thinking_config": map[string]interface{}{
			"thinking_level": getThinkingLevel(effort),
		},
	}
}

// getThinkingLevel maps a ladder effort level to a Gemini 3 thinking_level
// using one provider-wide mapping: minimal/low/medium/high remain distinct,
// while xhigh and max collapse to high.
func getThinkingLevel(effort string) string {
	switch effort {
	case thinking.LevelMinimal:
		return "minimal"
	case thinking.LevelLow:
		return "low"
	case thinking.LevelMedium:
		return "medium"
	default: // high / xhigh / max / unknown
		return "high"
	}
}

// getThinkingBudget maps a ladder effort level to a Gemini 2.5 thinking_budget
// via the canonical thinking.BudgetMapping. Flash-family models cap the budget
// at 24576 (Pro allows up to 32768).
func getThinkingBudget(model, effort string) int {
	budget, ok := thinking.BudgetMapping[effort]
	if !ok {
		budget = thinking.BudgetMapping[thinking.LevelLow]
	}
	if strings.Contains(model, "flash") && budget > 24576 {
		budget = 24576
	}
	return int(budget)
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
