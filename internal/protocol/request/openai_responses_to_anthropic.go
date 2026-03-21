package request

import (
	"encoding/json"

	"github.com/anthropics/anthropic-sdk-go"
	"github.com/openai/openai-go/v3/packages/param"
	"github.com/openai/openai-go/v3/responses"
	"github.com/sirupsen/logrus"
)

// ConvertOpenAIResponsesToAnthropicRequest converts OpenAI Responses API params to Anthropic v1 Message API format
//
// Note: The following Responses API fields are not supported in Anthropic's Message API:
//   - MaxToolCalls: Anthropic doesn't have an equivalent limit
//   - ParallelToolCalls: Anthropic supports parallel tool calls by default
//   - Include (web_search sources, code interpreter outputs, etc.): Not supported
//   - PreviousResponseID: Requires conversation state management
//   - TopLogprobs: Anthropic Message API doesn't support logprobs
//   - Reasoning tokens: Special handling required for o1/o3 models
func ConvertOpenAIResponsesToAnthropicRequest(
	params responses.ResponseNewParams,
	defaultMaxTokens int64,
) anthropic.MessageNewParams {
	anthropicParams := anthropic.MessageNewParams{}

	// Convert model
	if params.Model != "" {
		anthropicParams.Model = anthropic.Model(params.Model)
	}

	// Convert instructions to system messages
	if !param.IsOmitted(params.Instructions) {
		systemStr := params.Instructions.Value
		if systemStr != "" {
			anthropicParams.System = []anthropic.TextBlockParam{
				{Text: systemStr},
			}
		}
	}

	// Convert input to messages
	if !param.IsOmitted(params.Input.OfInputItemList) {
		messages := convertResponsesInputToAnthropicMessages(params.Input.OfInputItemList)
		if len(messages) > 0 {
			anthropicParams.Messages = messages
		}
	} else if !param.IsOmitted(params.Input.OfString) {
		// Simple string input - create a single user message
		inputStr := params.Input.OfString.Value
		if inputStr != "" {
			anthropicParams.Messages = []anthropic.MessageParam{
				anthropic.NewUserMessage(anthropic.NewTextBlock(inputStr)),
			}
		}
	}

	// Convert max_output_tokens to max_tokens
	if !param.IsOmitted(params.MaxOutputTokens) {
		anthropicParams.MaxTokens = params.MaxOutputTokens.Value
	} else {
		anthropicParams.MaxTokens = defaultMaxTokens
	}

	// Convert temperature
	if !param.IsOmitted(params.Temperature) {
		anthropicParams.Temperature = AnthropicParamOpt(params.Temperature.Value)
	}

	// Convert top_p
	if !param.IsOmitted(params.TopP) {
		anthropicParams.TopP = AnthropicParamOpt(params.TopP.Value)
	}

	// Convert tools
	if !param.IsOmitted(params.Tools) && len(params.Tools) > 0 {
		anthropicParams.Tools = ConvertResponsesToolsToAnthropic(params.Tools)
		// Convert tool choice
		if !param.IsOmitted(params.ToolChoice) {
			anthropicParams.ToolChoice = ConvertResponsesToolChoiceToAnthropic(params.ToolChoice)
		}
	}

	return anthropicParams
}

