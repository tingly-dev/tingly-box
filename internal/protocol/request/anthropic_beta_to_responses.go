package request

import (
	"encoding/json"

	"github.com/anthropics/anthropic-sdk-go"
	"github.com/openai/openai-go/v3/packages/param"
	"github.com/openai/openai-go/v3/responses"
	"github.com/openai/openai-go/v3/shared"
)

// ConvertAnthropicBetaToResponsesRequest converts Anthropic beta request to OpenAI Responses API format
// The Responses API has a different structure than Chat Completions
func ConvertAnthropicBetaToResponsesRequest(anthropicReq *anthropic.BetaMessageNewParams) *responses.ResponseNewParams {
	params := &responses.ResponseNewParams{}
	params.Model = shared.ResponsesModel(anthropicReq.Model)
	hasSystemCacheControl := false

	// Convert system messages to Instructions (system/developer role)
	if len(anthropicReq.System) > 0 {
		for _, block := range anthropicReq.System {
			hasSystemCacheControl = hasSystemCacheControl || !param.IsOmitted(block.CacheControl)
		}
		if !hasSystemCacheControl {
			params.Instructions = ParamOpt(ConvertBetaTextBlocksToString(anthropicReq.System))
		}
	}

	// Convert messages to Response API Input items
	// Build conversation as a list of input items
	var inputItems []responses.ResponseInputItemUnionParam
	if hasSystemCacheControl {
		content := make(responses.ResponseInputMessageContentListParam, 0, len(anthropicReq.System))
		for _, block := range anthropicReq.System {
			part := &responses.ResponseInputTextParam{Text: block.Text}
			if !param.IsOmitted(block.CacheControl) {
				part.PromptCacheBreakpoint = responses.NewResponseInputTextPromptCacheBreakpointParam()
			}
			content = append(content, responses.ResponseInputContentUnionParam{OfInputText: part})
		}
		inputItems = append(inputItems, responseMessageWithContent("system", content))
	}

	for _, msg := range anthropicReq.Messages {
		if string(msg.Role) == "user" {
			items := convertBetaUserMessageToResponsesInput(msg)
			inputItems = append(inputItems, items...)
		} else if string(msg.Role) == "assistant" {
			items := convertBetaAssistantMessageToResponsesInput(msg)
			inputItems = append(inputItems, items...)
		}
	}

	// Set input - always use list format so the field is well-typed.
	params.Input = responses.ResponseNewParamsInputUnion{
		OfInputItemList: inputItems,
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

		// Convert tool choice
		// for some providers (like `vllm`), they require tool choice like `auto` in general usage
		params.ToolChoice = ConvertAnthropicBetaToolChoiceToResponses(&anthropicReq.ToolChoice)
	}

	hasRepresentableCacheControl := hasSystemCacheControl || anthropicBetaMessagesHaveRepresentableCacheControl(anthropicReq.Messages)
	hasFallbackCacheControl := anthropicBetaToolsHaveCacheControl(anthropicReq.Tools) ||
		anthropicBetaMessagesHaveToolUseCacheControl(anthropicReq.Messages)
	hasAutomaticCacheControl := !param.IsOmitted(anthropicReq.CacheControl)
	if hasAutomaticCacheControl || hasRepresentableCacheControl || hasFallbackCacheControl {
		if hasAutomaticCacheControl {
			params.PromptCacheOptions.Mode = "implicit"
		} else {
			params.PromptCacheOptions.Mode = "explicit"
		}
		if !hasRepresentableCacheControl && hasFallbackCacheControl {
			applyFirstResponsesCacheBreakpoint(params)
		}
	}

	//// Convert stop sequences
	//if len(anthropicReq.StopSequences) > 0 {
	//	// Responses API uses Stop as a union type
	//	params.Sto = ParamOpt(anthropicReq.StopSequences)
	//}

	return params
}

