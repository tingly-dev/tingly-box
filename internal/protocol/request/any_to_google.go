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

// ConvertAnthropicToGoogleRequest converts Anthropic request to Google format.
// The conversion is shared with the Beta variant via the normalized request
// view (see anthropic_view.go).
func ConvertAnthropicToGoogleRequest(anthropicReq *anthropic.MessageNewParams, defaultMaxTokens int64) (string, []*genai.Content, *genai.GenerateContentConfig) {
	_ = defaultMaxTokens
	return convertAnthropicViewToGoogleRequest(viewAnthropicV1Request(anthropicReq))
}

func ConvertAnthropicToGoogleTools(tools []anthropic.ToolUnionParam) []*genai.FunctionDeclaration {
	return convertAnthropicToolViewsToGoogle(tools)
}

func ConvertAnthropicToGoogleToolChoice(tc *anthropic.ToolChoiceUnionParam) *genai.ToolConfig {
	if tc == nil {
		return convertAnthropicToolChoiceViewToGoogle(anthropic.ToolChoiceUnionParam{})
	}
	return convertAnthropicToolChoiceViewToGoogle(*tc)
}

// ConvertAnthropicBetaToGoogleRequest converts Anthropic beta request to
// Google format via the shared normalized request view.
func ConvertAnthropicBetaToGoogleRequest(anthropicReq *anthropic.BetaMessageNewParams, defaultMaxTokens int64) (string, []*genai.Content, *genai.GenerateContentConfig) {
	_ = defaultMaxTokens
	return convertAnthropicViewToGoogleRequest(viewAnthropicBetaRequest(anthropicReq))
}

func ConvertAnthropicBetaToGoogleTools(tools []anthropic.BetaToolUnionParam) []*genai.FunctionDeclaration {
	return convertAnthropicToolViewsToGoogle(viewAnthropicBetaTools(tools))
}

func ConvertAnthropicBetaToGoogleToolChoice(tc *anthropic.BetaToolChoiceUnionParam) *genai.ToolConfig {
	return convertAnthropicToolChoiceViewToGoogle(viewAnthropicBetaToolChoice(tc))
}
