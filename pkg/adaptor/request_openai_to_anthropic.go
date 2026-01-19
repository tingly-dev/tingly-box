package adaptor

import (
	"encoding/json"

	"github.com/anthropics/anthropic-sdk-go"
	"github.com/openai/openai-go/v3"
)

// ConvertOpenAIToAnthropicRequest converts OpenAI ChatCompletionNewParams to Anthropic SDK format
func ConvertOpenAIToAnthropicRequest(req *openai.ChatCompletionNewParams, defaultMaxTokens int64) anthropic.MessageNewParams {
	messages := make([]anthropic.MessageParam, 0, len(req.Messages))
	var systemParts []string

	for _, msg := range req.Messages {
		// For Union types, we need to use JSON serialization/deserialization
		// to properly extract the content and role
		raw, _ := json.Marshal(msg)
		var m map[string]interface{}
		if err := json.Unmarshal(raw, &m); err != nil {
			continue
		}

		role, _ := m["role"].(string)

		switch role {
		case "system":
			// System message → params.System
			if content, ok := m["content"].(string); ok && content != "" {
				systemParts = append(systemParts, content)
			}

		case "user":
			// User message
			var blocks []anthropic.ContentBlockParamUnion

			if content, ok := m["content"].(string); ok && content != "" {
				// Simple text content
				blocks = append(blocks, anthropic.NewTextBlock(content))
			} else if contentParts, ok := m["content"].([]interface{}); ok {
				// Array of content parts (multimodal)
				for _, part := range contentParts {
					if partMap, ok := part.(map[string]interface{}); ok {
						if text, ok := partMap["text"].(string); ok {
							blocks = append(blocks, anthropic.NewTextBlock(text))
						}
					}
				}
			}

			if len(blocks) > 0 {
				messages = append(messages, anthropic.NewUserMessage(blocks...))
			}

		case "assistant":
			// Assistant message
			var blocks []anthropic.ContentBlockParamUnion

			// Add text content if present
			if content, ok := m["content"].(string); ok && content != "" {
				blocks = append(blocks, anthropic.NewTextBlock(content))
			}

			// Convert tool calls to tool_use blocks
			if toolCalls, ok := m["tool_calls"].([]interface{}); ok {
				for _, tc := range toolCalls {
					if call, ok := tc.(map[string]interface{}); ok {
						if fn, ok := call["function"].(map[string]interface{}); ok {
							id, _ := call["id"].(string)
							name, _ := fn["name"].(string)

							var argsInput interface{}
							if argsStr, ok := fn["arguments"].(string); ok {
								_ = json.Unmarshal([]byte(argsStr), &argsInput)
							}

							blocks = append(blocks,
								anthropic.NewToolUseBlock(id, argsInput, name),
							)
						}
					}
				}
			}

			if len(blocks) > 0 {
				messages = append(messages, anthropic.NewAssistantMessage(blocks...))
			}

		case "tool":
			// Tool result message → tool_result block (must be USER role)
			toolCallID, _ := m["tool_call_id"].(string)
			content, _ := m["content"].(string)

			blocks := []anthropic.ContentBlockParamUnion{
				anthropic.NewToolResultBlock(toolCallID, content, false),
			}
			messages = append(messages, anthropic.NewUserMessage(blocks...))
		}
	}

	// Determine max_tokens - use default if not set
	maxTokens := req.MaxTokens.Value
	if maxTokens == 0 {
		maxTokens = defaultMaxTokens
	}

	params := anthropic.MessageNewParams{
		Model:     anthropic.Model(req.Model),
		Messages:  messages,
		MaxTokens: maxTokens,
	}

	// Add system parts if any
	if len(systemParts) > 0 {
		params.System = make([]anthropic.TextBlockParam, len(systemParts))
		for i, part := range systemParts {
			params.System[i] = anthropic.TextBlockParam{Text: part}
		}
	}

	// Convert tools from OpenAI format to Anthropic format
	if len(req.Tools) > 0 {
		params.Tools = ConvertOpenAIToAnthropicTools(req.Tools)
	}

	// Convert tool choice
	// ToolChoice is a Union type, check if any field is set
	if req.ToolChoice.OfAuto.Value != "" || req.ToolChoice.OfAllowedTools != nil ||
		req.ToolChoice.OfFunctionToolChoice != nil || req.ToolChoice.OfCustomToolChoice != nil {
		params.ToolChoice = ConvertOpenAIToAnthropicToolChoice(&req.ToolChoice)
	}

	return params
}

func ConvertOpenAIToAnthropicTools(tools []openai.ChatCompletionToolUnionParam) []anthropic.ToolUnionParam {

	if len(tools) == 0 {
		return nil
	}

	out := make([]anthropic.ToolUnionParam, 0, len(tools))

	for _, t := range tools {
		fn := t.GetFunction()
		if fn == nil {
			continue
		}

		// Convert OpenAI function schema to Anthropic input schema
		var inputSchema map[string]interface{}
		if fn.Parameters != nil {
			if bytes, err := json.Marshal(fn.Parameters); err == nil {
				if err := json.Unmarshal(bytes, &inputSchema); err == nil {
					// Create tool with input schema
					var tool anthropic.ToolUnionParam
					if inputSchema != nil {
						// Convert map[string]interface{} to the proper structure
						if schemaBytes, err := json.Marshal(inputSchema); err == nil {
							var schemaParam anthropic.ToolInputSchemaParam
							if err := json.Unmarshal(schemaBytes, &schemaParam); err == nil {
								tool = anthropic.ToolUnionParam{
									OfTool: &anthropic.ToolParam{
										Name:        fn.Name,
										InputSchema: schemaParam,
									},
								}
							}
						}
					} else {
						tool = anthropic.ToolUnionParam{
							OfTool: &anthropic.ToolParam{
								Name: fn.Name,
							},
						}
					}

					// Set description if available
					if fn.Description.Value != "" && tool.OfTool != nil {
						tool.OfTool.Description = anthropic.Opt(fn.Description.Value)
					}
					out = append(out, tool)
				}
			}
		}
	}

	return out
}

func ConvertOpenAIToAnthropicToolChoice(tc *openai.ChatCompletionToolChoiceOptionUnionParam) anthropic.ToolChoiceUnionParam {

	// Check the different variants
	if auto := tc.OfAuto.Value; auto != "" {
		if auto == "auto" {
			return anthropic.ToolChoiceUnionParam{
				OfAuto: &anthropic.ToolChoiceAutoParam{},
			}
		}
	}

	if tc.OfAllowedTools != nil {
		// Default to auto for allowed tools
		return anthropic.ToolChoiceUnionParam{
			OfAuto: &anthropic.ToolChoiceAutoParam{},
		}
	}

	if funcChoice := tc.OfFunctionToolChoice; funcChoice != nil {
		if name := funcChoice.Function.Name; name != "" {
			return anthropic.ToolChoiceParamOfTool(name)
		}
	}

	if tc.OfCustomToolChoice != nil {
		// Default to auto for custom tool choice
		return anthropic.ToolChoiceUnionParam{
			OfAuto: &anthropic.ToolChoiceAutoParam{},
		}
	}

	// Default to auto
	return anthropic.ToolChoiceUnionParam{
		OfAuto: &anthropic.ToolChoiceAutoParam{},
	}
}
