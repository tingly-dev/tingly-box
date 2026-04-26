package request

import (
	"encoding/json"
	"testing"

	"github.com/openai/openai-go/v3"
	"github.com/openai/openai-go/v3/packages/param"
	"github.com/openai/openai-go/v3/responses"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestConvertChatToOpenAIResponses(t *testing.T) {
	t.Run("simple user message", func(t *testing.T) {
		params := &openai.ChatCompletionNewParams{
			Model: openai.ChatModel("gpt-4"),
			Messages: []openai.ChatCompletionMessageParamUnion{
				openai.UserMessage("Hello, world!"),
			},
			MaxTokens: param.Opt(int64(100)),
		}

		result := ConvertChatToOpenAIResponses(params, 4096)

		assert.Equal(t, "gpt-4", result.Model)
		assert.Equal(t, int64(100), result.MaxOutputTokens.Value)
		assert.Len(t, result.Input.OfInputItemList, 1)
		assert.Equal(t, "user", string(result.Input.OfInputItemList[0].OfMessage.Role))
		assert.Equal(t, "Hello, world!", result.Input.OfInputItemList[0].OfMessage.Content.OfString.Value)
	})

	t.Run("system instructions and user message", func(t *testing.T) {
		params := &openai.ChatCompletionNewParams{
			Model: openai.ChatModel("gpt-4"),
			Messages: []openai.ChatCompletionMessageParamUnion{
				openai.SystemMessage("You are a helpful assistant."),
				openai.UserMessage("What is the capital of France?"),
			},
			MaxTokens: param.Opt(int64(200)),
		}

		result := ConvertChatToOpenAIResponses(params, 4096)

		assert.Equal(t, "You are a helpful assistant.", result.Instructions.Value)
		assert.Len(t, result.Input.OfInputItemList, 1)
		assert.Equal(t, "user", string(result.Input.OfInputItemList[0].OfMessage.Role))
		assert.Equal(t, "What is the capital of France?", result.Input.OfInputItemList[0].OfMessage.Content.OfString.Value)
	})

	t.Run("multiple system messages concatenated", func(t *testing.T) {
		params := &openai.ChatCompletionNewParams{
			Model: openai.ChatModel("gpt-4"),
			Messages: []openai.ChatCompletionMessageParamUnion{
				openai.SystemMessage("You are a helpful assistant."),
				openai.SystemMessage("Be concise in your answers."),
				openai.UserMessage("Hello"),
			},
		}

		result := ConvertChatToOpenAIResponses(params, 4096)

		assert.Equal(t, "You are a helpful assistant.\n\nBe concise in your answers.", result.Instructions.Value)
		assert.Len(t, result.Input.OfInputItemList, 1)
	})

	t.Run("assistant message with text", func(t *testing.T) {
		params := &openai.ChatCompletionNewParams{
			Model: openai.ChatModel("gpt-4"),
			Messages: []openai.ChatCompletionMessageParamUnion{
				openai.AssistantMessage("The capital of France is Paris."),
			},
		}

		result := ConvertChatToOpenAIResponses(params, 4096)

		assert.Len(t, result.Input.OfInputItemList, 1)
		assert.Equal(t, "assistant", string(result.Input.OfInputItemList[0].OfMessage.Role))
		assert.Equal(t, "The capital of France is Paris.", result.Input.OfInputItemList[0].OfMessage.Content.OfString.Value)
	})

	t.Run("with temperature and top_p", func(t *testing.T) {
		params := &openai.ChatCompletionNewParams{
			Model:       openai.ChatModel("gpt-4"),
			Temperature: param.Opt(0.7),
			TopP:        param.Opt(0.9),
			Messages: []openai.ChatCompletionMessageParamUnion{
				openai.UserMessage("Hello"),
			},
		}

		result := ConvertChatToOpenAIResponses(params, 4096)

		assert.InDelta(t, 0.7, result.Temperature.Value, 0.01)
		assert.InDelta(t, 0.9, result.TopP.Value, 0.01)
	})

	t.Run("default max tokens when not set", func(t *testing.T) {
		params := &openai.ChatCompletionNewParams{
			Model: openai.ChatModel("gpt-4"),
			Messages: []openai.ChatCompletionMessageParamUnion{
				openai.UserMessage("Hello"),
			},
		}

		result := ConvertChatToOpenAIResponses(params, 4096)

		assert.Equal(t, int64(4096), result.MaxOutputTokens.Value)
	})

	t.Run("multi-turn conversation", func(t *testing.T) {
		params := &openai.ChatCompletionNewParams{
			Model: openai.ChatModel("gpt-4"),
			Messages: []openai.ChatCompletionMessageParamUnion{
				openai.SystemMessage("You are a helpful assistant."),
				openai.UserMessage("What's 2+2?"),
				openai.AssistantMessage("2+2 equals 4."),
				openai.UserMessage("And what's 3+3?"),
			},
		}

		result := ConvertChatToOpenAIResponses(params, 4096)

		assert.Equal(t, "You are a helpful assistant.", result.Instructions.Value)
		assert.Len(t, result.Input.OfInputItemList, 3)
		assert.Equal(t, "user", string(result.Input.OfInputItemList[0].OfMessage.Role))
		assert.Equal(t, "assistant", string(result.Input.OfInputItemList[1].OfMessage.Role))
		assert.Equal(t, "user", string(result.Input.OfInputItemList[2].OfMessage.Role))
	})

	t.Run("tool call conversion", func(t *testing.T) {
		params := &openai.ChatCompletionNewParams{
			Model: openai.ChatModel("gpt-4"),
			Messages: []openai.ChatCompletionMessageParamUnion{
				openai.UserMessage("What's the weather in NYC?"),
				createAssistantWithToolCallsMessage(),
			},
		}

		result := ConvertChatToOpenAIResponses(params, 4096)

		assert.Len(t, result.Input.OfInputItemList, 2)
		assert.Equal(t, "user", string(result.Input.OfInputItemList[0].OfMessage.Role))

		// Second item should be function_call
		fnCall := result.Input.OfInputItemList[1].OfFunctionCall
		assert.NotNil(t, fnCall)
		assert.Equal(t, "call_123", fnCall.CallID)
		assert.Equal(t, "get_weather", fnCall.Name)
		assert.Equal(t, `{"location":"NYC"}`, fnCall.Arguments)
	})

	t.Run("tool result conversion", func(t *testing.T) {
		params := &openai.ChatCompletionNewParams{
			Model: openai.ChatModel("gpt-4"),
			Messages: []openai.ChatCompletionMessageParamUnion{
				openai.UserMessage("What's the weather in NYC?"),
				createAssistantWithToolCallsMessage(),
				createToolResultMessage(),
			},
		}

		result := ConvertChatToOpenAIResponses(params, 4096)

		assert.Len(t, result.Input.OfInputItemList, 3)

		// Third item should be function_call_output
		fnOutput := result.Input.OfInputItemList[2].OfFunctionCallOutput
		assert.NotNil(t, fnOutput)
		assert.Equal(t, "call_123", fnOutput.CallID)
		assert.Equal(t, "Sunny, 22°C", fnOutput.Output.OfString.Value)
	})

	t.Run("with tools", func(t *testing.T) {
		params := &openai.ChatCompletionNewParams{
			Model: openai.ChatModel("gpt-4"),
			Messages: []openai.ChatCompletionMessageParamUnion{
				openai.UserMessage("Hello"),
			},
			Tools: []openai.ChatCompletionToolUnionParam{
				openai.Functions(createGetWeatherFunction()),
			},
		}

		result := ConvertChatToOpenAIResponses(params, 4096)

		assert.Len(t, result.Tools, 1)
		tool := result.Tools[0].OfFunction
		assert.NotNil(t, tool)
		assert.Equal(t, "get_weather", tool.Name)
		assert.Equal(t, "Get the current weather", tool.Description.Value)
		assert.NotNil(t, tool.Parameters)
	})

	t.Run("tool choice auto", func(t *testing.T) {
		params := &openai.ChatCompletionNewParams{
			Model: openai.ChatModel("gpt-4"),
			Messages: []openai.ChatCompletionMessageParamUnion{
				openai.UserMessage("Hello"),
			},
			ToolChoice: openai.ChatCompletionToolChoiceOptionUnionParam{
				OfAuto: param.Opt("auto"),
			},
		}

		result := ConvertChatToOpenAIResponses(params, 4096)

		assert.Equal(t, "auto", result.ToolChoice.OfToolChoiceMode.Value)
	})

	t.Run("tool choice specific function", func(t *testing.T) {
		params := &openai.ChatCompletionNewParams{
			Model: openai.ChatModel("gpt-4"),
			Messages: []openai.ChatCompletionMessageParamUnion{
				openai.UserMessage("Hello"),
			},
			ToolChoice: openai.ChatCompletionToolChoiceOptionUnionParam{
				OfFunctionToolChoice: &openai.ChatCompletionNamedToolChoiceFunctionParam{
					Function: openai.ChatCompletionNamedToolChoiceFunction{
						Name: "get_weather",
					},
				},
			},
		}

		result := ConvertChatToOpenAIResponses(params, 4096)

		assert.NotNil(t, result.ToolChoice.OfFunctionTool)
		assert.Equal(t, "get_weather", result.ToolChoice.OfFunctionTool.Name)
	})
}

func TestConvertChatMessagesToResponsesInput(t *testing.T) {
	t.Run("empty input", func(t *testing.T) {
		messages := ConvertChatMessagesToResponsesInput([]openai.ChatCompletionMessageParamUnion{})
		assert.Nil(t, messages)
	})
}

func TestConvertChatToolsToResponsesTools(t *testing.T) {
	t.Run("empty tools", func(t *testing.T) {
		tools := ConvertChatToolsToResponsesTools([]openai.ChatCompletionToolUnionParam{})
		assert.Nil(t, tools)
	})

	t.Run("function tool without parameters", func(t *testing.T) {
		tools := ConvertChatToolsToResponsesTools([]openai.ChatCompletionToolUnionParam{
			openai.Functions(openai.FunctionDefinitionParams{
				Name:        "simple_tool",
				Description: param.Opt("A simple tool"),
			}),
		})

		require.Len(t, tools, 1)
		tool := tools[0].OfFunction
		require.NotNil(t, tool)
		assert.Equal(t, "simple_tool", tool.Name)
		assert.Equal(t, "A simple tool", tool.Description.Value)
	})
}

func TestConvertChatToolChoiceToResponsesToolChoice(t *testing.T) {
	t.Run("auto mode", func(t *testing.T) {
		choice := &openai.ChatCompletionToolChoiceOptionUnionParam{
			OfAuto: param.Opt("auto"),
		}

		result := ConvertChatToolChoiceToResponsesToolChoice(choice)
		assert.Equal(t, "auto", result.OfToolChoiceMode.Value)
	})

	t.Run("none mode", func(t *testing.T) {
		choice := &openai.ChatCompletionToolChoiceOptionUnionParam{
			OfAuto: param.Opt("none"),
		}

		result := ConvertChatToolChoiceToResponsesToolChoice(choice)
		assert.Equal(t, "none", result.OfToolChoiceMode.Value)
	})

	t.Run("required mode", func(t *testing.T) {
		choice := &openai.ChatCompletionToolChoiceOptionUnionParam{
			OfAuto: param.Opt("required"),
		}

		result := ConvertChatToolChoiceToResponsesToolChoice(choice)
		assert.Equal(t, "required", result.OfToolChoiceMode.Value)
	})

	t.Run("specific function", func(t *testing.T) {
		choice := &openai.ChatCompletionToolChoiceOptionUnionParam{
			OfFunctionToolChoice: &openai.ChatCompletionNamedToolChoiceFunctionParam{
				Function: openai.ChatCompletionNamedToolChoiceFunction{
					Name: "get_weather",
				},
			},
		}

		result := ConvertChatToolChoiceToResponsesToolChoice(choice)
		assert.NotNil(t, result.OfFunctionTool)
		assert.Equal(t, "get_weather", result.OfFunctionTool.Name)
	})
}

// Helper functions for test data

func createAssistantWithToolCallsMessage() openai.ChatCompletionMessageParamUnion {
	// Create a message with tool calls using JSON construction
	msgJSON := `{
		"role": "assistant",
		"content": "",
		"tool_calls": [
			{
				"id": "call_123",
				"type": "function",
				"function": {
					"name": "get_weather",
					"arguments": "{\"location\":\"NYC\"}"
				}
			}
		]
	}`

	var msg openai.ChatCompletionMessageParamUnion
	json.Unmarshal([]byte(msgJSON), &msg)
	return msg
}

func createToolResultMessage() openai.ChatCompletionMessageParamUnion {
	msgJSON := `{
		"role": "tool",
		"tool_call_id": "call_123",
		"content": "Sunny, 22°C"
	}`

	var msg openai.ChatCompletionMessageParamUnion
	json.Unmarshal([]byte(msgJSON), &msg)
	return msg
}

func createGetWeatherFunction() openai.FunctionDefinitionParams {
	return openai.FunctionDefinitionParams{
		Name:        "get_weather",
		Description: param.Opt("Get the current weather"),
		Parameters: map[string]interface{}{
			"type": "object",
			"properties": map[string]interface{}{
				"location": map[string]interface{}{
					"type":        "string",
					"description": "The location",
				},
			},
			"required": []string{"location"},
		},
	}
}
