package adaptor

import (
	"encoding/json"
	"strings"

	"github.com/anthropics/anthropic-sdk-go"
	"github.com/openai/openai-go/v3"
	"google.golang.org/genai"
)

// normalizeSchemaTypes converts lowercase JSON Schema types to Google's uppercase format
// This recursively processes all schemas, including nested properties and array items
func normalizeSchemaTypes(schema *genai.Schema) {
	if schema == nil {
		return
	}

	// Convert lowercase type string to Google's uppercase format
	if schema.Type != "" {
		upperType := strings.ToUpper(string(schema.Type))
		switch upperType {
		case "OBJECT":
			schema.Type = genai.TypeObject
		case "STRING":
			schema.Type = genai.TypeString
		case "NUMBER":
			schema.Type = genai.TypeNumber
		case "INTEGER":
			schema.Type = genai.TypeInteger
		case "BOOLEAN":
			schema.Type = genai.TypeBoolean
		case "ARRAY":
			schema.Type = genai.TypeArray
		case "NULL":
			schema.Type = genai.TypeNULL
		default:
			// Keep original if unknown
		}
	}

	// Recursively normalize nested property schemas
	for _, propSchema := range schema.Properties {
		normalizeSchemaTypes(propSchema)
	}

	// Normalize array item schema
	if schema.Items != nil {
		normalizeSchemaTypes(schema.Items)
	}

	// Normalize anyOf schemas
	for _, anyOfSchema := range schema.AnyOf {
		normalizeSchemaTypes(anyOfSchema)
	}
}

