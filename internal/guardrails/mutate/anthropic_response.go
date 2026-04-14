package mutate

import (
	"encoding/json"

	"github.com/anthropics/anthropic-sdk-go"

	guardrailscore "github.com/tingly-dev/tingly-box/internal/guardrails/core"
	guardrailsevaluate "github.com/tingly-dev/tingly-box/internal/guardrails/evaluate"
)

// MutateAnthropicV1Response applies Guardrails evaluation output to a fully
// assembled Anthropic v1 response.
func MutateAnthropicV1Response(resp *anthropic.Message, evaluation guardrailsevaluate.Evaluation) (bool, string) {
	if resp == nil || evaluation.Result.Verdict != guardrailscore.VerdictBlock {
		return false, ""
	}

	blockMessage := BlockMessageForEvaluation(evaluation)
	resp.Content = []anthropic.ContentBlockUnion{{
		Type: "text",
		Text: blockMessage,
	}}
	resp.StopReason = anthropic.StopReasonEndTurn
	return true, blockMessage
}

// MutateAnthropicV1BetaResponse applies Guardrails evaluation output to a fully
// assembled Anthropic beta response.
func MutateAnthropicV1BetaResponse(resp *anthropic.BetaMessage, evaluation guardrailsevaluate.Evaluation) (bool, string) {
	if resp == nil || evaluation.Result.Verdict != guardrailscore.VerdictBlock {
		return false, ""
	}

	blockMessage := BlockMessageForEvaluation(evaluation)
	resp.Content = []anthropic.BetaContentBlockUnion{{
		Type: "text",
		Text: blockMessage,
	}}
	resp.StopReason = anthropic.BetaStopReasonEndTurn
	return true, blockMessage
}

func RestoreAnthropicV1ResponseCredentials(state *guardrailscore.CredentialMaskState, resp *anthropic.Message) bool {
	if resp == nil {
		return false
	}
	return restoreAnthropicResponseBlocks(resp.Content, state)
}

func RestoreAnthropicV1BetaResponseCredentials(state *guardrailscore.CredentialMaskState, resp *anthropic.BetaMessage) bool {
	if resp == nil {
		return false
	}
	return restoreAnthropicBetaResponseBlocks(resp.Content, state)
}

func restoreAnthropicResponseBlocks(blocks []anthropic.ContentBlockUnion, state *guardrailscore.CredentialMaskState) bool {
	if state == nil || len(state.AliasToReal) == 0 {
		return false
	}
	changed := false
	for i := range blocks {
		block := &blocks[i]
		if guardrailscore.MayContainAliasToken(block.Text) {
			if text, ok := guardrailscore.RestoreText(block.Text, state); ok {
				block.Text = text
				changed = true
			}
		}
		if len(block.Input) == 0 || !guardrailscore.MayContainAliasToken(string(block.Input)) {
			continue
		}
		var parsed interface{}
		if err := json.Unmarshal(block.Input, &parsed); err != nil {
			if restored, ok := guardrailscore.RestoreText(string(block.Input), state); ok {
				block.Input = json.RawMessage(restored)
				changed = true
			}
			continue
		}
		restored, ok := guardrailscore.RestoreStructuredValue(parsed, state)
		if !ok {
			continue
		}
		payload, err := json.Marshal(restored)
		if err != nil {
			continue
		}
		block.Input = payload
		changed = true
	}
	return changed
}

func restoreAnthropicBetaResponseBlocks(blocks []anthropic.BetaContentBlockUnion, state *guardrailscore.CredentialMaskState) bool {
	if state == nil || len(state.AliasToReal) == 0 {
		return false
	}
	changed := false
	for i := range blocks {
		block := &blocks[i]
		if guardrailscore.MayContainAliasToken(block.Text) {
			if text, ok := guardrailscore.RestoreText(block.Text, state); ok {
				block.Text = text
				changed = true
			}
		}
		if len(block.Input) == 0 || !guardrailscore.MayContainAliasToken(string(block.Input)) {
			continue
		}
		var parsed interface{}
		if err := json.Unmarshal(block.Input, &parsed); err != nil {
			if restored, ok := guardrailscore.RestoreText(string(block.Input), state); ok {
				block.Input = json.RawMessage(restored)
				changed = true
			}
			continue
		}
		restored, ok := guardrailscore.RestoreStructuredValue(parsed, state)
		if !ok {
			continue
		}
		payload, err := json.Marshal(restored)
		if err != nil {
			continue
		}
		block.Input = payload
		changed = true
	}
	return changed
}
