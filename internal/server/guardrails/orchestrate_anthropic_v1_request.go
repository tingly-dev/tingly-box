package serverguardrails

import (
	"context"

	"github.com/anthropics/anthropic-sdk-go"

	"github.com/tingly-dev/tingly-box/internal/guardrails"
)

// EvaluateAnthropicV1ToolResultRequest is the v1 equivalent of
// EvaluateAnthropicBetaToolResultRequest. It is intentionally kept separate so
// protocol-specific request types stay explicit in the adapter layer.
func EvaluateAnthropicV1ToolResultRequest(
	ctx context.Context,
	engine guardrails.Guardrails,
	session SessionMeta,
	req *anthropic.MessageNewParams,
) (ToolResultMutation, error) {
	adapted := AdaptToolResultRequestFromAnthropicV1(req)
	if !adapted.HasToolResult {
		return ToolResultMutation{Adapted: adapted}, nil
	}

	evaluation, err := EvaluateRequestView(ctx, engine, session, adapted.View)
	if err != nil {
		return ToolResultMutation{}, err
	}

	changed, message := MutateAnthropicV1ToolResultRequest(req, evaluation)
	return ToolResultMutation{
		Adapted:    adapted,
		Evaluation: evaluation,
		Changed:    changed,
		Message:    message,
	}, nil
}