// ConvertOpenAIToGoogleRequest converts OpenAI ChatCompletionNewParams to Google SDK format
func ConvertOpenAIToGoogleRequest(req *openai.ChatCompletionNewParams, defaultMaxTokens int64) (string, []*genai.Content, *genai.GenerateContentConfig) {
	model := string(req.Model)
	contents := make([]*genai.Content, 0, len(req.Messages))
	config := &genai.GenerateContentConfig{}

	// Set max_tokens - Google uses int32 directly
	if req.MaxTokens.Value > 0 {
		config.MaxOutputTokens = int32(req.MaxTokens.Value)
	} else {
		config.MaxOutputTokens = int32(defaultMaxTokens)
	}

	// Set temperature if provided - Google uses *float32
	if req.Temperature.Value > 0 {
		temp := float32(req.Temperature.Value)
		config.Temperature = &temp
	}

	// Set top_p if provided - Google uses *float32
	if req.TopP.Value > 0 {
		topP := float32(req.TopP.Value)
		config.TopP = &topP
	}

	// Convert messages
	var systemInstructions string
	for _, msg := range req.Messages {
		// Use JSON serialization to extract message content
		raw, _ := json.Marshal(msg)
		var m map[string]interface{}
		if err := json.Unmarshal(raw, &m); err != nil {
			continue
		}

		role, _ := m["role"].(string)

		switch role {
		case "system":
			// System message → system_instruction
			if content, ok := m["content"].(string); ok && content != "" {
				systemInstructions += content + "\n"
			}

		case "user":
			// User message
			content := &genai.Content{
				Role:  "user",
				Parts: []*genai.Part{},
			}

			// Handle text content
			if textContent, ok := m["content"].(string); ok && textContent != "" {
				// Simple text content
				content.Parts = append(content.Parts, genai.NewPartFromText(textContent))
			} else if contentParts, ok := m["content"].([]interface{}); ok {
				// Array of content parts (multimodal)
				for _, part := range contentParts {
					if partMap, ok := part.(map[string]interface{}); ok {
						if text, ok := partMap["text"].(string); ok {
							content.Parts = append(content.Parts, genai.NewPartFromText(text))
						}
						// Handle images or other content types if needed
					}
				}
			}

			// Handle tool result messages (from tool role in OpenAI, converted to user content in Google)
			if toolCallID, ok := m["tool_call_id"].(string); ok {
				if toolContent, ok := m["content"].(string); ok {
					// Convert to function_response
					content.Parts = append(content.Parts, &genai.Part{
						FunctionResponse: &genai.FunctionResponse{
							Name: toolCallID, // Use tool_call_id as reference name
							Parts: []*genai.FunctionResponsePart{
								{
									InlineData: &genai.FunctionResponseBlob{
										Data: []byte(toolContent),
									},
								},
							},
						},
					})
				}
			}

			if len(content.Parts) > 0 {
				contents = append(contents, content)
			}

		case "assistant":
			// Assistant message
			content := &genai.Content{
				Role:  "model",
				Parts: []*genai.Part{},
			}

			// Add text content if present
			if textContent, ok := m["content"].(string); ok && textContent != "" {
				content.Parts = append(content.Parts, genai.NewPartFromText(textContent))
			}

			// Convert tool_calls to function_call parts
			if toolCalls, ok := m["tool_calls"].([]interface{}); ok {
				for _, tc := range toolCalls {
					if call, ok := tc.(map[string]interface{}); ok {
						if fn, ok := call["function"].(map[string]interface{}); ok {
							id, _ := call["id"].(string)
							name, _ := fn["name"].(string)

							var argsInput map[string]interface{}
							if argsStr, ok := fn["arguments"].(string); ok {
								_ = json.Unmarshal([]byte(argsStr), &argsInput)
							}

							content.Parts = append(content.Parts, &genai.Part{
								FunctionCall: &genai.FunctionCall{
									ID:   id,
									Name: name,
									Args: argsInput,
								},
							})
						}
					}
				}
			}

			if len(content.Parts) > 0 {
				contents = append(contents, content)
			}

		case "tool":
			// Tool result message → function_response in user content
			toolCallID, _ := m["tool_call_id"].(string)
			content, _ := m["content"].(string)

			toolContent := &genai.Content{
				Role: "user",
				Parts: []*genai.Part{
					{
						FunctionResponse: &genai.FunctionResponse{
							Name: toolCallID,
							Parts: []*genai.FunctionResponsePart{
								{
									InlineData: &genai.FunctionResponseBlob{
										Data: []byte(content),
									},
								},
							},
						},
					},
				},
			}
			contents = append(contents, toolContent)
		}
	}

	// Set system instruction if we have one
	if systemInstructions != "" {
		config.SystemInstruction = &genai.Content{
			Role:  "system",
			Parts: []*genai.Part{genai.NewPartFromText(systemInstructions)},
		}
	}

	// Convert tools from OpenAI format to Google format
	if len(req.Tools) > 0 {
		config.Tools = []*genai.Tool{
			{
				FunctionDeclarations: ConvertOpenAIToGoogleTools(req.Tools),
			},
		}
	}

	// Convert tool choice
	if req.ToolChoice.OfAuto.Value != "" || req.ToolChoice.OfAllowedTools != nil ||
		req.ToolChoice.OfFunctionToolChoice != nil || req.ToolChoice.OfCustomToolChoice != nil {
		config.ToolConfig = ConvertOpenAIToGoogleToolChoice(&req.ToolChoice)
	}

	return model, contents, config
}

func ConvertOpenAIToGoogleTools(tools []openai.ChatCompletionToolUnionParam) []*genai.FunctionDeclaration {
	if len(tools) == 0 {
		return nil
	}

	out := make([]*genai.FunctionDeclaration, 0, len(tools))

	for _, t := range tools {
		fn := t.GetFunction()
		if fn == nil {
			continue
		}

		// Convert OpenAI function parameters to Google format
		var parameters *genai.Schema
		if fn.Parameters != nil {
			// Convert map[string]interface{} to Google Schema
			if schemaBytes, err := json.Marshal(fn.Parameters); err == nil {
				_ = json.Unmarshal(schemaBytes, &parameters)
				// Normalize schema types from lowercase (JSON Schema) to uppercase (Google format)
				normalizeSchemaTypes(parameters)
			}
		}

		// Create function declaration
		funcDecl := &genai.FunctionDeclaration{
			Name:        fn.Name,
			Description: fn.Description.Value,
			Parameters:  parameters,
		}
		out = append(out, funcDecl)
	}

	return out
}

