package request

import (
	"encoding/json"
	"strings"

	"github.com/openai/openai-go/v3"
	"github.com/openai/openai-go/v3/packages/param"
	"github.com/openai/openai-go/v3/responses"
)

// ConvertChatToOpenAIResponses converts OpenAI Chat Completions params to Responses API format.
// This enables using Chat Completions format with Responses API providers.
func ConvertChatToOpenAIResponses(params *openai.ChatCompletionNewParams, defaultMaxTokens int64) *responses.ResponseNewParams {
	result := &responses.ResponseNewParams{
		Model: string(params.Model),
	}

	var systemParts []string
	var otherMessages []openai.ChatCompletionMessageParamUnion

	// Separate system messages from other messages
	for _, msg := range params.Messages {
		raw, _ := json.Marshal(msg)
		var m map[string]interface{}
		if err := json.Unmarshal(raw, &m); err != nil {
			// If we can't parse, skip this message
			continue
		}

		role, _ := m["role"].(string)
		if role == "system" {
			// Extract system message content
			if content, ok := m["content"].(string); ok && content != "" {
				systemParts = append(systemParts, content)
			}
		} else {
			otherMessages = append(otherMessages, msg)
		}
	}

	// Set instructions from system messages
	if len(systemParts) > 0 {
		result.Instructions = param.NewOpt(strings.Join(systemParts, "\n\n"))
	}

	// Convert messages to input items
	if len(otherMessages) > 0 {
		inputItems := ConvertChatMessagesToResponsesInput(otherMessages)
		result.Input = responses.ResponseNewParamsInputUnion{
			OfInputItemList: inputItems,
		}
	}

	// Convert max_tokens to max_output_tokens
	if params.MaxTokens.Value > 0 {
		result.MaxOutputTokens = param.NewOpt(params.MaxTokens.Value)
	} else if defaultMaxTokens > 0 {
		result.MaxOutputTokens = param.NewOpt(defaultMaxTokens)
	}

	// Copy temperature
	if params.Temperature.Value > 0 {
		result.Temperature = param.NewOpt(params.Temperature.Value)
	}

	// Copy top_p
	if params.TopP.Value > 0 {
		result.TopP = param.NewOpt(params.TopP.Value)
	}

	// Convert tools if present
	if len(params.Tools) > 0 {
		result.Tools = ConvertChatToolsToResponsesTools(params.Tools)
	}

	// Convert tool choice if present
	result.ToolChoice = ConvertChatToolChoiceToResponsesToolChoice(&params.ToolChoice)

	return result
}

// ConvertChatMessagesToResponsesInput converts Chat Completion messages to Responses API input items.
func ConvertChatMessagesToResponsesInput(messages []openai.ChatCompletionMessageParamUnion) responses.ResponseInputParam {
	var result responses.ResponseInputParam

	for _, msg := range messages {
		raw, _ := json.Marshal(msg)
		var m map[string]interface{}
		if err := json.Unmarshal(raw, &m); err != nil {
			continue
		}

		role, _ := m["role"].(string)

		switch role {
		case "user":
			result = append(result, convertChatUserMessageToResponses(m))

		case "assistant":
			// Check if assistant has tool_calls
			if toolCalls, ok := m["tool_calls"].([]interface{}); ok && len(toolCalls) > 0 {
				// Convert each tool call to function_call item
				for _, tc := range toolCalls {
					if call, ok := tc.(map[string]interface{}); ok {
						if fn, ok := call["function"].(map[string]interface{}); ok {
							callID, _ := call["id"].(string)
							name, _ := fn["name"].(string)
							arguments, _ := fn["arguments"].(string)

							result = append(result, responses.ResponseInputItemUnionParam{
								OfFunctionCall: &responses.ResponseFunctionToolCallParam{
									CallID:    callID,
									Name:      name,
									Arguments: arguments,
								},
							})
						}
					}
				}
			} else {
				// Regular assistant message
				result = append(result, convertChatAssistantMessageToResponses(m))
			}

		case "tool":
			// Tool result message → function_call_output item
			result = append(result, convertChatToolMessageToResponses(m))
		}
	}

	return result
}

