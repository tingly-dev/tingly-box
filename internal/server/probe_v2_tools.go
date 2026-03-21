package server

import (
	"github.com/anthropics/anthropic-sdk-go"
	"github.com/openai/openai-go/v3"
	"github.com/openai/openai-go/v3/packages/param"
	"github.com/openai/openai-go/v3/shared"
)

// GetProbeToolsAnthropic returns predefined tools in Anthropic format for probe testing
func GetProbeToolsAnthropic() []anthropic.ToolUnionParam {
	return []anthropic.ToolUnionParam{
		{
			OfTool: &anthropic.ToolParam{
				Name: "add_numbers",
				InputSchema: anthropic.ToolInputSchemaParam{
					Type: "object",
					Properties: map[string]interface{}{
						"a": map[string]interface{}{
							"type":        "number",
							"description": "The first number to add",
						},
						"b": map[string]interface{}{
							"type":        "number",
							"description": "The second number to add",
						},
					},
					Required: []string{"a", "b"},
				},
			},
		},
	}
}

// GetProbeToolsOpenAI returns predefined tools in OpenAI format (as JSON map)
func GetProbeToolsOpenAI() []openai.ChatCompletionToolUnionParam {
	// Add tools for tool mode using raw JSON map
	return []openai.ChatCompletionToolUnionParam{
		openai.ChatCompletionFunctionTool(shared.FunctionDefinitionParam{
			Name:        "add_numbers",
			Description: param.NewOpt("Add two numbers"),
			Parameters: shared.FunctionParameters{
				"type": "object",
				"properties": map[string]interface{}{
					"a": map[string]interface{}{
						"type":        "number",
						"description": "The first number to add",
					},
					"b": map[string]interface{}{
						"type":        "number",
						"description": "The second number to add",
					},
				},
				"required": []string{"a", "b"},
			},
		}),
	}
}

// GetProbeToolChoiceAutoAnthropic returns auto tool choice for testing
func GetProbeToolChoiceAutoAnthropic() anthropic.ToolChoiceUnionParam {
	return anthropic.ToolChoiceUnionParam{
		OfAuto: &anthropic.ToolChoiceAutoParam{},
	}
}

// GetProbeToolChoiceOpenAI returns auto tool choice for OpenAI (as JSON map)
func GetProbeToolChoiceOpenAI() map[string]interface{} {
	return map[string]interface{}{
		"type": "function",
		"function": map[string]interface{}{
			"name": "add_numbers",
		},
	}
}