func ConvertOpenAIToGoogleToolChoice(tc *openai.ChatCompletionToolChoiceOptionUnionParam) *genai.ToolConfig {
	config := &genai.ToolConfig{
		FunctionCallingConfig: &genai.FunctionCallingConfig{},
	}

	// Check the different variants
	if auto := tc.OfAuto.Value; auto != "" {
		if auto == "auto" {
			config.FunctionCallingConfig.Mode = genai.FunctionCallingConfigModeAuto
		}
	}

	if tc.OfAllowedTools != nil {
		// Default to auto for allowed tools
		config.FunctionCallingConfig.Mode = genai.FunctionCallingConfigModeAuto
	}

	if funcChoice := tc.OfFunctionToolChoice; funcChoice != nil {
		if name := funcChoice.Function.Name; name != "" {
			config.FunctionCallingConfig.Mode = genai.FunctionCallingConfigModeAny
			config.FunctionCallingConfig.AllowedFunctionNames = []string{name}
		}
	}

	if tc.OfCustomToolChoice != nil {
		// Default to auto for custom tool choice
		config.FunctionCallingConfig.Mode = genai.FunctionCallingConfigModeAuto
	}

	// Default to auto
	if config.FunctionCallingConfig.Mode == "" {
		config.FunctionCallingConfig.Mode = genai.FunctionCallingConfigModeAuto
	}

	return config
}

