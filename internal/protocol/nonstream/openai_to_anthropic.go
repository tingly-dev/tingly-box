package nonstream

import (
	"encoding/json"
	"fmt"
	"time"

	"github.com/anthropics/anthropic-sdk-go"
	"github.com/google/uuid"
	"github.com/openai/openai-go/v3"
	"github.com/openai/openai-go/v3/responses"
)

func ConvertOpenAIToAnthropicResponse(openaiResp *openai.ChatCompletion, model string) anthropic.Message {
	// Create the response as JSON first, then unmarshal into Message
	// This is a workaround for the complex union types
	responseJSON := map[string]interface{}{
		"id":            fmt.Sprintf("msg_%d", time.Now().Unix()),
		"type":          "message",
		"role":          "assistant",
		"content":       []map[string]interface{}{},
		"model":         model,
		"stop_reason":   "end_turn",
		"stop_sequence": "",
		"usage": map[string]interface{}{
			"input_tokens":  openaiResp.Usage.PromptTokens,
			"output_tokens": openaiResp.Usage.CompletionTokens,
		},
	}

	// Preserve server_tool_use from ExtraFields if present
	if openaiResp.JSON.ExtraFields != nil {
		if serverToolUse, exists := openaiResp.JSON.ExtraFields["server_tool_use"]; exists && serverToolUse.Valid() {
			responseJSON["server_tool_use"] = serverToolUse.Raw()
		}
	}

	// Add content from OpenAI response
	var contentBlocks []anthropic.ContentBlockParamUnion
	for _, choice := range openaiResp.Choices {
		// Handle refusal (when model refuses to respond due to safety policies)
		if choice.Message.Refusal != "" {
			contentBlocks = append(contentBlocks, anthropic.NewTextBlock(choice.Message.Refusal))
		}

		// Add text content if present
		if choice.Message.Content != "" {
			contentBlocks = append(contentBlocks, anthropic.NewTextBlock(choice.Message.Content))
		}

		if extra := choice.Message.JSON.ExtraFields; extra != nil {
			if thinking, ok := extra["reasoning_content"]; ok {
				// a fake signature for thinking block
				contentBlocks = append(contentBlocks, anthropic.NewThinkingBlock("thinking-"+uuid.New().String()[0:6], fmt.Sprintf("%s", thinking.Raw())))
			}
		}

		// Convert tool_calls to tool_use blocks
		if len(choice.Message.ToolCalls) > 0 {
			for _, toolCall := range choice.Message.ToolCalls {
				contentBlocks = append(contentBlocks, anthropic.NewToolUseBlock(toolCall.ID, toolCall.Function.Arguments, toolCall.Function.Name))
			}

			// If there were tool calls, set stop_reason to tool_use
			if choice.FinishReason == "tool_calls" {
				responseJSON["stop_reason"] = "tool_use"
			}
		}
		break
	}

	responseJSON["content"] = contentBlocks

	// Marshal and unmarshal to create proper Message struct
	jsonBytes, _ := json.Marshal(responseJSON)
	var msg anthropic.Message
	json.Unmarshal(jsonBytes, &msg)

	return msg
}

// ConvertOpenAIToAnthropicBetaResponse converts OpenAI response to Anthropic beta format
func ConvertOpenAIToAnthropicBetaResponse(openaiResp *openai.ChatCompletion, model string) anthropic.BetaMessage {
	// Create the response as JSON first, then unmarshal into BetaMessage
	// This is a workaround for the complex union types
	responseJSON := map[string]interface{}{
		"id":            fmt.Sprintf("msg_%d", time.Now().Unix()),
		"type":          "message",
		"role":          "assistant",
		"content":       []map[string]interface{}{},
		"model":         model,
		"stop_reason":   string(anthropic.BetaStopReasonEndTurn),
		"stop_sequence": "",
		"usage": map[string]interface{}{
			"input_tokens":  openaiResp.Usage.PromptTokens,
			"output_tokens": openaiResp.Usage.CompletionTokens,
		},
	}

	// Preserve server_tool_use from ExtraFields if present
	if openaiResp.JSON.ExtraFields != nil {
		if serverToolUse, exists := openaiResp.JSON.ExtraFields["server_tool_use"]; exists && serverToolUse.Valid() {
			responseJSON["server_tool_use"] = serverToolUse.Raw()
		}
	}

	// Add content from OpenAI response
	var contentBlocks []anthropic.BetaContentBlockParamUnion
	for _, choice := range openaiResp.Choices {
		// Handle refusal (when model refuses to respond due to safety policies)
		if choice.Message.Refusal != "" {
			contentBlocks = append(contentBlocks, anthropic.NewBetaTextBlock(choice.Message.Refusal))
		}

		// Add text content if present
		if choice.Message.Content != "" {
			contentBlocks = append(contentBlocks, anthropic.NewBetaTextBlock(choice.Message.Content))
		}

		if extra := choice.Message.JSON.ExtraFields; extra != nil {
			if thinking, ok := extra["reasoning_content"]; ok {
				// a fake signature for thinking block
				contentBlocks = append(contentBlocks, anthropic.NewBetaThinkingBlock("thinking-"+uuid.New().String()[0:6], fmt.Sprintf("%s", thinking.Raw())))
			}
		}

		// Convert tool_calls to tool_use blocks
		if len(choice.Message.ToolCalls) > 0 {
			for _, toolCall := range choice.Message.ToolCalls {
				contentBlocks = append(contentBlocks, anthropic.NewBetaToolUseBlock(toolCall.ID, toolCall.Function.Arguments, toolCall.Function.Name))
			}

			// If there were tool calls, set stop_reason to tool_use
			if choice.FinishReason == "tool_calls" {
				responseJSON["stop_reason"] = string(anthropic.BetaStopReasonToolUse)
			}
		}
		break
	}

	responseJSON["content"] = contentBlocks

	// Marshal and unmarshal to create proper BetaMessage struct
	jsonBytes, _ := json.Marshal(responseJSON)
	var msg anthropic.BetaMessage
	json.Unmarshal(jsonBytes, &msg)

	return msg
}