// ConvertOpenAIResponsesToAnthropicBetaRequest converts OpenAI Responses API params to Anthropic Beta Message API format
func ConvertOpenAIResponsesToAnthropicBetaRequest(
	params responses.ResponseNewParams,
	defaultMaxTokens int64,
) anthropic.BetaMessageNewParams {
	anthropicParams := anthropic.BetaMessageNewParams{}

	// Convert model
	if params.Model != "" {
		anthropicParams.Model = anthropic.Model(params.Model)
	}

	// Convert instructions to system messages
	if !param.IsOmitted(params.Instructions) {
		systemStr := params.Instructions.Value
		if systemStr != "" {
			anthropicParams.System = []anthropic.BetaTextBlockParam{
				{Text: systemStr},
			}
		}
	}

	// Convert input to messages
	if !param.IsOmitted(params.Input.OfInputItemList) {
		messages := convertResponsesInputToAnthropicBetaMessages(params.Input.OfInputItemList)
		if len(messages) > 0 {
			anthropicParams.Messages = messages
		}
	} else if !param.IsOmitted(params.Input.OfString) {
		// Simple string input - create a single user message
		inputStr := params.Input.OfString.Value
		if inputStr != "" {
			anthropicParams.Messages = []anthropic.BetaMessageParam{
				anthropic.NewBetaUserMessage(anthropic.NewBetaTextBlock(inputStr)),
			}
		}
	}

	// Convert max_output_tokens to max_tokens
	if !param.IsOmitted(params.MaxOutputTokens) {
		anthropicParams.MaxTokens = params.MaxOutputTokens.Value
	} else {
		anthropicParams.MaxTokens = defaultMaxTokens
	}

	// Convert temperature
	if !param.IsOmitted(params.Temperature) {
		anthropicParams.Temperature = AnthropicParamOpt(params.Temperature.Value)
	}

	// Convert top_p
	if !param.IsOmitted(params.TopP) {
		anthropicParams.TopP = AnthropicParamOpt(params.TopP.Value)
	}

	// Convert tools
	if !param.IsOmitted(params.Tools) && len(params.Tools) > 0 {
		anthropicParams.Tools = ConvertResponsesToolsToAnthropicBeta(params.Tools)
		// Convert tool choice
		if !param.IsOmitted(params.ToolChoice) {
			anthropicParams.ToolChoice = ConvertResponsesToolChoiceToAnthropicBeta(params.ToolChoice)
		}
	}

	return anthropicParams
}

// convertResponsesInputToAnthropicMessages converts Responses API input items to Anthropic v1 messages
func convertResponsesInputToAnthropicMessages(inputItems responses.ResponseInputParam) []anthropic.MessageParam {
	var messages []anthropic.MessageParam

	for _, item := range inputItems {
		// Handle message items
		if !param.IsOmitted(item.OfMessage) {
			msg := item.OfMessage
			if string(msg.Role) == "user" {
				messages = append(messages, convertResponsesUserMessageToAnthropic(msg))
			} else if string(msg.Role) == "assistant" {
				messages = append(messages, convertResponsesAssistantMessageToAnthropic(msg))
			}
			continue
		}

		// Handle function call items (tool_use)
		if !param.IsOmitted(item.OfFunctionCall) {
			messages = append(messages, convertResponsesFunctionCallToAnthropic(item.OfFunctionCall))
			continue
		}

		// Handle function call output items (tool_result)
		if !param.IsOmitted(item.OfFunctionCallOutput) {
			messages = append(messages, convertResponsesFunctionCallOutputToAnthropic(item.OfFunctionCallOutput))
			continue
		}
	}

	return messages
}

// convertResponsesInputToAnthropicBetaMessages converts Responses API input items to Anthropic Beta messages
func convertResponsesInputToAnthropicBetaMessages(inputItems responses.ResponseInputParam) []anthropic.BetaMessageParam {
	var messages []anthropic.BetaMessageParam

	for _, item := range inputItems {
		// Handle message items
		if !param.IsOmitted(item.OfMessage) {
			msg := item.OfMessage
			if string(msg.Role) == "user" {
				messages = append(messages, convertResponsesUserMessageToAnthropicBeta(msg))
			} else if string(msg.Role) == "assistant" {
				messages = append(messages, convertResponsesAssistantMessageToAnthropicBeta(msg))
			}
			continue
		}

		// Handle function call items (tool_use)
		if !param.IsOmitted(item.OfFunctionCall) {
			messages = append(messages, convertResponsesFunctionCallToAnthropicBeta(item.OfFunctionCall))
			continue
		}

		// Handle function call output items (tool_result)
		if !param.IsOmitted(item.OfFunctionCallOutput) {
			messages = append(messages, convertResponsesFunctionCallOutputToAnthropicBeta(item.OfFunctionCallOutput))
			continue
		}
	}

	return messages
}

// convertResponsesUserMessageToAnthropic converts Responses API user message to Anthropic v1 format
func convertResponsesUserMessageToAnthropic(msg *responses.EasyInputMessageParam) anthropic.MessageParam {
	// Check for simple string content
	if !param.IsOmitted(msg.Content.OfString) {
		return anthropic.NewUserMessage(anthropic.NewTextBlock(msg.Content.OfString.Value))
	}

	// Handle array content
	if !param.IsOmitted(msg.Content.OfInputItemContentList) {
		var blocks []anthropic.ContentBlockParamUnion
		for _, contentItem := range msg.Content.OfInputItemContentList {
			if !param.IsOmitted(contentItem.OfInputText) {
				blocks = append(blocks, anthropic.NewTextBlock(contentItem.OfInputText.Text))
			} else {
				// Log unsupported content types (images, audio, etc.)
				logrus.Warnf("Unsupported content type in Responses API user message, skipping. Content types available: %v", contentItem)
			}
		}
		if len(blocks) > 0 {
			return anthropic.NewUserMessage(blocks...)
		}
	}

	// Default empty user message
	return anthropic.NewUserMessage(anthropic.NewTextBlock(""))
}