// ConvertAnthropicToGoogleRequest converts Anthropic request to Google format
func ConvertAnthropicToGoogleRequest(anthropicReq *anthropic.MessageNewParams, defaultMaxTokens int64) (string, []*genai.Content, *genai.GenerateContentConfig) {
	model := string(anthropicReq.Model)
	contents := make([]*genai.Content, 0, len(anthropicReq.Messages))
	config := &genai.GenerateContentConfig{}

	// Set max_tokens
	config.MaxOutputTokens = int32(anthropicReq.MaxTokens)

	// Convert system message
	if len(anthropicReq.System) > 0 {
		var systemText string
		for _, sysBlock := range anthropicReq.System {
			systemText += sysBlock.Text + "\n"
		}
		config.SystemInstruction = &genai.Content{
			Role:  "system",
			Parts: []*genai.Part{genai.NewPartFromText(systemText)},
		}
	}

	// Build a map of tool_use ID to function name for proper function_response conversion
	// This is needed because tool_result only contains ToolUseID, not the function name
	toolUseIDToFunctionName := make(map[string]string)
	for _, msg := range anthropicReq.Messages {
		if string(msg.Role) == "assistant" {
			for _, block := range msg.Content {
				if block.OfToolUse != nil {
					toolUseIDToFunctionName[block.OfToolUse.ID] = block.OfToolUse.Name
				}
			}
		}
	}

	// Convert messages
	for _, msg := range anthropicReq.Messages {
		switch string(msg.Role) {
		case "user":
			content := &genai.Content{
				Role:  "user",
				Parts: []*genai.Part{},
			}

			// Handle different content types
			for _, block := range msg.Content {
				switch {
				case block.OfText != nil:
					content.Parts = append(content.Parts, genai.NewPartFromText(block.OfText.Text))
				case block.OfImage != nil:
					// Convert image to inline data
					// For Google API, images need to be passed as inline data with MIME type
					if block.OfImage.Source.OfBase64 != nil {
						content.Parts = append(content.Parts, &genai.Part{
							InlineData: &genai.Blob{
								MIMEType: string(block.OfImage.Source.OfBase64.MediaType),
								Data:     []byte(block.OfImage.Source.OfBase64.Data),
							},
						})
					} else if block.OfImage.Source.OfURL != nil {
						// For URL images, we'd need to fetch them first
						// For now, skip or handle as text reference
						content.Parts = append(content.Parts, genai.NewPartFromText("[Image: "+block.OfImage.Source.OfURL.URL+"]"))
					}
				case block.OfToolResult != nil:
					// Convert tool_result to function_response
					// Get the function name from the tool_use ID mapping
					functionName := toolUseIDToFunctionName[block.OfToolResult.ToolUseID]
					if functionName == "" {
						// Fallback: use ToolUseID if we couldn't find the function name
						// This shouldn't happen in a well-formed conversation
						functionName = block.OfToolResult.ToolUseID
					}

					resultText := ""
					for _, c := range block.OfToolResult.Content {
						if c.OfText != nil {
							resultText += c.OfText.Text
						}
					}

					// Build the response map for Google's FunctionResponse
					// Try to parse as JSON first, if it fails, treat as plain text output
					var response map[string]any
					if err := json.Unmarshal([]byte(resultText), &response); err != nil {
						// Not valid JSON, wrap in "output" key
						response = map[string]any{"output": resultText}
					}

					content.Parts = append(content.Parts, &genai.Part{
						FunctionResponse: &genai.FunctionResponse{
							Name:     functionName,
							Response: response,
						},
					})
				case block.OfThinking != nil, block.OfRedactedThinking != nil:
					// Skip thinking blocks - Google API doesn't support them
				}
			}

			if len(content.Parts) > 0 {
				contents = append(contents, content)
			}

		case "assistant":
			content := &genai.Content{
				Role:  "model",
				Parts: []*genai.Part{},
			}

			// Handle different content types
			for _, block := range msg.Content {
				switch {
				case block.OfText != nil:
					content.Parts = append(content.Parts, genai.NewPartFromText(block.OfText.Text))
				case block.OfToolUse != nil:
					// Convert tool_use to function_call
					var argsInput map[string]interface{}
					if inputBytes, ok := block.OfToolUse.Input.([]byte); ok {
						_ = json.Unmarshal(inputBytes, &argsInput)
					}

					content.Parts = append(content.Parts, &genai.Part{
						FunctionCall: &genai.FunctionCall{
							ID:   block.OfToolUse.ID,
							Name: block.OfToolUse.Name,
							Args: argsInput,
						},
					})
				case block.OfThinking != nil, block.OfRedactedThinking != nil:
					// Skip thinking blocks - Google API doesn't support them
				}
			}

			if len(content.Parts) > 0 {
				contents = append(contents, content)
			}
		}
	}

	// Convert tools from Anthropic format to Google format
	if len(anthropicReq.Tools) > 0 {
		config.Tools = []*genai.Tool{
			{
				FunctionDeclarations: ConvertAnthropicToGoogleTools(anthropicReq.Tools),
			},
		}
	}

	// Convert tool choice
	if anthropicReq.ToolChoice.OfAuto != nil || anthropicReq.ToolChoice.OfTool != nil ||
		anthropicReq.ToolChoice.OfAny != nil {
		config.ToolConfig = ConvertAnthropicToGoogleToolChoice(&anthropicReq.ToolChoice)
	}

	return model, contents, config
}

func ConvertAnthropicToGoogleTools(tools []anthropic.ToolUnionParam) []*genai.FunctionDeclaration {
	if len(tools) == 0 {
		return nil
	}

	out := make([]*genai.FunctionDeclaration, 0, len(tools))

	for _, t := range tools {
		tool := t.OfTool
		if tool == nil {
			continue
		}

		// Convert Anthropic input schema to Google parameters
		var parameters *genai.Schema
		if tool.InputSchema.Properties != nil {
			if schemaBytes, err := json.Marshal(tool.InputSchema); err == nil {
				_ = json.Unmarshal(schemaBytes, &parameters)
				// Normalize schema types from lowercase (JSON Schema) to uppercase (Google format)
				normalizeSchemaTypes(parameters)
			}
		}

		funcDecl := &genai.FunctionDeclaration{
			Name:        tool.Name,
			Description: tool.Description.Value,
			Parameters:  parameters,
		}
		out = append(out, funcDecl)
	}

	return out
}

