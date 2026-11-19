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
