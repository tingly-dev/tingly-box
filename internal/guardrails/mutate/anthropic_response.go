package mutate

import (
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
