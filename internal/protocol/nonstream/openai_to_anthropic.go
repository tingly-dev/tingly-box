package nonstream

import (
	"encoding/json"
	"fmt"

	"github.com/anthropics/anthropic-sdk-go"
	"github.com/google/uuid"
	"github.com/openai/openai-go/v3"
	"github.com/openai/openai-go/v3/responses"
	"github.com/tingly-dev/tingly-box/internal/protocol/wire"
)

func HandleOpenAIChatToAnthropic(chat *openai.ChatCompletion, model string) *anthropic.BetaMessage {
	value, err := marshalOpenAIChatToAnthropic(chat, model)
	if err != nil {
		return &anthropic.BetaMessage{}
	}
	var msg anthropic.BetaMessage
	if err := json.Unmarshal(value, &msg); err != nil {
		return &anthropic.BetaMessage{}
	}
	return &msg
}

// ConvertOpenAIChatToAnthropicV1 converts an OpenAI Chat completion to a typed
// Anthropic v1 response without writing it to an HTTP transport.
func ConvertOpenAIChatToAnthropicV1(chat *openai.ChatCompletion, model string) (*anthropic.Message, error) {
	value, err := marshalOpenAIChatToAnthropic(chat, model)
	if err != nil {
		return nil, err
	}
	var msg anthropic.Message
	if err := json.Unmarshal(value, &msg); err != nil {
		return nil, fmt.Errorf("decode Anthropic v1 response: %w", err)
	}
	return &msg, nil
}

func marshalOpenAIChatToAnthropic(chat *openai.ChatCompletion, model string) ([]byte, error) {
	if chat == nil {
		return nil, fmt.Errorf("convert OpenAI Chat response to Anthropic: response is nil")
	}
	result := wire.AnthropicMsgWire{
		ID:           "msg_" + uuid.NewString(),
		Type:         "message",
		Role:         "assistant",
		Content:      []interface{}{},
		Model:        model,
		StopReason:   "end_turn",
		StopSequence: "",
		// Anthropic wire: input_tokens = uncached only; OpenAI PromptTokens = total.
		Usage: wire.AnthropicUsageWire{
			InputTokens:          chat.Usage.PromptTokens - chat.Usage.PromptTokensDetails.CachedTokens,
			OutputTokens:         chat.Usage.CompletionTokens,
			CacheReadInputTokens: chat.Usage.PromptTokensDetails.CachedTokens,
		},
	}

	// Preserve server_tool_use from ExtraFields if present
	if chat.JSON.ExtraFields != nil {
		if serverToolUse, exists := chat.JSON.ExtraFields["server_tool_use"]; exists && serverToolUse.Valid() {
			result.ServerToolUse = json.RawMessage(serverToolUse.Raw())
		}
	}

	var contentBlocks []anthropic.ContentBlockParamUnion
	for _, choice := range chat.Choices {
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
			result.StopReason = "tool_use"
		} else if choice.FinishReason == "length" {
			result.StopReason = "max_tokens"
		}
		break
	}
	result.Content = contentBlocks

	jsonBytes, err := json.Marshal(result)
	if err != nil {
		return nil, fmt.Errorf("encode Anthropic response: %w", err)
	}
	return jsonBytes, nil
}

// HandleOpenAIChatToAnthropicBeta converts OpenAI response to Anthropic beta format
func HandleOpenAIChatToAnthropicBeta(chat *openai.ChatCompletion, model string) anthropic.BetaMessage {
	msg, err := ConvertOpenAIChatToAnthropicBeta(chat, model)
	if err != nil {
		return anthropic.BetaMessage{}
	}
	return *msg
}

// ConvertOpenAIChatToAnthropicBeta converts an OpenAI Chat completion to a
// typed Anthropic beta response without writing it to an HTTP transport.
func ConvertOpenAIChatToAnthropicBeta(chat *openai.ChatCompletion, model string) (*anthropic.BetaMessage, error) {
	if chat == nil {
		return nil, fmt.Errorf("convert OpenAI Chat response to Anthropic beta: response is nil")
	}
	result := wire.AnthropicMsgWire{
		ID:           "msg_" + uuid.NewString(),
		Type:         "message",
		Role:         "assistant",
		Content:      []interface{}{},
		Model:        model,
		StopReason:   string(anthropic.BetaStopReasonEndTurn),
		StopSequence: "",
		// Anthropic wire: input_tokens = uncached only; OpenAI PromptTokens = total.
		Usage: wire.AnthropicUsageWire{
			InputTokens:          chat.Usage.PromptTokens - chat.Usage.PromptTokensDetails.CachedTokens,
			OutputTokens:         chat.Usage.CompletionTokens,
			CacheReadInputTokens: chat.Usage.PromptTokensDetails.CachedTokens,
		},
	}

	if chat.JSON.ExtraFields != nil {
		if serverToolUse, exists := chat.JSON.ExtraFields["server_tool_use"]; exists && serverToolUse.Valid() {
			result.ServerToolUse = json.RawMessage(serverToolUse.Raw())
		}
	}

	var contentBlocks []anthropic.BetaContentBlockParamUnion
	for _, choice := range chat.Choices {
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
			result.StopReason = string(anthropic.BetaStopReasonToolUse)
		} else if choice.FinishReason == "length" {
			result.StopReason = string(anthropic.BetaStopReasonMaxTokens)
		}
		break
	}
	result.Content = contentBlocks

	jsonBytes, err := json.Marshal(result)
	if err != nil {
		return nil, fmt.Errorf("encode Anthropic beta response: %w", err)
	}
	var msg anthropic.BetaMessage
	if err := json.Unmarshal(jsonBytes, &msg); err != nil {
		return nil, fmt.Errorf("decode Anthropic beta response: %w", err)
	}
	return &msg, nil
}

