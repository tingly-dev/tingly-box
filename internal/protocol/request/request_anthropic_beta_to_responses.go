package request

import (
	"encoding/json"

	"github.com/anthropics/anthropic-sdk-go"
	"github.com/openai/openai-go/v3/responses"

	"github.com/tingly-dev/tingly-box/internal/typ"
)

// ConvertAnthropicBetaToResponsesRequestWithProvider converts Anthropic beta request to OpenAI Responses API format
// and applies provider-specific transformations
func ConvertAnthropicBetaToResponsesRequestWithProvider(
	anthropicReq *anthropic.BetaMessageNewParams,
	provider *typ.Provider,
	model string,
) responses.ResponseNewParams {
	responsesReq := ConvertAnthropicBetaToResponsesRequest(anthropicReq)
	responsesReq.Model = model
	return responsesReq
}

// ConvertAnthropicBetaToResponsesRequest converts Anthropic beta request to OpenAI Responses API format
// The Responses API has a different structure than Chat Completions
func ConvertAnthropicBetaToResponsesRequest(anthropicReq *anthropic.BetaMessageNewParams) responses.ResponseNewParams {
	params := responses.ResponseNewParams{}

	// Convert system messages to Instructions (system/developer role)
	if len(anthropicReq.System) > 0 {
		params.Instructions = ParamOpt(ConvertBetaTextBlocksToString(anthropicReq.System))
	}

	// Convert messages to Response API Input items
	// Build conversation as a list of input items
	var inputItems []responses.ResponseInputItemUnionParam

	for _, msg := range anthropicReq.Messages {
		if string(msg.Role) == "user" {
			items := convertBetaUserMessageToResponsesInput(msg)
			inputItems = append(inputItems, items...)
		} else if string(msg.Role) == "assistant" {
			items := convertBetaAssistantMessageToResponsesInput(msg)
			inputItems = append(inputItems, items...)
		}
	}

	// Set input - use list format if we have multiple items or complex content
	if len(inputItems) > 0 {
		params.Input = responses.ResponseNewParamsInputUnion{
			OfInputItemList: inputItems,
		}
	} else {
		// Fallback to simple string input
		params.Input = responses.ResponseNewParamsInputUnion{
			OfString: ParamOpt(""),
		}
	}

	// Convert MaxTokens to MaxOutputTokens
	if anthropicReq.MaxTokens > 0 {
		params.MaxOutputTokens = ParamOpt(anthropicReq.MaxTokens)
	}

	// Convert temperature
	if anthropicReq.Temperature.Valid() {
		params.Temperature = ParamOpt(anthropicReq.Temperature.Value)
	}

	// Convert top_p
	if anthropicReq.TopP.Valid() {
		params.TopP = ParamOpt(anthropicReq.TopP.Value)
	}

	// Convert tools from Anthropic format to Responses API format
	if len(anthropicReq.Tools) > 0 {
		params.Tools = ConvertAnthropicBetaToolsToResponses(anthropicReq.Tools)
	}

	// Convert tool choice
	if anthropicReq.ToolChoice.OfAuto != nil || anthropicReq.ToolChoice.OfTool != nil ||
		anthropicReq.ToolChoice.OfAny != nil {
		params.ToolChoice = ConvertAnthropicBetaToolChoiceToResponses(&anthropicReq.ToolChoice)
	}

	//// Convert stop sequences
	//if len(anthropicReq.StopSequences) > 0 {
	//	// Responses API uses Stop as a union type
	//	params.Sto = ParamOpt(anthropicReq.StopSequences)
	//}

	return params
}

// ConvertAnthropicBetaToResponsesRequestWithStreaming converts Anthropic beta request to OpenAI Responses API format
// for streaming requests
func ConvertAnthropicBetaToResponsesRequestWithStreaming(
	anthropicReq *anthropic.BetaMessageNewParams,
	provider *typ.Provider,
	model string,
) responses.ResponseNewParams {
	return ConvertAnthropicBetaToResponsesRequestWithProvider(anthropicReq, provider, model)
}

// convertBetaUserMessageToResponsesInput converts Anthropic beta user message to Responses API input items
// Handles text content and tool_result blocks
func convertBetaUserMessageToResponsesInput(msg anthropic.BetaMessageParam) []responses.ResponseInputItemUnionParam {
	var items []responses.ResponseInputItemUnionParam

	// Check for tool_result blocks
	var hasToolResult bool
	for _, block := range msg.Content {
		if block.OfToolResult != nil {
			hasToolResult = true
			break
		}
	}

	if hasToolResult {
		// When there are tool_result blocks, we need to create separate items
		for _, block := range msg.Content {
			if block.OfToolResult != nil {
				// Convert tool_result to Responses API function call output
				outputItem := responses.ResponseInputItemFunctionCallOutputParam{
					CallID: block.OfToolResult.ToolUseID,
					Output: responses.ResponseInputItemFunctionCallOutputOutputUnionParam{
						OfString: ParamOpt(convertBetaToolResultContent(block.OfToolResult.Content)),
					},
					Status: "completed",
				}
				items = append(items, responses.ResponseInputItemUnionParam{
					OfFunctionCallOutput: &outputItem,
				})
			} else if block.OfText != nil {
				// Text content alongside tool results
				messageItem := responses.EasyInputMessageParam{
					Role: responses.EasyInputMessageRole("user"),
					Content: responses.EasyInputMessageContentUnionParam{
						OfString: ParamOpt(block.OfText.Text),
					},
				}
				items = append(items, responses.ResponseInputItemUnionParam{
					OfMessage: &messageItem,
				})
			}
		}
	} else {
		// Simple text-only user message
		contentStr := ConvertBetaContentBlocksToString(msg.Content)
		if contentStr != "" {
			messageItem := responses.EasyInputMessageParam{
				Role: responses.EasyInputMessageRole("user"),
				Content: responses.EasyInputMessageContentUnionParam{
					OfString: ParamOpt(contentStr),
				},
			}
			items = append(items, responses.ResponseInputItemUnionParam{
				OfMessage: &messageItem,
			})
		}
	}

	return items
}

