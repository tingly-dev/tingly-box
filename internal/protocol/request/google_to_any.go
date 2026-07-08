package request

import (
	"encoding/json"
	"strings"

	"github.com/anthropics/anthropic-sdk-go"
	"github.com/openai/openai-go/v3"
	"github.com/openai/openai-go/v3/packages/param"
	"github.com/openai/openai-go/v3/shared"
	"google.golang.org/genai"
)

// ConvertGoogleToOpenAIRequest converts Google Content and config to OpenAI format
func ConvertGoogleToOpenAIRequest(model string, contents []*genai.Content, config *genai.GenerateContentConfig) *openai.ChatCompletionNewParams {
	openaiReq := &openai.ChatCompletionNewParams{
		Model: openai.ChatModel(model),
	}

	// Set MaxTokens - Google uses int32
	if config != nil && config.MaxOutputTokens > 0 {
		openaiReq.MaxTokens = openai.Opt(int64(config.MaxOutputTokens))
	}

	// Set Temperature - Google uses *float32, OpenAI uses float64
	if config != nil && config.Temperature != nil {
		openaiReq.Temperature = openai.Opt(float64(*config.Temperature))
	}

	// Set TopP - Google uses *float32, OpenAI uses float64
	if config != nil && config.TopP != nil {
		openaiReq.TopP = openai.Opt(float64(*config.TopP))
	}

	// Convert contents to messages
	for _, content := range contents {
		if content.Role == "system" {
			// System message
			systemText := ConvertGooglePartsToString(content.Parts)
			if systemText != "" {
				sysMsg := openai.SystemMessage(systemText)
				// Insert at beginning
				openaiReq.Messages = append([]openai.ChatCompletionMessageParamUnion{sysMsg}, openaiReq.Messages...)
			}
		} else {
			// convertGoogleContentToOpenAI always returns a constructed
			// message (falling back to an empty user message), so no
			// marshal-based validity check is needed.
			openaiReq.Messages = append(openaiReq.Messages, convertGoogleContentToOpenAI(content))
		}
	}

	// Convert tools from Google format to OpenAI format
	if config != nil && len(config.Tools) > 0 {
		for _, tool := range config.Tools {
			if len(tool.FunctionDeclarations) > 0 {
				openaiReq.Tools = ConvertGoogleToolsToOpenAI(tool.FunctionDeclarations)
				break
			}
		}
		// Convert tool config
		if config.ToolConfig != nil && config.ToolConfig.FunctionCallingConfig != nil {
			openaiReq.ToolChoice = ConvertGoogleToolChoiceToOpenAI(config.ToolConfig.FunctionCallingConfig)
		}
	}

	return openaiReq
}

// convertGoogleContentToOpenAI converts a Google Content to OpenAI message format
func convertGoogleContentToOpenAI(content *genai.Content) openai.ChatCompletionMessageParamUnion {
	var textContent string
	var toolCalls []openai.ChatCompletionMessageToolCallUnionParam

	for _, part := range content.Parts {
		// Handle text parts
		if part.Text != "" {
			textContent += part.Text
		}

		// Handle function calls
		if part.FunctionCall != nil {
			// Marshal args to JSON string for OpenAI
			var args string
			if argsBytes, err := json.Marshal(part.FunctionCall.Args); err == nil {
				args = string(argsBytes)
			}
			toolCalls = append(toolCalls, openai.ChatCompletionMessageToolCallUnionParam{
				OfFunction: &openai.ChatCompletionMessageFunctionToolCallParam{
					ID: part.FunctionCall.ID,
					Function: openai.ChatCompletionMessageFunctionToolCallFunctionParam{
						Name:      part.FunctionCall.Name,
						Arguments: args,
					},
				},
			})
		}

		// Handle function responses (tool results)
		if part.FunctionResponse != nil {
			// Convert to OpenAI role="tool" message
			resultText := ""

			// Google's FunctionResponse has a Response field with the actual data
			if part.FunctionResponse.Response != nil {
				// Check if it has "output" key, if so, use that directly
				if output, exists := part.FunctionResponse.Response["output"]; exists {
					if outputStr, ok := output.(string); ok {
						resultText = outputStr
					} else {
						// Output is not a string, marshal the whole response
						if responseBytes, err := json.Marshal(part.FunctionResponse.Response); err == nil {
							resultText = string(responseBytes)
						}
					}
				} else {
					// No "output" key, use the whole response as JSON
					if responseBytes, err := json.Marshal(part.FunctionResponse.Response); err == nil {
						resultText = string(responseBytes)
					}
				}
			}

			return openai.ToolMessage(resultText, part.FunctionResponse.Name)
		}
	}

	// Build the message based on role
	if content.Role == "user" {
		// User message with text only
		if textContent != "" {
			return openai.UserMessage(textContent)
		}
	} else if content.Role == "model" {
		// Model (assistant) message
		if len(toolCalls) > 0 || textContent != "" {
			assistant := openai.ChatCompletionAssistantMessageParam{
				ToolCalls: toolCalls,
			}
			assistant.Content.OfString = openai.Opt(textContent)
			return openai.ChatCompletionMessageParamUnion{OfAssistant: &assistant}
		}
	}

	// Return empty user message as fallback
	return openai.UserMessage("")
}

