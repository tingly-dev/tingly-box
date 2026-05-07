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
		switch {
		case !param.IsOmitted(msg.OfSystem):
			// Extract system message content
			if sysMsg := msg.OfSystem; !param.IsOmitted(sysMsg.Content.OfString) && sysMsg.Content.OfString.Value != "" {
				systemParts = append(systemParts, sysMsg.Content.OfString.Value)
			}

		default:
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
		switch {
		case !param.IsOmitted(msg.OfUser):
			result = append(result, convertChatUserMessageToResponses(msg.OfUser))

		case !param.IsOmitted(msg.OfAssistant):
			assistantMsg := msg.OfAssistant
			// Check if assistant has tool calls
			if len(assistantMsg.ToolCalls) > 0 {
				// Convert each tool call to function_call item
				for _, tc := range assistantMsg.ToolCalls {
					if !param.IsOmitted(tc.OfFunction) {
						fnCall := tc.OfFunction
						result = append(result, responses.ResponseInputItemUnionParam{
							OfFunctionCall: &responses.ResponseFunctionToolCallParam{
								CallID:    fnCall.ID,
								Name:      fnCall.Function.Name,
								Arguments: fnCall.Function.Arguments,
							},
						})
					}
				}
			} else {
				// Regular assistant message
				result = append(result, convertChatAssistantMessageToResponses(assistantMsg))
			}

		case !param.IsOmitted(msg.OfTool):
			result = append(result, convertChatToolMessageToResponses(msg.OfTool))
		}
	}

	return result
}

// convertChatUserMessageToResponses converts a Chat user message to Responses format.
func convertChatUserMessageToResponses(userMsg *openai.ChatCompletionUserMessageParam) responses.ResponseInputItemUnionParam {
	if userMsg.Content.OfString.Valid() {
		return responses.ResponseInputItemUnionParam{
			OfMessage: &responses.EasyInputMessageParam{
				Type: responses.EasyInputMessageTypeMessage,
				Role: responses.EasyInputMessageRole("user"),
				Content: responses.EasyInputMessageContentUnionParam{
					OfString: param.NewOpt(userMsg.Content.OfString.Value),
				},
			},
		}
	}

	// Multipart content: forward text + image_url parts as input_text + input_image.
	if len(userMsg.Content.OfArrayOfContentParts) > 0 {
		contentList := make(responses.ResponseInputMessageContentListParam, 0, len(userMsg.Content.OfArrayOfContentParts))
		for _, part := range userMsg.Content.OfArrayOfContentParts {
			switch {
			case part.OfText != nil:
				contentList = append(contentList, responses.ResponseInputContentUnionParam{
					OfInputText: &responses.ResponseInputTextParam{Text: part.OfText.Text},
				})
			case part.OfImageURL != nil:
				url := part.OfImageURL.ImageURL.URL
				if url == "" {
					continue
				}
				contentList = append(contentList, responses.ResponseInputContentUnionParam{
					OfInputImage: &responses.ResponseInputImageParam{
						ImageURL: param.NewOpt(url),
					},
				})
			}
		}
		if len(contentList) > 0 {
			return responses.ResponseInputItemUnionParam{
				OfMessage: &responses.EasyInputMessageParam{
					Type: responses.EasyInputMessageTypeMessage,
					Role: responses.EasyInputMessageRole("user"),
					Content: responses.EasyInputMessageContentUnionParam{
						OfInputItemContentList: contentList,
					},
				},
			}
		}
	}

	// Fall back to an empty user message to preserve prior behavior.
	return responses.ResponseInputItemUnionParam{
		OfMessage: &responses.EasyInputMessageParam{
			Type: responses.EasyInputMessageTypeMessage,
			Role: responses.EasyInputMessageRole("user"),
			Content: responses.EasyInputMessageContentUnionParam{
				OfString: param.NewOpt(""),
			},
		},
	}
}

// convertChatAssistantMessageToResponses converts a Chat assistant message to Responses format.
func convertChatAssistantMessageToResponses(assistantMsg *openai.ChatCompletionAssistantMessageParam) responses.ResponseInputItemUnionParam {
	content := ""
	if !param.IsOmitted(assistantMsg.Content.OfString) && assistantMsg.Content.OfString.Value != "" {
		content = assistantMsg.Content.OfString.Value
	}

	return responses.ResponseInputItemUnionParam{
		OfMessage: &responses.EasyInputMessageParam{
			Type: responses.EasyInputMessageTypeMessage,
			Role: responses.EasyInputMessageRole("assistant"),
			Content: responses.EasyInputMessageContentUnionParam{
				OfString: param.NewOpt(content),
			},
		},
	}
}

// convertChatToolMessageToResponses converts a Chat tool message to Responses function_call_output format.
func convertChatToolMessageToResponses(toolMsg *openai.ChatCompletionToolMessageParam) responses.ResponseInputItemUnionParam {
	content := ""
	if !param.IsOmitted(toolMsg.Content.OfString) && toolMsg.Content.OfString.Value != "" {
		content = toolMsg.Content.OfString.Value
	}

	return responses.ResponseInputItemUnionParam{
		OfFunctionCallOutput: &responses.ResponseInputItemFunctionCallOutputParam{
			CallID: toolMsg.ToolCallID,
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
		if !param.IsOmitted(fn.Description) && fn.Description.Value != "" {
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
	if !param.IsOmitted(choice.OfAuto) && choice.OfAuto.Value != "" {
		mode := choice.OfAuto.Value
		return responses.ResponseNewParamsToolChoiceUnion{
			OfToolChoiceMode: param.NewOpt(responses.ToolChoiceOptions(mode)),
		}
	}

	// Handle OfAllowedTools - default to auto
	if !param.IsOmitted(choice.OfAllowedTools) {
		return responses.ResponseNewParamsToolChoiceUnion{
			OfToolChoiceMode: param.NewOpt(responses.ToolChoiceOptions("auto")),
		}
	}

	// Handle specific function tool choice
	if !param.IsOmitted(choice.OfFunctionToolChoice) {
		fn := choice.OfFunctionToolChoice.Function
		return responses.ResponseNewParamsToolChoiceUnion{
			OfFunctionTool: &responses.ToolChoiceFunctionParam{
				Name: fn.Name,
			},
		}
	}

	// Handle OfCustomToolChoice - default to auto
	if !param.IsOmitted(choice.OfCustomToolChoice) {
		return responses.ResponseNewParamsToolChoiceUnion{
			OfToolChoiceMode: param.NewOpt(responses.ToolChoiceOptions("auto")),
		}
	}

	// Default to auto
	return responses.ResponseNewParamsToolChoiceUnion{
		OfToolChoiceMode: param.NewOpt(responses.ToolChoiceOptions("auto")),
	}
}
