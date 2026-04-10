package mutate

import (
	"github.com/anthropics/anthropic-sdk-go"

	guardrailscore "github.com/tingly-dev/tingly-box/internal/guardrails/core"
)

func MaskAnthropicV1RequestCredentials(
	req *anthropic.MessageNewParams,
	credentials []guardrailscore.ProtectedCredential,
	state *guardrailscore.CredentialMaskState,
) (bool, bool) {
	if req == nil || len(credentials) == 0 {
		return false, false
	}

	changed := false
	latestTurnChanged := false
	for i := range req.System {
		if next, ok := guardrailscore.AliasText(req.System[i].Text, credentials, state); ok {
			req.System[i].Text = next
			changed = true
		}
	}
	for i := range req.Messages {
		messageChanged, tailChanged := maskAnthropicV1Blocks(req.Messages[i].Content, credentials, state)
		if messageChanged {
			changed = true
		}
		if i == len(req.Messages)-1 && tailChanged {
			latestTurnChanged = true
		}
	}
	return changed, latestTurnChanged
}

func maskAnthropicV1Blocks(
	blocks []anthropic.ContentBlockParamUnion,
	credentials []guardrailscore.ProtectedCredential,
	state *guardrailscore.CredentialMaskState,
) (bool, bool) {
	changed := false
	tailChanged := false
	for i := range blocks {
		block := &blocks[i]
		blockChanged := false
		if block.OfText != nil {
			if next, ok := guardrailscore.AliasText(block.OfText.Text, credentials, state); ok {
				block.OfText.Text = next
				changed = true
				blockChanged = true
			}
		}
		if block.OfToolResult != nil {
			for j := range block.OfToolResult.Content {
				content := &block.OfToolResult.Content[j]
				if content.OfText != nil {
					if next, ok := guardrailscore.AliasText(content.OfText.Text, credentials, state); ok {
						content.OfText.Text = next
						changed = true
						blockChanged = true
					}
				}
			}
		}
		if block.OfToolUse != nil {
			if next, ok := guardrailscore.AliasStructuredValue(block.OfToolUse.Input, credentials, state); ok {
				if args, ok := next.(map[string]interface{}); ok {
					block.OfToolUse.Input = args
					changed = true
					blockChanged = true
				}
			}
		}
		if i == len(blocks)-1 && blockChanged {
			tailChanged = true
		}
	}
	return changed, tailChanged
}

func MaskAnthropicBetaRequestCredentials(
	req *anthropic.BetaMessageNewParams,
	credentials []guardrailscore.ProtectedCredential,
	state *guardrailscore.CredentialMaskState,
) (bool, bool) {
	if req == nil || len(credentials) == 0 {
		return false, false
	}

	changed := false
	latestTurnChanged := false
	for i := range req.System {
		if next, ok := guardrailscore.AliasText(req.System[i].Text, credentials, state); ok {
			req.System[i].Text = next
			changed = true
		}
	}
	for i := range req.Messages {
		blockChanged := maskAnthropicBetaBlocks(req.Messages[i].Content, credentials, state)
		if blockChanged {
			changed = true
		}
		if i == len(req.Messages)-1 && blockChanged {
			latestTurnChanged = true
		}
	}
	return changed, latestTurnChanged
}

func maskAnthropicBetaBlocks(
	blocks []anthropic.BetaContentBlockParamUnion,
	credentials []guardrailscore.ProtectedCredential,
	state *guardrailscore.CredentialMaskState,
) bool {
	changed := false
	for i := range blocks {
		block := &blocks[i]
		if block.OfText != nil {
			if next, ok := guardrailscore.AliasText(block.OfText.Text, credentials, state); ok {
				block.OfText.Text = next
				changed = true
			}
		}
		if block.OfToolResult != nil {
			for j := range block.OfToolResult.Content {
				content := &block.OfToolResult.Content[j]
				if content.OfText != nil {
					if next, ok := guardrailscore.AliasText(content.OfText.Text, credentials, state); ok {
						content.OfText.Text = next
						changed = true
					}
				}
			}
		}
		if block.OfToolUse != nil {
			if next, ok := guardrailscore.AliasStructuredValue(block.OfToolUse.Input, credentials, state); ok {
				if args, ok := next.(map[string]interface{}); ok {
					block.OfToolUse.Input = args
					changed = true
				}
			}
		}
	}
	return changed
}