func ConvertGoogleToolsToOpenAI(funcs []*genai.FunctionDeclaration) []openai.ChatCompletionToolUnionParam {
	if len(funcs) == 0 {
		return nil
	}

	out := make([]openai.ChatCompletionToolUnionParam, 0, len(funcs))

	for _, f := range funcs {
		var parameters map[string]interface{}
		if f.Parameters != nil {
			// Convert Schema to map[string]interface{}
			if schemaBytes, err := json.Marshal(f.Parameters); err == nil {
				_ = json.Unmarshal(schemaBytes, &parameters)
				// Normalize type field from uppercase (OBJECT, ARRAY) to lowercase
				parameters = normalizeGoogleSchemaTypes(parameters)
			}
		}

		fn := shared.FunctionDefinitionParam{
			Name:        f.Name,
			Description: param.Opt[string]{Value: f.Description},
			Parameters:  parameters,
		}
		out = append(out, openai.ChatCompletionFunctionTool(fn))
	}

	return out
}

// normalizeGoogleSchemaTypes recursively converts uppercase type names to lowercase
// Google genai SDK uses OBJECT, ARRAY, STRING, etc. but OpenAI expects lowercase
func normalizeGoogleSchemaTypes(schema map[string]interface{}) map[string]interface{} {
	if schema == nil {
		return nil
	}
	result := make(map[string]interface{})
	for k, v := range schema {
		if k == "type" {
			if typeStr, ok := v.(string); ok {
				result[k] = strings.ToLower(typeStr)
			} else {
				result[k] = v
			}
		} else if k == "properties" {
			if props, ok := v.(map[string]interface{}); ok {
				result[k] = normalizeGoogleSchemaProperties(props)
			} else {
				result[k] = v
			}
		} else if k == "items" {
			if items, ok := v.(map[string]interface{}); ok {
				result[k] = normalizeGoogleSchemaTypes(items)
			} else {
				result[k] = v
			}
		} else {
			result[k] = v
		}
	}
	return result
}

// normalizeGoogleSchemaProperties normalizes all property schemas
func normalizeGoogleSchemaProperties(props map[string]interface{}) map[string]interface{} {
	result := make(map[string]interface{})
	for k, v := range props {
		if propSchema, ok := v.(map[string]interface{}); ok {
			result[k] = normalizeGoogleSchemaTypes(propSchema)
		} else {
			result[k] = v
		}
	}
	return result
}

func ConvertGoogleToolChoiceToOpenAI(config *genai.FunctionCallingConfig) openai.ChatCompletionToolChoiceOptionUnionParam {
	switch config.Mode {
	case genai.FunctionCallingConfigModeAuto:
		return openai.ChatCompletionToolChoiceOptionUnionParam{
			OfAuto: openai.Opt("auto"),
		}
	case genai.FunctionCallingConfigModeAny:
		if len(config.AllowedFunctionNames) > 0 {
			return openai.ToolChoiceOptionFunctionToolChoice(
				openai.ChatCompletionNamedToolChoiceFunctionParam{
					Name: config.AllowedFunctionNames[0],
				},
			)
		}
		// Any mode without specific functions - map to auto
		return openai.ChatCompletionToolChoiceOptionUnionParam{
			OfAuto: openai.Opt("auto"),
		}
	case genai.FunctionCallingConfigModeNone:
		// OpenAI's "none" equivalent - just don't include tools
		return openai.ChatCompletionToolChoiceOptionUnionParam{
			OfAuto: openai.Opt("auto"),
		}
	default:
		return openai.ChatCompletionToolChoiceOptionUnionParam{
			OfAuto: openai.Opt("auto"),
		}
	}
}

// ConvertGoogleToAnthropicRequest converts Google Content and config to Anthropic format
func ConvertGoogleToAnthropicRequest(model string, contents []*genai.Content, config *genai.GenerateContentConfig) anthropic.MessageNewParams {
	params := anthropic.MessageNewParams{
		Model:     anthropic.Model(model),
		Messages:  []anthropic.MessageParam{},
		MaxTokens: int64(4096), // Default
	}

	// Set max_tokens
	if config != nil && config.MaxOutputTokens > 0 {
		params.MaxTokens = int64(config.MaxOutputTokens)
	}

	// Convert contents
	var systemParts []string

	for _, content := range contents {
		if content.Role == "system" {
			// System message → system instruction
			systemText := ConvertGooglePartsToString(content.Parts)
			if systemText != "" {
				systemParts = append(systemParts, systemText)
			}
		} else {
			// convertGoogleContentToAnthropic always returns a constructed
			// message (falling back to an empty user message), so no
			// marshal-based validity check is needed.
			params.Messages = append(params.Messages, convertGoogleContentToAnthropic(content))
		}
	}

	// Add system parts if any
	if len(systemParts) > 0 {
		params.System = make([]anthropic.TextBlockParam, len(systemParts))
		for i, part := range systemParts {
			params.System[i] = anthropic.TextBlockParam{Text: part}
		}
	}

	// Convert tools
	if config != nil && len(config.Tools) > 0 {
		for _, tool := range config.Tools {
			if len(tool.FunctionDeclarations) > 0 {
				params.Tools = ConvertGoogleToolsToAnthropic(tool.FunctionDeclarations)
				break
			}
		}
	}

	// Convert tool choice
	if config != nil && config.ToolConfig != nil && config.ToolConfig.FunctionCallingConfig != nil {
		params.ToolChoice = ConvertGoogleToolChoiceToAnthropic(config.ToolConfig.FunctionCallingConfig)
	}

	return params
}

