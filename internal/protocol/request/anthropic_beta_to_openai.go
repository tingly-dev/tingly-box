package request

import (
	"strings"

	"github.com/anthropics/anthropic-sdk-go"
	"github.com/openai/openai-go/v3"
	"github.com/tingly-dev/tingly-box/internal/protocol"
)

// ConvertAnthropicBetaToOpenAIRequest converts Anthropic beta request to
// OpenAI format. Returns the OpenAI request and a config object with metadata
// for provider transforms. The conversion itself is shared with the v1
// variant via the normalized request view (see anthropic_view.go).
func ConvertAnthropicBetaToOpenAIRequest(anthropicReq *anthropic.BetaMessageNewParams, compatible bool, isStreaming bool, disableStreamUsage bool) (*openai.ChatCompletionNewParams, *protocol.OpenAIConfig) {
	// compatible historically selected a schema-transforming tool converter,
	// which is now an alias of the plain one (provider transforms own schema
	// rewrites), so it no longer affects the conversion.
	_ = compatible
	return convertAnthropicViewToOpenAIRequest(viewAnthropicBetaRequest(anthropicReq), isStreaming, disableStreamUsage)
}

// ConvertAnthropicBetaToolsToOpenAI converts Anthropic beta tools to OpenAI format
func ConvertAnthropicBetaToolsToOpenAI(tools []anthropic.BetaToolUnionParam) []openai.ChatCompletionToolUnionParam {
	return convertAnthropicToolViewsToOpenAI(viewAnthropicBetaTools(tools))
}

// ConvertAnthropicBetaToolsToOpenAIWithTransformedSchema is an alias for ConvertAnthropicBetaToolsToOpenAI
// Schema transformation is handled by provider-specific transforms
func ConvertAnthropicBetaToolsToOpenAIWithTransformedSchema(tools []anthropic.BetaToolUnionParam) []openai.ChatCompletionToolUnionParam {
	return ConvertAnthropicBetaToolsToOpenAI(tools)
}

// ConvertAnthropicBetaToolChoiceToOpenAI converts Anthropic beta tool_choice to OpenAI format
func ConvertAnthropicBetaToolChoiceToOpenAI(tc *anthropic.BetaToolChoiceUnionParam) openai.ChatCompletionToolChoiceOptionUnionParam {
	return convertAnthropicToolChoiceViewToOpenAI(viewAnthropicBetaToolChoice(tc))
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
		// Use the AsText helper if available, or check the type
		if block.OfText != nil {
			result.WriteString(block.OfText.Text)
		}
	}
	return result.String()
}

// betaImageBlockToOpenAIURL converts an Anthropic beta image block source into
// the URL string OpenAI's image_url content part expects. Base64 sources become
// a data: URL; URL sources are passed through. Returns "" for unsupported
// sources (e.g. file IDs that have no OpenAI equivalent).
func betaImageBlockToOpenAIURL(img *anthropic.BetaImageBlockParam) string {
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

// convertAnthropicBetaAssistantMessageToOpenAI converts Anthropic beta assistant message to OpenAI format
// Note: thinking content is preserved in "x_thinking" field for provider-specific transforms
func convertAnthropicBetaAssistantMessageToOpenAI(msg anthropic.BetaMessageParam) openai.ChatCompletionMessageParamUnion {
	return convertAnthropicViewAssistantToOpenAI(viewAnthropicBetaMessage(msg).Content)
}

// convertAnthropicBetaUserMessageToOpenAI converts Anthropic beta user message to OpenAI format
func convertAnthropicBetaUserMessageToOpenAI(msg anthropic.BetaMessageParam) []openai.ChatCompletionMessageParamUnion {
	return convertAnthropicViewUserToOpenAI(viewAnthropicBetaMessage(msg).Content)
}
