package request

import (
	"encoding/json"
	"strings"

	"github.com/anthropics/anthropic-sdk-go"
	"github.com/openai/openai-go/v3"
	"github.com/openai/openai-go/v3/packages/param"
	"github.com/openai/openai-go/v3/shared"
	"github.com/tingly-dev/tingly-box/internal/protocol/transformer"
)

// ConvertAnthropicBetaToOpenAIRequest converts Anthropic beta request to OpenAI format
// Returns the OpenAI request and a config object with metadata for provider transforms
func ConvertAnthropicBetaToOpenAIRequest(anthropicReq *anthropic.BetaMessageNewParams, compatible bool, isStreaming bool, disableStreamUsage bool) (*openai.ChatCompletionNewParams, *transformer.OpenAIConfig) {
	openaiReq := &openai.ChatCompletionNewParams{
		Model: openai.ChatModel(anthropicReq.Model),
	}

	isThinking := IsThinkingEnabledBeta(anthropicReq)

	// Set MaxTokens
	openaiReq.MaxTokens = openai.Opt(anthropicReq.MaxTokens)

	// Convert messages
	for _, msg := range anthropicReq.Messages {
		if string(msg.Role) == "user" {
			// User messages may contain tool_result blocks - need special handling
			messages := convertAnthropicBetaUserMessageToOpenAI(msg)
			openaiReq.Messages = append(openaiReq.Messages, messages...)
		} else if string(msg.Role) == "assistant" {
			// Convert assistant message with potential tool_use blocks
			openaiMsg := convertAnthropicBetaAssistantMessageToOpenAI(msg)
			openaiReq.Messages = append(openaiReq.Messages, openaiMsg)
		}
	}

	// Convert system message
	if len(anthropicReq.System) > 0 {
		systemStr := ConvertBetaTextBlocksToString(anthropicReq.System)
		systemMsg := openai.SystemMessage(systemStr)
		// Add system message at the beginning
		openaiReq.Messages = append([]openai.ChatCompletionMessageParamUnion{systemMsg}, openaiReq.Messages...)
	}

	// Convert tools from Anthropic format to OpenAI format
	if len(anthropicReq.Tools) > 0 {
		if compatible {
			openaiReq.Tools = ConvertAnthropicBetaToolsToOpenAIWithTransformedSchema(anthropicReq.Tools)
		} else {
			openaiReq.Tools = ConvertAnthropicBetaToolsToOpenAI(anthropicReq.Tools)
		}
		// Convert tool choice
		openaiReq.ToolChoice = ConvertAnthropicBetaToolChoiceToOpenAI(&anthropicReq.ToolChoice)
	}

	config := &transformer.OpenAIConfig{
		HasThinking:     isThinking,
		ReasoningEffort: "low", // Default to "low" for OpenAI-compatible APIs
	}

	// Only set stream_options for streaming requests (per OpenAI API spec)
	if isStreaming && !disableStreamUsage {
		openaiReq.StreamOptions.IncludeUsage = param.Opt[bool]{Value: true}
	}
	return openaiReq, config
}

// ConvertAnthropicBetaToolsToOpenAI converts Anthropic beta tools to OpenAI format
func ConvertAnthropicBetaToolsToOpenAI(tools []anthropic.BetaToolUnionParam) []openai.ChatCompletionToolUnionParam {
	if len(tools) == 0 {
		return nil
	}

	out := make([]openai.ChatCompletionToolUnionParam, 0, len(tools))

	for _, t := range tools {
		tool := t.OfTool
		if tool == nil {
			continue
		}

		// Convert Anthropic input schema to OpenAI function parameters
		var parameters map[string]interface{}
		if tool.InputSchema.Properties != nil || len(tool.InputSchema.Required) > 0 {
			parameters = make(map[string]interface{})
			parameters["type"] = "object"

			if tool.InputSchema.Properties != nil {
				parameters["properties"] = tool.InputSchema.Properties
			}

			if len(tool.InputSchema.Required) > 0 {
				parameters["required"] = tool.InputSchema.Required
			}
		}

		// Create function with parameters
		fn := shared.FunctionDefinitionParam{
			Name:        tool.Name,
			Description: param.Opt[string]{Value: tool.Description.Value},
			Parameters:  parameters,
		}

		out = append(out, openai.ChatCompletionFunctionTool(fn))
	}

	return out
}

// ConvertAnthropicBetaToolsToOpenAIWithTransformedSchema is an alias for ConvertAnthropicBetaToolsToOpenAI
// Schema transformation is handled by provider-specific transforms
func ConvertAnthropicBetaToolsToOpenAIWithTransformedSchema(tools []anthropic.BetaToolUnionParam) []openai.ChatCompletionToolUnionParam {
	return ConvertAnthropicBetaToolsToOpenAI(tools)
}

// ConvertAnthropicBetaToolChoiceToOpenAI converts Anthropic beta tool_choice to OpenAI format
func ConvertAnthropicBetaToolChoiceToOpenAI(tc *anthropic.BetaToolChoiceUnionParam) openai.ChatCompletionToolChoiceOptionUnionParam {
	if tc.OfAuto != nil {
		return openai.ChatCompletionToolChoiceOptionUnionParam{
			OfAuto: openai.Opt("auto"),
		}
	}

	if tc.OfTool != nil {
		return openai.ToolChoiceOptionFunctionToolChoice(
			openai.ChatCompletionNamedToolChoiceFunctionParam{
				Name: tc.OfTool.Name,
			},
		)
	}

	// OfAny (Anthropic's "required") - map to auto as OpenAI doesn't have direct equivalent
	// In the future, we could use OfAllowedTools with all tools listed to achieve similar behavior
	if tc.OfAny != nil {
		return openai.ChatCompletionToolChoiceOptionUnionParam{
			OfAuto: openai.Opt("auto"),
		}
	}

	// Default to auto
	return openai.ChatCompletionToolChoiceOptionUnionParam{
		OfAuto: openai.Opt("auto"),
	}
}

