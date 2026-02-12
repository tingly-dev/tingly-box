package request

import (
	"encoding/json"
	"strings"

	"github.com/openai/openai-go/v3/packages/param"
	"github.com/openai/openai-go/v3/responses"
	"github.com/sirupsen/logrus"

	"github.com/tingly-dev/tingly-box/internal/typ"
)

// ChatGPTBackendRequest represents a request to the ChatGPT backend API.
type ChatGPTBackendRequest struct {
	Model         string        `json:"model"`
	Stream        bool          `json:"stream"`
	Instructions  string        `json:"instructions"`
	Input         []interface{} `json:"input,omitempty"`
	Tools         []interface{} `json:"tools,omitempty"`
	ToolChoice    interface{}   `json:"tool_choice,omitempty"`
	Store         bool          `json:"store"`
	Include       []string      `json:"include"`
	MaxTokens     int           `json:"max_tokens,omitempty"`
	MaxCompletion int           `json:"max_completion_tokens,omitempty"`
	Temperature   float64       `json:"temperature,omitempty"`
	TopP          float64       `json:"top_p,omitempty"`
}

// ConvertResponsesToChatGPTFormat converts OpenAI Responses API params to ChatGPT backend API format.
func ConvertResponsesToChatGPTFormat(params responses.ResponseNewParams, provider *typ.Provider) *ChatGPTBackendRequest {
	req := &ChatGPTBackendRequest{
		Model:      string(params.Model),
		Stream:     true,
		Tools:      []interface{}{},
		ToolChoice: "auto",
		Store:      false,
		Include:    []string{},
	}

	// Add instructions if present, otherwise use default
	// ChatGPT backend API requires instructions to be present
	if !param.IsOmitted(params.Instructions) {
		req.Instructions = params.Instructions.Value
	} else {
		req.Instructions = "You are a helpful AI assistant."
	}

	// Convert input to ChatGPT backend API format
	if !param.IsOmitted(params.Input.OfInputItemList) {
		req.Input = ConvertResponseInputToChatGPTFormat(params.Input.OfInputItemList)
	}

	// Convert tools to ChatGPT backend API format
	if !param.IsOmitted(params.Tools) && len(params.Tools) > 0 {
		req.Tools = ConvertResponseToolsToChatGPTFormat(params.Tools)
	}

	// Convert tool_choice if present
	if !param.IsOmitted(params.ToolChoice) {
		req.ToolChoice = ConvertResponseToolChoiceToChatGPTFormat(params.ToolChoice)
	}

	// Copy other fields if present
	// Note: Codex OAuth providers do not support max_tokens, max_completion_tokens, temperature, or top_p
	if !param.IsOmitted(params.MaxOutputTokens) {
		// Skip for Codex OAuth providers
		if !IsCodexOAuthProvider(provider) {
			if RequiresMaxCompletionTokens(string(params.Model)) {
				req.MaxCompletion = int(params.MaxOutputTokens.Value)
			} else {
				req.MaxTokens = int(params.MaxOutputTokens.Value)
			}
		}
	}
	if !param.IsOmitted(params.Temperature) {
		// Skip for Codex OAuth providers
		if !IsCodexOAuthProvider(provider) {
			req.Temperature = params.Temperature.Value
		}
	}
	if !param.IsOmitted(params.TopP) {
		// Skip for Codex OAuth providers
		if !IsCodexOAuthProvider(provider) {
			req.TopP = params.TopP.Value
		}
	}

	return req
}

// IsCodexOAuthProvider checks if the provider is a Codex OAuth provider.
// Codex OAuth providers do not support max_tokens or max_completion_tokens parameters.
func IsCodexOAuthProvider(provider *typ.Provider) bool {
	if provider == nil || provider.OAuthDetail == nil {
		return false
	}
	return provider.OAuthDetail.ProviderType == "codex"
}

// RequiresMaxCompletionTokens checks if the model requires max_completion_tokens instead of max_tokens.
// Newer OpenAI models (gpt-4o, o1 series, gpt-4.1) use max_completion_tokens.
func RequiresMaxCompletionTokens(model string) bool {
	// Models that require max_completion_tokens
	modelsRequiringMaxCompletionTokens := []string{
		"gpt-4o",
		"gpt-4o-",
		"gpt-4o-mini",
		"gpt-4o-mini-",
		"o1-",
		"o1-2024",
		"chatgpt-4o",
		"chatgpt-4o-",
		"chatgpt-4o-mini",
		"chatgpt-4o-mini-",
		"gpt-4.1",
		"gpt-4.1-",
	}

	for _, prefix := range modelsRequiringMaxCompletionTokens {
		if strings.HasPrefix(model, prefix) {
			return true
		}
	}
	return false
}

