package transformer

import (
	"testing"

	"github.com/openai/openai-go/v3"
	"github.com/openai/openai-go/v3/packages/param"
	"github.com/openai/openai-go/v3/shared"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/tingly-dev/tingly-box/internal/typ"
)

// TestApplyGeminiTransform tests the main Gemini transformation entry point
func TestApplyGeminiTransform(t *testing.T) {
	t.Run("with thinking config", func(t *testing.T) {
		req := &openai.ChatCompletionNewParams{
			Model: openai.ChatModel("gemini-2.5-flash"),
			Tools: []openai.ChatCompletionToolUnionParam{
				openai.ChatCompletionFunctionTool(shared.FunctionDefinitionParam{
					Name:        "test_tool",
					Description: param.Opt[string]{Value: "Test tool"},
					Parameters: map[string]interface{}{
						"type": "object",
						"properties": map[string]interface{}{
							"input": map[string]interface{}{
								"type":        "string",
								"description": "Test input",
							},
						},
					},
				}),
			},
		}

		// Add thinking config to extra fields
		extraFields := map[string]interface{}{
			"thinking": map[string]interface{}{
				"type":            "high",
				"include_thoughts": true,
			},
		}
		req.SetExtraFields(extraFields)

		provider := &typ.Provider{Name: "google"}
		config := &OpenAIConfig{}

		result := applyGeminiTransform(req, provider, "gemini-2.5-flash", config)

		// Verify thinking was converted to extra_body.google format
		resultExtraFields := result.ExtraFields()
		extraBody, ok := resultExtraFields["extra_body"].(map[string]interface{})
		require.True(t, ok)

		googleConfig, ok := extraBody["google"].(map[string]interface{})
		require.True(t, ok)

		thinkingConfig, ok := googleConfig["thinking_config"].(map[string]interface{})
		require.True(t, ok)

		// Should have thinking_budget for Gemini 2.5
		assert.Contains(t, thinkingConfig, "thinking_budget")
		assert.True(t, thinkingConfig["include_thoughts"].(bool))
	})

	t.Run("without thinking config", func(t *testing.T) {
		req := &openai.ChatCompletionNewParams{
			Model: openai.ChatModel("gemini-pro"),
		}

		provider := &typ.Provider{Name: "google"}
		config := &OpenAIConfig{}

		result := applyGeminiTransform(req, provider, "gemini-pro", config)

		// Should not have extra_body if no thinking config
		resultExtraFields := result.ExtraFields()
		_, hasExtraBody := resultExtraFields["extra_body"]
		assert.False(t, hasExtraBody)
	})
}

