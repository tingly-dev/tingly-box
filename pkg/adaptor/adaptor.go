package adaptor

import (
	"encoding/json"
	"fmt"
	"strings"
	"time"

	"github.com/anthropics/anthropic-sdk-go"
	"github.com/openai/openai-go/v3"
)

// ConvertAnthropicResponseToOpenAI converts an Anthropic response to OpenAI format
func ConvertAnthropicResponseToOpenAI(
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

// ConvertOpenAIToAnthropicRequest converts OpenAI ChatCompletionNewParams to Anthropic SDK format
func ConvertOpenAIToAnthropicRequest(req *openai.ChatCompletionNewParams) anthropic.MessageNewParams {
	messages := make([]anthropic.MessageParam, 0, len(req.Messages))
	var systemParts []string

	for _, msg := range req.Messages {
		rolePtr := msg.GetRole()
		if rolePtr == nil {
			continue
		}
		role := *rolePtr

		// Marshal to map for flexible access
		raw, _ := json.Marshal(msg)
		var m map[string]interface{}
		if err := json.Unmarshal(raw, &m); err != nil {
			continue
		}

		// Extract text content
		content, _ := m["content"].(string)

		//1 SYSTEM → params.System
		if role == "system" {
			if content != "" {
				systemParts = append(systemParts, content)
			}
			continue
		}

		var blocks []anthropic.ContentBlockParamUnion

		//2 Normal text content
		if content, ok := m["content"].(string); ok && content != "" {
			blocks = append(blocks, anthropic.NewTextBlock(content))
		}

		//3 Assistant tool calls → tool_use blocks
		if role == "assistant" {
			if toolCalls, ok := m["tool_calls"].([]interface{}); ok {
				for _, tc := range toolCalls {
					call := tc.(map[string]interface{})
					fn := call["function"].(map[string]interface{})

					// Parse the arguments as JSON to maintain proper structure
					var argsInput interface{}
					if argsStr, ok := fn["arguments"].(string); ok {
						json.Unmarshal([]byte(argsStr), &argsInput)
					}

					blocks = append(blocks,
						anthropic.NewToolUseBlock(
							call["id"].(string),
							argsInput,
							fn["name"].(string),
						),
					)
				}
			}

			if len(blocks) > 0 {
				messages = append(messages, anthropic.NewAssistantMessage(blocks...))
			}
			continue
		}

		//4 Tool result message → tool_result block (must be USER role)
		if role == "tool" {
			toolID, _ := m["tool_call_id"].(string)
			content, _ := m["content"].(string)

			blocks = append(blocks,
				anthropic.NewToolResultBlock(
					toolID,
					content,
					false, // is_error
				),
			)

			messages = append(messages, anthropic.NewUserMessage(blocks...))
			continue
		}

		//5 Normal user message
		if (role == "user") && len(blocks) > 0 {
			messages = append(messages, anthropic.NewUserMessage(blocks...))
		}
	}

	params := anthropic.MessageNewParams{
		Model:     anthropic.Model(req.Model),
		Messages:  messages,
		MaxTokens: req.MaxTokens.Value,
	}

	return params
}

func ConvertOpenAIToolsToAnthropic(tools []openai.ChatCompletionToolUnionParam) []anthropic.ToolUnionParam {

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

func ConvertOpenAIToolChoice(tc *openai.ChatCompletionToolChoiceOptionUnionParam) anthropic.ToolChoiceUnionParam {

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

// Helper functions to convert between formats
func ConvertAnthropicToOpenAI(anthropicReq *anthropic.MessageNewParams) *openai.ChatCompletionNewParams {
	openaiReq := &openai.ChatCompletionNewParams{
		Model: openai.ChatModel(anthropicReq.Model),
	}

	// Set MaxTokens
	openaiReq.MaxTokens = openai.Opt(int64(anthropicReq.MaxTokens))

	// Convert messages
	for _, msg := range anthropicReq.Messages {
		if string(msg.Role) == "user" {
			// Convert content blocks to string for OpenAI
			contentStr := ConvertContentBlocksToString(msg.Content)
			openaiMsg := openai.UserMessage(contentStr)
			openaiReq.Messages = append(openaiReq.Messages, openaiMsg)
		} else if string(msg.Role) == "assistant" {
			// Convert content blocks to string for OpenAI
			contentStr := ConvertContentBlocksToString(msg.Content)
			openaiMsg := openai.AssistantMessage(contentStr)
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

	return openaiReq
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

func ConvertOpenAIToAnthropic(openaiResp *openai.ChatCompletion, model string) anthropic.Message {
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
