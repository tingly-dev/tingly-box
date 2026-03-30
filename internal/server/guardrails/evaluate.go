package serverguardrails

import (
	"context"
	"errors"

	"github.com/tingly-dev/tingly-box/internal/guardrails"
)

var ErrNoGuardrailsEngine = errors.New("guardrails engine is nil")

// Evaluation holds both the normalized guardrails input and the aggregated
// engine result. Keeping both makes later mutation/enforcement stages explicit:
// adapters build the input, evaluate consumes it, and protocol writers can use
// the same input/result pair without rebuilding anything.
type Evaluation struct {
	Input  guardrails.Input
	Result guardrails.Result
}

// EvaluateResponseView runs Guardrails evaluation on an adapted response view.
// It is a standalone building block for the future Adapt -> Evaluate -> Mutate
// pipeline and is not yet wired into the existing server handlers.
func EvaluateResponseView(ctx context.Context, engine guardrails.Guardrails, session SessionMeta, view ResponseView) (Evaluation, error) {
	if engine == nil {
		return Evaluation{}, ErrNoGuardrailsEngine
	}

	input := ToResponseInput(session, view)
	result, err := engine.Evaluate(ctx, input)
	if err != nil {
		return Evaluation{}, err
	}

	return Evaluation{
		Input:  input,
		Result: result,
	}, nil
}

// EvaluateRequestView runs Guardrails evaluation on an adapted request view.
// This is primarily for request-side checks such as tool_result filtering and
// keeps the request path aligned with the same adapter/evaluate boundary.
func EvaluateRequestView(ctx context.Context, engine guardrails.Guardrails, session SessionMeta, view RequestView) (Evaluation, error) {
	if engine == nil {
		return Evaluation{}, ErrNoGuardrailsEngine
	}

	input := ToRequestInput(session, view)
	result, err := engine.Evaluate(ctx, input)
	if err != nil {
		return Evaluation{}, err
	}

	return Evaluation{
		Input:  input,
		Result: result,
	}, nil
}
