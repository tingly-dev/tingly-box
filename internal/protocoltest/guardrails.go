package protocoltest

import (
	"context"

	"github.com/tingly-dev/tingly-box/internal/guardrails"
	guardrailscore "github.com/tingly-dev/tingly-box/internal/guardrails/core"
)

// NewAllowGuardrailsRuntime returns a deterministic active runtime for matrix
// compatibility checks. It evaluates every lifecycle phase but never changes
// the semantic response expected by the shared scenarios.
func NewAllowGuardrailsRuntime() *guardrails.Guardrails {
	return &guardrails.Guardrails{
		Policy:            allowGuardrailsPolicy{},
		HasActivePolicies: true,
	}
}

type allowGuardrailsPolicy struct{}

func (allowGuardrailsPolicy) Evaluate(context.Context, guardrailscore.Input) (guardrailscore.Result, error) {
	return guardrailscore.Result{Verdict: guardrailscore.VerdictAllow}, nil
}
