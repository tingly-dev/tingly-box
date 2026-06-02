package nonstream

import (
	"encoding/json"
	"time"

	"github.com/anthropics/anthropic-sdk-go"
	"github.com/tingly-dev/tingly-box/internal/protocol/wire"
)

// ConvertAnthropicToOpenAIResponse converts an Anthropic BetaMessage to the
// OpenAI Chat Completions wire format.
func ConvertAnthropicToOpenAIResponse(anthropicResp *anthropic.BetaMessage, responseModel string) wire.ChatCompletionWire {
	var toolCalls []wire.ChatCompletionToolCallWire
	var textContent string
	var thinking string

	for _, block := range anthropicResp.Content {
		switch block.Type {
		case "text":
			textContent += block.Text
		case "tool_use":
			argsJSON, _ := json.Marshal(block.Input)
			toolCalls = append(toolCalls, wire.ChatCompletionToolCallWire{
				ID:   block.ID,
				Type: "function",
				Function: wire.ChatCompletionFunctionWire{
					Name:      block.Name,
					Arguments: string(argsJSON),
				},
			})
		case "thinking":
			thinking += block.Text
		}
	}

	finishReason := "stop"
	switch anthropicResp.StopReason {
	case "tool_use":
		finishReason = "tool_calls"
	case "max_tokens":
		finishReason = "length"
	}

	msg := wire.ChatCompletionMessageWire{
		Role:             string(anthropicResp.Role),
		Content:          textContent,
		ReasoningContent: thinking,
	}
	if len(toolCalls) > 0 {
		msg.ToolCalls = toolCalls
	}

	// OpenAI wire: prompt_tokens = total (uncached + cache_read + cache_creation).
	promptTokens := anthropicResp.Usage.InputTokens +
		anthropicResp.Usage.CacheReadInputTokens +
		anthropicResp.Usage.CacheCreationInputTokens
	usage := wire.ChatCompletionUsageWire{
		PromptTokens:     promptTokens,
		CompletionTokens: anthropicResp.Usage.OutputTokens,
		TotalTokens:      promptTokens + anthropicResp.Usage.OutputTokens,
	}
	if anthropicResp.Usage.CacheReadInputTokens > 0 {
		usage.PromptTokensDetails = &wire.ChatCompletionPromptDetailsWire{
			CachedTokens: anthropicResp.Usage.CacheReadInputTokens,
		}
	}

	return wire.ChatCompletionWire{
		ID:      anthropicResp.ID,
		Object:  "chat.completion",
		Created: time.Now().Unix(),
		Model:   responseModel,
		Choices: []wire.ChatCompletionChoiceWire{
			{
				Index:        0,
				Message:      msg,
				FinishReason: finishReason,
			},
		},
		Usage: usage,
	}
}
