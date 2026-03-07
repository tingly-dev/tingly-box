package request

import (
	"encoding/json"
	"strings"

	"github.com/openai/openai-go/v3"
	"github.com/openai/openai-go/v3/packages/param"
	"github.com/openai/openai-go/v3/responses"
	"github.com/openai/openai-go/v3/shared"
)

// ConvertOpenAIResponsesToChat converts OpenAI Responses API params to Chat Completions format.
// This is useful when translating between the two API formats.
func ConvertOpenAIResponsesToChat(params responses.ResponseNewParams, defaultMaxTokens int64) *openai.ChatCompletionNewParams {
	result := &openai.ChatCompletionNewParams{
		Model: openai.ChatModel(params.Model),
	}

	// Convert instructions to system message if present
	if !param.IsOmitted(params.Instructions) && params.Instructions.Value != "" {
		result.Messages = append(result.Messages, openai.SystemMessage(params.Instructions.Value))
	}

	// Convert input items to messages
	if !param.IsOmitted(params.Input.OfInputItemList) {
		messages := ConvertResponsesInputToMessages(params.Input.OfInputItemList)
		result.Messages = append(result.Messages, messages...)
	}

	// Convert max_output_tokens to max_tokens
	if !param.IsOmitted(params.MaxOutputTokens) {
		result.MaxTokens = openai.Opt(params.MaxOutputTokens.Value)
	} else if defaultMaxTokens > 0 {
		result.MaxTokens = openai.Opt(defaultMaxTokens)
	}

	// Copy temperature
	if !param.IsOmitted(params.Temperature) {
		result.Temperature = openai.Opt(params.Temperature.Value)
	}

	// Copy top_p
	if !param.IsOmitted(params.TopP) {
		result.TopP = openai.Opt(params.TopP.Value)
	}

	// Convert tools if present
	if !param.IsOmitted(params.Tools) && len(params.Tools) > 0 {
		result.Tools = ConvertResponsesToolsToChatTools(params.Tools)
	}

	// Convert tool choice if present
	if !param.IsOmitted(params.ToolChoice) {
		result.ToolChoice = ConvertResponsesToolChoiceToChat(params.ToolChoice)
	}

	return result
}

// ConvertResponsesInputToMessages converts Responses API input items to Chat Completion messages.
func ConvertResponsesInputToMessages(items responses.ResponseInputParam) []openai.ChatCompletionMessageParamUnion {
	var messages []openai.ChatCompletionMessageParamUnion

	for _, item := range items {
		// Handle message items
		if !param.IsOmitted(item.OfMessage) {
			msg := item.OfMessage
			role := string(msg.Role)

			// Extract content based on type
			if !param.IsOmitted(msg.Content.OfString) {
				// Simple string content
				content := msg.Content.OfString.Value
				messages = append(messages, createMessage(role, content))
			} else if !param.IsOmitted(msg.Content.OfInputItemContentList) {
				// Array content - concatenate text items
				var contentStr string
				for _, contentItem := range msg.Content.OfInputItemContentList {
					if !param.IsOmitted(contentItem.OfInputText) {
						contentStr += contentItem.OfInputText.Text
					}
				}
				if contentStr != "" {
					messages = append(messages, createMessage(role, contentStr))
				}
			}
			continue
		}

		// Handle function call items (tool invocations from assistant)
		if !param.IsOmitted(item.OfFunctionCall) {
			fnCall := item.OfFunctionCall

			// Create assistant message with tool_calls
			toolCallMap := map[string]interface{}{
				"id":   fnCall.CallID,
				"type": "function",
				"function": map[string]interface{}{
					"name":      fnCall.Name,
					"arguments": fnCall.Arguments,
				},
			}

			msgMap := map[string]interface{}{
				"role":       "assistant",
				"content":    "",
				"tool_calls": []map[string]interface{}{toolCallMap},
			}

			msgBytes, _ := json.Marshal(msgMap)
			var result openai.ChatCompletionMessageParamUnion
			_ = json.Unmarshal(msgBytes, &result)
			messages = append(messages, result)
			continue
		}

		// Handle function call output items (tool results)
		if !param.IsOmitted(item.OfFunctionCallOutput) {
			output := item.OfFunctionCallOutput

			// Extract output content
			var content string
			if !param.IsOmitted(output.Output.OfString) {
				content = output.Output.OfString.Value
			}

			// Create tool message
			toolMsg := map[string]interface{}{
				"role":         "tool",
				"tool_call_id": output.CallID,
				"content":      content,
			}

			msgBytes, _ := json.Marshal(toolMsg)
			var result openai.ChatCompletionMessageParamUnion
			_ = json.Unmarshal(msgBytes, &result)
			messages = append(messages, result)
		}
	}

	return messages
}

// createMessage creates a ChatCompletionMessageParamUnion based on role and content.
func createMessage(role, content string) openai.ChatCompletionMessageParamUnion {
	switch strings.ToLower(role) {
	case "system":
		return openai.SystemMessage(content)
	case "user":
		return openai.UserMessage(content)
	case "assistant":
		return openai.AssistantMessage(content)
	default:
		// Default to user message for unknown roles
		return openai.UserMessage(content)
	}
}

// ConvertResponsesToolsToChatTools converts Responses API tools to Chat Completions tools.
func ConvertResponsesToolsToChatTools(tools []responses.ToolUnionParam) []openai.ChatCompletionToolUnionParam {
	if len(tools) == 0 {
		return nil
	}

	result := make([]openai.ChatCompletionToolUnionParam, 0, len(tools))

	for _, tool := range tools {
		// Handle function tools
		if !param.IsOmitted(tool.OfFunction) {
			fn := tool.OfFunction

			// Convert parameters map to proper format
			var parameters map[string]interface{}
			if fn.Parameters != nil {
				parameters = fn.Parameters
			} else {
				// Create empty parameters object
				parameters = map[string]interface{}{
					"type":       "object",
					"properties": map[string]interface{}{},
				}
			}

			functionDef := shared.FunctionDefinitionParam{
				Name:        fn.Name,
				Parameters:  parameters,
				Description: param.Opt[string]{},
			}

			// Set description if present
			if !param.IsOmitted(fn.Description) {
				functionDef.Description = fn.Description
			}

			// Set strict mode if present
			if !param.IsOmitted(fn.Strict) {
				// Note: strict mode is set via ExtraFields if needed
			}

			result = append(result, openai.ChatCompletionFunctionTool(functionDef))
		}
	}

	return result
}

// ConvertResponsesToolChoiceToChat converts Responses API tool choice to Chat Completions format.
func ConvertResponsesToolChoiceToChat(choice responses.ResponseNewParamsToolChoiceUnion) openai.ChatCompletionToolChoiceOptionUnionParam {
	// Handle "auto", "none", "required" modes
	if !param.IsOmitted(choice.OfToolChoiceMode) {
		mode := string(choice.OfToolChoiceMode.Value)
		switch mode {
		case "auto", "none", "required":
			return openai.ChatCompletionToolChoiceOptionUnionParam{
				OfAuto: openai.Opt(mode),
			}
		}
	}

	// Handle specific function tool choice
	if !param.IsOmitted(choice.OfFunctionTool) {
		fn := choice.OfFunctionTool
		functionChoice := openai.ChatCompletionNamedToolChoiceFunctionParam{
			Name: fn.Name,
		}
		return openai.ToolChoiceOptionFunctionToolChoice(functionChoice)
	}

	// Default to auto
	return openai.ChatCompletionToolChoiceOptionUnionParam{
		OfAuto: openai.Opt("auto"),
	}
}
