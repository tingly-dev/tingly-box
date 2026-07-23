package request

import (
	"strings"

	"github.com/anthropics/anthropic-sdk-go"
	"github.com/openai/openai-go/v3"
	"github.com/openai/openai-go/v3/shared"
	"github.com/tingly-dev/tingly-box/internal/protocol"
)

// ConvertAnthropicToOpenAIRequest converts Anthropic request to OpenAI format.
// Returns the OpenAI request and a config object with metadata for provider
// transforms. The conversion itself is shared with the Beta variant via the
// normalized request view (see anthropic_view.go).
func ConvertAnthropicToOpenAIRequest(anthropicReq *anthropic.MessageNewParams, compatible bool, isStreaming bool, disableStreamUsage bool) (*openai.ChatCompletionNewParams, *protocol.OpenAIConfig) {
	// compatible historically selected a schema-transforming tool converter,
	// which is now an alias of the plain one (provider transforms own schema
	// rewrites), so it no longer affects the conversion.
	_ = compatible
	return convertAnthropicViewToOpenAIRequest(viewAnthropicV1Request(anthropicReq), isStreaming, disableStreamUsage)
}

// ConvertAnthropicToolsToOpenAI converts Anthropic tools to OpenAI format
func ConvertAnthropicToolsToOpenAI(tools []anthropic.ToolUnionParam) []openai.ChatCompletionToolUnionParam {
	return convertAnthropicToolViewsToOpenAI(viewAnthropicV1Tools(tools))
}

func convertAnthropicInputSchemaToOpenAIParameters(properties any, required []string) shared.FunctionParameters {
	if properties == nil && len(required) == 0 {
		return shared.FunctionParameters{
			"type":       "object",
			"properties": map[string]any{},
		}
	}

	if schema, ok := properties.(map[string]any); ok {
		if _, hasNestedProperties := schema["properties"]; hasNestedProperties {
			parameters := make(shared.FunctionParameters, len(schema)+1)
			for key, value := range schema {
				parameters[key] = value
			}
			if _, ok := parameters["type"]; !ok {
				parameters["type"] = "object"
			}
			if len(required) > 0 {
				parameters["required"] = required
			}
			return parameters
		}
	}

	parameters := shared.FunctionParameters{
		"type": "object",
	}
	if properties != nil {
		parameters["properties"] = properties
	}
	if len(required) > 0 {
		parameters["required"] = required
	}
	return parameters
}

// ConvertAnthropicToolsToOpenAIWithTransformedSchema is an alias for ConvertAnthropicToolsToOpenAI
// Schema transformation is handled by provider-specific transforms
func ConvertAnthropicToolsToOpenAIWithTransformedSchema(tools []anthropic.ToolUnionParam) []openai.ChatCompletionToolUnionParam {
	return ConvertAnthropicToolsToOpenAI(tools)
}

// ConvertAnthropicToolChoiceToOpenAI converts Anthropic tool_choice to OpenAI format
func ConvertAnthropicToolChoiceToOpenAI(tc *anthropic.ToolChoiceUnionParam) openai.ChatCompletionToolChoiceOptionUnionParam {
	return convertAnthropicToolChoiceViewToOpenAI(viewAnthropicV1ToolChoice(tc))
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

// imageBlockToOpenAIURL converts an Anthropic v1 image block source into the
// URL string OpenAI's image_url content part expects. Base64 sources become a
// data: URL; URL sources are passed through.
func imageBlockToOpenAIURL(img *anthropic.ImageBlockParam) string {
	if img == nil {
		return ""
	}
	if img.Source.OfBase64 != nil {
		return "data:" + string(img.Source.OfBase64.MediaType) +
			";base64," + img.Source.OfBase64.Data
	}
	if img.Source.OfURL != nil {
		return img.Source.OfURL.URL
	}
	return ""
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
// Note: thinking content is preserved in "x_thinking" field for provider-specific transforms
func convertAnthropicAssistantMessageToOpenAI(msg anthropic.MessageParam) openai.ChatCompletionMessageParamUnion {
	return convertAnthropicViewAssistantToOpenAI(viewAnthropicV1Message(msg).Blocks)
}

// convertAnthropicUserMessageToOpenAI converts Anthropic user message to OpenAI format
// This handles text content and tool_result blocks
// tool_result blocks in Anthropic become separate role="tool" messages in OpenAI
// Returns a slice of messages because tool results become separate messages
func convertAnthropicUserMessageToOpenAI(msg anthropic.MessageParam) []openai.ChatCompletionMessageParamUnion {
	return convertAnthropicViewUserToOpenAI(viewAnthropicV1Message(msg).Blocks)
}
