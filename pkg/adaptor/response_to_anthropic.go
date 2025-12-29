package adaptor

import (
	"encoding/json"
	"fmt"
	"time"

	"github.com/anthropics/anthropic-sdk-go"
	"github.com/openai/openai-go/v3"
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

	// Add content from OpenAI response
	var contentBlocks []map[string]interface{}
	for _, choice := range openaiResp.Choices {
		// Add text content if present
		if choice.Message.Content != "" {
			contentBlocks = append(contentBlocks, map[string]interface{}{
				"type": "text",
				"text": choice.Message.Content,
			})
		}

		// Convert tool_calls to tool_use blocks
		if len(choice.Message.ToolCalls) > 0 {
			for _, toolCall := range choice.Message.ToolCalls {
				toolUseBlock := map[string]interface{}{
					"type": "tool_use",
					"id":   toolCall.ID,
					"name": toolCall.Function.Name,
				}

				// Parse function arguments
				if toolCall.Function.Arguments != "" {
					var args map[string]interface{}
					if err := json.Unmarshal([]byte(toolCall.Function.Arguments), &args); err == nil {
						toolUseBlock["input"] = args
					}
				}

				contentBlocks = append(contentBlocks, toolUseBlock)
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
