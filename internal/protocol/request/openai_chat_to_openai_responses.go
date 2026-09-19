package request

import (
	"encoding/json"
	"slices"
	"strings"

	"github.com/openai/openai-go/v3"
	"github.com/openai/openai-go/v3/packages/param"
	"github.com/openai/openai-go/v3/responses"
	"github.com/openai/openai-go/v3/shared"
)

// joinTextContentParts concatenates the text fields of text-only content parts
// (used by tool and system messages) into a single newline-separated string,
// letting converters handle the array-of-text-blocks content form allowed by
// the OpenAI spec.
func joinTextContentParts(parts []openai.ChatCompletionContentPartTextParam) string {
	if len(parts) == 0 {
		return ""
	}
	texts := make([]string, 0, len(parts))
	for _, part := range parts {
		if part.Text != "" {
			texts = append(texts, part.Text)
		}
	}
	return strings.Join(texts, "\n")
}

// ConvertChatToOpenAIResponses converts OpenAI Chat Completions params to Responses API format.
// This enables using Chat Completions format with Responses API providers.
func ConvertChatToOpenAIResponses(params *openai.ChatCompletionNewParams, defaultMaxTokens int64) *responses.ResponseNewParams {
	result := &responses.ResponseNewParams{
		Model: string(params.Model),
	}

	var systemParts []string
	var otherMessages []openai.ChatCompletionMessageParamUnion
	preserveSystemMessages := false
	for _, msg := range params.Messages {
		if msg.OfSystem != nil &&
			chatTextPartsHaveCacheBreakpoint(msg.OfSystem.Content.OfArrayOfContentParts) {
			preserveSystemMessages = true
			break
		}
	}

	// Separate system messages from other messages
	for _, msg := range params.Messages {
		switch {
		case !param.IsOmitted(msg.OfSystem):
			if preserveSystemMessages {
				// Responses' instructions field is a plain string and cannot
				// carry explicit cache boundaries. Keep every system message
				// in input so their order and breakpoints remain intact.
				otherMessages = append(otherMessages, msg)
				continue
			}
			// Extract system message content (string or array-of-text form)
			sysMsg := msg.OfSystem
			if !param.IsOmitted(sysMsg.Content.OfString) && sysMsg.Content.OfString.Value != "" {
				systemParts = append(systemParts, sysMsg.Content.OfString.Value)
			} else if text := joinTextContentParts(sysMsg.Content.OfArrayOfContentParts); text != "" {
				systemParts = append(systemParts, text)
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

	// Forward reasoning effort if the client supplied one
	if params.ReasoningEffort != "" {
		result.Reasoning = shared.ReasoningParam{
			Effort: params.ReasoningEffort,
		}
	}

	result.PromptCacheKey = params.PromptCacheKey
	result.PromptCacheOptions = responses.ResponseNewParamsPromptCacheOptions{
		Mode: params.PromptCacheOptions.Mode,
		Ttl:  params.PromptCacheOptions.Ttl,
	}
	result.PromptCacheRetention = responses.ResponseNewParamsPromptCacheRetention(params.PromptCacheRetention)

	return result
}

// ConvertChatMessagesToResponsesInput converts Chat Completion messages to Responses API input items.
func ConvertChatMessagesToResponsesInput(messages []openai.ChatCompletionMessageParamUnion) responses.ResponseInputParam {
	var result responses.ResponseInputParam

	for _, msg := range messages {
		switch {
		case !param.IsOmitted(msg.OfSystem):
			result = append(result, convertChatSystemMessageToResponses(msg.OfSystem)...)

		case !param.IsOmitted(msg.OfUser):
			result = append(result, convertChatUserMessageToResponses(msg.OfUser)...)

		case !param.IsOmitted(msg.OfAssistant):
			assistantMsg := msg.OfAssistant
			result = append(result, convertChatAssistantMessageToResponses(assistantMsg)...)
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
			}

		case !param.IsOmitted(msg.OfTool):
			result = append(result, convertChatToolMessageToResponses(msg.OfTool))
		}
	}

	return result
}

func convertChatSystemMessageToResponses(systemMsg *openai.ChatCompletionSystemMessageParam) []responses.ResponseInputItemUnionParam {
	if systemMsg.Content.OfString.Valid() && systemMsg.Content.OfString.Value != "" {
		return []responses.ResponseInputItemUnionParam{responseMessageWithString("system", systemMsg.Content.OfString.Value)}
	}
	content := responseContentFromChatTextParts(systemMsg.Content.OfArrayOfContentParts)
	if len(content) == 0 {
		return nil
	}
	return []responses.ResponseInputItemUnionParam{responseMessageWithContent("system", content)}
}

// convertChatUserMessageToResponses converts a Chat user message to Responses format.
// Returns nil if the message has no usable content.
func convertChatUserMessageToResponses(userMsg *openai.ChatCompletionUserMessageParam) []responses.ResponseInputItemUnionParam {
	if userMsg.Content.OfString.Valid() && userMsg.Content.OfString.Value != "" {
		return []responses.ResponseInputItemUnionParam{{
			OfMessage: &responses.EasyInputMessageParam{
				Type: responses.EasyInputMessageTypeMessage,
				Role: responses.EasyInputMessageRole("user"),
				Content: responses.EasyInputMessageContentUnionParam{
					OfString: param.NewOpt(userMsg.Content.OfString.Value),
				},
			},
		}}
	}

	// Multipart content: forward text + image_url parts as input_text + input_image.
	if len(userMsg.Content.OfArrayOfContentParts) > 0 {
		contentList := make(responses.ResponseInputMessageContentListParam, 0, len(userMsg.Content.OfArrayOfContentParts))
		for _, part := range userMsg.Content.OfArrayOfContentParts {
			switch {
			case part.OfText != nil:
				inputText := &responses.ResponseInputTextParam{Text: part.OfText.Text}
				if hasOpenAITextCacheBreakpoint(*part.OfText) {
					inputText.PromptCacheBreakpoint = responses.NewResponseInputTextPromptCacheBreakpointParam()
				}
				contentList = append(contentList, responses.ResponseInputContentUnionParam{
					OfInputText: inputText,
				})
			case part.OfImageURL != nil:
				url := part.OfImageURL.ImageURL.URL
				if url == "" {
					continue
				}
				inputImage := &responses.ResponseInputImageParam{ImageURL: param.NewOpt(url)}
				if !param.IsOmitted(part.OfImageURL.PromptCacheBreakpoint) {
					inputImage.PromptCacheBreakpoint = responses.NewResponseInputImagePromptCacheBreakpointParam()
				}
				contentList = append(contentList, responses.ResponseInputContentUnionParam{OfInputImage: inputImage})
			}
		}
		if len(contentList) > 0 {
			return []responses.ResponseInputItemUnionParam{{
				OfMessage: &responses.EasyInputMessageParam{
					Type: responses.EasyInputMessageTypeMessage,
					Role: responses.EasyInputMessageRole("user"),
					Content: responses.EasyInputMessageContentUnionParam{
						OfInputItemContentList: contentList,
					},
				},
			}}
		}
	}

	// No usable content — skip rather than emit an empty message.
	return nil
}

// convertChatAssistantMessageToResponses converts a Chat assistant message to Responses format.
// Returns nil if the message has no usable text content.
//
// Always emits the plain-string content form. The typed content-part form
// (responseContentFromChatTextParts) tags parts input_text, which the
// Responses API rejects on assistant-authored content — it requires
// output_text/refusal there — and a prompt-cache breakpoint has no
// output_text equivalent to preserve anyway, so there is nothing the part
// form buys an assistant message that the string form doesn't.
// convertChatAssistantMessageToResponses converts a Chat assistant message to Responses format.
// Returns nil if the message has no usable text content.
//
// Emits an output_message item (responseOutputMessageWithText), matching the
// native shape the Anthropic->Responses path emits for assistant text (see
// convertV1AssistantMessageToResponsesInput). The Responses API also accepts
// assistant content as a plain-string message item, but a mismatched
// content-part form on that path is what caused the input_text/output_text
// bug this converter is the twin of (cache_control.go) — using the one
// unambiguous shape here keeps the two conversion paths from producing two
// different but both-valid wire forms for the same input.
func convertChatAssistantMessageToResponses(assistantMsg *openai.ChatCompletionAssistantMessageParam) []responses.ResponseInputItemUnionParam {
	if content := assistantMsg.Content.OfString.Value; content != "" {
		return []responses.ResponseInputItemUnionParam{responseOutputMessageWithText(content)}
	}
	var parts []openai.ChatCompletionContentPartTextParam
	for _, part := range assistantMsg.Content.OfArrayOfContentParts {
		if part.OfText != nil {
			parts = append(parts, *part.OfText)
		}
	}
	content := joinTextContentParts(parts)
	if content == "" {
		return nil
	}
	return []responses.ResponseInputItemUnionParam{responseOutputMessageWithText(content)}
}

// convertChatToolMessageToResponses converts a Chat tool message to Responses function_call_output format.
// Tool content may carry image_url parts (agent screenshots) alongside text, so the
// array form is walked as the full content-part union rather than text-only.
func convertChatToolMessageToResponses(toolMsg *openai.ChatCompletionToolMessageParam) responses.ResponseInputItemUnionParam {
	content := toolMsg.Content.OfString.Value
	output := responses.ResponseInputItemFunctionCallOutputOutputUnionParam{}
	if content != "" {
		output.OfString = param.NewOpt(content)
	} else if !chatUnionPartsHaveMedia(toolMsg.Content.OfArrayOfContentParts) &&
		!chatUnionTextPartsHaveCacheBreakpoint(toolMsg.Content.OfArrayOfContentParts) {
		output.OfString = param.NewOpt(joinTextContentPartsFromUnion(toolMsg.Content.OfArrayOfContentParts))
	} else {
		items := make(responses.ResponseFunctionCallOutputItemListParam, 0, len(toolMsg.Content.OfArrayOfContentParts))
		for _, part := range toolMsg.Content.OfArrayOfContentParts {
			switch {
			case part.OfText != nil:
				if part.OfText.Text == "" {
					continue
				}
				item := &responses.ResponseInputTextContentParam{Text: part.OfText.Text}
				if hasOpenAITextCacheBreakpoint(*part.OfText) {
					item.PromptCacheBreakpoint = responses.NewResponseInputTextContentPromptCacheBreakpointParam()
				}
				items = append(items, responses.ResponseFunctionCallOutputItemUnionParam{OfInputText: item})
			case part.OfImageURL != nil:
				url := part.OfImageURL.ImageURL.URL
				if url == "" {
					continue
				}
				image := &responses.ResponseInputImageContentParam{ImageURL: param.NewOpt(url)}
				if !param.IsOmitted(part.OfImageURL.PromptCacheBreakpoint) {
					image.PromptCacheBreakpoint = responses.NewResponseInputImageContentPromptCacheBreakpointParam()
				}
				items = append(items, responses.ResponseFunctionCallOutputItemUnionParam{OfInputImage: image})
			}
		}
		if len(items) > 0 {
			output.OfResponseFunctionCallOutputItemArray = items
		} else {
			output.OfString = param.NewOpt("")
		}
	}

	return responses.ResponseInputItemUnionParam{
		OfFunctionCallOutput: &responses.ResponseInputItemFunctionCallOutputParam{
			CallID: param.NewOpt(toolMsg.ToolCallID),
			Output: output,
		},
	}
}

func responseContentFromChatTextParts(parts []openai.ChatCompletionContentPartTextParam) responses.ResponseInputMessageContentListParam {
	content := make(responses.ResponseInputMessageContentListParam, 0, len(parts))
	for _, part := range parts {
		if part.Text == "" {
			continue
		}
		item := &responses.ResponseInputTextParam{Text: part.Text}
		if hasOpenAITextCacheBreakpoint(part) {
			item.PromptCacheBreakpoint = responses.NewResponseInputTextPromptCacheBreakpointParam()
		}
		content = append(content, responses.ResponseInputContentUnionParam{OfInputText: item})
	}
	return content
}

func responseMessageWithString(role, content string) responses.ResponseInputItemUnionParam {
	return responses.ResponseInputItemUnionParam{OfMessage: &responses.EasyInputMessageParam{
		Type:    responses.EasyInputMessageTypeMessage,
		Role:    responses.EasyInputMessageRole(role),
		Content: responses.EasyInputMessageContentUnionParam{OfString: param.NewOpt(content)},
	}}
}

// responseOutputMessageWithText builds an output_message item for assistant
// text — the shape the Responses API actually expects for assistant-authored
// content (output_text, not input_text; see responsesOutputTextParts in
// cache_control.go, the Anthropic-path equivalent of this helper).
func responseOutputMessageWithText(text string) responses.ResponseInputItemUnionParam {
	return responses.ResponseInputItemParamOfOutputMessage(
		[]responses.ResponseOutputMessageContentUnionParam{{OfOutputText: &responses.ResponseOutputTextParam{Text: text}}},
		"",
		responses.ResponseOutputMessageStatusCompleted,
	)
}

func responseMessageWithContent(role string, content responses.ResponseInputMessageContentListParam) responses.ResponseInputItemUnionParam {
	return responses.ResponseInputItemUnionParam{OfMessage: &responses.EasyInputMessageParam{
		Type:    responses.EasyInputMessageTypeMessage,
		Role:    responses.EasyInputMessageRole(role),
		Content: responses.EasyInputMessageContentUnionParam{OfInputItemContentList: content},
	}}
}

func chatTextPartsHaveCacheBreakpoint(parts []openai.ChatCompletionContentPartTextParam) bool {
	return slices.ContainsFunc(parts, hasOpenAITextCacheBreakpoint)
}

// joinTextContentPartsFromUnion is joinTextContentParts for the full content-part
// union, ignoring non-text parts (used by tool messages, which may carry image_url
// parts alongside text).
func joinTextContentPartsFromUnion(parts []openai.ChatCompletionContentPartUnionParam) string {
	if len(parts) == 0 {
		return ""
	}
	texts := make([]string, 0, len(parts))
	for _, part := range parts {
		if part.OfText != nil && part.OfText.Text != "" {
			texts = append(texts, part.OfText.Text)
		}
	}
	return strings.Join(texts, "\n")
}

// chatUnionTextPartsHaveCacheBreakpoint reports whether any text part in the union
// carries a prompt-cache breakpoint.
func chatUnionTextPartsHaveCacheBreakpoint(parts []openai.ChatCompletionContentPartUnionParam) bool {
	return slices.ContainsFunc(parts, func(part openai.ChatCompletionContentPartUnionParam) bool {
		return part.OfText != nil && hasOpenAITextCacheBreakpoint(*part.OfText)
	})
}

// chatUnionPartsHaveMedia reports whether the union contains any non-text part
// (e.g. image_url), which forces the array output form even without text.
func chatUnionPartsHaveMedia(parts []openai.ChatCompletionContentPartUnionParam) bool {
	return slices.ContainsFunc(parts, func(part openai.ChatCompletionContentPartUnionParam) bool {
		return part.OfText == nil
	})
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
