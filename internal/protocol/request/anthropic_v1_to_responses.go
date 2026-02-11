package request

import (
	"encoding/json"

	"github.com/anthropics/anthropic-sdk-go"
	"github.com/openai/openai-go/v3/packages/param"
	"github.com/openai/openai-go/v3/responses"
	"github.com/tingly-dev/tingly-box/internal/typ"
)

// ConvertAnthropicV1ToResponsesRequestWithProvider converts Anthropic v1 request to OpenAI Responses API format
// and applies provider-specific transformations
func ConvertAnthropicV1ToResponsesRequestWithProvider(
	anthropicReq *anthropic.MessageNewParams,
	provider *typ.Provider,
	model string,
) responses.ResponseNewParams {
	responsesReq := ConvertAnthropicV1ToResponsesRequest(anthropicReq)
	responsesReq.Model = model
	return responsesReq
}

// ConvertAnthropicV1ToResponsesRequest converts Anthropic v1 request to OpenAI Responses API format
// The Responses API has a different structure than Chat Completions
func ConvertAnthropicV1ToResponsesRequest(anthropicReq *anthropic.MessageNewParams) responses.ResponseNewParams {
	params := responses.ResponseNewParams{}

	// Convert system messages to Instructions (system role in v1)
	// In v1, system messages are passed via the System param
	if len(anthropicReq.System) > 0 {
		// Join system text blocks into a single instruction string
		var instructionsStr string
		for _, block := range anthropicReq.System {
			instructionsStr += block.Text
		}
		if instructionsStr != "" {
			params.Instructions = param.NewOpt(instructionsStr)
		}
	}

	// Convert messages to Input items (Responses API format)
	if len(anthropicReq.Messages) > 0 {
		inputItems := convertV1MessagesToResponsesInput(anthropicReq.Messages)
		if len(inputItems) > 0 {
			params.Input = responses.ResponseNewParamsInputUnion{
				OfInputItemList: responses.ResponseInputParam(inputItems),
			}
		}
	}

	// Convert max_tokens to max_output_tokens
	if anthropicReq.MaxTokens > 0 {
		params.MaxOutputTokens = param.NewOpt(anthropicReq.MaxTokens)
	}

	// Copy temperature
	if anthropicReq.Temperature.Value > 0 {
		params.Temperature = param.NewOpt(anthropicReq.Temperature.Value)
	}

	// Copy top_p
	if anthropicReq.TopP.Value > 0 {
		params.TopP = param.NewOpt(anthropicReq.TopP.Value)
	}

	// Convert tools
	if len(anthropicReq.Tools) > 0 {
		params.Tools = ConvertAnthropicV1ToolsToResponses(anthropicReq.Tools)

		// Convert tool choice
		// for some providers (like `vllm`), they require tool choice like `auto` in general usage
		params.ToolChoice = ConvertAnthropicV1ToolChoiceToResponses(&anthropicReq.ToolChoice)
	}

	return params
}

// convertV1MessagesToResponsesInput converts Anthropic v1 messages to Responses API input items
func convertV1MessagesToResponsesInput(messages []anthropic.MessageParam) responses.ResponseInputParam {
	var inputItems responses.ResponseInputParam

	for _, msg := range messages {
		if string(msg.Role) == "user" {
			items := convertV1UserMessageToResponsesInput(msg)
			inputItems = append(inputItems, items...)
		} else if string(msg.Role) == "assistant" {
			items := convertV1AssistantMessageToResponsesInput(msg)
			inputItems = append(inputItems, items...)
		}
	}

	return inputItems
}

// convertV1UserMessageToResponsesInput converts Anthropic v1 user message to Responses API input items
func convertV1UserMessageToResponsesInput(msg anthropic.MessageParam) []responses.ResponseInputItemUnionParam {
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
				// Convert tool_result to function_call_output
				outputItem := responses.ResponseInputItemFunctionCallOutputParam{
					CallID: block.OfToolResult.ToolUseID,
					Output: responses.ResponseInputItemFunctionCallOutputOutputUnionParam{
						OfString: param.NewOpt(convertV1ToolResultContentToString(block.OfToolResult.Content)),
					},
					Status: "completed",
				}
				items = append(items, responses.ResponseInputItemUnionParam{
					OfFunctionCallOutput: &outputItem,
				})
			} else if block.OfText != nil {
				// Text content alongside tool results
				messageItem := responses.EasyInputMessageParam{
					Type: responses.EasyInputMessageTypeMessage,
					Role: responses.EasyInputMessageRole("user"),
					Content: responses.EasyInputMessageContentUnionParam{
						OfString: param.NewOpt(block.OfText.Text),
					},
				}
				items = append(items, responses.ResponseInputItemUnionParam{
					OfMessage: &messageItem,
				})
			}
		}
		return items
	}

	// Simple text-only user message
	contentStr := convertV1ContentBlocksToString(msg.Content)
	if contentStr != "" {
		messageItem := responses.EasyInputMessageParam{
			Type: responses.EasyInputMessageTypeMessage,
			Role: responses.EasyInputMessageRole("user"),
			Content: responses.EasyInputMessageContentUnionParam{
				OfString: param.NewOpt(contentStr),
			},
		}
		items = append(items, responses.ResponseInputItemUnionParam{
			OfMessage: &messageItem,
		})
	}

	return items
}