// ConvertResponsesToAnthropicBetaResponse converts OpenAI Responses API response to Anthropic beta format
func ConvertResponsesToAnthropicBetaResponse(responsesResp *responses.Response, model string) anthropic.BetaMessage {
	// Create the response as JSON first, then unmarshal into BetaMessage
	responseJSON := map[string]interface{}{
		"id":            responsesResp.ID,
		"type":          "message",
		"role":          "assistant",
		"content":       []map[string]interface{}{},
		"model":         model,
		"stop_reason":   string(anthropic.BetaStopReasonEndTurn),
		"stop_sequence": "",
		"usage": map[string]interface{}{
			"input_tokens":  responsesResp.Usage.InputTokens,
			"output_tokens": responsesResp.Usage.OutputTokens,
		},
	}

	// Add content from Responses API response
	// The Responses API has a different output structure
	var contentBlocks []anthropic.BetaContentBlockParamUnion

	// Process the output array from Responses API
	for _, output := range responsesResp.Output {
		// Handle text content
		for _, content := range output.Content {
			if content.Type == "output_text" {
				contentBlocks = append(contentBlocks, anthropic.NewBetaTextBlock(content.Text))
			}
			// Handle other content types as needed
		}

		// Handle tool calls (function_call, custom_tool_call, mcp_call)
		if output.Type == "function_call" || output.Type == "custom_tool_call" || output.Type == "mcp_call" {
			contentBlocks = append(contentBlocks, anthropic.NewBetaToolUseBlock(
				output.ID,
				output.Arguments,
				output.Name,
			))

			// If there were tool calls, set stop_reason to tool_use
			responseJSON["stop_reason"] = string(anthropic.BetaStopReasonToolUse)
		}
	}

	// Handle reasoning content if present
	for _, output := range responsesResp.Output {
		for _, content := range output.Content {
			if content.Type == "reasoning_text" && content.Text != "" {
				contentBlocks = append(contentBlocks, anthropic.NewBetaThinkingBlock(
					"thinking-"+uuid.New().String()[0:6],
					content.Text,
				))
			}
		}
	}

	// Handle refusal if present
	for _, output := range responsesResp.Output {
		for _, content := range output.Content {
			if content.Type == "refusal" && content.Text != "" {
				contentBlocks = append(contentBlocks, anthropic.NewBetaTextBlock(content.Text))
				// Set stop_reason to refusal if present
				responseJSON["stop_reason"] = string(anthropic.BetaStopReasonRefusal)
			}
		}
	}

	responseJSON["content"] = contentBlocks

	// Marshal and unmarshal to create proper BetaMessage struct
	jsonBytes, _ := json.Marshal(responseJSON)
	var msg anthropic.BetaMessage
	json.Unmarshal(jsonBytes, &msg)

	return msg
}

// ConvertResponsesToAnthropicV1Response converts OpenAI Responses API response to Anthropic v1 format
func ConvertResponsesToAnthropicV1Response(responsesResp *responses.Response, model string) anthropic.Message {
	// Create the response as JSON first, then unmarshal into Message
	responseJSON := map[string]interface{}{
		"id":            responsesResp.ID,
		"type":          "message",
		"role":          "assistant",
		"content":       []map[string]interface{}{},
		"model":         model,
		"stop_reason":   "end_turn",
		"stop_sequence": "",
		"usage": map[string]interface{}{
			"input_tokens":  responsesResp.Usage.InputTokens,
			"output_tokens": responsesResp.Usage.OutputTokens,
		},
	}

	// Add content from Responses API response
	// The Responses API has a different output structure
	var contentBlocks []anthropic.ContentBlockParamUnion

	// Process the output array from Responses API
	for _, output := range responsesResp.Output {
		// Handle text content
		for _, content := range output.Content {
			if content.Type == "output_text" {
				contentBlocks = append(contentBlocks, anthropic.NewTextBlock(content.Text))
			}
			// Handle other content types as needed
		}

		// Handle tool calls (function_call, custom_tool_call, mcp_call)
		if output.Type == "function_call" || output.Type == "custom_tool_call" || output.Type == "mcp_call" {
			contentBlocks = append(contentBlocks, anthropic.NewToolUseBlock(
				output.ID,
				output.Arguments,
				output.Name,
			))

			// If there were tool calls, set stop_reason to tool_use
			responseJSON["stop_reason"] = "tool_use"
		}
	}

	// Handle reasoning content if present
	for _, output := range responsesResp.Output {
		for _, content := range output.Content {
			if content.Type == "reasoning_text" && content.Text != "" {
				contentBlocks = append(contentBlocks, anthropic.NewThinkingBlock(
					"thinking-"+uuid.New().String()[0:6],
					content.Text,
				))
			}
		}
	}

	// Handle refusal if present
	for _, output := range responsesResp.Output {
		for _, content := range output.Content {
			if content.Type == "refusal" && content.Text != "" {
				contentBlocks = append(contentBlocks, anthropic.NewTextBlock(content.Text))
				// Set stop_reason to refusal if present
				responseJSON["stop_reason"] = "refusal"
			}
		}
	}

	responseJSON["content"] = contentBlocks

	// Marshal and unmarshal to create proper Message struct
	jsonBytes, _ := json.Marshal(responseJSON)
	var msg anthropic.Message
	json.Unmarshal(jsonBytes, &msg)

	return msg
}
