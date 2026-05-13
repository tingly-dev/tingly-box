package client

import (
	"github.com/anthropics/anthropic-sdk-go"
	"github.com/openai/openai-go/v3"
	"github.com/openai/openai-go/v3/packages/param"
	"github.com/openai/openai-go/v3/responses"
	"github.com/openai/openai-go/v3/shared"
)

// GetProbeToolsAnthropic returns predefined tools in Anthropic format for probe testing
// Uses bash tool to execute simple file system operations
func GetProbeToolsAnthropic() []anthropic.ToolUnionParam {
	return []anthropic.ToolUnionParam{
		{
			OfTool: &anthropic.ToolParam{
				Name: "bash",
				InputSchema: anthropic.ToolInputSchemaParam{
					Type: "object",
					Properties: map[string]interface{}{
						"command": map[string]interface{}{
							"type":        "string",
							"description": "The bash command to execute (e.g., 'ls -la', 'pwd', 'cat file.txt')",
						},
					},
					Required: []string{"command"},
				},
			},
		},
		{
			OfTool: &anthropic.ToolParam{
				Name: "get_status",
				InputSchema: anthropic.ToolInputSchemaParam{
					Type: "object",
					Properties: map[string]interface{}{
						"verbose": map[string]interface{}{
							"type":        "boolean",
							"description": "Whether to include verbose status information",
						},
					},
				},
			},
		},
	}
}

// GetProbeToolsOpenAI returns predefined tools in OpenAI format for probe testing
// Uses bash tool to execute simple file system operations
func GetProbeToolsOpenAI() []openai.ChatCompletionToolUnionParam {
	return []openai.ChatCompletionToolUnionParam{
		openai.ChatCompletionFunctionTool(shared.FunctionDefinitionParam{
			Name:        "bash",
			Description: param.NewOpt("Execute bash commands for file system operations. Supports commands like: ls, pwd, cat, grep, find, git status, etc."),
			Parameters: shared.FunctionParameters{
				"type:":                "object",
				"additionalProperties": false,
				"properties": map[string]interface{}{
					"command": map[string]interface{}{
						"type":        "string",
						"description": "The bash command to execute",
					},
				},
				"required": []string{"command"},
			},
		}),
		openai.ChatCompletionFunctionTool(shared.FunctionDefinitionParam{
			Name:        "get_status",
			Description: param.NewOpt("Get current status including working directory, git branch, and system info"),
			Parameters: shared.FunctionParameters{
				"type":                 "object",
				"additionalProperties": false,
				"properties": map[string]interface{}{
					"verbose": map[string]interface{}{
						"type":        "boolean",
						"description": "Whether to include verbose information",
					},
				},
			},
		}),
	}
}

// GetProbeToolsResponses returns predefined tools in Responses API format for probe testing
// Uses bash tool to execute simple file system operations
func GetProbeToolsResponses() []responses.ToolUnionParam {
	return []responses.ToolUnionParam{
		responses.ToolParamOfFunction(
			"bash",
			map[string]any{
				"type":                 "object",
				"additionalProperties": false,
				"properties": map[string]interface{}{
					"command": map[string]interface{}{
						"type":        "string",
						"description": "The bash command to execute",
					},
				},
				"required": []string{"command"},
			},
			true,
		),
		responses.ToolParamOfFunction(
			"get_status",
			map[string]any{
				"type":                 "object",
				"additionalProperties": false,
				"properties": map[string]interface{}{
					"verbose": map[string]interface{}{
						"type":        "boolean",
						"description": "Whether to include verbose information",
					},
				},
			},
			false,
		),
	}
}

// GetProbeToolChoiceAutoAnthropic returns auto tool choice for testing
func GetProbeToolChoiceAutoAnthropic() anthropic.ToolChoiceUnionParam {
	return anthropic.ToolChoiceUnionParam{
		OfAuto: &anthropic.ToolChoiceAutoParam{},
	}
}
