package serverguardrails

import (
	"context"

	"github.com/anthropics/anthropic-sdk-go"

	"github.com/tingly-dev/tingly-box/internal/guardrails"
)

// ToolResultMutation captures the request-side mutation outcome for an
// Anthropic beta tool_result request. It keeps the adapted view, evaluation
// result, and mutation output together so the caller can inspect the full
// pipeline without reparsing the request.
type ToolResultMutation struct {
	Adapted    ToolResultRequestView
	Evaluation Evaluation
	Changed    bool
	Message    string
}

// EvaluateAnthropicBetaToolResultRequest is a standalone Adapt -> Evaluate ->
// Mutate pipeline for request-side tool_result filtering. It is intentionally
// not wired into the existing server handler yet.
func EvaluateAnthropicBetaToolResultRequest(
	ctx context.Context,
	engine guardrails.Guardrails,
	session SessionMeta,
	req *anthropic.BetaMessageNewParams,
) (ToolResultMutation, error) {
	adapted := AdaptToolResultRequestFromAnthropicBeta(req)
	if !adapted.HasToolResult {
		return ToolResultMutation{Adapted: adapted}, nil
	}

	evaluation, err := EvaluateRequestView(ctx, engine, session, adapted.View)
	if err != nil {
		return ToolResultMutation{}, err
	}

	changed, message := MutateAnthropicBetaToolResultRequest(req, evaluation)
	return ToolResultMutation{
		Adapted:    adapted,
		Evaluation: evaluation,
		Changed:    changed,
		Message:    message,
	}, nil
}
