package mutate

import (
	"strings"

	"github.com/anthropics/anthropic-sdk-go"

	guardrailsadapter "github.com/tingly-dev/tingly-box/internal/guardrails/adapter"
	guardrailscore "github.com/tingly-dev/tingly-box/internal/guardrails/core"
	guardrailsevaluate "github.com/tingly-dev/tingly-box/internal/guardrails/evaluate"
)

// MutateAnthropicV1ToolResultRequest applies request-side Guardrails
// enforcement to Anthropic v1 tool_result payloads.
func MutateAnthropicV1ToolResultRequest(req *anthropic.MessageNewParams, evaluation guardrailsevaluate.Evaluation) (bool, string) {
	if req == nil || evaluation.Result.Verdict != guardrailscore.VerdictBlock {
		return false, ""
	}

	message := BlockMessageForToolResult(evaluation.Result)
	if message == "" || strings.HasPrefix(evaluation.Input.Content.Text, guardrailsadapter.BlockPrefix) {
		return false, ""
	}

	guardrailsadapter.ReplaceToolResultContentV1(req.Messages, message)
	return true, message
}

// MutateAnthropicBetaToolResultRequest applies request-side Guardrails
// enforcement to Anthropic beta tool_result payloads.
func MutateAnthropicBetaToolResultRequest(req *anthropic.BetaMessageNewParams, evaluation guardrailsevaluate.Evaluation) (bool, string) {
	if req == nil || evaluation.Result.Verdict != guardrailscore.VerdictBlock {
		return false, ""
	}

	message := BlockMessageForToolResult(evaluation.Result)
	if message == "" || strings.HasPrefix(evaluation.Input.Content.Text, guardrailsadapter.BlockPrefix) {
		return false, ""
	}

	guardrailsadapter.ReplaceToolResultContentV1Beta(req.Messages, message)
	return true, message
}
