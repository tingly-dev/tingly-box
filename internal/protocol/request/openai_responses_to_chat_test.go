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

func TestConvertOpenAIResponsesToChat(t *testing.T) {
	t.Run("simple user message", func(t *testing.T) {
		params := responses.ResponseNewParams{
			Model: "gpt-4",
			Input: responses.ResponseNewParamsInputUnion{
				OfInputItemList: responses.ResponseInputParam{
					{
						OfMessage: &responses.EasyInputMessageParam{
							Type: responses.EasyInputMessageTypeMessage,
							Role: responses.EasyInputMessageRole("user"),
							Content: responses.EasyInputMessageContentUnionParam{
								OfString: param.NewOpt("Hello, world!"),
							},
						},
					},
				},
			},
			MaxOutputTokens: param.NewOpt(int64(100)),
		}

		result := ConvertOpenAIResponsesToChat(params, 4096)

		assert.Equal(t, openai.ChatModel("gpt-4"), result.Model)
		assert.Equal(t, int64(100), result.MaxTokens.Value)
		assert.Len(t, result.Messages, 1)
		assert.Equal(t, "user", getMessageRole(t, result.Messages[0]))
		assert.Equal(t, "Hello, world!", getMessageContent(t, result.Messages[0]))
	})

	t.Run("system instructions and user message", func(t *testing.T) {
		params := responses.ResponseNewParams{
			Model:        "gpt-4",
			Instructions: param.NewOpt("You are a helpful assistant."),
			Input: responses.ResponseNewParamsInputUnion{
				OfInputItemList: responses.ResponseInputParam{
					{
						OfMessage: &responses.EasyInputMessageParam{
							Type: responses.EasyInputMessageTypeMessage,
							Role: responses.EasyInputMessageRole("user"),
							Content: responses.EasyInputMessageContentUnionParam{
								OfString: param.NewOpt("What is the capital of France?"),
							},
						},
					},
				},
			},
			MaxOutputTokens: param.NewOpt(int64(200)),
		}

		result := ConvertOpenAIResponsesToChat(params, 4096)

		assert.Len(t, result.Messages, 2)
		assert.Equal(t, "system", getMessageRole(t, result.Messages[0]))
		assert.Equal(t, "You are a helpful assistant.", getMessageContent(t, result.Messages[0]))
		assert.Equal(t, "user", getMessageRole(t, result.Messages[1]))
		assert.Equal(t, "What is the capital of France?", getMessageContent(t, result.Messages[1]))
	})

	t.Run("assistant message with text", func(t *testing.T) {
		params := responses.ResponseNewParams{
			Model: "gpt-4",
			Input: responses.ResponseNewParamsInputUnion{
				OfInputItemList: responses.ResponseInputParam{
					{
						OfMessage: &responses.EasyInputMessageParam{
							Type: responses.EasyInputMessageTypeMessage,
							Role: responses.EasyInputMessageRole("assistant"),
							Content: responses.EasyInputMessageContentUnionParam{
								OfString: param.NewOpt("The capital of France is Paris."),
							},
						},
					},
				},
			},
		}

		result := ConvertOpenAIResponsesToChat(params, 4096)

		assert.Len(t, result.Messages, 1)
		assert.Equal(t, "assistant", getMessageRole(t, result.Messages[0]))
		assert.Equal(t, "The capital of France is Paris.", getMessageContent(t, result.Messages[0]))
	})

	t.Run("with temperature and top_p", func(t *testing.T) {
		params := responses.ResponseNewParams{
			Model:           "gpt-4",
			Temperature:     param.NewOpt(0.7),
			TopP:            param.NewOpt(0.9),
			MaxOutputTokens: param.NewOpt(int64(100)),
			Input: responses.ResponseNewParamsInputUnion{
				OfInputItemList: responses.ResponseInputParam{
					{
						OfMessage: &responses.EasyInputMessageParam{
							Type: responses.EasyInputMessageTypeMessage,
							Role: responses.EasyInputMessageRole("user"),
							Content: responses.EasyInputMessageContentUnionParam{
								OfString: param.NewOpt("Hello"),
							},
						},
					},
				},
			},
		}

		result := ConvertOpenAIResponsesToChat(params, 4096)

		assert.InDelta(t, 0.7, result.Temperature.Value, 0.01)
		assert.InDelta(t, 0.9, result.TopP.Value, 0.01)
	})

	t.Run("default max tokens when not set", func(t *testing.T) {
		params := responses.ResponseNewParams{
			Model: "gpt-4",
			Input: responses.ResponseNewParamsInputUnion{
				OfInputItemList: responses.ResponseInputParam{
					{
						OfMessage: &responses.EasyInputMessageParam{
							Type: responses.EasyInputMessageTypeMessage,
							Role: responses.EasyInputMessageRole("user"),
							Content: responses.EasyInputMessageContentUnionParam{
								OfString: param.NewOpt("Hello"),
							},
						},
					},
				},
			},
		}

		result := ConvertOpenAIResponsesToChat(params, 4096)

		assert.Equal(t, int64(4096), result.MaxTokens.Value)
	})

	t.Run("multi-turn conversation", func(t *testing.T) {
		params := responses.ResponseNewParams{
			Model:        "gpt-4",
			Instructions: param.NewOpt("You are a helpful assistant."),
			Input: responses.ResponseNewParamsInputUnion{
				OfInputItemList: responses.ResponseInputParam{
					// User message
					{
						OfMessage: &responses.EasyInputMessageParam{
							Type: responses.EasyInputMessageTypeMessage,
							Role: responses.EasyInputMessageRole("user"),
							Content: responses.EasyInputMessageContentUnionParam{
								OfString: param.NewOpt("What's 2+2?"),
							},
						},
					},
					// Assistant message
					{
						OfMessage: &responses.EasyInputMessageParam{
							Type: responses.EasyInputMessageTypeMessage,
							Role: responses.EasyInputMessageRole("assistant"),
							Content: responses.EasyInputMessageContentUnionParam{
								OfString: param.NewOpt("2+2 equals 4."),
							},
						},
					},
					// User follow-up
					{
						OfMessage: &responses.EasyInputMessageParam{
							Type: responses.EasyInputMessageTypeMessage,
							Role: responses.EasyInputMessageRole("user"),
							Content: responses.EasyInputMessageContentUnionParam{
								OfString: param.NewOpt("And what's 3+3?"),
							},
						},
					},
				},
			},
		}

		result := ConvertOpenAIResponsesToChat(params, 4096)

		assert.Len(t, result.Messages, 4) // system + 3 messages
		assert.Equal(t, "system", getMessageRole(t, result.Messages[0]))
		assert.Equal(t, "user", getMessageRole(t, result.Messages[1]))
		assert.Equal(t, "assistant", getMessageRole(t, result.Messages[2]))
		assert.Equal(t, "user", getMessageRole(t, result.Messages[3]))
	})

	t.Run("function call and output", func(t *testing.T) {
		params := responses.ResponseNewParams{
			Model: "gpt-4",
			Input: responses.ResponseNewParamsInputUnion{
				OfInputItemList: responses.ResponseInputParam{
					// User message
					{
						OfMessage: &responses.EasyInputMessageParam{
							Type: responses.EasyInputMessageTypeMessage,
							Role: responses.EasyInputMessageRole("user"),
							Content: responses.EasyInputMessageContentUnionParam{
								OfString: param.NewOpt("What's the weather in NYC?"),
							},
						},
					},
					// Function call
					{
						OfFunctionCall: &responses.ResponseFunctionToolCallParam{
							CallID:    "call_123",
							Name:      "get_weather",
							Arguments: `{"location":"NYC"}`,
						},
					},
					// Function output
					{
						OfFunctionCallOutput: &responses.ResponseInputItemFunctionCallOutputParam{
							CallID: "call_123",
							Output: responses.ResponseInputItemFunctionCallOutputOutputUnionParam{
								OfString: param.NewOpt("Sunny, 22°C"),
							},
							Status: "completed",
						},
					},
				},
			},
		}

		result := ConvertOpenAIResponsesToChat(params, 4096)

		assert.Len(t, result.Messages, 3)

		// First message: user
		assert.Equal(t, "user", getMessageRole(t, result.Messages[0]))

		// Second message: assistant with tool_calls
		assert.Equal(t, "assistant", getMessageRole(t, result.Messages[1]))
		toolCalls := getToolCalls(t, result.Messages[1])
		require.Len(t, toolCalls, 1)
		assert.Equal(t, "call_123", toolCalls[0]["id"])
		assert.Equal(t, "get_weather", toolCalls[0]["function"].(map[string]interface{})["name"])

		// Third message: tool result
		assert.Equal(t, "tool", getMessageRole(t, result.Messages[2]))
		assert.Equal(t, "call_123", getToolCallID(t, result.Messages[2]))
	})

	t.Run("with tools", func(t *testing.T) {
		params := responses.ResponseNewParams{
			Model: "gpt-4",
			Input: responses.ResponseNewParamsInputUnion{
				OfInputItemList: responses.ResponseInputParam{
					{
						OfMessage: &responses.EasyInputMessageParam{
							Type: responses.EasyInputMessageTypeMessage,
							Role: responses.EasyInputMessageRole("user"),
							Content: responses.EasyInputMessageContentUnionParam{
								OfString: param.NewOpt("Hello"),
							},
						},
					},
				},
			},
			Tools: []responses.ToolUnionParam{
				{
					OfFunction: &responses.FunctionToolParam{
						Type:        "function",
						Name:        "get_weather",
						Description: param.NewOpt("Get the current weather"),
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
					},
				},
			},
		}

		result := ConvertOpenAIResponsesToChat(params, 4096)

		require.Len(t, result.Tools, 1)
		tool := result.Tools[0].GetFunction()
		require.NotNil(t, tool)
		assert.Equal(t, "get_weather", tool.Name)
		assert.Equal(t, "Get the current weather", tool.Description.Value)
	})

	t.Run("with tool choice auto", func(t *testing.T) {
		params := responses.ResponseNewParams{
			Model: "gpt-4",
			Input: responses.ResponseNewParamsInputUnion{
				OfInputItemList: responses.ResponseInputParam{
					{
						OfMessage: &responses.EasyInputMessageParam{
							Type: responses.EasyInputMessageTypeMessage,
							Role: responses.EasyInputMessageRole("user"),
							Content: responses.EasyInputMessageContentUnionParam{
								OfString: param.NewOpt("Hello"),
							},
						},
					},
				},
			},
			ToolChoice: responses.ResponseNewParamsToolChoiceUnion{
				OfToolChoiceMode: param.NewOpt(responses.ToolChoiceOptions("auto")),
			},
		}

		result := ConvertOpenAIResponsesToChat(params, 4096)

		assert.Equal(t, "auto", result.ToolChoice.OfAuto.Value)
	})

	t.Run("with tool choice specific function", func(t *testing.T) {
		params := responses.ResponseNewParams{
			Model: "gpt-4",
			Input: responses.ResponseNewParamsInputUnion{
				OfInputItemList: responses.ResponseInputParam{
					{
						OfMessage: &responses.EasyInputMessageParam{
							Type: responses.EasyInputMessageTypeMessage,
							Role: responses.EasyInputMessageRole("user"),
							Content: responses.EasyInputMessageContentUnionParam{
								OfString: param.NewOpt("Hello"),
							},
						},
					},
				},
			},
			ToolChoice: responses.ResponseNewParamsToolChoiceUnion{
				OfFunctionTool: &responses.ToolChoiceFunctionParam{
					Name: "get_weather",
				},
			},
		}

		result := ConvertOpenAIResponsesToChat(params, 4096)

		assert.NotNil(t, result.ToolChoice.OfFunctionToolChoice)
		assert.Equal(t, "get_weather", result.ToolChoice.OfFunctionToolChoice.Function.Name)
	})
}

