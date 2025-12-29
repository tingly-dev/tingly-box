package adaptor

import (
	"encoding/json"
	"strings"
	"time"

	"github.com/anthropics/anthropic-sdk-go"
	"github.com/openai/openai-go/v3"
	"github.com/openai/openai-go/v3/packages/param"
	"github.com/openai/openai-go/v3/shared"
)

// ConvertAnthropicToOpenAIResponse converts an Anthropic response to OpenAI format
func ConvertAnthropicToOpenAIResponse(
	anthropicResp *anthropic.Message,
	responseModel string,
) map[string]interface{} {

	message := make(map[string]interface{})
	var toolCalls []map[string]interface{}
	var textContent string

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

// ConvertAnthropicToolsToOpenAI converts Anthropic tools to OpenAI format
func ConvertAnthropicToolsToOpenAI(tools []anthropic.ToolUnionParam) []openai.ChatCompletionToolUnionParam {
	if len(tools) == 0 {
		return nil
	}

	out := make([]openai.ChatCompletionToolUnionParam, 0, len(tools))

	for _, t := range tools {
		tool := t.OfTool
		if tool == nil {
			continue
		}

		// Convert Anthropic input schema to OpenAI function parameters (map[string]interface{})
		var parameters map[string]interface{}
		if tool.InputSchema.Properties != nil {
			if schemaBytes, err := json.Marshal(tool.InputSchema); err == nil {
				_ = json.Unmarshal(schemaBytes, &parameters)
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

// ConvertAnthropicToolChoiceToOpenAI converts Anthropic tool_choice to OpenAI format
func ConvertAnthropicToolChoiceToOpenAI(tc *anthropic.ToolChoiceUnionParam) openai.ChatCompletionToolChoiceOptionUnionParam {
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

// ConvertAnthropicToOpenAIRequest converts Anthropic request to OpenAI format
func ConvertAnthropicToOpenAIRequest(anthropicReq *anthropic.MessageNewParams) *openai.ChatCompletionNewParams {
	openaiReq := &openai.ChatCompletionNewParams{
		Model: openai.ChatModel(anthropicReq.Model),
	}

	// Set MaxTokens
	openaiReq.MaxTokens = openai.Opt(anthropicReq.MaxTokens)

	// Convert messages
	for _, msg := range anthropicReq.Messages {
		if string(msg.Role) == "user" {
			// User messages may contain tool_result blocks - need special handling
			messages := convertAnthropicUserMessageToOpenAI(msg)
			openaiReq.Messages = append(openaiReq.Messages, messages...)
		} else if string(msg.Role) == "assistant" {
			// Convert assistant message with potential tool_use blocks
			openaiMsg := convertAnthropicAssistantMessageToOpenAI(msg)
			openaiReq.Messages = append(openaiReq.Messages, openaiMsg)
		}
	}

	// Convert system message
	if len(anthropicReq.System) > 0 {
		systemStr := ConvertTextBlocksToString(anthropicReq.System)
		systemMsg := openai.SystemMessage(systemStr)
		// Add system message at the beginning
		openaiReq.Messages = append([]openai.ChatCompletionMessageParamUnion{systemMsg}, openaiReq.Messages...)
	}

	// Convert tools from Anthropic format to OpenAI format
	if len(anthropicReq.Tools) > 0 {
		openaiReq.Tools = ConvertAnthropicToolsToOpenAI(anthropicReq.Tools)
	}

	// Convert tool choice
	if anthropicReq.ToolChoice.OfAuto != nil || anthropicReq.ToolChoice.OfTool != nil ||
		anthropicReq.ToolChoice.OfAny != nil {
		openaiReq.ToolChoice = ConvertAnthropicToolChoiceToOpenAI(&anthropicReq.ToolChoice)
	}

	return openaiReq
}

// convertToolResultContent extracts the content from a tool result block
// The content is a list of content blocks (typically just one text block)
func convertToolResultContent(content []anthropic.ToolResultBlockParamContentUnion) string {
	var result strings.Builder
	for _, c := range content {
		// Handle text content
		if c.OfText != nil {
			result.WriteString(c.OfText.Text)
		}
	}
	return result.String()
}

// ConvertContentBlocksToString converts Anthropic content blocks to string
func ConvertContentBlocksToString(blocks []anthropic.ContentBlockParamUnion) string {
	var result strings.Builder
	for _, block := range blocks {
		// Use the AsText helper if available, or check the type
		if block.OfText != nil {
			result.WriteString(block.OfText.Text)
		}
	}
	return result.String()
}

// ConvertTextBlocksToString converts Anthropic TextBlockParam array to string
func ConvertTextBlocksToString(blocks []anthropic.TextBlockParam) string {
	var result strings.Builder
	for _, block := range blocks {
		result.WriteString(block.Text)
	}
	return result.String()
}

// convertAnthropicAssistantMessageToOpenAI converts Anthropic assistant message to OpenAI format
// This handles both text content and tool_use blocks
func convertAnthropicAssistantMessageToOpenAI(msg anthropic.MessageParam) openai.ChatCompletionMessageParamUnion {
	var textContent string
	var toolCalls []map[string]interface{}

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
		}
	}

	// Build the message based on what we have
	if len(toolCalls) > 0 {
		// Use JSON marshaling to create a message with tool_calls
		msgMap := map[string]interface{}{
			"role":    "assistant",
			"content": textContent,
		}
		if len(toolCalls) > 0 {
			msgMap["tool_calls"] = toolCalls
		}
		msgBytes, _ := json.Marshal(msgMap)
		var result openai.ChatCompletionMessageParamUnion
		_ = json.Unmarshal(msgBytes, &result)
		return result
	}

	// Simple text-only assistant message
	return openai.AssistantMessage(textContent)
}

// convertAnthropicUserMessageToOpenAI converts Anthropic user message to OpenAI format
// This handles text content and tool_result blocks
// tool_result blocks in Anthropic become separate role="tool" messages in OpenAI
// Returns a slice of messages because tool results become separate messages
func convertAnthropicUserMessageToOpenAI(msg anthropic.MessageParam) []openai.ChatCompletionMessageParamUnion {
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
				toolMsg := map[string]interface{}{
					"role":         "tool",
					"tool_call_id": block.OfToolResult.ToolUseID,
					"content":      convertToolResultContent(block.OfToolResult.Content),
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
		contentStr := ConvertContentBlocksToString(msg.Content)
		if contentStr != "" {
			result = append(result, openai.UserMessage(contentStr))
		}
	}

	return result
}