// HandleResponsesToAnthropicBeta converts OpenAI Responses API response to Anthropic beta format
func HandleResponsesToAnthropicBeta(rs *responses.Response, model string) anthropic.BetaMessage {
	wire := wire.AnthropicMsgWire{
		ID:           rs.ID,
		Type:         "message",
		Role:         "assistant",
		Content:      []interface{}{},
		Model:        model,
		StopReason:   string(anthropic.BetaStopReasonEndTurn),
		StopSequence: "",
		// Anthropic wire: input_tokens = uncached only; OpenAI Responses InputTokens = total.
		Usage: wire.AnthropicUsageWire{
			InputTokens:          rs.Usage.InputTokens - rs.Usage.InputTokensDetails.CachedTokens,
			OutputTokens:         rs.Usage.OutputTokens,
			CacheReadInputTokens: rs.Usage.InputTokensDetails.CachedTokens,
		},
	}

	var contentBlocks []anthropic.BetaContentBlockParamUnion

	for _, output := range rs.Output {
		for _, content := range output.Content {
			if content.Type == "output_text" {
				contentBlocks = append(contentBlocks, anthropic.NewBetaTextBlock(content.Text))
			}
		}
		if output.Type == "function_call" || output.Type == "custom_tool_call" || output.Type == "mcp_call" {
			argsStr := resolveResponsesArguments(rs, output)
			var arguments map[string]interface{}
			if err := json.Unmarshal([]byte(argsStr), &arguments); err != nil {
				arguments = make(map[string]interface{})
			}
			contentBlocks = append(contentBlocks, anthropic.NewBetaToolUseBlock(responsesToolCallID(output), arguments, output.Name))
			wire.StopReason = string(anthropic.BetaStopReasonToolUse)
		}
	}

	for _, output := range rs.Output {
		for _, content := range output.Content {
			if content.Type == "reasoning_text" && content.Text != "" {
				contentBlocks = append(contentBlocks, anthropic.NewBetaThinkingBlock("thinking-"+uuid.New().String()[0:6], content.Text))
			}
		}
	}
	if rs.Status == "incomplete" {
		if rs.IncompleteDetails.Reason == "content_filter" {
			wire.StopReason = string(anthropic.BetaStopReasonRefusal)
		} else {
			wire.StopReason = string(anthropic.BetaStopReasonMaxTokens)
		}
	}

	for _, output := range rs.Output {
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

// HandleResponsesToAnthropicV1 converts OpenAI Responses API response to Anthropic v1 format
func HandleResponsesToAnthropicV1(rs *responses.Response, model string) anthropic.Message {
	wire := wire.AnthropicMsgWire{
		ID:           rs.ID,
		Type:         "message",
		Role:         "assistant",
		Content:      []interface{}{},
		Model:        model,
		StopReason:   "end_turn",
		StopSequence: "",
		// Anthropic wire: input_tokens = uncached only; OpenAI Responses InputTokens = total.
		Usage: wire.AnthropicUsageWire{
			InputTokens:          rs.Usage.InputTokens - rs.Usage.InputTokensDetails.CachedTokens,
			OutputTokens:         rs.Usage.OutputTokens,
			CacheReadInputTokens: rs.Usage.InputTokensDetails.CachedTokens,
		},
	}

	var contentBlocks []anthropic.ContentBlockParamUnion

	for _, output := range rs.Output {
		for _, content := range output.Content {
			if content.Type == "output_text" {
				contentBlocks = append(contentBlocks, anthropic.NewTextBlock(content.Text))
			}
		}
		if output.Type == "function_call" || output.Type == "custom_tool_call" || output.Type == "mcp_call" {
			argsStr := resolveResponsesArguments(rs, output)
			var arguments map[string]interface{}
			if err := json.Unmarshal([]byte(argsStr), &arguments); err != nil {
				arguments = make(map[string]interface{})
			}
			contentBlocks = append(contentBlocks, anthropic.NewToolUseBlock(responsesToolCallID(output), arguments, output.Name))
			wire.StopReason = "tool_use"
		}
	}

	for _, output := range rs.Output {
		for _, content := range output.Content {
			if content.Type == "reasoning_text" && content.Text != "" {
				contentBlocks = append(contentBlocks, anthropic.NewThinkingBlock("thinking-"+uuid.New().String()[0:6], content.Text))
			}
		}
	}
	if rs.Status == "incomplete" {
		if rs.IncompleteDetails.Reason == "content_filter" {
			wire.StopReason = "refusal"
		} else {
			wire.StopReason = "max_tokens"
		}
	}

	for _, output := range rs.Output {
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

func responsesToolCallID(output responses.ResponseOutputItemUnion) string {
	if output.CallID != "" {
		return output.CallID
	}
	return output.ID
}

// resolveResponsesArguments extracts the arguments string from a Responses API output item.
func resolveResponsesArguments(rs *responses.Response, output responses.ResponseOutputItemUnion) string {
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