func TestConvertResponsesInputToMessages(t *testing.T) {
	t.Run("empty input", func(t *testing.T) {
		messages := ConvertResponsesInputToMessages(responses.ResponseInputParam{})
		assert.Nil(t, messages)
	})

	t.Run("content array with multiple text items", func(t *testing.T) {
		input := responses.ResponseInputParam{
			{
				OfMessage: &responses.EasyInputMessageParam{
					Type: responses.EasyInputMessageTypeMessage,
					Role: responses.EasyInputMessageRole("user"),
					Content: responses.EasyInputMessageContentUnionParam{
						OfInputItemContentList: responses.ResponseInputMessageContentListParam{
							{
								OfInputText: &responses.ResponseInputTextParam{
									Type: "input_text",
									Text: "Hello, ",
								},
							},
							{
								OfInputText: &responses.ResponseInputTextParam{
									Type: "input_text",
									Text: "world!",
								},
							},
						},
					},
				},
			},
		}

		messages := ConvertResponsesInputToMessages(input)

		require.Len(t, messages, 1)
		assert.Equal(t, "user", getMessageRole(t, messages[0]))
		assert.Equal(t, "Hello, world!", getMessageContent(t, messages[0]))
	})
}

func TestConvertResponsesToolsToChatTools(t *testing.T) {
	t.Run("empty tools", func(t *testing.T) {
		tools := ConvertResponsesToolsToChatTools([]responses.ToolUnionParam{})
		assert.Nil(t, tools)
	})

	t.Run("function tool without parameters", func(t *testing.T) {
		tools := ConvertResponsesToolsToChatTools([]responses.ToolUnionParam{
			{
				OfFunction: &responses.FunctionToolParam{
					Type:        "function",
					Name:        "simple_tool",
					Description: param.NewOpt("A simple tool"),
				},
			},
		})

		require.Len(t, tools, 1)
		tool := tools[0].GetFunction()
		require.NotNil(t, tool)
		assert.Equal(t, "simple_tool", tool.Name)
		assert.NotNil(t, tool.Parameters)
	})
}