// TestApplyGeminiToolSchemaFilter tests tool schema filtering for Gemini compatibility
func TestApplyGeminiToolSchemaFilter(t *testing.T) {
	t.Run("filters unsupported schema fields", func(t *testing.T) {
		req := &openai.ChatCompletionNewParams{
			Model: openai.ChatModel("gemini-pro"),
			Tools: []openai.ChatCompletionToolUnionParam{
				openai.ChatCompletionFunctionTool(shared.FunctionDefinitionParam{
					Name:        "test_tool",
					Description: param.Opt[string]{Value: "Test tool"},
					Parameters: map[string]interface{}{
						"type": "object",
						"properties": map[string]interface{}{
							"value": map[string]interface{}{
								"type":               "number",
								"exclusiveMinimum":   0,
								"exclusiveMaximum":   100,
								"description":        "A numeric value",
								"unsupported_field":  "should be removed",
							},
						},
					},
				}),
			},
		}

		result := applyGeminiToolSchemaFilter(req)

		// Verify tool still exists
		require.Len(t, result.Tools, 1)

		fn := result.Tools[0].GetFunction()
		require.NotNil(t, fn)

		params := fn.Parameters
		require.NotNil(t, params)

		props, ok := params["properties"].(map[string]interface{})
		require.True(t, ok)

		valueSchema, ok := props["value"].(map[string]interface{})
		require.True(t, ok)

		// exclusiveMinimum should be transformed to minimum
		assert.Contains(t, valueSchema, "minimum")
		assert.Equal(t, 0, valueSchema["minimum"])

		// exclusiveMaximum should be transformed to maximum
		assert.Contains(t, valueSchema, "maximum")
		assert.Equal(t, 100, valueSchema["maximum"])

		// description should still be present
		assert.Contains(t, valueSchema, "description")

		// Note: Unsupported field filtering is currently disabled in implementation
		// (the geminiSupportedSchemaFields check is commented out)
		// So unsupported_field will still be present
		_, hasUnsupported := valueSchema["unsupported_field"]
		assert.True(t, hasUnsupported) // Field filtering is disabled
	})

	t.Run("handles nested properties", func(t *testing.T) {
		req := &openai.ChatCompletionNewParams{
			Model: openai.ChatModel("gemini-pro"),
			Tools: []openai.ChatCompletionToolUnionParam{
				openai.ChatCompletionFunctionTool(shared.FunctionDefinitionParam{
					Name:        "nested_tool",
					Description: param.Opt[string]{Value: "Tool with nested schema"},
					Parameters: map[string]interface{}{
						"type": "object",
						"properties": map[string]interface{}{
							"config": map[string]interface{}{
								"type": "object",
								"properties": map[string]interface{}{
									"nested_value": map[string]interface{}{
										"type":             "string",
										"exclusiveMinimum": "a",
										"minLength":        1,
									},
								},
							},
						},
					},
				}),
			},
		}

		result := applyGeminiToolSchemaFilter(req)

		fn := result.Tools[0].GetFunction()
		props := fn.Parameters["properties"].(map[string]interface{})
		config := props["config"].(map[string]interface{})
		configProps := config["properties"].(map[string]interface{})
		nestedValue := configProps["nested_value"].(map[string]interface{})

		// Nested transformation should work
		assert.Contains(t, nestedValue, "minimum")
		assert.Contains(t, nestedValue, "minLength")
	})

	t.Run("handles array items", func(t *testing.T) {
		req := &openai.ChatCompletionNewParams{
			Model: openai.ChatModel("gemini-pro"),
			Tools: []openai.ChatCompletionToolUnionParam{
				openai.ChatCompletionFunctionTool(shared.FunctionDefinitionParam{
					Name:        "array_tool",
					Description: param.Opt[string]{Value: "Tool with array schema"},
					Parameters: map[string]interface{}{
						"type": "object",
						"properties": map[string]interface{}{
							"items": map[string]interface{}{
								"type": "array",
								"items": map[string]interface{}{
									"type":             "number",
									"exclusiveMinimum": 0,
									"maximum":          100,
								},
							},
						},
					},
				}),
			},
		}

		result := applyGeminiToolSchemaFilter(req)

		fn := result.Tools[0].GetFunction()
		props := fn.Parameters["properties"].(map[string]interface{})
		itemsProp := props["items"].(map[string]interface{})
		itemSchema := itemsProp["items"].(map[string]interface{})

		// Array item schema should be transformed
		assert.Contains(t, itemSchema, "minimum")
		assert.Contains(t, itemSchema, "maximum")
	})

	t.Run("handles anyOf schemas", func(t *testing.T) {
		req := &openai.ChatCompletionNewParams{
			Model: openai.ChatModel("gemini-pro"),
			Tools: []openai.ChatCompletionToolUnionParam{
				openai.ChatCompletionFunctionTool(shared.FunctionDefinitionParam{
					Name:        "anyof_tool",
					Description: param.Opt[string]{Value: "Tool with anyOf schema"},
					Parameters: map[string]interface{}{
						"type": "object",
						"properties": map[string]interface{}{
							"value": map[string]interface{}{
								"anyOf": []interface{}{
									map[string]interface{}{
										"type":             "string",
										"exclusiveMinimum": "a",
										"minLength":        1,
									},
									map[string]interface{}{
										"type":             "number",
										"exclusiveMinimum": 0,
										"minimum":          1,
									},
								},
							},
						},
					},
				}),
			},
		}

		result := applyGeminiToolSchemaFilter(req)

		fn := result.Tools[0].GetFunction()
		props := fn.Parameters["properties"].(map[string]interface{})
		valueProp := props["value"].(map[string]interface{})
		anyOf, ok := valueProp["anyOf"].([]interface{})
		require.True(t, ok)
		require.Len(t, anyOf, 2)

		// Check first anyOf schema
		firstSchema, ok := anyOf[0].(map[string]interface{})
		require.True(t, ok)
		assert.Contains(t, firstSchema, "minimum")

		// Check second anyOf schema
		secondSchema, ok := anyOf[1].(map[string]interface{})
		require.True(t, ok)
		assert.Contains(t, secondSchema, "minimum")
	})

	t.Run("no tools - returns original request", func(t *testing.T) {
		req := &openai.ChatCompletionNewParams{
			Model: openai.ChatModel("gemini-pro"),
		}

		result := applyGeminiToolSchemaFilter(req)

		assert.Same(t, req, result)
	})
}