// convertV1AssistantMessageToResponsesInput converts Anthropic v1 assistant message to Responses API input items
func convertV1AssistantMessageToResponsesInput(msg anthropic.MessageParam) []responses.ResponseInputItemUnionParam {
	var items []responses.ResponseInputItemUnionParam
	var textContent string

	// Process content blocks to collect text and find tool_use blocks
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
			Type: responses.EasyInputMessageTypeMessage,
			Role: responses.EasyInputMessageRole("assistant"),
			Content: responses.EasyInputMessageContentUnionParam{
				OfString: param.NewOpt(textContent),
			},
		}
		items = append(items, responses.ResponseInputItemUnionParam{
			OfMessage: &messageItem,
		})
	}

	// If no items were created, create an empty assistant message
	if len(items) == 0 {
		messageItem := responses.EasyInputMessageParam{
			Type: responses.EasyInputMessageTypeMessage,
			Role: responses.EasyInputMessageRole("assistant"),
			Content: responses.EasyInputMessageContentUnionParam{
				OfString: param.NewOpt(""),
			},
		}
		items = append(items, responses.ResponseInputItemUnionParam{
			OfMessage: &messageItem,
		})
	}

	return items
}

// convertV1ContentBlocksToString converts v1 content blocks to string
func convertV1ContentBlocksToString(blocks []anthropic.ContentBlockParamUnion) string {
	var result string
	for _, block := range blocks {
		if block.OfText != nil {
			result += block.OfText.Text
		}
	}
	return result
}

// convertV1ToolResultContentToString converts tool result content to string
func convertV1ToolResultContentToString(content []anthropic.ToolResultBlockParamContentUnion) string {
	var result string
	for _, c := range content {
		if c.OfText != nil {
			result += c.OfText.Text
		}
	}
	return result
}

// ConvertAnthropicV1ToolsToResponses converts Anthropic v1 tools to Responses API format
func ConvertAnthropicV1ToolsToResponses(tools []anthropic.ToolUnionParam) []responses.ToolUnionParam {
	if len(tools) == 0 {
		return nil
	}

	out := make([]responses.ToolUnionParam, 0, len(tools))

	for _, t := range tools {
		tool := t.OfTool
		if tool == nil {
			continue
		}

		// Convert Anthropic input schema to OpenAI function parameters
		// Always initialize parameters to avoid omitting the field (omitzero tag)
		parameters := make(map[string]interface{})
		parameters["type"] = "object"

		if tool.InputSchema.Properties != nil {
			parameters["properties"] = tool.InputSchema.Properties
		} else {
			// Initialize empty properties if none provided
			parameters["properties"] = make(map[string]interface{})
		}

		if len(tool.InputSchema.Required) > 0 {
			parameters["required"] = tool.InputSchema.Required
		}

		// Create function tool
		fn := &responses.FunctionToolParam{
			Name:        tool.Name,
			Description: ParamOpt(tool.Description.Value),
			Parameters:  parameters,
			Type:        "function",
		}

		out = append(out, responses.ToolUnionParam{
			OfFunction: fn,
		})
	}

	return out
}

// ConvertAnthropicV1ToolChoiceToResponses converts Anthropic v1 tool_choice to Responses API format
func ConvertAnthropicV1ToolChoiceToResponses(tc *anthropic.ToolChoiceUnionParam) responses.ResponseNewParamsToolChoiceUnion {
	// Handle "auto" mode (model decides whether to call tools)
	if tc.OfAuto != nil {
		return responses.ResponseNewParamsToolChoiceUnion{
			OfToolChoiceMode: param.NewOpt(responses.ToolChoiceOptions("auto")),
		}
	}

	// Handle "any" mode (required - force model to call at least one tool)
	if tc.OfAny != nil {
		return responses.ResponseNewParamsToolChoiceUnion{
			OfToolChoiceMode: param.NewOpt(responses.ToolChoiceOptions("required")),
		}
	}

	// Handle specific tool choice
	if tc.OfTool != nil {
		toolParam := responses.ToolChoiceFunctionParam{
			Name: tc.OfTool.Name,
		}
		return responses.ResponseNewParamsToolChoiceUnion{
			OfFunctionTool: &toolParam,
		}
	}

	// Default to auto
	return responses.ResponseNewParamsToolChoiceUnion{
		OfToolChoiceMode: param.NewOpt(responses.ToolChoiceOptions("auto")),
	}
}
