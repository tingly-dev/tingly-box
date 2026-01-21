package adaptor

import (
	"encoding/json"
	"time"

	"github.com/anthropics/anthropic-sdk-go"

	"tingly-box/internal/typ"
	"tingly-box/pkg/adaptor/extension"
)

// ConvertAnthropicToOpenAIResponse converts an Anthropic response to OpenAI format
func ConvertAnthropicToOpenAIResponse(
	anthropicResp *anthropic.Message,
	responseModel string,
) map[string]interface{} {

	message := make(map[string]interface{})
	var toolCalls []map[string]interface{}
	var textContent string
	var thinking string

	// Walk Anthropic content blocks
	for _, block := range anthropicResp.Content {

		switch block.Type {

		case "text":
			textContent += block.Text

		case "tool_use":
			// Anthropic → OpenAI tool call
			toolCalls = append(toolCalls, map[string]interface{}{
				"id":   block.ID,
				"type": "function",
				"function": map[string]interface{}{
					"name":      block.Name,
					"arguments": block.Input, // map[string]any (NOT stringified yet)
				},
			})

		case "thinking":
			// Collect thinking content for reasoning_content field
			thinking += block.Text
		}
	}

	// OpenAI expects arguments as STRING
	for _, tc := range toolCalls {
		fn := tc["function"].(map[string]interface{})
		if args, ok := fn["arguments"]; ok {
			if b, err := json.Marshal(args); err == nil {
				fn["arguments"] = string(b)
			}
		}
	}

	// Set role from Anthropic response (required by OpenAI format)
	message["role"] = string(anthropicResp.Role)

	if textContent != "" {
		message["content"] = textContent
	}
	if len(toolCalls) > 0 {
		message["tool_calls"] = toolCalls
	}
	// Add reasoning_content if thinking blocks were present
	if thinking != "" {
		message["reasoning_content"] = thinking
	}

	// Map stop reason
	finishReason := "stop"
	switch anthropicResp.StopReason {
	case "tool_use":
		finishReason = "tool_calls"
	case "max_tokens":
		finishReason = "length"
	}

	response := map[string]interface{}{
		"id":      anthropicResp.ID,
		"object":  "chat.completion",
		"created": time.Now().Unix(),
		"model":   responseModel,
		"choices": []map[string]interface{}{
			{
				"index":         0,
				"message":       message,
				"finish_reason": finishReason,
			},
		},
		"usage": map[string]interface{}{
			"prompt_tokens":     anthropicResp.Usage.InputTokens,
			"completion_tokens": anthropicResp.Usage.OutputTokens,
			"total_tokens":      anthropicResp.Usage.InputTokens + anthropicResp.Usage.OutputTokens,
		},
	}

	return response
}

// ConvertAnthropicToOpenAIResponseWithProvider converts an Anthropic response to OpenAI format
// and applies provider-specific transformations to the response
func ConvertAnthropicToOpenAIResponseWithProvider(
	anthropicResp *anthropic.Message,
	responseModel string,
	provider *typ.Provider,
	model string,
) map[string]interface{} {
	// Base conversion
	openaiResp := ConvertAnthropicToOpenAIResponse(anthropicResp, responseModel)

	// Apply provider-specific transformations using the transform system
	return extension.ApplyResponseTransforms(openaiResp, provider, model)
}