// convertGoogleContentToAnthropic converts a Google Content to Anthropic message format
func convertGoogleContentToAnthropic(content *genai.Content) anthropic.MessageParam {
	var blocks []anthropic.ContentBlockParamUnion

	for _, part := range content.Parts {
		// Handle text parts
		if part.Text != "" {
			blocks = append(blocks, anthropic.NewTextBlock(part.Text))
		}

		// Handle function calls
		if part.FunctionCall != nil {
			blocks = append(blocks,
				anthropic.NewToolUseBlock(part.FunctionCall.ID, part.FunctionCall.Args, part.FunctionCall.Name),
			)
		}

		// Handle function responses (tool results)
		if part.FunctionResponse != nil {
			// Convert to tool_result block (in USER role)
			resultText := ""

			// Google's FunctionResponse has a Response field with the actual data
			if part.FunctionResponse.Response != nil {
				// Check if it has "output" key, if so, use that directly
				if output, exists := part.FunctionResponse.Response["output"]; exists {
					if outputStr, ok := output.(string); ok {
						resultText = outputStr
					} else {
						// Output is not a string, marshal the whole response
						if responseBytes, err := json.Marshal(part.FunctionResponse.Response); err == nil {
							resultText = string(responseBytes)
						}
					}
				} else {
					// No "output" key, use the whole response as JSON
					if responseBytes, err := json.Marshal(part.FunctionResponse.Response); err == nil {
						resultText = string(responseBytes)
					}
				}
			}

			// Return as user message with tool_result
			return anthropic.NewUserMessage(
				anthropic.NewToolResultBlock(part.FunctionResponse.Name, resultText, false),
			)
		}
	}

	// Build the message based on role
	if content.Role == "user" {
		return anthropic.NewUserMessage(blocks...)
	} else if content.Role == "model" {
		return anthropic.NewAssistantMessage(blocks...)
	}

	return anthropic.NewUserMessage()
}

func ConvertGoogleToolsToAnthropic(funcs []*genai.FunctionDeclaration) []anthropic.ToolUnionParam {
	if len(funcs) == 0 {
		return nil
	}

	out := make([]anthropic.ToolUnionParam, 0, len(funcs))

	for _, f := range funcs {
		// Convert Google Schema to Anthropic input schema
		var inputSchema anthropic.ToolInputSchemaParam
		if f.Parameters != nil {
			if schemaBytes, err := json.Marshal(f.Parameters); err == nil {
				_ = json.Unmarshal(schemaBytes, &inputSchema)
			}
		}

		tool := anthropic.ToolUnionParam{
			OfTool: &anthropic.ToolParam{
				Name:        f.Name,
				Description: anthropic.Opt(f.Description),
				InputSchema: inputSchema,
			},
		}
		out = append(out, tool)
	}

	return out
}

func ConvertGoogleToolChoiceToAnthropic(config *genai.FunctionCallingConfig) anthropic.ToolChoiceUnionParam {
	switch config.Mode {
	case genai.FunctionCallingConfigModeAuto:
		return anthropic.ToolChoiceUnionParam{
			OfAuto: &anthropic.ToolChoiceAutoParam{},
		}
	case genai.FunctionCallingConfigModeAny:
		if len(config.AllowedFunctionNames) > 0 {
			return anthropic.ToolChoiceParamOfTool(config.AllowedFunctionNames[0])
		}
		// Any mode without specific functions - map to auto
		return anthropic.ToolChoiceUnionParam{
			OfAuto: &anthropic.ToolChoiceAutoParam{},
		}
	case genai.FunctionCallingConfigModeNone:
		// Anthropic doesn't have a direct "none" mode, so we don't pass tools
		return anthropic.ToolChoiceUnionParam{
			OfAuto: &anthropic.ToolChoiceAutoParam{},
		}
	default:
		return anthropic.ToolChoiceUnionParam{
			OfAuto: &anthropic.ToolChoiceAutoParam{},
		}
	}
}

// ConvertGooglePartsToString converts Google parts to a single string
func ConvertGooglePartsToString(parts []*genai.Part) string {
	var result strings.Builder
	for _, part := range parts {
		if part.Text != "" {
			result.WriteString(part.Text)
		}
	}
	return result.String()
}
