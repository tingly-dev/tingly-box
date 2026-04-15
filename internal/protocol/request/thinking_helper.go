package request

import (
	"strings"

	"github.com/anthropics/anthropic-sdk-go"
	"github.com/openai/openai-go/v3"
)

// IsThinkingEnabled checks if thinking mode is enabled in the Anthropic request
func IsThinkingEnabled(anthropicReq *anthropic.MessageNewParams) bool {
	isThinking := anthropicReq.Thinking.OfEnabled != nil
	for _, msg := range anthropicReq.Messages {
		for _, block := range msg.Content {
			if block.OfThinking != nil {
				return true
			}

		}
	}
	return isThinking
}

// IsThinkingEnabledBeta checks if thinking mode is enabled in the Anthropic beta request
func IsThinkingEnabledBeta(anthropicReq *anthropic.BetaMessageNewParams) bool {
	isThinking := anthropicReq.Thinking.OfEnabled != nil
	for _, msg := range anthropicReq.Messages {
		for _, block := range msg.Content {
			if block.OfThinking != nil {
				return true
			}

		}
	}
	return isThinking
}

// convertBetaToolResultContent extracts the content from a beta tool result block
func convertBetaToolResultContent(content []anthropic.BetaToolResultBlockParamContentUnion) string {
	var result strings.Builder
	for _, c := range content {
		// Handle text content
		if c.OfText != nil {
			result.WriteString(c.OfText.Text)
		}
	}
	return result.String()
}

// CleanupOpenaiFields removes temporary fields used during transformation
// Note: This should be called AFTER vendor transforms have processed x_thinking
func CleanupOpenaiFields(req *openai.ChatCompletionNewParams) {
	// Clear empty tools array
	if len(req.Tools) == 0 {
		req.Tools = nil
	}

	for i := range req.Messages {
		if req.Messages[i].OfAssistant != nil {
			// Convert to map to remove temporary fields
			msgMap := req.Messages[i].ExtraFields()
			if msgMap == nil {
				continue
			}

			// Remove x_thinking field:
			// - If reasoning_content exists, the vendor transform (DeepSeek/Moonshot) has converted it
			// - If reasoning_content doesn't exist, x_thinking is not needed by other providers
			//   (they don't support thinking mode in the same way)
			delete(msgMap, "x_thinking")

			req.Messages[i].SetExtraFields(msgMap)
		}
	}
}