// ConvertResponseInputToChatGPTFormat converts ResponseInputParam to ChatGPT backend API format.
func ConvertResponseInputToChatGPTFormat(inputItems responses.ResponseInputParam) []interface{} {
	var result []interface{}

	for _, item := range inputItems {
		// Handle message items
		if !param.IsOmitted(item.OfMessage) {
			msg := item.OfMessage
			chatGPTItem := map[string]interface{}{
				"type": "message",
				"role": string(msg.Role),
			}

			// Determine content type based on role
			contentType := "input_text"
			if string(msg.Role) == "assistant" {
				contentType = "output_text"
			}

			// Handle content - check if it's a simple string
			if !param.IsOmitted(msg.Content.OfString) {
				// Simple string content - convert to ChatGPT format
				chatGPTItem["content"] = []map[string]string{
					{"type": contentType, "text": msg.Content.OfString.Value},
				}
			} else if !param.IsOmitted(msg.Content.OfInputItemContentList) {
				// Array content - convert each content item to ChatGPT format
				var contentItems []map[string]interface{}
				for _, contentItem := range msg.Content.OfInputItemContentList {
					if !param.IsOmitted(contentItem.OfInputText) {
						contentItems = append(contentItems, map[string]interface{}{
							"type": contentType,
							"text": contentItem.OfInputText.Text,
						})
					}
					// Handle other content types as needed (images, audio, etc.)
				}
				if len(contentItems) > 0 {
					chatGPTItem["content"] = contentItems
				}
			}

			// Only add if content was successfully set
			if _, hasContent := chatGPTItem["content"]; hasContent {
				result = append(result, chatGPTItem)
			}
			continue
		}

		// Handle function call items (tool invocations)
		if !param.IsOmitted(item.OfFunctionCall) {
			if chatGPTItem := ConvertFunctionCallToChatGPTFormat(item.OfFunctionCall); chatGPTItem != nil {
				result = append(result, chatGPTItem)
			}
			continue
		}

		// Handle function call output items (tool results)
		if !param.IsOmitted(item.OfFunctionCallOutput) {
			if chatGPTItem := ConvertFunctionCallOutputToChatGPTFormat(item.OfFunctionCallOutput); chatGPTItem != nil {
				result = append(result, chatGPTItem)
			}
		}
	}

	return result
}

// ConvertFunctionCallOutputToChatGPTFormat converts function_call_output items to ChatGPT backend format.
func ConvertFunctionCallOutputToChatGPTFormat(output *responses.ResponseInputItemFunctionCallOutputParam) map[string]interface{} {
	if output == nil {
		return nil
	}

	data, err := json.Marshal(output)
	if err != nil {
		logrus.Debugf("Failed to marshal function call output: %v", err)
		return nil
	}

	var chatGPTItem map[string]interface{}
	if err := json.Unmarshal(data, &chatGPTItem); err != nil {
		logrus.Debugf("Failed to unmarshal function call output: %v", err)
		return nil
	}

	return chatGPTItem
}

// ConvertFunctionCallToChatGPTFormat converts function_call items to ChatGPT backend format.
func ConvertFunctionCallToChatGPTFormat(call *responses.ResponseFunctionToolCallParam) map[string]interface{} {
	if call == nil {
		return nil
	}

	data, err := json.Marshal(call)
	if err != nil {
		logrus.Debugf("Failed to marshal function call: %v", err)
		return nil
	}

	var chatGPTItem map[string]interface{}
	if err := json.Unmarshal(data, &chatGPTItem); err != nil {
		logrus.Debugf("Failed to unmarshal function call: %v", err)
		return nil
	}

	return chatGPTItem
}