// convertResponsesAssistantMessageToAnthropic converts Responses API assistant message to Anthropic v1 format
func convertResponsesAssistantMessageToAnthropic(msg *responses.EasyInputMessageParam) anthropic.MessageParam {
	// Check for simple string content
	if !param.IsOmitted(msg.Content.OfString) {
		return anthropic.NewAssistantMessage(anthropic.NewTextBlock(msg.Content.OfString.Value))
	}

	// Handle array content
	if !param.IsOmitted(msg.Content.OfInputItemContentList) {
		var blocks []anthropic.ContentBlockParamUnion
		for _, contentItem := range msg.Content.OfInputItemContentList {
			if !param.IsOmitted(contentItem.OfInputText) {
				blocks = append(blocks, anthropic.NewTextBlock(contentItem.OfInputText.Text))
			} else {
				// Log unsupported content types
				logrus.Warnf("Unsupported content type in Responses API assistant message, skipping. Content types available: %v", contentItem)
			}
		}
		if len(blocks) > 0 {
			return anthropic.NewAssistantMessage(blocks...)
		}
	}

	// Default empty assistant message
	return anthropic.NewAssistantMessage(anthropic.NewTextBlock(""))
}

// convertResponsesFunctionCallToAnthropic converts Responses API function_call to Anthropic tool_use block
func convertResponsesFunctionCallToAnthropic(call *responses.ResponseFunctionToolCallParam) anthropic.MessageParam {
	// Parse arguments JSON
	var argsInput interface{}
	if call.Arguments != "" {
		if err := json.Unmarshal([]byte(call.Arguments), &argsInput); err != nil {
			logrus.Warnf("Failed to parse function call arguments JSON for tool %s: %v", call.Name, err)
			// Set to empty map to avoid nil issues
			argsInput = map[string]interface{}{}
		}
	}

	// Create assistant message with tool_use block
	return anthropic.NewAssistantMessage(
		anthropic.NewToolUseBlock(call.CallID, argsInput, call.Name),
	)
}

// convertResponsesFunctionCallOutputToAnthropic converts Responses API function_call_output to Anthropic tool_result block
func convertResponsesFunctionCallOutputToAnthropic(output *responses.ResponseInputItemFunctionCallOutputParam) anthropic.MessageParam {
	// Extract output content
	var outputStr string
	if !param.IsOmitted(output.Output.OfString) {
		outputStr = output.Output.OfString.Value
	}

	// Create user message with tool_result block
	return anthropic.NewUserMessage(
		anthropic.NewToolResultBlock(output.CallID, outputStr, false),
	)
}

// Beta versions of the message conversion functions

// convertResponsesUserMessageToAnthropicBeta converts Responses API user message to Anthropic Beta format
func convertResponsesUserMessageToAnthropicBeta(msg *responses.EasyInputMessageParam) anthropic.BetaMessageParam {
	// Check for simple string content
	if !param.IsOmitted(msg.Content.OfString) {
		return anthropic.NewBetaUserMessage(anthropic.NewBetaTextBlock(msg.Content.OfString.Value))
	}

	// Handle array content
	if !param.IsOmitted(msg.Content.OfInputItemContentList) {
		var blocks []anthropic.BetaContentBlockParamUnion
		for _, contentItem := range msg.Content.OfInputItemContentList {
			if !param.IsOmitted(contentItem.OfInputText) {
				blocks = append(blocks, anthropic.NewBetaTextBlock(contentItem.OfInputText.Text))
			} else {
				// Log unsupported content types
				logrus.Warnf("Unsupported content type in Responses API beta user message, skipping. Content types available: %v", contentItem)
			}
		}
		if len(blocks) > 0 {
			return anthropic.NewBetaUserMessage(blocks...)
		}
	}

	// Default empty user message
	return anthropic.NewBetaUserMessage(anthropic.NewBetaTextBlock(""))
}

