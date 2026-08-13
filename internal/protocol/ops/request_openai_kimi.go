package ops

import (
	"github.com/openai/openai-go/v3"
	"github.com/tingly-dev/tingly-box/internal/protocol"
)

// applyKimiTransform applies Moonshot/Kimi's request shaping: the
// reasoning_content message conversion shared with DeepSeek (see
// convertThinkingToReasoningContent), plus Kimi K3's own reasoning_effort
// forwarding onto the same low/high/max tiers DeepSeek uses (see
// applyLowHighMaxReasoningEffort).
func applyKimiTransform(req *openai.ChatCompletionNewParams, providerURL, model string, config *protocol.OpenAIConfig) *openai.ChatCompletionNewParams {
	applyLowHighMaxReasoningEffort(req, config)
	convertThinkingToReasoningContent(req)
	return req
}
