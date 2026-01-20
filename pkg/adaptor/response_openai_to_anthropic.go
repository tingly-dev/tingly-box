package adaptor

import (
	"encoding/json"
	"fmt"
	"time"

	"github.com/anthropics/anthropic-sdk-go"
	"github.com/google/uuid"
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
	var contentBlocks []anthropic.ContentBlockParamUnion
	for _, choice := range openaiResp.Choices {
		// Add text content if present
		if choice.Message.Content != "" {
			contentBlocks = append(contentBlocks, anthropic.NewTextBlock(choice.Message.Content))
		}

		if extra := choice.Message.JSON.ExtraFields; extra != nil {
			if thinking, ok := extra["reasoning_content"]; ok {
				// a fake signature
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

	// Add content from OpenAI response
	var contentBlocks []anthropic.BetaContentBlockParamUnion
	for _, choice := range openaiResp.Choices {
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
