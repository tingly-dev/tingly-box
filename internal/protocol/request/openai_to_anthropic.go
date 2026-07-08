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
		// Read the typed union fields directly — no JSON round-trip needed.
		switch {
		case msg.OfSystem != nil:
			// System message → params.System
			if content := msg.OfSystem.Content.OfString.Value; content != "" {
				systemParts = append(systemParts, content)
			}

		case msg.OfUser != nil:
			// User message
			var blocks []anthropic.BetaContentBlockParamUnion

			if content := msg.OfUser.Content.OfString.Value; content != "" {
				// Simple text content
				blocks = append(blocks, anthropic.NewBetaTextBlock(content))
			} else {
				// Array of content parts (multimodal)
				for _, part := range msg.OfUser.Content.OfArrayOfContentParts {
					switch {
					case part.OfText != nil:
						if part.OfText.Text != "" {
							blocks = append(blocks, anthropic.NewBetaTextBlock(part.OfText.Text))
						}
					case part.OfImageURL != nil:
						if block, ok := openAIImageURLToAnthropicBetaBlock(part.OfImageURL.ImageURL.URL); ok {
							blocks = append(blocks, block)
						}
					}
				}
			}

			if len(blocks) > 0 {
				messages = append(messages, anthropic.NewBetaUserMessage(blocks...))
			}

		case msg.OfAssistant != nil:
			// Assistant message
			var blocks []anthropic.BetaContentBlockParamUnion

			// Add text content if present
			if content := msg.OfAssistant.Content.OfString.Value; content != "" {
				blocks = append(blocks, anthropic.NewBetaTextBlock(content))
			}

			// Convert tool calls to tool_use blocks
			for _, tc := range msg.OfAssistant.ToolCalls {
				fn := tc.OfFunction
				if fn == nil {
					continue
				}
				var argsInput interface{}
				if fn.Function.Arguments != "" {
					_ = json.Unmarshal([]byte(fn.Function.Arguments), &argsInput)
				}
				blocks = append(blocks,
					anthropic.NewBetaToolUseBlock(fn.ID, argsInput, fn.Function.Name),
				)
			}

			if len(blocks) > 0 {
				messages = append(messages, anthropic.BetaMessageParam{
					Content: blocks,
					Role:    anthropic.BetaMessageParamRoleAssistant,
				})
			}

		case msg.OfTool != nil:
			// Tool result message → tool_result block (must be USER role)
			blocks := []anthropic.BetaContentBlockParamUnion{
				anthropic.NewBetaToolResultBlock(msg.OfTool.ToolCallID, msg.OfTool.Content.OfString.Value, false),
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
	mediaType, data, remoteURL := ParseImageURLToAnthropicSource(url)
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

func ConvertOpenAIToAnthropicTools(tools []openai.ChatCompletionToolUnionParam) []anthropic.BetaToolUnionParam {

	if len(tools) == 0 {
		return nil
	}

	out := make([]anthropic.BetaToolUnionParam, 0, len(tools))

	for _, t := range tools {
		fn := t.GetFunction()
		if fn == nil || fn.Parameters == nil {
			continue
		}

		// Convert OpenAI function schema to Anthropic input schema with a
		// single marshal/unmarshal pass.
		schemaBytes, err := json.Marshal(fn.Parameters)
		if err != nil {
			continue
		}
		var schemaParam anthropic.BetaToolInputSchemaParam
		if err := json.Unmarshal(schemaBytes, &schemaParam); err != nil {
			continue
		}
		tool := anthropic.BetaToolUnionParam{
			OfTool: &anthropic.BetaToolParam{
				Name:        fn.Name,
				InputSchema: schemaParam,
			},
		}
		if fn.Description.Value != "" {
			tool.OfTool.Description = anthropic.Opt(fn.Description.Value)
		}
		out = append(out, tool)
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
