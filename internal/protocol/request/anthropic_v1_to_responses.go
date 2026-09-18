package request

import (
	"encoding/json"

	"github.com/anthropics/anthropic-sdk-go"
	"github.com/openai/openai-go/v3/packages/param"
	"github.com/openai/openai-go/v3/responses"
	"github.com/openai/openai-go/v3/shared"
)

// ConvertAnthropicV1ToResponsesRequest converts Anthropic v1 request to OpenAI Responses API format
// The Responses API has a different structure than Chat Completions
func ConvertAnthropicV1ToResponsesRequest(anthropicReq *anthropic.MessageNewParams) *responses.ResponseNewParams {
	params := &responses.ResponseNewParams{}
	params.Model = shared.ResponsesModel(anthropicReq.Model)
	hasSystemCacheControl := false

	// Convert system messages to Instructions (system role in v1)
	// In v1, system messages are passed via the System param
	if len(anthropicReq.System) > 0 {
		for _, block := range anthropicReq.System {
			hasSystemCacheControl = hasSystemCacheControl || !param.IsOmitted(block.CacheControl)
		}
		// Join system text blocks into a single instruction string
		var instructionsStr string
		for _, block := range anthropicReq.System {
			instructionsStr += block.Text
		}
		if instructionsStr != "" && !hasSystemCacheControl {
			params.Instructions = param.NewOpt(instructionsStr)
		}
	}

	// Convert messages to Input items (Responses API format)
	// Always set Input field, even if empty, as Responses API requires it
	var inputItems responses.ResponseInputParam
	if hasSystemCacheControl {
		inputItems = append(inputItems, responseMessageWithContent("system", responsesTextParts(anthropicReq.System)))
	}
	inputItems = append(inputItems, convertV1MessagesToResponsesInput(anthropicReq.Messages)...)
	params.Input = responses.ResponseNewParamsInputUnion{
		OfInputItemList: responses.ResponseInputParam(inputItems),
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

	// Affinity hint for the upstream prompt cache — Anthropic has no equivalent
	// field, so it is derived from metadata.user_id.
	params.PromptCacheKey = openAIPromptCacheKey(anthropicReq.Metadata.UserID.Or(""))

	hasRepresentableCacheControl := hasSystemCacheControl || anthropicV1MessagesHaveRepresentableCacheControl(anthropicReq.Messages)
	hasFallbackCacheControl := anthropicV1ToolsHaveCacheControl(anthropicReq.Tools) ||
		anthropicV1MessagesHaveToolUseCacheControl(anthropicReq.Messages)
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

// markFunctionCallOutputBreakpoint stamps a prompt-cache breakpoint on a
// function_call_output output item, whichever variant it holds.
func markFunctionCallOutputBreakpoint(item *responses.ResponseFunctionCallOutputItemUnionParam) {
	switch {
	case item.OfInputText != nil:
		item.OfInputText.PromptCacheBreakpoint = responses.NewResponseInputTextContentPromptCacheBreakpointParam()
	case item.OfInputImage != nil:
		item.OfInputImage.PromptCacheBreakpoint = responses.NewResponseInputImageContentPromptCacheBreakpointParam()
	}
}

// responsesFunctionCallOutputFromToolResult converts a tool_result block into
// a Responses function_call_output input item, always as the structured output
// item list. Shared by the v1 and beta converters — the beta path bridges
// through viewAnthropicBetaBlock first.
//
// The list shape is unconditional on purpose — see the cache-shape invariant
// documented in cache_control.go.
// A text-only result used to collapse to the compact output string whenever it
// carried no cache breakpoint, so the same tool result serialized one way while
// a breakpoint sat on it and another way once Claude Code rolled that
// breakpoint forward — re-writing conversation history the upstream cache had
// already been keyed on.
func responsesFunctionCallOutputFromToolResult(block *anthropic.ToolResultBlockParam) responses.ResponseInputItemUnionParam {
	output := responses.ResponseInputItemFunctionCallOutputOutputUnionParam{}
	hasCache := !param.IsOmitted(block.CacheControl)

	items := make(responses.ResponseFunctionCallOutputItemListParam, 0, len(block.Content))
	for _, c := range block.Content {
		switch {
		case c.OfText != nil:
			if c.OfText.Text == "" {
				continue
			}
			items = append(items, responses.ResponseFunctionCallOutputItemUnionParam{
				OfInputText: &responses.ResponseInputTextContentParam{Text: c.OfText.Text},
			})
		case c.OfImage != nil:
			if url := imageBlockToOpenAIURL(c.OfImage); url != "" {
				items = append(items, responses.ResponseFunctionCallOutputItemUnionParam{
					OfInputImage: &responses.ResponseInputImageContentParam{ImageURL: param.NewOpt(url)},
				})
			}
		}
	}
	if len(items) == 0 {
		output.OfString = param.NewOpt("")
	} else {
		if hasCache {
			markFunctionCallOutputBreakpoint(&items[len(items)-1])
		}
		output.OfResponseFunctionCallOutputItemArray = items
	}

	return responses.ResponseInputItemUnionParam{
		OfFunctionCallOutput: &responses.ResponseInputItemFunctionCallOutputParam{
			CallID: param.NewOpt(block.ToolUseID),
			Output: output,
			Status: "completed",
		},
	}
}

// convertV1UserMessageToResponsesInput converts Anthropic v1 user message to Responses API input items
func convertV1UserMessageToResponsesInput(msg anthropic.MessageParam) []responses.ResponseInputItemUnionParam {
	var items []responses.ResponseInputItemUnionParam

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
				items = append(items, responsesFunctionCallOutputFromToolResult(block.OfToolResult))
			} else if block.OfImage != nil {
				// Image content alongside tool results (issue #1606): forward
				// as a user message with an input_image part instead of
				// dropping it.
				if url := imageBlockToOpenAIURL(block.OfImage); url != "" {
					items = append(items, responseMessageWithContent("user", responses.ResponseInputMessageContentListParam{
						{OfInputImage: &responses.ResponseInputImageParam{ImageURL: param.NewOpt(url)}},
					}))
				}
			} else if block.OfText != nil && block.OfText.Text != "" {
				// Text content alongside tool results
				items = append(items, responseMessageWithContent("user",
					responses.ResponseInputMessageContentListParam{
						{OfInputText: responsesInputTextPart(block.OfText.Text, !param.IsOmitted(block.OfText.CacheControl))},
					}))
			}
		}
		return items
	}

	contentList := make(responses.ResponseInputMessageContentListParam, 0, len(msg.Content))
	for _, block := range msg.Content {
		switch {
		case block.OfText != nil:
			if block.OfText.Text == "" {
				continue
			}
			contentList = append(contentList, responses.ResponseInputContentUnionParam{
				OfInputText: responsesInputTextPart(block.OfText.Text, !param.IsOmitted(block.OfText.CacheControl)),
			})
		case block.OfImage != nil:
			url := imageBlockToOpenAIURL(block.OfImage)
			if url == "" {
				continue
			}
			image := &responses.ResponseInputImageParam{ImageURL: param.NewOpt(url)}
			if !param.IsOmitted(block.OfImage.CacheControl) {
				image.PromptCacheBreakpoint = responses.NewResponseInputImagePromptCacheBreakpointParam()
			}
			contentList = append(contentList, responses.ResponseInputContentUnionParam{OfInputImage: image})
		}
	}
	if len(contentList) > 0 {
		items = append(items, responseMessageWithContent("user", contentList))
	}

	return items
}

