package nonstream

import (
	"encoding/json"
	"time"

	"github.com/anthropics/anthropic-sdk-go"
)

// ChatCompletionWire is the OpenAI Chat Completions response wire format.
type ChatCompletionWire struct {
	ID      string                    `json:"id"`
	Object  string                    `json:"object"`
	Created int64                     `json:"created"`
	Model   string                    `json:"model"`
	Choices []ChatCompletionChoiceWire `json:"choices"`
	Usage   ChatCompletionUsageWire   `json:"usage"`
}

// ToMap serializes to a generic map for callers that apply runtime transforms.
func (r ChatCompletionWire) ToMap() map[string]interface{} {
	raw, _ := json.Marshal(r)
	var m map[string]interface{}
	_ = json.Unmarshal(raw, &m)
	return m
}

// ChatCompletionChoiceWire is a single choice in the OpenAI Chat Completions response.
type ChatCompletionChoiceWire struct {
	Index        int                      `json:"index"`
	Message      ChatCompletionMessageWire `json:"message"`
	FinishReason string                   `json:"finish_reason"`
}

// ChatCompletionMessageWire is the message inside a choice.
type ChatCompletionMessageWire struct {
	Role             string                       `json:"role"`
	Content          string                       `json:"content,omitempty"`
	ToolCalls        []ChatCompletionToolCallWire `json:"tool_calls,omitempty"`
	ReasoningContent string                       `json:"reasoning_content,omitempty"`
}

// ChatCompletionToolCallWire is a single tool call inside a message.
type ChatCompletionToolCallWire struct {
	ID       string                      `json:"id"`
	Type     string                      `json:"type"`
	Function ChatCompletionFunctionWire  `json:"function"`
}

// ChatCompletionFunctionWire carries the function name and JSON-encoded arguments.
type ChatCompletionFunctionWire struct {
	Name      string `json:"name"`
	Arguments string `json:"arguments"`
}

// ChatCompletionUsageWire is the usage block in the OpenAI Chat Completions response.
// prompt_tokens = TOTAL (uncached + cached); cached_tokens is a reported subset.
type ChatCompletionUsageWire struct {
	PromptTokens     int64                              `json:"prompt_tokens"`
	CompletionTokens int64                              `json:"completion_tokens"`
	TotalTokens      int64                              `json:"total_tokens"`
	PromptTokensDetails *ChatCompletionPromptDetailsWire `json:"prompt_tokens_details,omitempty"`
}

// ChatCompletionPromptDetailsWire breaks down prompt token categories.
type ChatCompletionPromptDetailsWire struct {
	CachedTokens int64 `json:"cached_tokens"`
}

// ConvertAnthropicToOpenAIResponse converts an Anthropic BetaMessage to the
// OpenAI Chat Completions wire format.
func ConvertAnthropicToOpenAIResponse(anthropicResp *anthropic.BetaMessage, responseModel string) ChatCompletionWire {
	var toolCalls []ChatCompletionToolCallWire
	var textContent string
	var thinking string

	for _, block := range anthropicResp.Content {
		switch block.Type {
		case "text":
			textContent += block.Text
		case "tool_use":
			argsJSON, _ := json.Marshal(block.Input)
			toolCalls = append(toolCalls, ChatCompletionToolCallWire{
				ID:   block.ID,
				Type: "function",
				Function: ChatCompletionFunctionWire{
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

	msg := ChatCompletionMessageWire{
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
	usage := ChatCompletionUsageWire{
		PromptTokens:     promptTokens,
		CompletionTokens: anthropicResp.Usage.OutputTokens,
		TotalTokens:      promptTokens + anthropicResp.Usage.OutputTokens,
	}
	if anthropicResp.Usage.CacheReadInputTokens > 0 {
		usage.PromptTokensDetails = &ChatCompletionPromptDetailsWire{
			CachedTokens: anthropicResp.Usage.CacheReadInputTokens,
		}
	}

	return ChatCompletionWire{
		ID:      anthropicResp.ID,
		Object:  "chat.completion",
		Created: time.Now().Unix(),
		Model:   responseModel,
		Choices: []ChatCompletionChoiceWire{
			{
				Index:        0,
				Message:      msg,
				FinishReason: finishReason,
			},
		},
		Usage: usage,
	}
}