func ConvertAnthropicToGoogleToolChoice(tc *anthropic.ToolChoiceUnionParam) *genai.ToolConfig {
	config := &genai.ToolConfig{
		FunctionCallingConfig: &genai.FunctionCallingConfig{},
	}

	if tc.OfAuto != nil {
		config.FunctionCallingConfig.Mode = genai.FunctionCallingConfigModeAuto
	}

	if tc.OfTool != nil {
		config.FunctionCallingConfig.Mode = genai.FunctionCallingConfigModeAny
		config.FunctionCallingConfig.AllowedFunctionNames = []string{tc.OfTool.Name}
	}

	if tc.OfAny != nil {
		config.FunctionCallingConfig.Mode = genai.FunctionCallingConfigModeAny
	}

	// Default to auto
	if config.FunctionCallingConfig.Mode == "" {
		config.FunctionCallingConfig.Mode = genai.FunctionCallingConfigModeAuto
	}

	return config
}

// ConvertAnthropicBetaToGoogleRequest converts Anthropic request to Google format
func ConvertAnthropicBetaToGoogleRequest(anthropicReq *anthropic.BetaMessageNewParams, defaultMaxTokens int64) (string, []*genai.Content, *genai.GenerateContentConfig) {
	model := string(anthropicReq.Model)
	contents := make([]*genai.Content, 0, len(anthropicReq.Messages))
	config := &genai.GenerateContentConfig{}

	// Set max_tokens
	config.MaxOutputTokens = int32(anthropicReq.MaxTokens)

	// Convert system message
	if len(anthropicReq.System) > 0 {
		var systemText string
		for _, sysBlock := range anthropicReq.System {
			systemText += sysBlock.Text + "\n"
		}
		config.SystemInstruction = &genai.Content{
			Role:  "system",
			Parts: []*genai.Part{genai.NewPartFromText(systemText)},
		}
	}

	// Build a map of tool_use ID to function name for proper function_response conversion
	// This is needed because tool_result only contains ToolUseID, not the function name
	toolUseIDToFunctionName := make(map[string]string)
	for _, msg := range anthropicReq.Messages {
		if string(msg.Role) == "assistant" {
			for _, block := range msg.Content {
				if block.OfToolUse != nil {
					toolUseIDToFunctionName[block.OfToolUse.ID] = block.OfToolUse.Name
				}
			}
		}
	}

	// Convert messages
	for _, msg := range anthropicReq.Messages {
		switch string(msg.Role) {
		case "user":
			content := &genai.Content{
				Role:  "user",
				Parts: []*genai.Part{},
			}

			// Handle different content types
			for _, block := range msg.Content {
				switch {
				case block.OfText != nil:
					content.Parts = append(content.Parts, genai.NewPartFromText(block.OfText.Text))
				case block.OfImage != nil:
					// Convert image to inline data
					// For Google API, images need to be passed as inline data with MIME type
					if block.OfImage.Source.OfBase64 != nil {
						content.Parts = append(content.Parts, &genai.Part{
							InlineData: &genai.Blob{
								MIMEType: string(block.OfImage.Source.OfBase64.MediaType),
								Data:     []byte(block.OfImage.Source.OfBase64.Data),
							},
						})
					} else if block.OfImage.Source.OfURL != nil {
						// For URL images, we'd need to fetch them first
						// For now, skip or handle as text reference
						content.Parts = append(content.Parts, genai.NewPartFromText("[Image: "+block.OfImage.Source.OfURL.URL+"]"))
					}
				case block.OfToolResult != nil:
					// Convert tool_result to function_response
					// Get the function name from the tool_use ID mapping
					functionName := toolUseIDToFunctionName[block.OfToolResult.ToolUseID]
					if functionName == "" {
						// Fallback: use ToolUseID if we couldn't find the function name
						// This shouldn't happen in a well-formed conversation
						functionName = block.OfToolResult.ToolUseID
					}

					resultText := ""
					for _, c := range block.OfToolResult.Content {
						if c.OfText != nil {
							resultText += c.OfText.Text
						}
					}

					// Build the response map for Google's FunctionResponse
					// Try to parse as JSON first, if it fails, treat as plain text output
					var response map[string]any
					if err := json.Unmarshal([]byte(resultText), &response); err != nil {
						// Not valid JSON, wrap in "output" key
						response = map[string]any{"output": resultText}
					}

					content.Parts = append(content.Parts, &genai.Part{
						FunctionResponse: &genai.FunctionResponse{
							Name:     functionName,
							Response: response,
						},
					})
				case block.OfThinking != nil, block.OfRedactedThinking != nil:
					// Skip thinking blocks - Google API doesn't support them
				}
			}

			if len(content.Parts) > 0 {
				contents = append(contents, content)
			}

		case "assistant":
			content := &genai.Content{
				Role:  "model",
				Parts: []*genai.Part{},
			}

			// Handle different content types
			for _, block := range msg.Content {
				switch {
				case block.OfText != nil:
					content.Parts = append(content.Parts, genai.NewPartFromText(block.OfText.Text))
				case block.OfToolUse != nil:
					// Convert tool_use to function_call
					var argsInput map[string]interface{}
					if inputBytes, ok := block.OfToolUse.Input.([]byte); ok {
						_ = json.Unmarshal(inputBytes, &argsInput)
					}

					content.Parts = append(content.Parts, &genai.Part{
						FunctionCall: &genai.FunctionCall{
							ID:   block.OfToolUse.ID,
							Name: block.OfToolUse.Name,
							Args: argsInput,
						},
					})
				case block.OfThinking != nil, block.OfRedactedThinking != nil:
					// Skip thinking blocks - Google API doesn't support them
				}
			}

			if len(content.Parts) > 0 {
				contents = append(contents, content)
			}
		}
	}

	// Convert tools from Anthropic format to Google format
	if len(anthropicReq.Tools) > 0 {
		config.Tools = []*genai.Tool{
			{
				FunctionDeclarations: ConvertAnthropicBetaToGoogleTools(anthropicReq.Tools),
			},
		}
	}

	// Convert tool choice
	if anthropicReq.ToolChoice.OfAuto != nil || anthropicReq.ToolChoice.OfTool != nil ||
		anthropicReq.ToolChoice.OfAny != nil {
		config.ToolConfig = ConvertAnthropicBetaToGoogleToolChoice(&anthropicReq.ToolChoice)
	}

	return model, contents, config
}