// convertV1AssistantMessageToResponsesInput converts Anthropic v1 assistant message to Responses API input items
func convertV1AssistantMessageToResponsesInput(msg anthropic.MessageParam) []responses.ResponseInputItemUnionParam {
	var items []responses.ResponseInputItemUnionParam
	var textBlocks []anthropic.TextBlockParam

	// Process content blocks to collect text and find tool_use blocks
	for _, block := range msg.Content {
		if block.OfText != nil {
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
	if parts := responsesTextParts(textBlocks); len(parts) > 0 {
		items = append(items, responseMessageWithContent("assistant", parts))
	}

	// An assistant message with no text and no tool_use blocks is empty — skip it.
	return items
}

func anthropicV1MessagesHaveRepresentableCacheControl(messages []anthropic.MessageParam) bool {
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

func anthropicV1MessagesHaveToolUseCacheControl(messages []anthropic.MessageParam) bool {
	for _, message := range messages {
		for _, block := range message.Content {
			if block.OfToolUse != nil && !param.IsOmitted(block.OfToolUse.CacheControl) {
				return true
			}
		}
	}
	return false
}

func anthropicV1ToolsHaveCacheControl(tools []anthropic.ToolUnionParam) bool {
	for _, tool := range tools {
		cacheControl := tool.GetCacheControl()
		if cacheControl != nil && !param.IsOmitted(*cacheControl) {
			return true
		}
	}
	return false
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
