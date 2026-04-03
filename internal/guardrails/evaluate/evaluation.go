package evaluate

import (
	"context"
	"errors"

	guardrailscore "github.com/tingly-dev/tingly-box/internal/guardrails/core"
)

var ErrNoGuardrailsRuntime = errors.New("guardrails runtime or policy engine is nil")

// Runtime is the minimal evaluation surface needed by EvaluateInput.
type Runtime interface {
	Evaluate(ctx context.Context, input guardrailscore.Input) (guardrailscore.Result, error)
}

// Evaluation holds both the normalized guardrails input and the aggregated
// engine result so later mutation stages can reuse the exact evaluated input.
type Evaluation struct {
	Input  guardrailscore.Input
	Result guardrailscore.Result
}

// EvaluateInput runs Guardrails evaluation on an already normalized input.
func EvaluateInput(ctx context.Context, runtime Runtime, input guardrailscore.Input) (Evaluation, error) {
	if runtime == nil {
		return Evaluation{}, ErrNoGuardrailsRuntime
	}

	result, err := runtime.Evaluate(ctx, input)
	if err != nil {
		return Evaluation{}, err
	}

	return Evaluation{
		Input:  input,
		Result: result,
	}, nil
}
