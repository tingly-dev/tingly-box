package serverguardrails

import (
	"github.com/anthropics/anthropic-sdk-go"

	"github.com/tingly-dev/tingly-box/internal/guardrails"
)

// MutateAnthropicV1Response applies Guardrails evaluation output to a fully
// assembled Anthropic v1 response. The first mutation supported here is block
// enforcement: when the aggregated verdict is block, the original content is
// replaced with a single text block carrying the block message.
func MutateAnthropicV1Response(resp *anthropic.Message, evaluation Evaluation) (bool, string) {
	if resp == nil || evaluation.Result.Verdict != guardrails.VerdictBlock {
		return false, ""
	}

	blockMessage := blockMessageForEvaluation(evaluation)
	resp.Content = []anthropic.ContentBlockUnion{{
		Type: "text",
		Text: blockMessage,
	}}
	resp.StopReason = anthropic.StopReasonEndTurn
	return true, blockMessage
}
