package ops

import (
	"github.com/openai/openai-go/v3"
	"github.com/tingly-dev/tingly-box/internal/protocol"
)

// kimiEffortTiers is Moonshot/Kimi's own reasoning_effort tier map (K3
// documents the same low/high/max scheme as DeepSeek V4 today), built fresh
// so it holds an independent map from deepSeekEffortTiers rather than an
// alias of the same one — see deepSeekEffortTiers for why that matters.
var kimiEffortTiers = lowHighMaxEffortTiers()

// applyKimiTransform applies Moonshot/Kimi's request shaping: the
// reasoning_content message conversion shared with DeepSeek (see
// convertThinkingToReasoningContent), plus Kimi's own reasoning_effort
// forwarding through kimiEffortTiers.
func applyKimiTransform(req *openai.ChatCompletionNewParams, providerURL, model string, config *protocol.OpenAIConfig) *openai.ChatCompletionNewParams {
	applyReasoningEffortTier(req, config, kimiEffortTiers)
	convertThinkingToReasoningContent(req)
	return req
}