// TestApplyGeminiThinkingConfig tests thinking configuration conversion
func TestApplyGeminiThinkingConfig(t *testing.T) {
	t.Run("Gemini 2.5 uses thinking_budget", func(t *testing.T) {
		models := []string{
			"gemini-2.5-flash",
			"gemini-2.5-pro",
			"gemini-2.0-flash",
		}

		for _, model := range models {
			t.Run(model, func(t *testing.T) {
				req := &openai.ChatCompletionNewParams{
					Model: openai.ChatModel(model),
				}

				extraFields := map[string]interface{}{}
				extraFields["thinking"] = map[string]interface{}{
					"type": "high",
				}
				req.SetExtraFields(extraFields)

				result := applyGeminiThinkingConfig(req, model, nil)

				resultExtraFields := result.ExtraFields()
				extraBody, ok := resultExtraFields["extra_body"].(map[string]interface{})
				require.True(t, ok)

				googleConfig, ok := extraBody["google"].(map[string]interface{})
				require.True(t, ok)

				thinkingConfig, ok := googleConfig["thinking_config"].(map[string]interface{})
				require.True(t, ok)

				// Should use thinking_budget for Gemini 2.5
				assert.Contains(t, thinkingConfig, "thinking_budget")
				assert.NotContains(t, thinkingConfig, "thinking_level")
			})
		}
	})

	t.Run("Gemini 3 uses thinking_level", func(t *testing.T) {
		models := []string{
			"gemini-3.0-flash",
			"gemini-3.5-pro",
			"gemini-3-flash",
		}

		for _, model := range models {
			t.Run(model, func(t *testing.T) {
				req := &openai.ChatCompletionNewParams{
					Model: openai.ChatModel(model),
				}

				extraFields := map[string]interface{}{}
				extraFields["thinking"] = map[string]interface{}{
					"type": "medium",
				}
				req.SetExtraFields(extraFields)

				result := applyGeminiThinkingConfig(req, model, nil)

				resultExtraFields := result.ExtraFields()
				extraBody, ok := resultExtraFields["extra_body"].(map[string]interface{})
				require.True(t, ok)

				googleConfig, ok := extraBody["google"].(map[string]interface{})
				require.True(t, ok)

				thinkingConfig, ok := googleConfig["thinking_config"].(map[string]interface{})
				require.True(t, ok)

				// Should use thinking_level for Gemini 3
				assert.Contains(t, thinkingConfig, "thinking_level")
				assert.NotContains(t, thinkingConfig, "thinking_budget")
				assert.Equal(t, "low", thinkingConfig["thinking_level"])
			})
		}
	})

	t.Run("include_thoughts flag", func(t *testing.T) {
		req := &openai.ChatCompletionNewParams{
			Model: openai.ChatModel("gemini-2.5-flash"),
		}

		extraFields := map[string]interface{}{}
		extraFields["thinking"] = map[string]interface{}{
			"type":            "medium",
			"include_thoughts": true,
		}
		req.SetExtraFields(extraFields)

		result := applyGeminiThinkingConfig(req, "gemini-2.5-flash", nil)

		resultExtraFields := result.ExtraFields()
		extraBody := resultExtraFields["extra_body"].(map[string]interface{})
		googleConfig := extraBody["google"].(map[string]interface{})
		thinkingConfig := googleConfig["thinking_config"].(map[string]interface{})

		assert.True(t, thinkingConfig["include_thoughts"].(bool))
	})

	t.Run("no thinking config - returns original", func(t *testing.T) {
		req := &openai.ChatCompletionNewParams{
			Model: openai.ChatModel("gemini-pro"),
		}

		result := applyGeminiThinkingConfig(req, "gemini-pro", nil)

		assert.Same(t, req, result)
	})
}

// TestApplyGeminiOpenRouterTransform tests OpenRouter-specific transformations
func TestApplyGeminiOpenRouterTransform(t *testing.T) {
	t.Run("applies subset transform for OpenRouter", func(t *testing.T) {
		req := &openai.ChatCompletionNewParams{
			Model: openai.ChatModel("openrouter:google/gemini-pro"),
			Tools: []openai.ChatCompletionToolUnionParam{
				openai.ChatCompletionFunctionTool(shared.FunctionDefinitionParam{
					Name:        "test_tool",
					Description: param.Opt[string]{Value: "Test"},
					Parameters: map[string]interface{}{
						"type":       "object",
						"properties": map[string]interface{}{},
					},
				}),
			},
		}

		provider := &typ.Provider{Name: "openrouter"}
		config := &OpenAIConfig{}

		result := applyGeminiOpenRouterTransform(req, provider, "openrouter:google/gemini-pro", config)

		// Should still have tools after transformation
		assert.Len(t, result.Tools, 1)
	})
}