func TestConvertResponsesToolChoiceToChat(t *testing.T) {
	t.Run("auto mode", func(t *testing.T) {
		choice := responses.ResponseNewParamsToolChoiceUnion{
			OfToolChoiceMode: param.NewOpt(responses.ToolChoiceOptions("auto")),
		}

		result := ConvertResponsesToolChoiceToChat(choice)
		assert.Equal(t, "auto", result.OfAuto.Value)
	})

	t.Run("required mode", func(t *testing.T) {
		choice := responses.ResponseNewParamsToolChoiceUnion{
			OfToolChoiceMode: param.NewOpt(responses.ToolChoiceOptions("required")),
		}

		result := ConvertResponsesToolChoiceToChat(choice)
		assert.Equal(t, "required", result.OfAuto.Value)
	})

	t.Run("specific function", func(t *testing.T) {
		choice := responses.ResponseNewParamsToolChoiceUnion{
			OfFunctionTool: &responses.ToolChoiceFunctionParam{
				Name: "get_weather",
			},
		}

		result := ConvertResponsesToolChoiceToChat(choice)
		assert.NotNil(t, result.OfFunctionToolChoice)
		assert.Equal(t, "get_weather", result.OfFunctionToolChoice.Function.Name)
	})
}

// Helper functions to extract message properties