// convertResponsesAssistantMessageToAnthropicBeta converts Responses API assistant message to Anthropic Beta format
func convertResponsesAssistantMessageToAnthropicBeta(msg *responses.EasyInputMessageParam) anthropic.BetaMessageParam {
	// Check for simple string content
	if !param.IsOmitted(msg.Content.OfString) {
		return anthropic.BetaMessageParam{
			Role:    anthropic.BetaMessageParamRoleAssistant,
			Content: []anthropic.BetaContentBlockParamUnion{anthropic.NewBetaTextBlock(msg.Content.OfString.Value)},
		}
	}

	// Handle array content
	if !param.IsOmitted(msg.Content.OfInputItemContentList) {
		var blocks []anthropic.BetaContentBlockParamUnion
		for _, contentItem := range msg.Content.OfInputItemContentList {
			if !param.IsOmitted(contentItem.OfInputText) {
				blocks = append(blocks, anthropic.NewBetaTextBlock(contentItem.OfInputText.Text))
			} else {
				// Log unsupported content types
				logrus.Warnf("Unsupported content type in Responses API beta user message, skipping. Content types available: %v", contentItem)
			}
		}
		if len(blocks) > 0 {
			return anthropic.BetaMessageParam{
				Role:    anthropic.BetaMessageParamRoleAssistant,
				Content: blocks,
			}
		}
	}

	// Default empty assistant message
	return anthropic.BetaMessageParam{
		Role:    anthropic.BetaMessageParamRoleAssistant,
		Content: []anthropic.BetaContentBlockParamUnion{anthropic.NewBetaTextBlock("")},
	}
}

// convertResponsesFunctionCallToAnthropicBeta converts Responses API function_call to Anthropic Beta tool_use block
func convertResponsesFunctionCallToAnthropicBeta(call *responses.ResponseFunctionToolCallParam) anthropic.BetaMessageParam {
	// Parse arguments JSON
	var argsInput interface{}
	if call.Arguments != "" {
		if err := json.Unmarshal([]byte(call.Arguments), &argsInput); err != nil {
			logrus.Warnf("Failed to parse function call arguments JSON for tool %s: %v", call.Name, err)
			// Set to empty map to avoid nil issues
			argsInput = map[string]interface{}{}
		}
	}

	// Create assistant message with tool_use block
	return anthropic.BetaMessageParam{
		Role:    anthropic.BetaMessageParamRoleAssistant,
		Content: []anthropic.BetaContentBlockParamUnion{anthropic.NewBetaToolUseBlock(call.CallID, argsInput, call.Name)},
	}
}

// convertResponsesFunctionCallOutputToAnthropicBeta converts Responses API function_call_output to Anthropic Beta tool_result block
func convertResponsesFunctionCallOutputToAnthropicBeta(output *responses.ResponseInputItemFunctionCallOutputParam) anthropic.BetaMessageParam {
	// Extract output content
	var outputStr string
	if !param.IsOmitted(output.Output.OfString) {
		outputStr = output.Output.OfString.Value
	}

	// Create user message with tool_result block
	return anthropic.NewBetaUserMessage(
		anthropic.NewBetaToolResultBlock(output.CallID, outputStr, false),
	)
}

// ConvertResponsesToolsToAnthropic converts Responses API tools to Anthropic v1 tools
func ConvertResponsesToolsToAnthropic(tools []responses.ToolUnionParam) []anthropic.ToolUnionParam {
	if len(tools) == 0 {
		return nil
	}

	out := make([]anthropic.ToolUnionParam, 0, len(tools))

	for _, t := range tools {
		fn := t.OfFunction
		if fn == nil {
			continue
		}

		// Convert OpenAI function schema to Anthropic input schema
		var inputSchema anthropic.ToolInputSchemaParam
		if fn.Parameters != nil {
			if schemaBytes, err := json.Marshal(fn.Parameters); err == nil {
				if err := json.Unmarshal(schemaBytes, &inputSchema); err != nil {
					logrus.Warnf("Failed to convert tool schema for %s: %v", fn.Name, err)
					// Use default empty schema
					inputSchema = anthropic.ToolInputSchemaParam{}
				}
			}
		}

		// Create tool
		tool := anthropic.ToolUnionParam{
			OfTool: &anthropic.ToolParam{
				Name:        fn.Name,
				InputSchema: inputSchema,
			},
		}

		// Set description if available
		if !param.IsOmitted(fn.Description) && fn.Description.Value != "" {
			tool.OfTool.Description = anthropic.Opt(fn.Description.Value)
		}

		out = append(out, tool)
	}

	return out
}

