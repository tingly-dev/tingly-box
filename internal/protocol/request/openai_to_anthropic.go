package request

import (
	"encoding/json"

	"github.com/anthropics/anthropic-sdk-go"
	"github.com/openai/openai-go/v3"
)

// ConvertOpenAIToAnthropicRequest converts OpenAI ChatCompletionNewParams to Anthropic SDK format
func ConvertOpenAIToAnthropicRequest(req *openai.ChatCompletionNewParams, defaultMaxTokens int64) *anthropic.BetaMessageNewParams {
	messages := make([]anthropic.BetaMessageParam, 0, len(req.Messages))
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
			var blocks []anthropic.BetaContentBlockParamUnion

			if content, ok := m["content"].(string); ok && content != "" {
				// Simple text content
				blocks = append(blocks, anthropic.NewBetaTextBlock(content))
			} else if contentParts, ok := m["content"].([]interface{}); ok {
				// Array of content parts (multimodal)
				for _, part := range contentParts {
					partMap, ok := part.(map[string]interface{})
					if !ok {
						continue
					}
					switch partMap["type"] {
					case "text":
						if text, ok := partMap["text"].(string); ok {
							blocks = append(blocks, anthropic.NewBetaTextBlock(text))
						}
					case "image_url":
						imageURL, _ := partMap["image_url"].(map[string]interface{})
						if imageURL == nil {
							continue
						}
						url, _ := imageURL["url"].(string)
						if block, ok := openAIImageURLToAnthropicBetaBlock(url); ok {
							blocks = append(blocks, block)
						}
					}
				}
			}

			if len(blocks) > 0 {
				messages = append(messages, anthropic.NewBetaUserMessage(blocks...))
			}

		case "assistant":
			// Assistant message
			var blocks []anthropic.BetaContentBlockParamUnion

			// Add text content if present
			if content, ok := m["content"].(string); ok && content != "" {
				blocks = append(blocks, anthropic.NewBetaTextBlock(content))
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
								anthropic.NewBetaToolUseBlock(id, argsInput, name),
							)
						}
					}
				}
			}

			if len(blocks) > 0 {
				messages = append(messages, anthropic.BetaMessageParam{
					Content: blocks,
					Role:    anthropic.BetaMessageParamRoleAssistant,
				})
			}

		case "tool":
			// Tool result message → tool_result block (must be USER role)
			toolCallID, _ := m["tool_call_id"].(string)
			content, _ := m["content"].(string)

			blocks := []anthropic.BetaContentBlockParamUnion{
				anthropic.NewBetaToolResultBlock(toolCallID, content, false),
			}
			messages = append(messages, anthropic.NewBetaUserMessage(blocks...))
		}
	}

	// Determine max_tokens - use default if not set
	maxTokens := req.MaxTokens.Value
	if maxTokens == 0 {
		maxTokens = defaultMaxTokens
	}

	params := &anthropic.BetaMessageNewParams{
		Model:     anthropic.Model(req.Model),
		Messages:  messages,
		MaxTokens: maxTokens,
	}

	// Add system parts if any
	if len(systemParts) > 0 {
		params.System = make([]anthropic.BetaTextBlockParam, len(systemParts))
		for i, part := range systemParts {
			params.System[i] = anthropic.BetaTextBlockParam{Text: part}
		}
	}

	// Convert tools from OpenAI format to Anthropic format
	if len(req.Tools) > 0 {
		params.Tools = ConvertOpenAIToAnthropicTools(req.Tools)
		// Convert tool choice
		// ToolChoice is a Union type, check if any field is set
		params.ToolChoice = ConvertOpenAIToAnthropicToolChoice(&req.ToolChoice)
	}

	return params
}

// openAIImageURLToAnthropicBetaBlock turns an OpenAI image_url.url string into
// an Anthropic beta image content block. Data URLs become base64 image sources,
// remote URLs become URL image sources. Returns ok=false for empty/malformed
// inputs the caller should drop.
func openAIImageURLToAnthropicBetaBlock(url string) (anthropic.BetaContentBlockParamUnion, bool) {
	mediaType, data, remoteURL := parseImageURLToAnthropicSource(url)
	switch {
	case mediaType != "" && data != "":
		return anthropic.NewBetaImageBlock(anthropic.BetaBase64ImageSourceParam{
			Data:      data,
			MediaType: anthropic.BetaBase64ImageSourceMediaType(mediaType),
		}), true
	case remoteURL != "":
		return anthropic.NewBetaImageBlock(anthropic.BetaURLImageSourceParam{
			URL: remoteURL,
		}), true
	}
	return anthropic.BetaContentBlockParamUnion{}, false
}

// openAIImageURLToAnthropicV1Block is the v1 counterpart of
// openAIImageURLToAnthropicBetaBlock.
func openAIImageURLToAnthropicV1Block(url string) (anthropic.ContentBlockParamUnion, bool) {
	mediaType, data, remoteURL := parseImageURLToAnthropicSource(url)
	switch {
	case mediaType != "" && data != "":
		return anthropic.NewImageBlock(anthropic.Base64ImageSourceParam{
			Data:      data,
			MediaType: anthropic.Base64ImageSourceMediaType(mediaType),
		}), true
	case remoteURL != "":
		return anthropic.NewImageBlock(anthropic.URLImageSourceParam{
			URL: remoteURL,
		}), true
	}
	return anthropic.ContentBlockParamUnion{}, false
}

func ConvertOpenAIToAnthropicTools(tools []openai.ChatCompletionToolUnionParam) []anthropic.BetaToolUnionParam {

	if len(tools) == 0 {
		return nil
	}

	out := make([]anthropic.BetaToolUnionParam, 0, len(tools))

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
					var tool anthropic.BetaToolUnionParam
					if inputSchema != nil {
						// Convert map[string]interface{} to the proper structure
						if schemaBytes, err := json.Marshal(inputSchema); err == nil {
							var schemaParam anthropic.BetaToolInputSchemaParam
							if err := json.Unmarshal(schemaBytes, &schemaParam); err == nil {
								tool = anthropic.BetaToolUnionParam{
									OfTool: &anthropic.BetaToolParam{
										Name:        fn.Name,
										InputSchema: schemaParam,
									},
								}
							}
						}
					} else {
						tool = anthropic.BetaToolUnionParam{
							OfTool: &anthropic.BetaToolParam{
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

func ConvertOpenAIToAnthropicToolChoice(tc *openai.ChatCompletionToolChoiceOptionUnionParam) anthropic.BetaToolChoiceUnionParam {

	// Check the different variants
	if auto := tc.OfAuto.Value; auto != "" {
		if auto == "auto" {
			return anthropic.BetaToolChoiceUnionParam{
				OfAuto: &anthropic.BetaToolChoiceAutoParam{},
			}
		}
	}

	if tc.OfAllowedTools != nil {
		// Default to auto for allowed tools
		return anthropic.BetaToolChoiceUnionParam{
			OfAuto: &anthropic.BetaToolChoiceAutoParam{},
		}
	}

	if funcChoice := tc.OfFunctionToolChoice; funcChoice != nil {
		if name := funcChoice.Function.Name; name != "" {
			return anthropic.BetaToolChoiceParamOfTool(name)
		}
	}

	if tc.OfCustomToolChoice != nil {
		// Default to auto for custom tool choice
		return anthropic.BetaToolChoiceUnionParam{
			OfAuto: &anthropic.BetaToolChoiceAutoParam{},
		}
	}

	// Default to auto
	return anthropic.BetaToolChoiceUnionParam{
		OfAuto: &anthropic.BetaToolChoiceAutoParam{},
	}
}
