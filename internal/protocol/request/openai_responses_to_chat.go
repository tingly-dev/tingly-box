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
func ConvertOpenAIResponsesToChat(params *responses.ResponseNewParams, defaultMaxTokens int64) *openai.ChatCompletionNewParams {
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

// pendingToolCall holds a single tool call during input-to-message conversion.
// Consecutive function_call input items are accumulated and flushed together
// as a single assistant message with all tool_calls, so the resulting message
// sequence satisfies providers (DeepSeek) that require tool messages to
// immediately follow the assistant message that requested them.
type pendingToolCall struct {
	CallID    string
	Name      string
	Arguments string
}

// ConvertResponsesInputToMessages converts Responses API input items to Chat Completion messages.
func ConvertResponsesInputToMessages(items responses.ResponseInputParam) []openai.ChatCompletionMessageParamUnion {
	var messages []openai.ChatCompletionMessageParamUnion

	flushCalls := func(calls []pendingToolCall) {
		if len(calls) == 0 {
			return
		}
		toolCalls := make([]map[string]interface{}, 0, len(calls))
		for _, tc := range calls {
			toolCalls = append(toolCalls, map[string]interface{}{
				"id":   tc.CallID,
				"type": "function",
				"function": map[string]interface{}{
					"name":      tc.Name,
					"arguments": tc.Arguments,
				},
			})
		}
		msgMap := map[string]interface{}{
			"role":       "assistant",
			"content":    "",
			"tool_calls": toolCalls,
		}
		msgBytes, _ := json.Marshal(msgMap)
		var result openai.ChatCompletionMessageParamUnion
		_ = json.Unmarshal(msgBytes, &result)
		messages = append(messages, result)
	}

	var pendingCalls []pendingToolCall

	for _, item := range items {
		// Handle message items — do NOT flush pending function_calls.
		// function_call_output flushes them so that assistant(tool_calls)
		// appears immediately before the corresponding tool messages.
		// Flushing here would cause messages to be inserted between
		// assistant(tool_calls) and its tool responses.
		if !param.IsOmitted(item.OfMessage) {
			msg := item.OfMessage
			role := string(msg.Role)

			// Extract content based on type
			if !param.IsOmitted(msg.Content.OfString) {
				// Simple string content
				content := msg.Content.OfString.Value
				messages = append(messages, createMessage(role, content))
			} else if !param.IsOmitted(msg.Content.OfInputItemContentList) {
				// Array content. If any input_image is present, preserve the
				// multipart shape for OpenAI Chat Completions so vision input
				// survives the conversion. Otherwise concatenate text items.
				var hasImage bool
				for _, contentItem := range msg.Content.OfInputItemContentList {
					if !param.IsOmitted(contentItem.OfInputImage) {
						hasImage = true
						break
					}
				}

				if hasImage && strings.EqualFold(role, "user") {
					parts := make([]map[string]interface{}, 0, len(msg.Content.OfInputItemContentList))
					for _, contentItem := range msg.Content.OfInputItemContentList {
						switch {
						case !param.IsOmitted(contentItem.OfInputText):
							parts = append(parts, map[string]interface{}{
								"type": "text",
								"text": contentItem.OfInputText.Text,
							})
						case !param.IsOmitted(contentItem.OfInputImage):
							img := contentItem.OfInputImage
							if !img.ImageURL.Valid() || img.ImageURL.Value == "" {
								continue
							}
							parts = append(parts, map[string]interface{}{
								"type":      "image_url",
								"image_url": map[string]interface{}{"url": img.ImageURL.Value},
							})
						}
					}
					if len(parts) > 0 {
						msgMap := map[string]interface{}{
							"role":    "user",
							"content": parts,
						}
						msgBytes, _ := json.Marshal(msgMap)
						var userMsg openai.ChatCompletionMessageParamUnion
						_ = json.Unmarshal(msgBytes, &userMsg)
						messages = append(messages, userMsg)
					}
					continue
				}

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

			// Accumulate consecutive function_call items into a single assistant message.
		// Flushed on the next message boundary or first function_call_output.
		if !param.IsOmitted(item.OfFunctionCall) {
			fnCall := item.OfFunctionCall
			pendingCalls = append(pendingCalls, pendingToolCall{
				CallID:    fnCall.CallID,
				Name:      fnCall.Name,
				Arguments: fnCall.Arguments,
			})
			continue
		}

		// Handle function call output items (tool results)
		// Flush pending function calls as a single assistant message first
		if !param.IsOmitted(item.OfFunctionCallOutput) {
			flushCalls(pendingCalls)
			pendingCalls = nil

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

	// Flush remaining pending calls at end of input
	flushCalls(pendingCalls)

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
