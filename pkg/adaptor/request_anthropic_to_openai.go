package adaptor

import (
	"encoding/json"
	"strings"

	"github.com/anthropics/anthropic-sdk-go"
	"github.com/openai/openai-go/v3"
	"github.com/openai/openai-go/v3/packages/param"
	"github.com/openai/openai-go/v3/shared"
)

type handler func(map[string]interface{}) map[string]interface{}

// schemaFieldTransforms defines JSON Schema fields that should be transformed or excluded
// key: source field name
// value: target field name (empty string means exclude the field)
var schemaFieldTransforms = map[string]string{
	"exclusiveMinimum": "minimum", // convert exclusiveMinimum to minimum
	"exclusiveMaximum": "maximum", // convert exclusiveMaximum to maximum
}

// transformProperties recursively transforms and filters a JSON Schema
// Fields in schemaFieldTransforms are either renamed or excluded
func transformProperties(props map[string]interface{}) map[string]interface{} {
	result := make(map[string]interface{})

	for k, v := range props {
		if nestedSchema, ok := v.(map[string]interface{}); ok {
			// Apply field transformations to property schemas
			result[k] = transformPropertySchema(nestedSchema)
		} else {
			result[k] = v
		}
	}

	return result
}

// transformPropertySchema transforms field names in a property schema
// This is where we handle things like exclusiveMinimum → minimum
func transformPropertySchema(schema map[string]interface{}) map[string]interface{} {
	if schema == nil {
		return nil
	}

	result := make(map[string]interface{})

	for key, value := range schema {
		// Check if this field needs to be transformed or excluded
		if targetKey, needsTransform := schemaFieldTransforms[key]; needsTransform {
			if targetKey == "" {
				// Empty target means exclude this field
				continue
			}
			// Transform to new field name
			key = targetKey
		}
		result[key] = value
	}

	return result
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

		// Convert Anthropic input schema to OpenAI function parameters
		// Only include standard JSON Schema fields that OpenAI accepts
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

// ConvertAnthropicToolsToOpenAIWithTransformedSchema converts Anthropic tools to OpenAI format
// with schema field transformation. Fields in schemaFieldTransforms are either renamed
// or excluded to provide better compatibility with OpenAI's schema validation.
func ConvertAnthropicToolsToOpenAIWithTransformedSchema(tools []anthropic.ToolUnionParam) []openai.ChatCompletionToolUnionParam {
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
		// Transform excluded fields and apply field name conversions
		var parameters map[string]interface{}
		if tool.InputSchema.Properties != nil || len(tool.InputSchema.Required) > 0 {
			// Build the raw schema first
			rawSchema := make(map[string]interface{})
			rawSchema["type"] = "object"

			if tool.InputSchema.Properties != nil {
				if m, ok := tool.InputSchema.Properties.(map[string]interface{}); ok {
					rawSchema["properties"] = transformProperties(m)
				} else {
					rawSchema["properties"] = tool.InputSchema.Properties
				}
			}

			if len(tool.InputSchema.Required) > 0 {
				rawSchema["required"] = tool.InputSchema.Required
			}

			parameters = rawSchema
		}

		// Create function with filtered parameters
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
func ConvertAnthropicToOpenAIRequest(anthropicReq *anthropic.MessageNewParams, compatible bool) *openai.ChatCompletionNewParams {
	openaiReq := &openai.ChatCompletionNewParams{
		Model: openai.ChatModel(anthropicReq.Model),
	}

	isThinking := IsThinkingEnabled(anthropicReq)
	if isThinking {
		openaiReq.SetExtraFields(
			map[string]interface{}{
				"thinking": map[string]interface{}{
					"type": "enabled",
				},
			},
		)
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
			// Guard reasoning_content here
			if extra := openaiMsg.ExtraFields(); extra != nil {
				if _, ok := extra["reasoning_content"]; !ok {
					extra["reasoning_content"] = ""
				}
				openaiMsg.SetExtraFields(extra)
			} else {
				openaiMsg.SetExtraFields(map[string]any{"reasoning_content": ""})
			}

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
		if compatible {
			openaiReq.Tools = ConvertAnthropicToolsToOpenAIWithTransformedSchema(anthropicReq.Tools)
		} else {
			openaiReq.Tools = ConvertAnthropicToolsToOpenAI(anthropicReq.Tools)
		}
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
			"role":              "assistant",
			"content":           textContent,
			"reasoning_content": thinking, // Always include for DeepSeek
		}
		if len(toolCalls) > 0 {
			msgMap["tool_calls"] = toolCalls
		}

		msgBytes, _ := json.Marshal(msgMap)
		var result openai.ChatCompletionMessageParamUnion
		_ = json.Unmarshal(msgBytes, &result)
		return result
	}

	// For all other cases, always include reasoning_content
	msgMap := map[string]interface{}{
		"role":              "assistant",
		"content":           textContent,
		"reasoning_content": thinking,
	}
	msgBytes, _ := json.Marshal(msgMap)
	var result openai.ChatCompletionMessageParamUnion
	_ = json.Unmarshal(msgBytes, &result)
	return result
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

// IsThinkingEnabled checks if thinking mode is enabled in the Anthropic request
func IsThinkingEnabled(anthropicReq *anthropic.MessageNewParams) bool {
	isThinking := anthropicReq.Thinking.OfEnabled != nil
	for _, msg := range anthropicReq.Messages {
		for _, block := range msg.Content {
			if block.OfThinking != nil {
				return true
			}

		}
	}
	return isThinking
}

// IsThinkingEnabledBeta checks if thinking mode is enabled in the Anthropic beta request
func IsThinkingEnabledBeta(anthropicReq *anthropic.BetaMessageNewParams) bool {
	isThinking := anthropicReq.Thinking.OfEnabled != nil
	for _, msg := range anthropicReq.Messages {
		for _, block := range msg.Content {
			if block.OfThinking != nil {
				return true
			}

		}
	}
	return isThinking
}

// ConvertAnthropicBetaToOpenAIRequest converts Anthropic beta request to OpenAI format
func ConvertAnthropicBetaToOpenAIRequest(anthropicReq *anthropic.BetaMessageNewParams, compatible bool) *openai.ChatCompletionNewParams {
	openaiReq := &openai.ChatCompletionNewParams{
		Model: openai.ChatModel(anthropicReq.Model),
	}

	isThinking := IsThinkingEnabledBeta(anthropicReq)
	if isThinking {
		openaiReq.SetExtraFields(
			map[string]interface{}{
				"thinking": map[string]interface{}{
					"type": "enabled",
				},
			},
		)
	}

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
			// Guard reasoning_content here
			if extra := openaiMsg.ExtraFields(); extra != nil {
				if _, ok := extra["reasoning_content"]; !ok {
					extra["reasoning_content"] = ""
				}
				openaiMsg.SetExtraFields(extra)
			} else {
				openaiMsg.SetExtraFields(map[string]any{"reasoning_content": ""})
			}

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
	}

	// Convert tool choice
	if anthropicReq.ToolChoice.OfAuto != nil || anthropicReq.ToolChoice.OfTool != nil ||
		anthropicReq.ToolChoice.OfAny != nil {
		openaiReq.ToolChoice = ConvertAnthropicBetaToolChoiceToOpenAI(&anthropicReq.ToolChoice)
	}

	return openaiReq
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
		// Only include standard JSON Schema fields that OpenAI accepts
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

// ConvertAnthropicBetaToolsToOpenAIWithTransformedSchema converts Anthropic beta tools to OpenAI format
// with schema field transformation.
func ConvertAnthropicBetaToolsToOpenAIWithTransformedSchema(tools []anthropic.BetaToolUnionParam) []openai.ChatCompletionToolUnionParam {
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
		// Transform excluded fields and apply field name conversions
		var parameters map[string]interface{}
		if tool.InputSchema.Properties != nil || len(tool.InputSchema.Required) > 0 {
			// Build the raw schema first
			rawSchema := make(map[string]interface{})
			rawSchema["type"] = "object"

			if tool.InputSchema.Properties != nil {
				if m, ok := tool.InputSchema.Properties.(map[string]interface{}); ok {
					rawSchema["properties"] = transformProperties(m)
				} else {
					rawSchema["properties"] = tool.InputSchema.Properties
				}
			}

			if len(tool.InputSchema.Required) > 0 {
				rawSchema["required"] = tool.InputSchema.Required
			}

			parameters = rawSchema
		}

		// Create function with filtered parameters
		fn := shared.FunctionDefinitionParam{
			Name:        tool.Name,
			Description: param.Opt[string]{Value: tool.Description.Value},
			Parameters:  parameters,
		}

		out = append(out, openai.ChatCompletionFunctionTool(fn))
	}

	return out
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
		if block.OfText != nil {
			result.WriteString(block.OfText.Text)
		}
	}
	return result.String()
}

