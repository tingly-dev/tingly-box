package serverguardrails

import (
	"strings"

	"github.com/anthropics/anthropic-sdk-go"

	"github.com/tingly-dev/tingly-box/internal/guardrails"
)

// MutateAnthropicV1ToolResultRequest applies request-side Guardrails
// enforcement to Anthropic v1 tool_result payloads. At the moment the only
// supported mutation is block enforcement, which rewrites tool_result content to
// the standard guardrails block message.
func MutateAnthropicV1ToolResultRequest(req *anthropic.MessageNewParams, evaluation Evaluation) (bool, string) {
	if req == nil || evaluation.Result.Verdict != guardrails.VerdictBlock {
		return false, ""
	}

	message := BlockMessageForToolResult(evaluation.Result)
	if message == "" || strings.HasPrefix(evaluation.Input.Content.Text, BlockPrefix) {
		return false, ""
	}

	ReplaceToolResultContentV1(req.Messages, message)
	return true, message
}
