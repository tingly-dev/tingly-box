package request

import (
	"encoding/json"
	"strings"

	"github.com/anthropics/anthropic-sdk-go"
	"github.com/openai/openai-go/v3"
	"google.golang.org/genai"
)

// NormalizeSchemaTypes converts lowercase JSON Schema types to Google's uppercase format
// This recursively processes all schemas, including nested properties and array items
func NormalizeSchemaTypes(schema *genai.Schema) {
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
		NormalizeSchemaTypes(propSchema)
	}

	// Normalize array item schema
	if schema.Items != nil {
		NormalizeSchemaTypes(schema.Items)
	}

	// Normalize anyOf schemas
	for _, anyOfSchema := range schema.AnyOf {
		NormalizeSchemaTypes(anyOfSchema)
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
	var systemInstructions strings.Builder
	for _, msg := range req.Messages {
		// Read the typed union fields directly — no JSON round-trip needed.
		switch {
		case msg.OfSystem != nil:
			// System message → system_instruction
			if content := msg.OfSystem.Content.OfString.Value; content != "" {
				systemInstructions.WriteString(content)
				systemInstructions.WriteString("\n")
			}

		case msg.OfUser != nil:
			// User message
			content := &genai.Content{
				Role:  "user",
				Parts: []*genai.Part{},
			}

			// Handle text content
			if textContent := msg.OfUser.Content.OfString.Value; textContent != "" {
				// Simple text content
				content.Parts = append(content.Parts, genai.NewPartFromText(textContent))
			} else {
				// Array of content parts (multimodal)
				for _, part := range msg.OfUser.Content.OfArrayOfContentParts {
					if part.OfText != nil {
						content.Parts = append(content.Parts, genai.NewPartFromText(part.OfText.Text))
					}
					// Handle images or other content types if needed
				}
			}

			if len(content.Parts) > 0 {
				contents = append(contents, content)
			}

		case msg.OfAssistant != nil:
			// Assistant message
			content := &genai.Content{
				Role:  "model",
				Parts: []*genai.Part{},
			}

			// Add text content if present
			if textContent := msg.OfAssistant.Content.OfString.Value; textContent != "" {
				content.Parts = append(content.Parts, genai.NewPartFromText(textContent))
			}

			// Convert tool_calls to function_call parts
			for _, tc := range msg.OfAssistant.ToolCalls {
				fn := tc.OfFunction
				if fn == nil {
					continue
				}
				var argsInput map[string]interface{}
				if fn.Function.Arguments != "" {
					_ = json.Unmarshal([]byte(fn.Function.Arguments), &argsInput)
				}
				content.Parts = append(content.Parts, &genai.Part{
					FunctionCall: &genai.FunctionCall{
						ID:   fn.ID,
						Name: fn.Function.Name,
						Args: argsInput,
					},
				})
			}

			if len(content.Parts) > 0 {
				contents = append(contents, content)
			}

		case msg.OfTool != nil:
			// Tool result message → function_response in user content
			toolContent := &genai.Content{
				Role: "user",
				Parts: []*genai.Part{
					{
						FunctionResponse: &genai.FunctionResponse{
							Name: msg.OfTool.ToolCallID,
							Parts: []*genai.FunctionResponsePart{
								{
									InlineData: &genai.FunctionResponseBlob{
										Data: []byte(msg.OfTool.Content.OfString.Value),
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
	if systemInstructions.Len() > 0 {
		config.SystemInstruction = &genai.Content{
			Role:  "system",
			Parts: []*genai.Part{genai.NewPartFromText(systemInstructions.String())},
		}
	}

	// Convert tools from OpenAI format to Google format
	if len(req.Tools) > 0 {
		config.Tools = []*genai.Tool{
			{
				FunctionDeclarations: ConvertOpenAIToGoogleTools(req.Tools),
			},
		}
		// Convert tool choice
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
				NormalizeSchemaTypes(parameters)
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
					// FunctionResponse.Name should be the tool_use ID for Google API

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
							Name:     block.OfToolResult.ToolUseID, // Use tool_use ID as Name
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
				NormalizeSchemaTypes(parameters)
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
					// FunctionResponse.Name should be the tool_use ID for Google API

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
							Name:     block.OfToolResult.ToolUseID, // Use tool_use ID as Name
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
				NormalizeSchemaTypes(parameters)
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