// convertBetaAssistantMessageToResponsesInput converts Anthropic beta assistant message to Responses API input items
// Handles text content, tool_use blocks, and thinking blocks
func convertBetaAssistantMessageToResponsesInput(msg anthropic.BetaMessageParam) []responses.ResponseInputItemUnionParam {
	var items []responses.ResponseInputItemUnionParam
	var textContent string

	// Process content blocks
	for _, block := range msg.Content {
		if block.OfText != nil {
			textContent += block.OfText.Text
		}
	}

	// First, handle tool_use blocks
	for _, block := range msg.Content {
		if block.OfToolUse != nil {
			// Convert tool_use to Responses API function call
			argsJSON, _ := json.Marshal(block.OfToolUse.Input)

			functionCall := responses.ResponseFunctionToolCallParam{
				CallID:    block.OfToolUse.ID,
				Name:      block.OfToolUse.Name,
				Arguments: string(argsJSON),
			}
			items = append(items, responses.ResponseInputItemUnionParam{
				OfFunctionCall: &functionCall,
			})
		}
	}

	// Add text content as a separate message if present
	if textContent != "" {
		messageItem := responses.EasyInputMessageParam{
			Role: responses.EasyInputMessageRole("assistant"),
			Content: responses.EasyInputMessageContentUnionParam{
				OfString: ParamOpt(textContent),
			},
		}
		items = append(items, responses.ResponseInputItemUnionParam{
			OfMessage: &messageItem,
		})
	}

	// If no items were created and we have no text, create an empty assistant message
	if len(items) == 0 {
		messageItem := responses.EasyInputMessageParam{
			Role: responses.EasyInputMessageRole("assistant"),
			Content: responses.EasyInputMessageContentUnionParam{
				OfString: ParamOpt(""),
			},
		}
		items = append(items, responses.ResponseInputItemUnionParam{
			OfMessage: &messageItem,
		})
	}

	return items
}

// ConvertAnthropicBetaToolsToResponses converts Anthropic beta tools to Responses API format
func ConvertAnthropicBetaToolsToResponses(tools []anthropic.BetaToolUnionParam) []responses.ToolUnionParam {
	if len(tools) == 0 {
		return nil
	}

	out := make([]responses.ToolUnionParam, 0, len(tools))

	for _, t := range tools {
		tool := t.OfTool
		if tool == nil {
			continue
		}

		// Convert Anthropic input schema to Responses API function parameters
		var parameters map[string]interface{}
		if tool.InputSchema.Properties != nil || len(tool.InputSchema.Required) > 0 {
			parameters = make(map[string]interface{})
			parameters["type"] = "object"

			if tool.InputSchema.Properties != nil {
				parameters["properties"] = tool.InputSchema.Properties
			}

			if len(tool.InputSchema.Required) > 0 {
				parameters["required"] = tool.InputSchema.Required
			}
		}

		// Create function tool
		fn := responses.FunctionToolParam{
			Name:        tool.Name,
			Description: ParamOpt(tool.Description.Value),
			Parameters:  parameters,
		}

		out = append(out, responses.ToolUnionParam{
			OfFunction: &fn,
		})
	}

	return out
}

// ConvertAnthropicBetaToolChoiceToResponses converts Anthropic beta tool_choice to Responses API format
func ConvertAnthropicBetaToolChoiceToResponses(tc *anthropic.BetaToolChoiceUnionParam) responses.ResponseNewParamsToolChoiceUnion {
	if tc.OfAuto != nil {
		return responses.ResponseNewParamsToolChoiceUnion{
			OfToolChoiceMode: ParamOpt(responses.ToolChoiceOptions("auto")),
		}
	}

	if tc.OfTool != nil {
		// Convert specific tool choice
		toolParam := responses.ToolChoiceFunctionParam{
			Name: tc.OfTool.Name,
		}
		return responses.ResponseNewParamsToolChoiceUnion{
			OfFunctionTool: &toolParam,
		}
	}

	// OfAny (Anthropic's "required") - map to auto as Responses API doesn't have direct equivalent
	if tc.OfAny != nil {
		return responses.ResponseNewParamsToolChoiceUnion{
			OfToolChoiceMode: ParamOpt(responses.ToolChoiceOptions("auto")),
		}
	}

	// Default to auto
	return responses.ResponseNewParamsToolChoiceUnion{
		OfToolChoiceMode: ParamOpt(responses.ToolChoiceOptions("auto")),
	}
}
