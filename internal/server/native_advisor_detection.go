package server

import (
	"encoding/json"
	"strings"

	"github.com/anthropics/anthropic-sdk-go"

	"github.com/tingly-dev/tingly-box/internal/protocol"
)

func hasNativeAdvisorBeta(req protocol.AnthropicBetaMessagesRequest) bool {
	return betaHistoryHasAdvisorBlocks(req.BetaMessageNewParams.Messages)
}

func betaHistoryHasAdvisorBlocks(messages []anthropic.BetaMessageParam) bool {
	for _, msg := range messages {
		for _, block := range msg.Content {
			var raw map[string]any
			bytes, err := json.Marshal(block)
			if err != nil {
				continue
			}
			if err := json.Unmarshal(bytes, &raw); err != nil {
				continue
			}
			if blockType, _ := raw["type"].(string); strings.EqualFold(blockType, "advisor_tool_result") {
				return true
			}
			if blockType, _ := raw["type"].(string); strings.EqualFold(blockType, "server_tool_use") {
				if name, _ := raw["name"].(string); strings.EqualFold(name, "advisor") {
					return true
				}
			}
		}
	}
	return false
}
