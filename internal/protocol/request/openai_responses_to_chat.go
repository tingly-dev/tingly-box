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

	// Convert input to messages. The Responses API accepts either a plain
	// string (shorthand for a single user message — the SDKs' idiomatic form)
	// or a list of input items.
	if !param.IsOmitted(params.Input.OfString) && params.Input.OfString.Value != "" {
		result.Messages = append(result.Messages, openai.UserMessage(params.Input.OfString.Value))
	} else if !param.IsOmitted(params.Input.OfInputItemList) {
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

	result.PromptCacheKey = params.PromptCacheKey
	result.PromptCacheOptions = openai.ChatCompletionNewParamsPromptCacheOptions{
		Mode: params.PromptCacheOptions.Mode,
		Ttl:  params.PromptCacheOptions.Ttl,
	}
	result.PromptCacheRetention = openai.ChatCompletionNewParamsPromptCacheRetention(params.PromptCacheRetention)

	return result
}

// pendingToolCall holds a single tool call during input-to-message conversion.
// Consecutive function_call input items are accumulated and flushed together
// as a single assistant message with all tool_calls.
type pendingToolCall struct {
	CallID    string
	Name      string
	Arguments string
}

// ConvertResponsesInputToMessages converts Responses API input items to Chat Completion messages.
//
// The input is first passed through RepairResponsesToolCalls, which
// guarantees that every function_call group is immediately followed by one
// function_call_output per call and that no output stands without a call.
// Chat Completions providers (DeepSeek, OpenAI) require exactly that shape:
// an assistant message carrying tool_calls must be followed by one tool
// message per call, and a tool message must answer a preceding tool_calls
// message. The conversion below therefore only has to preserve order.
func ConvertResponsesInputToMessages(items responses.ResponseInputParam) []openai.ChatCompletionMessageParamUnion {
	items = RepairResponsesToolCalls(items)

	var messages []openai.ChatCompletionMessageParamUnion
	var pendingCalls []pendingToolCall

	// flushCalls emits the accumulated function_calls as one assistant message.
	flushCalls := func() {
		if len(pendingCalls) == 0 {
			return
		}
		toolCalls := make([]map[string]interface{}, 0, len(pendingCalls))
		for _, tc := range pendingCalls {
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
		var assistant openai.ChatCompletionMessageParamUnion
		_ = json.Unmarshal(msgBytes, &assistant)
		messages = append(messages, assistant)
		pendingCalls = nil
	}

	for _, item := range items {
		if !param.IsOmitted(item.OfFunctionCall) {
			fnCall := item.OfFunctionCall
			pendingCalls = append(pendingCalls, pendingToolCall{
				CallID:    fnCall.CallID,
				Name:      fnCall.Name,
				Arguments: fnCall.Arguments,
			})
			continue
		}

		// Any other item ends the assistant tool-call turn.
		flushCalls()

		switch {
		case !param.IsOmitted(item.OfMessage):
			msg := item.OfMessage
			role := string(msg.Role)
			if !param.IsOmitted(msg.Content.OfString) {
				messages = append(messages, createMessage(role, msg.Content.OfString.Value))
			} else if !param.IsOmitted(msg.Content.OfInputItemContentList) {
				if converted, ok := createMessageFromResponsesContent(role, msg.Content.OfInputItemContentList); ok {
					messages = append(messages, converted)
				}
			}

		case !param.IsOmitted(item.OfOutputMessage):
			// The assistant-message counterpart to item.OfMessage — content
			// parts are output_text/refusal, never input_text.
			if text, ok := outputMessageText(item.OfOutputMessage); ok {
				messages = append(messages, createMessage("assistant", text))
			}

		case !param.IsOmitted(item.OfFunctionCallOutput):
			messages = append(messages, convertResponsesFunctionCallOutput(item.OfFunctionCallOutput))
		}
	}
	flushCalls()

	return messages
}

// convertResponsesFunctionCallOutput converts a function_call_output item to
// a Chat Completions tool message.
func convertResponsesFunctionCallOutput(output *responses.ResponseInputItemFunctionCallOutputParam) openai.ChatCompletionMessageParamUnion {
	callID := output.CallID.Value
	if !param.IsOmitted(output.Output.OfString) {
		return openai.ToolMessage(output.Output.OfString.Value, callID)
	}
	parts := make([]openai.ChatCompletionContentPartUnionParam, 0,
		len(output.Output.OfResponseFunctionCallOutputItemArray))
	for _, item := range output.Output.OfResponseFunctionCallOutputItemArray {
		switch {
		case item.OfInputText != nil && item.OfInputText.Text != "":
			part := openai.ChatCompletionContentPartTextParam{Text: item.OfInputText.Text}
			if !param.IsOmitted(item.OfInputText.PromptCacheBreakpoint) {
				part.PromptCacheBreakpoint = openai.NewChatCompletionContentPartTextPromptCacheBreakpointParam()
			}
			parts = append(parts, openai.ChatCompletionContentPartUnionParam{OfText: &part})
		case item.OfInputImage != nil && item.OfInputImage.ImageURL.Valid():
			part := openai.ChatCompletionContentPartImageParam{
				ImageURL: openai.ChatCompletionContentPartImageImageURLParam{URL: item.OfInputImage.ImageURL.Value},
			}
			if !param.IsOmitted(item.OfInputImage.PromptCacheBreakpoint) {
				part.PromptCacheBreakpoint = openai.NewChatCompletionContentPartImagePromptCacheBreakpointParam()
			}
			parts = append(parts, openai.ChatCompletionContentPartUnionParam{OfImageURL: &part})
		}
	}
	if len(parts) == 0 {
		return openai.ToolMessage("", callID)
	}
	return openai.ChatCompletionMessageParamUnion{
		OfTool: &openai.ChatCompletionToolMessageParam{
			ToolCallID: callID,
			Content: openai.ChatCompletionToolMessageParamContentUnion{
				OfArrayOfContentParts: parts,
			},
		},
	}
}

// outputMessageText concatenates the output_text/refusal parts of a Responses
// output_message item into a single string, for feeding into Chat's plain
// string content form.
func outputMessageText(msg *responses.ResponseOutputMessageParam) (string, bool) {
	var text string
	for _, item := range msg.Content {
		switch {
		case item.OfOutputText != nil:
			text += item.OfOutputText.Text
		case item.OfRefusal != nil:
			text += item.OfRefusal.Refusal
		}
	}
	return text, text != ""
}

func createMessageFromResponsesContent(role string, content responses.ResponseInputMessageContentListParam) (openai.ChatCompletionMessageParamUnion, bool) {	var text string
	var hasImage, hasCacheBreakpoint bool
	for _, item := range content {
		switch {
		case item.OfInputText != nil:
			text += item.OfInputText.Text
			hasCacheBreakpoint = hasCacheBreakpoint || !param.IsOmitted(item.OfInputText.PromptCacheBreakpoint)
		case item.OfInputImage != nil:
			hasImage = true
			hasCacheBreakpoint = hasCacheBreakpoint || !param.IsOmitted(item.OfInputImage.PromptCacheBreakpoint)
		}
	}

	if !hasImage && !hasCacheBreakpoint {
		if text == "" {
			return openai.ChatCompletionMessageParamUnion{}, false
		}
		return createMessage(role, text), true
	}

	switch strings.ToLower(role) {
	case "user":
		parts := make([]openai.ChatCompletionContentPartUnionParam, 0, len(content))
		for _, item := range content {
			switch {
			case item.OfInputText != nil:
				part := openai.ChatCompletionContentPartTextParam{Text: item.OfInputText.Text}
				if !param.IsOmitted(item.OfInputText.PromptCacheBreakpoint) {
					part.PromptCacheBreakpoint = openai.NewChatCompletionContentPartTextPromptCacheBreakpointParam()
				}
				parts = append(parts, openai.ChatCompletionContentPartUnionParam{OfText: &part})
			case item.OfInputImage != nil && item.OfInputImage.ImageURL.Valid():
				part := openai.ChatCompletionContentPartImageParam{
					ImageURL: openai.ChatCompletionContentPartImageImageURLParam{URL: item.OfInputImage.ImageURL.Value},
				}
				if !param.IsOmitted(item.OfInputImage.PromptCacheBreakpoint) {
					part.PromptCacheBreakpoint = openai.NewChatCompletionContentPartImagePromptCacheBreakpointParam()
				}
				parts = append(parts, openai.ChatCompletionContentPartUnionParam{OfImageURL: &part})
			}
		}
		if len(parts) == 0 {
			return openai.ChatCompletionMessageParamUnion{}, false
		}
		return openai.UserMessage(parts), true

	case "system":
		parts := responseTextContentToChatTextParts(content)
		if len(parts) == 0 {
			return openai.ChatCompletionMessageParamUnion{}, false
		}
		return openai.SystemMessage(parts), true

	case "developer":
		parts := responseTextContentToChatTextParts(content)
		if len(parts) == 0 {
			return openai.ChatCompletionMessageParamUnion{}, false
		}
		return openai.DeveloperMessage(parts), true

	case "assistant":
		textParts := responseTextContentToChatTextParts(content)
		if len(textParts) == 0 {
			return openai.ChatCompletionMessageParamUnion{}, false
		}
		parts := make([]openai.ChatCompletionAssistantMessageParamContentArrayOfContentPartUnion, 0, len(textParts))
		for i := range textParts {
			part := textParts[i]
			parts = append(parts, openai.ChatCompletionAssistantMessageParamContentArrayOfContentPartUnion{OfText: &part})
		}
		return openai.ChatCompletionMessageParamUnion{
			OfAssistant: &openai.ChatCompletionAssistantMessageParam{
				Content: openai.ChatCompletionAssistantMessageParamContentUnion{
					OfArrayOfContentParts: parts,
				},
			},
		}, true
	}
	return openai.ChatCompletionMessageParamUnion{}, false
}

func responseTextContentToChatTextParts(content responses.ResponseInputMessageContentListParam) []openai.ChatCompletionContentPartTextParam {
	parts := make([]openai.ChatCompletionContentPartTextParam, 0, len(content))
	for _, item := range content {
		if item.OfInputText == nil || item.OfInputText.Text == "" {
			continue
		}
		part := openai.ChatCompletionContentPartTextParam{Text: item.OfInputText.Text}
		if !param.IsOmitted(item.OfInputText.PromptCacheBreakpoint) {
			part.PromptCacheBreakpoint = openai.NewChatCompletionContentPartTextPromptCacheBreakpointParam()
		}
		parts = append(parts, part)
	}
	return parts
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