// convertAnthropicBetaAssistantMessageToOpenAI converts Anthropic beta assistant message to OpenAI format
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
			"role":              "assistant",
			"content":           textContent,
			"reasoning_content": thinking, // Always include for DeepSeek
		}
		if len(toolCalls) > 0 {
			msgMap["tool_calls"] = toolCalls
		}

		msgBytes, _ := json.Marshal(msgMap)
		var result openai.ChatCompletionMessageParamUnion
		_ = json.Unmarshal(msgBytes, &result)
		return result
	}

	// For all other cases, always include reasoning_content
	msgMap := map[string]interface{}{
		"role":              "assistant",
		"content":           textContent,
		"reasoning_content": thinking,
	}
	msgBytes, _ := json.Marshal(msgMap)
	var result openai.ChatCompletionMessageParamUnion
	_ = json.Unmarshal(msgBytes, &result)
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
				toolMsg := map[string]interface{}{
					"role":         "tool",
					"tool_call_id": block.OfToolResult.ToolUseID,
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

// convertBetaToolResultContent extracts the content from a beta tool result block
func convertBetaToolResultContent(content []anthropic.BetaToolResultBlockParamContentUnion) string {
	var result strings.Builder
	for _, c := range content {
		// Handle text content
		if c.OfText != nil {
			result.WriteString(c.OfText.Text)
		}
	}
	return result.String()
}
