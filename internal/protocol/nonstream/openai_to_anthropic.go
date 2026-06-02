package nonstream

import (
	"encoding/json"
	"fmt"
	"time"

	"github.com/anthropics/anthropic-sdk-go"
	"github.com/google/uuid"
	"github.com/openai/openai-go/v3"
	"github.com/openai/openai-go/v3/responses"
	"github.com/tingly-dev/tingly-box/internal/protocol/wire"
)

func ConvertOpenAIToAnthropicResponse(openaiResp *openai.ChatCompletion, model string) *anthropic.BetaMessage {
	wire := wire.AnthropicMsgWire{
		ID:           fmt.Sprintf("msg_%d", time.Now().Unix()),
		Type:         "message",
		Role:         "assistant",
		Content:      []interface{}{},
		Model:        model,
		StopReason:   "end_turn",
		StopSequence: "",
		// Anthropic wire: input_tokens = uncached only; OpenAI PromptTokens = total.
		Usage: wire.AnthropicUsageWire{
			InputTokens:          openaiResp.Usage.PromptTokens - openaiResp.Usage.PromptTokensDetails.CachedTokens,
			OutputTokens:         openaiResp.Usage.CompletionTokens,
			CacheReadInputTokens: openaiResp.Usage.PromptTokensDetails.CachedTokens,
		},
	}

	// Preserve server_tool_use from ExtraFields if present
	if openaiResp.JSON.ExtraFields != nil {
		if serverToolUse, exists := openaiResp.JSON.ExtraFields["server_tool_use"]; exists && serverToolUse.Valid() {
			wire.ServerToolUse = json.RawMessage(serverToolUse.Raw())
		}
	}

	var contentBlocks []anthropic.ContentBlockParamUnion
	for _, choice := range openaiResp.Choices {
		if choice.Message.Refusal != "" {
			contentBlocks = append(contentBlocks, anthropic.NewTextBlock(choice.Message.Refusal))
		}
		if choice.Message.Content != "" {
			contentBlocks = append(contentBlocks, anthropic.NewTextBlock(choice.Message.Content))
		}
		if extra := choice.Message.JSON.ExtraFields; extra != nil {
			if thinking, ok := extra["reasoning_content"]; ok {
				contentBlocks = append(contentBlocks, anthropic.NewThinkingBlock("thinking-"+uuid.New().String()[0:6], fmt.Sprintf("%s", thinking.Raw())))
			}
		}
		for _, toolCall := range choice.Message.ToolCalls {
			var input map[string]interface{}
			if err := json.Unmarshal([]byte(toolCall.Function.Arguments), &input); err != nil {
				input = make(map[string]interface{})
			}
			contentBlocks = append(contentBlocks, anthropic.NewToolUseBlock(toolCall.ID, input, toolCall.Function.Name))
		}
		if choice.FinishReason == "tool_calls" {
			wire.StopReason = "tool_use"
		}
		break
	}
	wire.Content = contentBlocks

	jsonBytes, _ := json.Marshal(wire)
	var msg anthropic.BetaMessage
	json.Unmarshal(jsonBytes, &msg)
	return &msg
}