func ConvertAnthropicBetaToGoogleTools(tools []anthropic.BetaToolUnionParam) []*genai.FunctionDeclaration {
	if len(tools) == 0 {
		return nil
	}

	out := make([]*genai.FunctionDeclaration, 0, len(tools))

	for _, t := range tools {
		tool := t.OfTool
		if tool == nil {
			continue
		}

		// Convert Anthropic beta input schema to Google parameters
		var parameters *genai.Schema
		if tool.InputSchema.Properties != nil {
			if schemaBytes, err := json.Marshal(tool.InputSchema); err == nil {
				_ = json.Unmarshal(schemaBytes, &parameters)
				// Normalize schema types from lowercase (JSON Schema) to uppercase (Google format)
				normalizeSchemaTypes(parameters)
			}
		}

		funcDecl := &genai.FunctionDeclaration{
			Name:        tool.Name,
			Description: tool.Description.Value,
			Parameters:  parameters,
		}
		out = append(out, funcDecl)
	}

	return out
}

func ConvertAnthropicBetaToGoogleToolChoice(tc *anthropic.BetaToolChoiceUnionParam) *genai.ToolConfig {
	config := &genai.ToolConfig{
		FunctionCallingConfig: &genai.FunctionCallingConfig{},
	}

	if tc.OfAuto != nil {
		config.FunctionCallingConfig.Mode = genai.FunctionCallingConfigModeAuto
	}

	if tc.OfTool != nil {
		config.FunctionCallingConfig.Mode = genai.FunctionCallingConfigModeAny
		config.FunctionCallingConfig.AllowedFunctionNames = []string{tc.OfTool.Name}
	}

	if tc.OfAny != nil {
		config.FunctionCallingConfig.Mode = genai.FunctionCallingConfigModeAny
	}

	// Default to auto
	if config.FunctionCallingConfig.Mode == "" {
		config.FunctionCallingConfig.Mode = genai.FunctionCallingConfigModeAuto
	}

	return config
}