func getMessageRole(t *testing.T, msg openai.ChatCompletionMessageParamUnion) string {
	t.Helper()
	raw, _ := json.Marshal(msg)
	var m map[string]interface{}
	require.NoError(t, json.Unmarshal(raw, &m))
	role, _ := m["role"].(string)
	return role
}

func getMessageContent(t *testing.T, msg openai.ChatCompletionMessageParamUnion) string {
	t.Helper()
	raw, _ := json.Marshal(msg)
	var m map[string]interface{}
	require.NoError(t, json.Unmarshal(raw, &m))
	content, _ := m["content"].(string)
	return content
}

func getToolCalls(t *testing.T, msg openai.ChatCompletionMessageParamUnion) []map[string]interface{} {
	t.Helper()
	raw, _ := json.Marshal(msg)
	var m map[string]interface{}
	require.NoError(t, json.Unmarshal(raw, &m))
	toolCalls, _ := m["tool_calls"].([]interface{})
	result := make([]map[string]interface{}, 0, len(toolCalls))
	for _, tc := range toolCalls {
		if tcMap, ok := tc.(map[string]interface{}); ok {
			result = append(result, tcMap)
		}
	}
	return result
}

func getToolCallID(t *testing.T, msg openai.ChatCompletionMessageParamUnion) string {
	t.Helper()
	raw, _ := json.Marshal(msg)
	var m map[string]interface{}
	require.NoError(t, json.Unmarshal(raw, &m))
	id, _ := m["tool_call_id"].(string)
	return id
}