// ConvertOpenAIToAnthropicBetaResponse converts OpenAI response to Anthropic beta format
func ConvertOpenAIToAnthropicBetaResponse(openaiResp *openai.ChatCompletion, model string) anthropic.BetaMessage {
	wire := wire.AnthropicMsgWire{
		ID:           fmt.Sprintf("msg_%d", time.Now().Unix()),
		Type:         "message",
		Role:         "assistant",
		Content:      []interface{}{},
		Model:        model,
		StopReason:   string(anthropic.BetaStopReasonEndTurn),
		StopSequence: "",
		// Anthropic wire: input_tokens = uncached only; OpenAI PromptTokens = total.
		Usage: wire.AnthropicUsageWire{
			InputTokens:          openaiResp.Usage.PromptTokens - openaiResp.Usage.PromptTokensDetails.CachedTokens,
			OutputTokens:         openaiResp.Usage.CompletionTokens,
			CacheReadInputTokens: openaiResp.Usage.PromptTokensDetails.CachedTokens,
		},
	}

	if openaiResp.JSON.ExtraFields != nil {
		if serverToolUse, exists := openaiResp.JSON.ExtraFields["server_tool_use"]; exists && serverToolUse.Valid() {
			wire.ServerToolUse = json.RawMessage(serverToolUse.Raw())
		}
	}

	var contentBlocks []anthropic.BetaContentBlockParamUnion
	for _, choice := range openaiResp.Choices {
		if choice.Message.Refusal != "" {
			contentBlocks = append(contentBlocks, anthropic.NewBetaTextBlock(choice.Message.Refusal))
		}
		if choice.Message.Content != "" {
			contentBlocks = append(contentBlocks, anthropic.NewBetaTextBlock(choice.Message.Content))
		}
		if extra := choice.Message.JSON.ExtraFields; extra != nil {
			if thinking, ok := extra["reasoning_content"]; ok {
				contentBlocks = append(contentBlocks, anthropic.NewBetaThinkingBlock("thinking-"+uuid.New().String()[0:6], fmt.Sprintf("%s", thinking.Raw())))
			}
		}
		for _, toolCall := range choice.Message.ToolCalls {
			var input map[string]interface{}
			if err := json.Unmarshal([]byte(toolCall.Function.Arguments), &input); err != nil {
				input = make(map[string]interface{})
			}
			contentBlocks = append(contentBlocks, anthropic.NewBetaToolUseBlock(toolCall.ID, input, toolCall.Function.Name))
		}
		if choice.FinishReason == "tool_calls" {
			wire.StopReason = string(anthropic.BetaStopReasonToolUse)
		}
		break
	}
	wire.Content = contentBlocks

	jsonBytes, _ := json.Marshal(wire)
	var msg anthropic.BetaMessage
	json.Unmarshal(jsonBytes, &msg)
	return msg
}

// ConvertResponsesToAnthropicBetaResponse converts OpenAI Responses API response to Anthropic beta format
func ConvertResponsesToAnthropicBetaResponse(responsesResp *responses.Response, model string) anthropic.BetaMessage {
	wire := wire.AnthropicMsgWire{
		ID:           responsesResp.ID,
		Type:         "message",
		Role:         "assistant",
		Content:      []interface{}{},
		Model:        model,
		StopReason:   string(anthropic.BetaStopReasonEndTurn),
		StopSequence: "",
		// Anthropic wire: input_tokens = uncached only; OpenAI Responses InputTokens = total.
		Usage: wire.AnthropicUsageWire{
			InputTokens:          responsesResp.Usage.InputTokens - responsesResp.Usage.InputTokensDetails.CachedTokens,
			OutputTokens:         responsesResp.Usage.OutputTokens,
			CacheReadInputTokens: responsesResp.Usage.InputTokensDetails.CachedTokens,
		},
	}

	var contentBlocks []anthropic.BetaContentBlockParamUnion

	for _, output := range responsesResp.Output {
		for _, content := range output.Content {
			if content.Type == "output_text" {
				contentBlocks = append(contentBlocks, anthropic.NewBetaTextBlock(content.Text))
			}
		}
		if output.Type == "function_call" || output.Type == "custom_tool_call" || output.Type == "mcp_call" {
			argsStr := resolveResponsesArguments(responsesResp, output)
			var arguments map[string]interface{}
			if err := json.Unmarshal([]byte(argsStr), &arguments); err != nil {
				arguments = make(map[string]interface{})
			}
			contentBlocks = append(contentBlocks, anthropic.NewBetaToolUseBlock(output.ID, arguments, output.Name))
			wire.StopReason = string(anthropic.BetaStopReasonToolUse)
		}
	}

	for _, output := range responsesResp.Output {
		for _, content := range output.Content {
			if content.Type == "reasoning_text" && content.Text != "" {
				contentBlocks = append(contentBlocks, anthropic.NewBetaThinkingBlock("thinking-"+uuid.New().String()[0:6], content.Text))
			}
		}
	}

	for _, output := range responsesResp.Output {
		for _, content := range output.Content {
			if content.Type == "refusal" && content.Text != "" {
				contentBlocks = append(contentBlocks, anthropic.NewBetaTextBlock(content.Text))
				wire.StopReason = string(anthropic.BetaStopReasonRefusal)
			}
		}
	}

	wire.Content = contentBlocks

	jsonBytes, _ := json.Marshal(wire)
	var msg anthropic.BetaMessage
	json.Unmarshal(jsonBytes, &msg)
	return msg
}

