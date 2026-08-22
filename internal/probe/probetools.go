package probe

import (
	"github.com/anthropics/anthropic-sdk-go"
	"github.com/openai/openai-go/v3"
	"github.com/openai/openai-go/v3/packages/param"
	"github.com/openai/openai-go/v3/responses"
	"github.com/openai/openai-go/v3/shared"
	"github.com/tingly-dev/tingly-box/internal/visionproxy"
)

// getProbeToolsAnthropic returns predefined tools in Anthropic format for probe
// testing. Uses a bash tool to execute simple file system operations.
func getProbeToolsAnthropic() []anthropic.ToolUnionParam {
	return []anthropic.ToolUnionParam{
		{
			OfTool: &anthropic.ToolParam{
				Name: "bash",
				InputSchema: anthropic.ToolInputSchemaParam{
					Type: "object",
					Properties: map[string]any{
						"command": map[string]any{
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
					Properties: map[string]any{
						"verbose": map[string]any{
							"type":        "boolean",
							"description": "Whether to include verbose status information",
						},
					},
				},
			},
		},
	}
}

// getProbeToolsOpenAI returns predefined tools in OpenAI format for probe
// testing. Uses a bash tool to execute simple file system operations.
func getProbeToolsOpenAI() []openai.ChatCompletionToolUnionParam {
	return []openai.ChatCompletionToolUnionParam{
		openai.ChatCompletionFunctionTool(shared.FunctionDefinitionParam{
			Name:        "bash",
			Description: param.NewOpt("Execute bash commands for file system operations. Supports commands like: ls, pwd, cat, grep, find, git status, etc."),
			Parameters: shared.FunctionParameters{
				"type":                 "object",
				"additionalProperties": false,
				"properties": map[string]any{
					"command": map[string]any{
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
				"properties": map[string]any{
					"verbose": map[string]any{
						"type":        "boolean",
						"description": "Whether to include verbose information",
					},
				},
			},
		}),
	}
}

// getProbeToolsResponses returns predefined tools in Responses API format for
// probe testing. Uses a bash tool to execute simple file system operations.
func getProbeToolsResponses() []responses.ToolUnionParam {
	return []responses.ToolUnionParam{
		responses.ToolParamOfFunction(
			"bash",
			map[string]any{
				"type":                 "object",
				"additionalProperties": false,
				"properties": map[string]any{
					"command": map[string]any{
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
				"properties": map[string]any{
					"verbose": map[string]any{
						"type":        "boolean",
						"description": "Whether to include verbose information",
					},
				},
			},
			false,
		),
	}
}

// getProbeToolChoiceAutoAnthropic returns auto tool choice for testing.
func getProbeToolChoiceAutoAnthropic() anthropic.ToolChoiceUnionParam {
	return anthropic.ToolChoiceUnionParam{
		OfAuto: &anthropic.ToolChoiceAutoParam{},
	}
}

// Vision-channel tool definitions: the synthetic capture tool the tool-channel
// vision probe pretends to have called (vision.ToolName). Declared so the
// tool-call history in the probe request is self-consistent for providers
// that validate tool references.

func getVisionToolOpenAI() openai.ChatCompletionToolUnionParam {
	return openai.ChatCompletionFunctionTool(shared.FunctionDefinitionParam{
		Name:        visionproxy.ToolName,
		Description: param.NewOpt("Capture an image for analysis"),
		Parameters: shared.FunctionParameters{
			"type":       "object",
			"properties": map[string]any{},
		},
	})
}

func getVisionToolResponses() responses.ToolUnionParam {
	return responses.ToolParamOfFunction(
		visionproxy.ToolName,
		map[string]any{
			"type":       "object",
			"properties": map[string]any{},
		},
		false,
	)
}

func getVisionToolAnthropic() anthropic.ToolUnionParam {
	return anthropic.ToolUnionParam{
		OfTool: &anthropic.ToolParam{
			Name: visionproxy.ToolName,
			InputSchema: anthropic.ToolInputSchemaParam{
				Type:       "object",
				Properties: map[string]any{},
			},
		},
	}
}