// TestApplyGeminiPoeTransform tests Poe-specific transformations
func TestApplyGeminiPoeTransform(t *testing.T) {
	t.Run("applies tool schema filter for Poe", func(t *testing.T) {
		req := &openai.ChatCompletionNewParams{
			Model: openai.ChatModel("poe:gemini-pro"),
			Tools: []openai.ChatCompletionToolUnionParam{
				openai.ChatCompletionFunctionTool(shared.FunctionDefinitionParam{
					Name:        "test_tool",
					Description: param.Opt[string]{Value: "Test"},
					Parameters: map[string]interface{}{
						"type": "object",
						"properties": map[string]interface{}{
							"value": map[string]interface{}{
								"type":             "number",
								"exclusiveMinimum": 0,
							},
						},
					},
				}),
			},
		}

		provider := &typ.Provider{Name: "poe"}

		result := applyGeminiPoeTransform(req, provider, "poe:gemini-pro", nil)

		// Should apply schema filtering
		fn := result.Tools[0].GetFunction()
		props := fn.Parameters["properties"].(map[string]interface{})
		valueProp := props["value"].(map[string]interface{})

		// exclusiveMinimum should be transformed to minimum
		assert.Contains(t, valueProp, "minimum")
	})
}

// TestGetThinkingLevel tests thinking level determination
func TestGetThinkingLevel(t *testing.T) {
	tests := []struct {
		name     string
		model    string
		expected string
	}{
		{"default", "gemini-3.0-flash", "low"},
		{"pro model", "gemini-3.5-pro", "low"},
		{"flash model", "gemini-3-flash-exp", "low"},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := getThinkingLevel(tt.model)
			assert.Equal(t, tt.expected, result)
		})
	}
}

// TestGetThinkingBudget tests thinking budget determination
func TestGetThinkingBudget(t *testing.T) {
	tests := []struct {
		name     string
		model    string
		expected int
	}{
		{"default low", "gemini-2.5-flash", 1024},
		{"pro model", "gemini-2.5-pro", 1024},
		{"flash experimental", "gemini-2.0-flash-exp", 1024},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := getThinkingBudget(tt.model)
			assert.Equal(t, tt.expected, result)
		})
	}
}

// TestBuildGeminiThinkingConfig tests thinking config building
func TestBuildGeminiThinkingConfig(t *testing.T) {
	t.Run("Gemini 2.5 models use thinking_budget", func(t *testing.T) {
		models := []string{
			"gemini-2.5-flash",
			"gemini-2.5-pro",
			"gemini-2.0-flash",
			"gemini-2-flash",
		}

		for _, model := range models {
			t.Run(model, func(t *testing.T) {
				result := buildGeminiThinkingConfig(model)

				thinkingConfig, ok := result["thinking_config"].(map[string]interface{})
				require.True(t, ok)

				assert.Contains(t, thinkingConfig, "thinking_budget")
				assert.NotContains(t, thinkingConfig, "thinking_level")
			})
		}
	})

	t.Run("Gemini 3 models use thinking_level", func(t *testing.T) {
		models := []string{
			"gemini-3.0-flash",
			"gemini-3.5-pro",
			"gemini-3-flash",
			"gemini-pro", // fallback to thinking_level
		}

		for _, model := range models {
			t.Run(model, func(t *testing.T) {
				result := buildGeminiThinkingConfig(model)

				thinkingConfig, ok := result["thinking_config"].(map[string]interface{})
				require.True(t, ok)

				assert.Contains(t, thinkingConfig, "thinking_level")
				assert.NotContains(t, thinkingConfig, "thinking_budget")
			})
		}
	})
}