// ConvertBetaTextBlocksToString converts Anthropic beta TextBlockParam array to string
func ConvertBetaTextBlocksToString(blocks []anthropic.BetaTextBlockParam) string {
	var result strings.Builder
	for _, block := range blocks {
		result.WriteString(block.Text)
	}
	return result.String()
}

// ConvertBetaContentBlocksToString converts Anthropic beta content blocks to string
func ConvertBetaContentBlocksToString(blocks []anthropic.BetaContentBlockParamUnion) string {
	var result strings.Builder
	for _, block := range blocks {
		// Use the AsText helper if available, or check the type
		if block.OfText != nil {
			result.WriteString(block.OfText.Text)
		}
	}
	return result.String()
}

// convertAnthropicBetaAssistantMessageToOpenAI converts Anthropic beta assistant message to OpenAI format
// Note: thinking content is preserved in "x_thinking" field for provider-specific transforms
func convertAnthropicBetaAssistantMessageToOpenAI(msg anthropic.BetaMessageParam) openai.ChatCompletionMessageParamUnion {
	var textContent string
	var toolCalls []map[string]interface{}
	var thinking string

	// Process content blocks
	for _, block := range msg.Content {
		if block.OfText != nil {
			textContent += block.OfText.Text
		} else if block.OfToolUse != nil {
			// Convert tool_use block to OpenAI tool_call format
			toolCall := map[string]interface{}{
				"id":   block.OfToolUse.ID,
				"type": "function",
				"function": map[string]interface{}{
					"name": block.OfToolUse.Name,
				},
			}
			// Marshal input to JSON string for OpenAI
			if argsBytes, err := json.Marshal(block.OfToolUse.Input); err == nil {
				toolCall["function"].(map[string]interface{})["arguments"] = string(argsBytes)
			}
			toolCalls = append(toolCalls, toolCall)
		} else if block.OfThinking != nil {
			thinking = block.OfThinking.Thinking
		}
	}

	// Build the message based on what we have
	if len(toolCalls) > 0 {
		// Use JSON marshaling to create a message with tool_calls
		msgMap := map[string]interface{}{
			"role":    "assistant",
			"content": textContent,
		}
		msgMap["tool_calls"] = toolCalls

		msgBytes, _ := json.Marshal(msgMap)
		var result openai.ChatCompletionMessageParamUnion
		_ = json.Unmarshal(msgBytes, &result)

		// Preserve x_thinking in ExtraFields for provider transforms (e.g., DeepSeek/Moonshot)
		// Always add the field, even if empty, for consistency
		extraFields := result.ExtraFields()
		if extraFields == nil {
			extraFields = map[string]any{}
		}
		extraFields["x_thinking"] = thinking
		result.SetExtraFields(extraFields)

		return result
	}

	// Simple text message
	msgMap := map[string]interface{}{
		"role":    "assistant",
		"content": textContent,
	}
	msgBytes, _ := json.Marshal(msgMap)
	var result openai.ChatCompletionMessageParamUnion
	_ = json.Unmarshal(msgBytes, &result)

	// Preserve x_thinking in ExtraFields for provider transforms
	// Always add the field, even if empty, for consistency
	extraFields := result.ExtraFields()
	if extraFields == nil {
		extraFields = map[string]any{}
	}
	extraFields["x_thinking"] = thinking
	result.SetExtraFields(extraFields)

	return result
}

// convertAnthropicBetaUserMessageToOpenAI converts Anthropic beta user message to OpenAI format
func convertAnthropicBetaUserMessageToOpenAI(msg anthropic.BetaMessageParam) []openai.ChatCompletionMessageParamUnion {
	var result []openai.ChatCompletionMessageParamUnion
	var textContent string
	var hasToolResult bool

	// First, check if there are any tool_result blocks
	for _, block := range msg.Content {
		if block.OfToolResult != nil {
			hasToolResult = true
			break
		}
	}

	// Process content blocks
	if hasToolResult {
		// When there are tool_result blocks, we need to create separate messages
		for _, block := range msg.Content {
			if block.OfText != nil {
				textContent += block.OfText.Text
			} else if block.OfToolResult != nil {
				// Convert tool_result to OpenAI role="tool" message
				// Truncate tool_call_id to meet OpenAI's 40 character limit
				truncatedID := truncateToolCallID(block.OfToolResult.ToolUseID)
				toolMsg := map[string]interface{}{
					"role":         "tool",
					"tool_call_id": truncatedID,
					"content":      convertBetaToolResultContent(block.OfToolResult.Content),
				}
				msgBytes, _ := json.Marshal(toolMsg)
				var toolResultMsg openai.ChatCompletionMessageParamUnion
				_ = json.Unmarshal(msgBytes, &toolResultMsg)
				result = append(result, toolResultMsg)
			}
		}
		// If there was text content alongside tool results, add it as a user message
		if textContent != "" {
			result = append(result, openai.UserMessage(textContent))
		}
	} else {
		// Simple text-only user message
		contentStr := ConvertBetaContentBlocksToString(msg.Content)
		if contentStr != "" {
			result = append(result, openai.UserMessage(contentStr))
		}
	}

	return result
}