// ConvertResponseToolsToChatGPTFormat converts Tools from Responses API format to ChatGPT backend API format.
func ConvertResponseToolsToChatGPTFormat(tools []responses.ToolUnionParam) []interface{} {
	if len(tools) == 0 {
		return nil
	}

	result := make([]interface{}, 0, len(tools))

	for _, tool := range tools {
		// Handle function tools (custom tools for function calling)
		if !param.IsOmitted(tool.OfFunction) {
			fn := tool.OfFunction
			toolMap := map[string]interface{}{
				"type":       "function",
				"name":       fn.Name,
				"parameters": fn.Parameters,
			}
			if !param.IsOmitted(fn.Description) {
				toolMap["description"] = fn.Description.Value
			}
			if !param.IsOmitted(fn.Strict) {
				toolMap["strict"] = fn.Strict.Value
			}
			result = append(result, toolMap)
		}
		// Add other tool types as needed (web_search, file_search, etc.)
		// For now, we only support function tools
	}

	return result
}

// ConvertResponseToolChoiceToChatGPTFormat converts ToolChoice from Responses API format to ChatGPT backend API format.
func ConvertResponseToolChoiceToChatGPTFormat(toolChoice responses.ResponseNewParamsToolChoiceUnion) interface{} {
	// Handle different tool_choice variants
	if !param.IsOmitted(toolChoice.OfToolChoiceMode) {
		// "auto", "none", "required" modes
		return toolChoice.OfToolChoiceMode.Value
	}
	if !param.IsOmitted(toolChoice.OfFunctionTool) {
		// Specific function tool choice
		fn := toolChoice.OfFunctionTool
		return map[string]interface{}{
			"type": "function",
			"name": fn.Name,
		}
	}
	// Default to auto
	return "auto"
}

// ConvertRawInputToChatGPTFormat converts raw input items to ChatGPT backend API format.
func ConvertRawInputToChatGPTFormat(raw map[string]interface{}) []interface{} {
	var inputItems []interface{}

	// Get input from raw params
	inputValue, ok := raw["input"]
	if !ok {
		return nil
	}

	// Handle string input (simple text prompt)
	if inputStr, ok := inputValue.(string); ok {
		inputItems = append(inputItems, map[string]interface{}{
			"type": "message",
			"role": "user",
			"content": []map[string]string{
				{"type": "input_text", "text": inputStr},
			},
		})
		return inputItems
	}

	// Handle array input (complex messages)
	if inputArray, ok := inputValue.([]interface{}); ok {
		for _, item := range inputArray {
			if itemMap, ok := item.(map[string]interface{}); ok {
				itemType, _ := itemMap["type"].(string)

				switch itemType {
				case "message":
					// Convert message format
					role, _ := itemMap["role"].(string)
					inputItem := map[string]interface{}{
						"type": "message",
						"role": role,
					}

					// Handle content as string or array
					if contentStr, ok := itemMap["content"].(string); ok {
						contentType := "input_text"
						if role == "assistant" {
							contentType = "output_text"
						}
						inputItem["content"] = []map[string]string{
							{"type": contentType, "text": contentStr},
						}
					} else if contentArray, ok := itemMap["content"].([]interface{}); ok {
						var contentItems []map[string]interface{}
						for _, c := range contentArray {
							if cMap, ok := c.(map[string]interface{}); ok {
								cType, _ := cMap["type"].(string)
								if cType == "input_text" || cType == "output_text" {
									text, _ := cMap["text"].(string)
									contentItems = append(contentItems, map[string]interface{}{
										"type": cType,
										"text": text,
									})
								}
							}
						}
						if len(contentItems) > 0 {
							inputItem["content"] = contentItems
						}
					}

					inputItems = append(inputItems, inputItem)

				case "function_call":
					// Pass through function calls
					inputItems = append(inputItems, itemMap)

				case "function_call_output":
					// Pass through function call outputs
					inputItems = append(inputItems, itemMap)
				}
			}
		}
	}

	return inputItems
}

// ExtractInstructions extracts system message content as instructions.
func ExtractInstructions(raw map[string]interface{}) string {
	// Check for instructions field directly
	if instructions, ok := raw["instructions"].(string); ok && instructions != "" {
		return instructions
	}

	// Try to extract from system messages in input
	if inputArray, ok := raw["input"].([]interface{}); ok {
		for _, item := range inputArray {
			if itemMap, ok := item.(map[string]interface{}); ok {
				if itemType, ok := itemMap["type"].(string); ok && itemType == "message" {
					if role, ok := itemMap["role"].(string); ok && role == "system" {
						if content, ok := itemMap["content"].(string); ok {
							return content
						}
					}
				}
			}
		}
	}

	return ""
}