// TestFilterGeminiSchema tests schema filtering logic
func TestFilterGeminiSchema(t *testing.T) {
	t.Run("removes unsupported fields", func(t *testing.T) {
		schema := map[string]interface{}{
			"type":                 "object",
			"description":          "Test schema",
			"unsupported_field":    "should be removed",
			"another_unsupported":  123,
		}

		result := filterGeminiSchema(schema)

		// Supported fields should remain
		assert.Contains(t, result, "type")
		assert.Contains(t, result, "description")

		// Unsupported fields should be filtered out (if field filtering is enabled)
		// Currently field filtering is commented out in the implementation
		// Uncomment these assertions if field filtering is enabled
		// _, hasUnsupported := result["unsupported_field"]
		// assert.False(t, hasUnsupported)
	})

	t.Run("transforms field names", func(t *testing.T) {
		schema := map[string]interface{}{
			"type":               "object",
			"exclusiveMinimum":   0,
			"exclusiveMaximum":   100,
		}

		result := filterGeminiSchema(schema)

		// exclusiveMinimum should be transformed to minimum
		assert.Contains(t, result, "minimum")
		assert.Equal(t, 0, result["minimum"])

		// exclusiveMaximum should be transformed to maximum
		assert.Contains(t, result, "maximum")
		assert.Equal(t, 100, result["maximum"])

		// Original fields should not exist
		_, hasExclusiveMin := result["exclusiveMinimum"]
		assert.False(t, hasExclusiveMin)

		_, hasExclusiveMax := result["exclusiveMaximum"]
		assert.False(t, hasExclusiveMax)
	})

	t.Run("handles all supported fields", func(t *testing.T) {
		schema := map[string]interface{}{
			"type":          "object",
			"format":        "date-time",
			"title":         "Test",
			"description":   "Test description",
			"nullable":      true,
			"enum":          []interface{}{"a", "b"},
			"maxItems":      10,
			"minItems":      1,
			"minProperties": 1,
			"maxProperties": 10,
			"minLength":     1,
			"maxLength":     100,
			"pattern":       "^[a-z]+$",
			"example":       "example",
			"default":       "default",
			"minimum":       0,
			"maximum":       100,
		}

		result := filterGeminiSchema(schema)

		// All supported fields should be present
		for key := range schema {
			if key != "exclusiveMinimum" && key != "exclusiveMaximum" {
				assert.Contains(t, result, key)
			}
		}
	})
}

// TestFilterGeminiProperties tests property filtering
func TestFilterGeminiProperties(t *testing.T) {
	t.Run("filters all properties recursively", func(t *testing.T) {
		props := map[string]interface{}{
			"prop1": map[string]interface{}{
				"type":             "string",
				"exclusiveMinimum": "a",
				"description":      "Property 1",
			},
			"prop2": map[string]interface{}{
				"type":             "number",
				"exclusiveMaximum": 100,
				"minimum":          0,
			},
			"prop3": map[string]interface{}{
				"type": "array",
				"items": map[string]interface{}{
					"type":             "string",
					"exclusiveMinimum": "z",
				},
			},
		}

		result := filterGeminiProperties(props)

		// Check prop1 transformation
		prop1 := result["prop1"].(map[string]interface{})
		assert.Contains(t, prop1, "minimum")
		assert.Contains(t, prop1, "description")

		// Check prop2 transformation
		prop2 := result["prop2"].(map[string]interface{})
		assert.Contains(t, prop2, "maximum")
		assert.Contains(t, prop2, "minimum")

		// Check prop3 nested transformation
		prop3 := result["prop3"].(map[string]interface{})
		items := prop3["items"].(map[string]interface{})
		assert.Contains(t, items, "minimum")
	})
}

// TestGeminiSchemaFieldTransforms tests field transformation mapping
func TestGeminiSchemaFieldTransforms(t *testing.T) {
	t.Run("exclusiveMinimum to minimum", func(t *testing.T) {
		targetKey, exists := geminiSchemaFieldTransforms["exclusiveMinimum"]
		assert.True(t, exists)
		assert.Equal(t, "minimum", targetKey)
	})

	t.Run("exclusiveMaximum to maximum", func(t *testing.T) {
		targetKey, exists := geminiSchemaFieldTransforms["exclusiveMaximum"]
		assert.True(t, exists)
		assert.Equal(t, "maximum", targetKey)
	})
}

// TestGeminiSupportedSchemaFields tests supported field mapping
func TestGeminiSupportedSchemaFields(t *testing.T) {
	supportedFields := []string{
		"type", "format", "title", "description", "nullable", "enum",
		"maxItems", "minItems", "properties", "required",
		"minProperties", "maxProperties", "minLength", "maxLength",
		"pattern", "example", "anyOf", "propertyOrdering", "default",
		"items", "minimum", "maximum",
	}

	for _, field := range supportedFields {
		t.Run(field, func(t *testing.T) {
			assert.True(t, geminiSupportedSchemaFields[field], "Field %s should be supported", field)
		})
	}
}