// ConvertResponsesToAnthropicV1Response converts OpenAI Responses API response to Anthropic v1 format
func ConvertResponsesToAnthropicV1Response(responsesResp *responses.Response, model string) anthropic.Message {
	wire := wire.AnthropicMsgWire{
		ID:           responsesResp.ID,
		Type:         "message",
		Role:         "assistant",
		Content:      []interface{}{},
		Model:        model,
		StopReason:   "end_turn",
		StopSequence: "",
		// Anthropic wire: input_tokens = uncached only; OpenAI Responses InputTokens = total.
		Usage: wire.AnthropicUsageWire{
			InputTokens:          responsesResp.Usage.InputTokens - responsesResp.Usage.InputTokensDetails.CachedTokens,
			OutputTokens:         responsesResp.Usage.OutputTokens,
			CacheReadInputTokens: responsesResp.Usage.InputTokensDetails.CachedTokens,
		},
	}

	var contentBlocks []anthropic.ContentBlockParamUnion

	for _, output := range responsesResp.Output {
		for _, content := range output.Content {
			if content.Type == "output_text" {
				contentBlocks = append(contentBlocks, anthropic.NewTextBlock(content.Text))
			}
		}
		if output.Type == "function_call" || output.Type == "custom_tool_call" || output.Type == "mcp_call" {
			argsStr := resolveResponsesArguments(responsesResp, output)
			var arguments map[string]interface{}
			if err := json.Unmarshal([]byte(argsStr), &arguments); err != nil {
				arguments = make(map[string]interface{})
			}
			contentBlocks = append(contentBlocks, anthropic.NewToolUseBlock(output.ID, arguments, output.Name))
			wire.StopReason = "tool_use"
		}
	}

	for _, output := range responsesResp.Output {
		for _, content := range output.Content {
			if content.Type == "reasoning_text" && content.Text != "" {
				contentBlocks = append(contentBlocks, anthropic.NewThinkingBlock("thinking-"+uuid.New().String()[0:6], content.Text))
			}
		}
	}

	for _, output := range responsesResp.Output {
		for _, content := range output.Content {
			if content.Type == "refusal" && content.Text != "" {
				contentBlocks = append(contentBlocks, anthropic.NewTextBlock(content.Text))
				wire.StopReason = "refusal"
			}
		}
	}

	wire.Content = contentBlocks

	jsonBytes, _ := json.Marshal(wire)
	var msg anthropic.Message
	json.Unmarshal(jsonBytes, &msg)
	return msg
}

// resolveResponsesArguments extracts the arguments string from a Responses API output item.
func resolveResponsesArguments(responsesResp *responses.Response, output responses.ResponseOutputItemUnion) string {
	if output.Arguments.OfString != "" {
		return output.Arguments.OfString
	}
	if output.Arguments.OfResponseToolSearchCallArguments != nil {
		if b, err := json.Marshal(output.Arguments.OfResponseToolSearchCallArguments); err == nil {
			return string(b)
		}
	}
	return "{}"
}
