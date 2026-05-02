package request

import (
	"encoding/json"
	"testing"

	"github.com/anthropics/anthropic-sdk-go"
	"github.com/openai/openai-go/v3/shared"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// TestConvertAnthropicInputSchemaToOpenAIParameters tests the helper function
// that converts Anthropic input schema to OpenAI function parameters.
// This tests the fix for double-escaping issue where nested schemas
// were being incorrectly handled.
func TestConvertAnthropicInputSchemaToOpenAIParameters(t *testing.T) {
	tests := []struct {
		name      string
		properties any
		required   []string
		want      shared.FunctionParameters
	}{
		{
			name:      "nil properties and empty required",
			properties: nil,
			required:   []string{},
			want:      nil,
		},
		{
			name:      "nil properties with required",
			properties: nil,
			required:   []string{"path"},
			want: shared.FunctionParameters{
				"type":     "object",
				"required": []string{"path"},
			},
		},
		{
			name: "simple properties map",
			properties: map[string]any{
				"path": map[string]any{
					"type":        "string",
					"description": "The directory path",
				},
			},
			required: []string{"path"},
			want: shared.FunctionParameters{
				"type": "object",
				"properties": map[string]any{
					"path": map[string]any{
						"type":        "string",
						"description": "The directory path",
					},
				},
				"required": []string{"path"},
			},
		},
		{
			name: "nested schema with properties key (full schema)",
			// This is the case from agentscope where the full schema is passed
			properties: map[string]any{
				"type": "object",
				"properties": map[string]any{
					"path": map[string]any{
						"type":        "string",
						"description": "The directory path",
					},
				},
				"required": []string{"path"},
			},
			required: []string{"path"},
			want: shared.FunctionParameters{
				"type": "object",
				"properties": map[string]any{
					"path": map[string]any{
						"type":        "string",
						"description": "The directory path",
					},
				},
				"required": []string{"path"},
			},
		},
		{
			name: "nested schema without type key",
			properties: map[string]any{
				"properties": map[string]any{
					"command": map[string]any{
						"type":        "string",
						"description": "The bash command",
					},
				},
			},
			required: []string{"command"},
			want: shared.FunctionParameters{
				"type": "object",
				"properties": map[string]any{
					"command": map[string]any{
						"type":        "string",
						"description": "The bash command",
					},
				},
				"required": []string{"command"},
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := convertAnthropicInputSchemaToOpenAIParameters(tt.properties, tt.required)

			// Compare JSON for better diff output
			gotJSON, _ := json.Marshal(got)
			wantJSON, _ := json.Marshal(tt.want)

			assert.JSONEq(t, string(wantJSON), string(gotJSON),
				"convertAnthropicInputSchemaToOpenAIParameters() mismatch")
		})
	}
}

// TestConvertAnthropicToolsToOpenAI_DoubleEscapingFix tests the fix for
// the double-escaping bug where tool schemas were incorrectly marshaled.
func TestConvertAnthropicToolsToOpenAI_DoubleEscapingFix(t *testing.T) {
	// Create a tool similar to what agentscope produces
	tool := anthropic.ToolParam{
		Name:        "change_workdir",
		Description: anthropic.String("Change the bound project directory"),
		InputSchema: anthropic.ToolInputSchemaParam{
			Type: "object",
			Properties: map[string]any{
				"type":       "object",
				"properties": map[string]any{
					"path": map[string]any{
						"type":        "string",
						"description": "The directory path to change to",
					},
				},
				"required": []string{"path"},
			},
			Required: []string{"path"},
		},
	}

	tools := []anthropic.ToolUnionParam{
		anthropic.ToolUnionParam{OfTool: &tool},
	}

	result := ConvertAnthropicToolsToOpenAI(tools)

	require.Len(t, result, 1)

	// Marshal to JSON to verify no double-escaping
	resultJSON, err := json.Marshal(result)
	require.NoError(t, err)

	var resultObj []map[string]interface{}
	err = json.Unmarshal(resultJSON, &resultObj)
	require.NoError(t, err)

	// Extract the function parameters
	function, ok := resultObj[0]["function"].(map[string]interface{})
	require.True(t, ok, "function field should exist")

	parameters, ok := function["parameters"].(map[string]interface{})
	require.True(t, ok, "parameters field should exist")

	// Verify properties is an object, not a string
	properties, ok := parameters["properties"].(map[string]interface{})
	require.True(t, ok, "properties should be a map, not a string")

	// Verify the path property exists
	pathProp, ok := properties["path"].(map[string]interface{})
	require.True(t, ok, "path property should exist")
	assert.Equal(t, "string", pathProp["type"])

	// Verify required is an array
	required, ok := parameters["required"].([]interface{})
	require.True(t, ok, "required should be an array")
	assert.Len(t, required, 1)
	assert.Equal(t, "path", required[0])
}

// TestConvertAnthropicToolsToOpenAI_EmptyTools tests conversion with no tools
func TestConvertAnthropicToolsToOpenAI_EmptyTools(t *testing.T) {
	result := ConvertAnthropicToolsToOpenAI([]anthropic.ToolUnionParam{})
	assert.Nil(t, result)
}

// TestConvertAnthropicToolsToOpenAI_MultipleTools tests conversion with multiple tools
func TestConvertAnthropicToolsToOpenAI_MultipleTools(t *testing.T) {
	tool1 := anthropic.ToolParam{
		Name:        "bash",
		Description: anthropic.String("Execute bash commands"),
		InputSchema: anthropic.ToolInputSchemaParam{
			Type: "object",
			Properties: map[string]any{
				"command": map[string]any{
					"type":        "string",
					"description": "The command to execute",
				},
			},
			Required: []string{"command"},
		},
	}

	tool2 := anthropic.ToolParam{
		Name:        "read_file",
		Description: anthropic.String("Read a file"),
		InputSchema: anthropic.ToolInputSchemaParam{
			Type: "object",
			Properties: map[string]any{
				"path": map[string]any{
					"type":        "string",
					"description": "File path",
				},
			},
			Required: []string{"path"},
		},
	}

	tools := []anthropic.ToolUnionParam{
		anthropic.ToolUnionParam{OfTool: &tool1},
		anthropic.ToolUnionParam{OfTool: &tool2},
	}

	result := ConvertAnthropicToolsToOpenAI(tools)

	require.Len(t, result, 2)

	// Verify both tools are converted
	resultJSON, _ := json.Marshal(result)
	var resultObj []map[string]interface{}
	json.Unmarshal(resultJSON, &resultObj)

	// Check first tool
	func1, _ := resultObj[0]["function"].(map[string]interface{})
	assert.Equal(t, "bash", func1["name"])

	// Check second tool
	func2, _ := resultObj[1]["function"].(map[string]interface{})
	assert.Equal(t, "read_file", func2["name"])
}
