package request

import (
	"testing"

	"github.com/openai/openai-go/v3"
	"github.com/openai/openai-go/v3/packages/param"
	"github.com/openai/openai-go/v3/responses"
	"github.com/openai/openai-go/v3/shared"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestConvertChatToOpenAIResponses(t *testing.T) {
	t.Run("simple user message", func(t *testing.T) {
		params := &openai.ChatCompletionNewParams{
			Model:     openai.ChatModel("gpt-4"),
			Messages:  []openai.ChatCompletionMessageParamUnion{openai.UserMessage("Hello, world!")},
			MaxTokens: param.NewOpt(int64(100)),
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
			Model:     openai.ChatModel("gpt-4"),
			Messages:  []openai.ChatCompletionMessageParamUnion{openai.SystemMessage("You are a helpful assistant."), openai.UserMessage("What is the capital of France?")},
			MaxTokens: param.NewOpt(int64(200)),
		}

		result := ConvertChatToOpenAIResponses(params, 4096)

		assert.Equal(t, "You are a helpful assistant.", result.Instructions.Value)
		assert.Len(t, result.Input.OfInputItemList, 1)
		assert.Equal(t, "user", string(result.Input.OfInputItemList[0].OfMessage.Role))
		assert.Equal(t, "What is the capital of France?", result.Input.OfInputItemList[0].OfMessage.Content.OfString.Value)
	})

	t.Run("multiple system messages concatenated", func(t *testing.T) {
		params := &openai.ChatCompletionNewParams{
			Model:    openai.ChatModel("gpt-4"),
			Messages: []openai.ChatCompletionMessageParamUnion{openai.SystemMessage("You are a helpful assistant."), openai.SystemMessage("Be concise in your answers."), openai.UserMessage("Hello")},
		}

		result := ConvertChatToOpenAIResponses(params, 4096)

		assert.Equal(t, "You are a helpful assistant.\n\nBe concise in your answers.", result.Instructions.Value)
		assert.Len(t, result.Input.OfInputItemList, 1)
	})

	t.Run("assistant message with text", func(t *testing.T) {
		params := &openai.ChatCompletionNewParams{
			Model:    openai.ChatModel("gpt-4"),
			Messages: []openai.ChatCompletionMessageParamUnion{openai.AssistantMessage("The capital of France is Paris.")},
		}

		result := ConvertChatToOpenAIResponses(params, 4096)

		assert.Len(t, result.Input.OfInputItemList, 1)
		assert.Equal(t, "assistant", string(result.Input.OfInputItemList[0].OfMessage.Role))
		assert.Equal(t, "The capital of France is Paris.", result.Input.OfInputItemList[0].OfMessage.Content.OfString.Value)
	})

	t.Run("with temperature and top_p", func(t *testing.T) {
		params := &openai.ChatCompletionNewParams{
			Model:       openai.ChatModel("gpt-4"),
			Temperature: param.NewOpt(0.7),
			TopP:        param.NewOpt(0.9),
			Messages:    []openai.ChatCompletionMessageParamUnion{openai.UserMessage("Hello")},
		}

		result := ConvertChatToOpenAIResponses(params, 4096)

		assert.InDelta(t, 0.7, result.Temperature.Value, 0.01)
		assert.InDelta(t, 0.9, result.TopP.Value, 0.01)
	})

	t.Run("default max tokens when not set", func(t *testing.T) {
		params := &openai.ChatCompletionNewParams{
			Model:    openai.ChatModel("gpt-4"),
			Messages: []openai.ChatCompletionMessageParamUnion{openai.UserMessage("Hello")},
		}

		result := ConvertChatToOpenAIResponses(params, 4096)

		assert.Equal(t, int64(4096), result.MaxOutputTokens.Value)
	})

	t.Run("multi-turn conversation", func(t *testing.T) {
		params := &openai.ChatCompletionNewParams{
			Model:    openai.ChatModel("gpt-4"),
			Messages: []openai.ChatCompletionMessageParamUnion{openai.SystemMessage("You are a helpful assistant."), openai.UserMessage("What's 2+2?"), openai.AssistantMessage("2+2 equals 4."), openai.UserMessage("And what's 3+3?")},
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
			Model:    openai.ChatModel("gpt-4"),
			Messages: []openai.ChatCompletionMessageParamUnion{openai.UserMessage("What's the weather in NYC?"), createAssistantWithToolCallsMessage()},
		}

		result := ConvertChatToOpenAIResponses(params, 4096)

		assert.Len(t, result.Input.OfInputItemList, 2)
		assert.Equal(t, "user", string(result.Input.OfInputItemList[0].OfMessage.Role))

		// Second item should be function_call
		fnCall := result.Input.OfInputItemList[1].OfFunctionCall
		require.NotNil(t, fnCall)
		assert.Equal(t, "call_123", fnCall.CallID)
		assert.Equal(t, "get_weather", fnCall.Name)
		assert.Equal(t, `{"location":"NYC"}`, fnCall.Arguments)
	})

	t.Run("tool result conversion", func(t *testing.T) {
		params := &openai.ChatCompletionNewParams{
			Model:    openai.ChatModel("gpt-4"),
			Messages: []openai.ChatCompletionMessageParamUnion{openai.UserMessage("What's the weather in NYC?"), createAssistantWithToolCallsMessage(), createToolResultMessage()},
		}

		result := ConvertChatToOpenAIResponses(params, 4096)

		assert.Len(t, result.Input.OfInputItemList, 3)

		// Third item should be function_call_output
		fnOutput := result.Input.OfInputItemList[2].OfFunctionCallOutput
		require.NotNil(t, fnOutput)
		assert.Equal(t, "call_123", fnOutput.CallID)
		assert.Equal(t, "Sunny, 22°C", fnOutput.Output.OfString.Value)
	})

	t.Run("with tools", func(t *testing.T) {
		params := &openai.ChatCompletionNewParams{
			Model:    openai.ChatModel("gpt-4"),
			Messages: []openai.ChatCompletionMessageParamUnion{openai.UserMessage("Hello")},
			Tools:    []openai.ChatCompletionToolUnionParam{openai.ChatCompletionFunctionTool(createGetWeatherFunction())},
		}

		result := ConvertChatToOpenAIResponses(params, 4096)

		require.Len(t, result.Tools, 1)
		tool := result.Tools[0].OfFunction
		require.NotNil(t, tool)
		assert.Equal(t, "get_weather", tool.Name)
		assert.Equal(t, "Get the current weather", tool.Description.Value)
		assert.NotNil(t, tool.Parameters)
	})

	t.Run("tool choice auto", func(t *testing.T) {
		params := &openai.ChatCompletionNewParams{
			Model:    openai.ChatModel("gpt-4"),
			Messages: []openai.ChatCompletionMessageParamUnion{openai.UserMessage("Hello")},
			ToolChoice: openai.ChatCompletionToolChoiceOptionUnionParam{
				OfAuto: param.NewOpt("auto"),
			},
		}

		result := ConvertChatToOpenAIResponses(params, 4096)

		assert.Equal(t, responses.ToolChoiceOptions("auto"), result.ToolChoice.OfToolChoiceMode.Value)
	})

	t.Run("tool choice none", func(t *testing.T) {
		params := &openai.ChatCompletionNewParams{
			Model:    openai.ChatModel("gpt-4"),
			Messages: []openai.ChatCompletionMessageParamUnion{openai.UserMessage("Hello")},
			ToolChoice: openai.ChatCompletionToolChoiceOptionUnionParam{
				OfAuto: param.NewOpt("none"),
			},
		}

		result := ConvertChatToOpenAIResponses(params, 4096)

		assert.Equal(t, responses.ToolChoiceOptions("none"), result.ToolChoice.OfToolChoiceMode.Value)
	})

	t.Run("tool choice required", func(t *testing.T) {
		params := &openai.ChatCompletionNewParams{
			Model:    openai.ChatModel("gpt-4"),
			Messages: []openai.ChatCompletionMessageParamUnion{openai.UserMessage("Hello")},
			ToolChoice: openai.ChatCompletionToolChoiceOptionUnionParam{
				OfAuto: param.NewOpt("required"),
			},
		}

		result := ConvertChatToOpenAIResponses(params, 4096)

		assert.Equal(t, responses.ToolChoiceOptions("required"), result.ToolChoice.OfToolChoiceMode.Value)
	})

	t.Run("tool choice specific function", func(t *testing.T) {
		params := &openai.ChatCompletionNewParams{
			Model:    openai.ChatModel("gpt-4"),
			Messages: []openai.ChatCompletionMessageParamUnion{openai.UserMessage("Hello")},
			ToolChoice: openai.ChatCompletionToolChoiceOptionUnionParam{
				OfFunctionToolChoice: &openai.ChatCompletionNamedToolChoiceParam{
					Function: openai.ChatCompletionNamedToolChoiceFunctionParam{
						Name: "get_weather",
					},
				},
			},
		}

		result := ConvertChatToOpenAIResponses(params, 4096)

		require.NotNil(t, result.ToolChoice.OfFunctionTool)
		assert.Equal(t, "get_weather", result.ToolChoice.OfFunctionTool.Name)
	})

	t.Run("developer message treated as system", func(t *testing.T) {
		params := &openai.ChatCompletionNewParams{
			Model:    openai.ChatModel("gpt-4"),
			Messages: []openai.ChatCompletionMessageParamUnion{openai.DeveloperMessage("You are a coding assistant."), openai.UserMessage("Hello")},
		}

		result := ConvertChatToOpenAIResponses(params, 4096)

		// Developer messages are not extracted as instructions, they go through default path
		assert.Equal(t, "", result.Instructions.Value)
		// But the user message should still be converted
		assert.Len(t, result.Input.OfInputItemList, 1)
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
			openai.ChatCompletionFunctionTool(shared.FunctionDefinitionParam{
				Name:        "simple_tool",
				Description: param.NewOpt("A simple tool"),
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
			OfAuto: param.NewOpt("auto"),
		}

		result := ConvertChatToolChoiceToResponsesToolChoice(choice)
		assert.Equal(t, responses.ToolChoiceOptions("auto"), result.OfToolChoiceMode.Value)
	})

	t.Run("none mode", func(t *testing.T) {
		choice := &openai.ChatCompletionToolChoiceOptionUnionParam{
			OfAuto: param.NewOpt("none"),
		}

		result := ConvertChatToolChoiceToResponsesToolChoice(choice)
		assert.Equal(t, responses.ToolChoiceOptions("none"), result.OfToolChoiceMode.Value)
	})

	t.Run("required mode", func(t *testing.T) {
		choice := &openai.ChatCompletionToolChoiceOptionUnionParam{
			OfAuto: param.NewOpt("required"),
		}

		result := ConvertChatToolChoiceToResponsesToolChoice(choice)
		assert.Equal(t, responses.ToolChoiceOptions("required"), result.OfToolChoiceMode.Value)
	})

	t.Run("specific function", func(t *testing.T) {
		choice := &openai.ChatCompletionToolChoiceOptionUnionParam{
			OfFunctionToolChoice: &openai.ChatCompletionNamedToolChoiceParam{
				Function: openai.ChatCompletionNamedToolChoiceFunctionParam{
					Name: "get_weather",
				},
			},
		}

		result := ConvertChatToolChoiceToResponsesToolChoice(choice)
		require.NotNil(t, result.OfFunctionTool)
		assert.Equal(t, "get_weather", result.OfFunctionTool.Name)
	})

	t.Run("default when empty", func(t *testing.T) {
		choice := &openai.ChatCompletionToolChoiceOptionUnionParam{}
		result := ConvertChatToolChoiceToResponsesToolChoice(choice)
		assert.Equal(t, responses.ToolChoiceOptions("auto"), result.OfToolChoiceMode.Value)
	})
}

// Helper functions for test data

// createAssistantWithToolCallsMessage creates an assistant message with tool calls using SDK
func createAssistantWithToolCallsMessage() openai.ChatCompletionMessageParamUnion {
	toolCall := openai.ChatCompletionMessageToolCallUnionParam{
		OfFunction: &openai.ChatCompletionMessageFunctionToolCallParam{
			ID: "call_123",
			Function: openai.ChatCompletionMessageFunctionToolCallFunctionParam{
				Name:      "get_weather",
				Arguments: `{"location":"NYC"}`,
			},
		},
	}

	return openai.ChatCompletionMessageParamUnion{
		OfAssistant: &openai.ChatCompletionAssistantMessageParam{
			Content: openai.ChatCompletionAssistantMessageParamContentUnion{
				OfString: param.NewOpt(""),
			},
			ToolCalls: []openai.ChatCompletionMessageToolCallUnionParam{toolCall},
		},
	}
}

// createToolResultMessage creates a tool result message using SDK
func createToolResultMessage() openai.ChatCompletionMessageParamUnion {
	return openai.ChatCompletionMessageParamUnion{
		OfTool: &openai.ChatCompletionToolMessageParam{
			ToolCallID: "call_123",
			Content: openai.ChatCompletionToolMessageParamContentUnion{
				OfString: param.NewOpt("Sunny, 22°C"),
			},
		},
	}
}

// createGetWeatherFunction creates a weather tool definition
func createGetWeatherFunction() shared.FunctionDefinitionParam {
	return shared.FunctionDefinitionParam{
		Name:        "get_weather",
		Description: param.NewOpt("Get the current weather"),
		Parameters: openai.FunctionParameters(map[string]interface{}{
			"type": "object",
			"properties": map[string]interface{}{
				"location": map[string]interface{}{
					"type":        "string",
					"description": "The location",
				},
			},
			"required": []string{"location"},
		}),
	}
}
