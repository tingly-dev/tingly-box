package serverguardrails

import (
	"strings"

	"github.com/anthropics/anthropic-sdk-go"

	"github.com/tingly-dev/tingly-box/internal/guardrails"
)

// MutateAnthropicBetaToolResultRequest applies request-side Guardrails
// enforcement to Anthropic beta tool_result payloads. At the moment the only
// supported mutation is block enforcement, which rewrites tool_result content to
// the standard guardrails block message.
func MutateAnthropicBetaToolResultRequest(req *anthropic.BetaMessageNewParams, evaluation Evaluation) (bool, string) {
	if req == nil || evaluation.Result.Verdict != guardrails.VerdictBlock {
		return false, ""
	}

	message := BlockMessageForToolResult(evaluation.Result)
	if message == "" || strings.HasPrefix(evaluation.Input.Content.Text, BlockPrefix) {
		return false, ""
	}

	ReplaceToolResultContentV1Beta(req.Messages, message)
	return true, message
}
