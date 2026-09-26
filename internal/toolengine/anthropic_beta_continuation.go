package toolengine

import "github.com/anthropics/anthropic-sdk-go"

// mergeAnthropicBetaContinuation splices a stored mixed round (the assistant
// turn and its server-tool results) into the follow-up request: the client's
// tool_result blocks join the stored results in one user message, placed
// where the client's copy of the assistant turn was.
func mergeAnthropicBetaContinuation(segment []anthropic.BetaMessageParam, messages []anthropic.BetaMessageParam) []anthropic.BetaMessageParam {
	if len(segment) == 0 {
		return append([]anthropic.BetaMessageParam{}, messages...)
	}
	if len(messages) == 0 {
		return append([]anthropic.BetaMessageParam{}, segment...)
	}

	assistantIdx := -1
	toolResultIdx := -1
	for idx, msg := range messages {
		if assistantIdx == -1 && msg.Role == anthropic.BetaMessageParamRoleAssistant {
			for _, block := range msg.Content {
				if block.OfToolUse != nil {
					assistantIdx = idx
					break
				}
			}
		}
		if toolResultIdx == -1 && msg.Role == anthropic.BetaMessageParamRoleUser {
			for _, block := range msg.Content {
				if block.OfToolResult != nil {
					toolResultIdx = idx
					break
				}
			}
		}
		if assistantIdx != -1 && toolResultIdx != -1 {
			break
		}
	}
	if toolResultIdx == -1 {
		return append(append([]anthropic.BetaMessageParam{}, segment...), messages...)
	}

	merged := append([]anthropic.BetaMessageParam{}, segment...)
	lastIdx := len(merged) - 1
	merged[lastIdx].Content = append(append([]anthropic.BetaContentBlockParamUnion{}, merged[lastIdx].Content...), messages[toolResultIdx].Content...)
	if assistantIdx == -1 || toolResultIdx < assistantIdx {
		result := append([]anthropic.BetaMessageParam{}, merged...)
		result = append(result, messages[:toolResultIdx]...)
		result = append(result, messages[toolResultIdx+1:]...)
		return result
	}

	result := append([]anthropic.BetaMessageParam{}, messages[:assistantIdx]...)
	result = append(result, merged...)
	result = append(result, messages[toolResultIdx+1:]...)
	return result
}