// convertBetaUserMessageToResponsesInput converts Anthropic beta user message to Responses API input items
// Handles text content and tool_result blocks
func convertBetaUserMessageToResponsesInput(msg anthropic.BetaMessageParam) []responses.ResponseInputItemUnionParam {
	var items []responses.ResponseInputItemUnionParam

	var hasToolResult, hasImage, hasCacheControl bool
	for _, block := range msg.Content {
		if block.OfToolResult != nil {
			hasToolResult = true
		}
		if block.OfImage != nil {
			hasImage = true
		}
		if cacheControl := block.GetCacheControl(); cacheControl != nil {
			hasCacheControl = hasCacheControl || !param.IsOmitted(*cacheControl)
		}
	}

	if hasToolResult {
		// When there are tool_result blocks, we need to create separate items
		for _, block := range msg.Content {
			if block.OfToolResult != nil {
				// Convert tool_result to Responses API function call output
				output := responses.ResponseInputItemFunctionCallOutputOutputUnionParam{}
				content := convertBetaToolResultContent(block.OfToolResult.Content)
				if !param.IsOmitted(block.OfToolResult.CacheControl) {
					text := &responses.ResponseInputTextContentParam{
						Text:                  content,
						PromptCacheBreakpoint: responses.NewResponseInputTextContentPromptCacheBreakpointParam(),
					}
					output.OfResponseFunctionCallOutputItemArray = responses.ResponseFunctionCallOutputItemListParam{
						{OfInputText: text},
					}
				} else {
					output.OfString = ParamOpt(content)
				}
				outputItem := responses.ResponseInputItemFunctionCallOutputParam{
					CallID: block.OfToolResult.ToolUseID,
					Output: output,
					Status: "completed",
				}
				items = append(items, responses.ResponseInputItemUnionParam{
					OfFunctionCallOutput: &outputItem,
				})
			} else if block.OfText != nil && block.OfText.Text != "" {
				// Text content alongside tool results
				content := responses.EasyInputMessageContentUnionParam{OfString: ParamOpt(block.OfText.Text)}
				if !param.IsOmitted(block.OfText.CacheControl) {
					text := &responses.ResponseInputTextParam{
						Text:                  block.OfText.Text,
						PromptCacheBreakpoint: responses.NewResponseInputTextPromptCacheBreakpointParam(),
					}
					content = responses.EasyInputMessageContentUnionParam{
						OfInputItemContentList: responses.ResponseInputMessageContentListParam{
							{OfInputText: text},
						},
					}
				}
				messageItem := responses.EasyInputMessageParam{
					Type:    responses.EasyInputMessageTypeMessage,
					Role:    responses.EasyInputMessageRole("user"),
					Content: content,
				}
				items = append(items, responses.ResponseInputItemUnionParam{
					OfMessage: &messageItem,
				})
			}
		}
	} else if hasImage || hasCacheControl {
		// Multimodal user message: emit input_text + input_image content parts
		contentList := make(responses.ResponseInputMessageContentListParam, 0, len(msg.Content))
		for _, block := range msg.Content {
			switch {
			case block.OfText != nil:
				text := &responses.ResponseInputTextParam{Text: block.OfText.Text}
				if !param.IsOmitted(block.OfText.CacheControl) {
					text.PromptCacheBreakpoint = responses.NewResponseInputTextPromptCacheBreakpointParam()
				}
				contentList = append(contentList, responses.ResponseInputContentUnionParam{
					OfInputText: text,
				})
			case block.OfImage != nil:
				url := betaImageBlockToOpenAIURL(block.OfImage)
				if url == "" {
					continue
				}
				image := &responses.ResponseInputImageParam{ImageURL: ParamOpt(url)}
				if !param.IsOmitted(block.OfImage.CacheControl) {
					image.PromptCacheBreakpoint = responses.NewResponseInputImagePromptCacheBreakpointParam()
				}
				contentList = append(contentList, responses.ResponseInputContentUnionParam{OfInputImage: image})
			}
		}
		if len(contentList) > 0 {
			messageItem := responses.EasyInputMessageParam{
				Type: responses.EasyInputMessageTypeMessage,
				Role: responses.EasyInputMessageRole("user"),
				Content: responses.EasyInputMessageContentUnionParam{
					OfInputItemContentList: contentList,
				},
			}
			items = append(items, responses.ResponseInputItemUnionParam{
				OfMessage: &messageItem,
			})
		}
	} else {
		// Simple text-only user message
		contentStr := ConvertBetaContentBlocksToString(msg.Content)
		if contentStr != "" {
			messageItem := responses.EasyInputMessageParam{
				Type: responses.EasyInputMessageTypeMessage,
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
	var textBlocks []anthropic.BetaTextBlockParam

	// Process content blocks
	for _, block := range msg.Content {
		if block.OfText != nil {
			textContent += block.OfText.Text
			textBlocks = append(textBlocks, *block.OfText)
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
		content := responses.EasyInputMessageContentUnionParam{OfString: ParamOpt(textContent)}
		for _, block := range textBlocks {
			if !param.IsOmitted(block.CacheControl) {
				parts := make(responses.ResponseInputMessageContentListParam, 0, len(textBlocks))
				for _, textBlock := range textBlocks {
					part := &responses.ResponseInputTextParam{Text: textBlock.Text}
					if !param.IsOmitted(textBlock.CacheControl) {
						part.PromptCacheBreakpoint = responses.NewResponseInputTextPromptCacheBreakpointParam()
					}
					parts = append(parts, responses.ResponseInputContentUnionParam{OfInputText: part})
				}
				content = responses.EasyInputMessageContentUnionParam{OfInputItemContentList: parts}
				break
			}
		}
		messageItem := responses.EasyInputMessageParam{
			Type:    responses.EasyInputMessageTypeMessage,
			Role:    responses.EasyInputMessageRole("assistant"),
			Content: content,
		}
		items = append(items, responses.ResponseInputItemUnionParam{
			OfMessage: &messageItem,
		})
	}

	// An assistant message with no text and no tool_use blocks is empty — skip it.
	return items
}

func anthropicBetaMessagesHaveRepresentableCacheControl(messages []anthropic.BetaMessageParam) bool {
	for _, message := range messages {
		for _, block := range message.Content {
			cacheControl := block.GetCacheControl()
			if cacheControl == nil || param.IsOmitted(*cacheControl) {
				continue
			}
			if block.OfText != nil || block.OfImage != nil || block.OfToolResult != nil {
				return true
			}
		}
	}
	return false
}

func anthropicBetaMessagesHaveToolUseCacheControl(messages []anthropic.BetaMessageParam) bool {
	for _, message := range messages {
		for _, block := range message.Content {
			if block.OfToolUse != nil && !param.IsOmitted(block.OfToolUse.CacheControl) {
				return true
			}
		}
	}
	return false
}

func anthropicBetaToolsHaveCacheControl(tools []anthropic.BetaToolUnionParam) bool {
	for _, tool := range tools {
		cacheControl := tool.GetCacheControl()
		if cacheControl != nil && !param.IsOmitted(*cacheControl) {
			return true
		}
	}
	return false
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

// ConvertAnthropicBetaToolChoiceToResponses converts Anthropic beta tool_choice to Responses API format
func ConvertAnthropicBetaToolChoiceToResponses(tc *anthropic.BetaToolChoiceUnionParam) responses.ResponseNewParamsToolChoiceUnion {
	// Handle "auto" mode (model decides whether to call tools)
	if tc.OfAuto != nil {
		return responses.ResponseNewParamsToolChoiceUnion{
			OfToolChoiceMode: ParamOpt(responses.ToolChoiceOptions("auto")),
		}
	}

	// Handle "any" mode (required - force model to call at least one tool)
	if tc.OfAny != nil {
		return responses.ResponseNewParamsToolChoiceUnion{
			OfToolChoiceMode: ParamOpt(responses.ToolChoiceOptions("required")),
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
		OfToolChoiceMode: ParamOpt(responses.ToolChoiceOptions("auto")),
	}
}