// convertChatUserMessageToResponses converts a Chat user message to Responses format.
func convertChatUserMessageToResponses(m map[string]interface{}) responses.ResponseInputItemUnionParam {
	content, _ := m["content"].(string)

	return responses.ResponseInputItemUnionParam{
		OfMessage: &responses.EasyInputMessageParam{
			Type: responses.EasyInputMessageTypeMessage,
			Role: responses.EasyInputMessageRoleUser,
			Content: responses.EasyInputMessageContentUnionParam{
				OfString: param.NewOpt(content),
			},
		},
	}
}

// convertChatAssistantMessageToResponses converts a Chat assistant message to Responses format.
func convertChatAssistantMessageToResponses(m map[string]interface{}) responses.ResponseInputItemUnionParam {
	content, _ := m["content"].(string)

	return responses.ResponseInputItemUnionParam{
		OfMessage: &responses.EasyInputMessageParam{
			Type: responses.EasyInputMessageTypeMessage,
			Role: responses.EasyInputMessageRoleAssistant,
			Content: responses.EasyInputMessageContentUnionParam{
				OfString: param.NewOpt(content),
			},
		},
	}
}

// convertChatToolMessageToResponses converts a Chat tool message to Responses function_call_output format.
func convertChatToolMessageToResponses(m map[string]interface{}) responses.ResponseInputItemUnionParam {
	toolCallID, _ := m["tool_call_id"].(string)
	content, _ := m["content"].(string)

	return responses.ResponseInputItemUnionParam{
		OfFunctionCallOutput: &responses.ResponseInputItemFunctionCallOutputParam{
			CallID: toolCallID,
			Output: responses.ResponseInputItemFunctionCallOutputOutputUnionParam{
				OfString: param.NewOpt(content),
			},
		},
	}
}

// ConvertChatToolsToResponsesTools converts Chat Completion tools to Responses API tools.
func ConvertChatToolsToResponsesTools(tools []openai.ChatCompletionToolUnionParam) []responses.ToolUnionParam {
	if len(tools) == 0 {
		return nil
	}

	result := make([]responses.ToolUnionParam, 0, len(tools))

	for _, tool := range tools {
		fn := tool.GetFunction()
		if fn == nil {
			continue
		}

		// Convert parameters to map[string]interface{}
		var parameters map[string]interface{}
		if fn.Parameters != nil {
			if bytes, err := json.Marshal(fn.Parameters); err == nil {
				if err := json.Unmarshal(bytes, &parameters); err == nil {
					// Successfully converted parameters
				}
			}
		}

		functionTool := &responses.FunctionToolParam{
			Type:       "function",
			Name:       fn.Name,
			Parameters: parameters,
		}

		// Set description if present
		if fn.Description.Value != "" {
			functionTool.Description = param.NewOpt(fn.Description.Value)
		}

		result = append(result, responses.ToolUnionParam{
			OfFunction: functionTool,
		})
	}

	return result
}

// ConvertChatToolChoiceToResponsesToolChoice converts Chat Completion tool_choice to Responses API format.
func ConvertChatToolChoiceToResponsesToolChoice(choice *openai.ChatCompletionToolChoiceOptionUnionParam) responses.ResponseNewParamsToolChoiceUnion {
	// Handle OfAuto (auto, none, required modes)
	if choice.OfAuto.Value != "" {
		mode := choice.OfAuto.Value
		return responses.ResponseNewParamsToolChoiceUnion{
			OfToolChoiceMode: param.NewOpt(responses.ToolChoiceOptions(mode)),
		}
	}

	// Handle OfAllowedTools - default to auto
	if choice.OfAllowedTools != nil {
		return responses.ResponseNewParamsToolChoiceUnion{
			OfToolChoiceMode: param.NewOpt(responses.ToolChoiceOptions("auto")),
		}
	}

	// Handle specific function tool choice
	if choice.OfFunctionToolChoice != nil {
		fn := choice.OfFunctionToolChoice.Function
		return responses.ResponseNewParamsToolChoiceUnion{
			OfFunctionTool: &responses.ToolChoiceFunctionParam{
				Name: fn.Name,
			},
		}
	}

	// Handle OfCustomToolChoice - default to auto
	if choice.OfCustomToolChoice != nil {
		return responses.ResponseNewParamsToolChoiceUnion{
			OfToolChoiceMode: param.NewOpt(responses.ToolChoiceOptions("auto")),
		}
	}

	// Default to auto
	return responses.ResponseNewParamsToolChoiceUnion{
		OfToolChoiceMode: param.NewOpt(responses.ToolChoiceOptions("auto")),
	}
}