// ConvertResponsesToolsToAnthropicBeta converts Responses API tools to Anthropic Beta tools
func ConvertResponsesToolsToAnthropicBeta(tools []responses.ToolUnionParam) []anthropic.BetaToolUnionParam {
	if len(tools) == 0 {
		return nil
	}

	out := make([]anthropic.BetaToolUnionParam, 0, len(tools))

	for _, t := range tools {
		fn := t.OfFunction
		if fn == nil {
			continue
		}

		// Convert OpenAI function schema to Anthropic input schema
		var inputSchema anthropic.BetaToolInputSchemaParam
		if fn.Parameters != nil {
			if schemaBytes, err := json.Marshal(fn.Parameters); err == nil {
				if err := json.Unmarshal(schemaBytes, &inputSchema); err != nil {
					logrus.Warnf("Failed to convert tool schema for %s: %v", fn.Name, err)
					// Use default empty schema
					inputSchema = anthropic.BetaToolInputSchemaParam{}
				}
			}
		}

		// Create tool
		tool := anthropic.BetaToolUnionParam{
			OfTool: &anthropic.BetaToolParam{
				Name:        fn.Name,
				InputSchema: inputSchema,
			},
		}

		// Set description if available
		if !param.IsOmitted(fn.Description) && fn.Description.Value != "" {
			tool.OfTool.Description = anthropic.Opt(fn.Description.Value)
		}

		out = append(out, tool)
	}

	return out
}

// ConvertResponsesToolChoiceToAnthropic converts Responses API tool_choice to Anthropic v1 format
func ConvertResponsesToolChoiceToAnthropic(tc responses.ResponseNewParamsToolChoiceUnion) anthropic.ToolChoiceUnionParam {
	// Handle "auto" mode
	if !param.IsOmitted(tc.OfToolChoiceMode) {
		mode := tc.OfToolChoiceMode.Value
		if mode == "auto" {
			return anthropic.ToolChoiceUnionParam{
				OfAuto: &anthropic.ToolChoiceAutoParam{},
			}
		}
		if mode == "required" {
			// Map "required" to Anthropic's "any" mode
			return anthropic.ToolChoiceUnionParam{
				OfAny: &anthropic.ToolChoiceAnyParam{},
			}
		}
		if mode == "none" {
			// "none" means don't use tools - in Anthropic, omit tool_choice when no tools should be used
			// Return auto mode since without tools defined, auto won't call tools
			return anthropic.ToolChoiceUnionParam{
				OfAuto: &anthropic.ToolChoiceAutoParam{},
			}
		}
	}

	// Handle specific function tool choice
	if !param.IsOmitted(tc.OfFunctionTool) {
		fn := tc.OfFunctionTool
		return anthropic.ToolChoiceParamOfTool(fn.Name)
	}

	// Default to auto
	return anthropic.ToolChoiceUnionParam{
		OfAuto: &anthropic.ToolChoiceAutoParam{},
	}
}

// ConvertResponsesToolChoiceToAnthropicBeta converts Responses API tool_choice to Anthropic Beta format
func ConvertResponsesToolChoiceToAnthropicBeta(tc responses.ResponseNewParamsToolChoiceUnion) anthropic.BetaToolChoiceUnionParam {
	// Handle "auto" mode
	if !param.IsOmitted(tc.OfToolChoiceMode) {
		mode := tc.OfToolChoiceMode.Value
		if mode == "auto" {
			return anthropic.BetaToolChoiceUnionParam{
				OfAuto: &anthropic.BetaToolChoiceAutoParam{},
			}
		}
		if mode == "required" {
			// Map "required" to Anthropic's "any" mode
			return anthropic.BetaToolChoiceUnionParam{
				OfAny: &anthropic.BetaToolChoiceAnyParam{},
			}
		}
		if mode == "none" {
			// "none" means don't use tools - in Anthropic, omit tool_choice when no tools should be used
			// Return auto mode since without tools defined, auto won't call tools
			return anthropic.BetaToolChoiceUnionParam{
				OfAuto: &anthropic.BetaToolChoiceAutoParam{},
			}
		}
	}

	// Handle specific function tool choice
	if !param.IsOmitted(tc.OfFunctionTool) {
		fn := tc.OfFunctionTool
		return anthropic.BetaToolChoiceParamOfTool(fn.Name)
	}

	// Default to auto
	return anthropic.BetaToolChoiceUnionParam{
		OfAuto: &anthropic.BetaToolChoiceAutoParam{},
	}
}
