package agenttask

import (
	"fmt"
	"strings"
)

// RepeatPolicy repeats the whole step pipeline until a structured condition
// over step outcomes holds, bounded by Max iterations. It is the tb-evaluated
// loop (design §12.3): the loop condition is a deterministic signal from a
// step, not agent judgment. Free-goal (no-steps) tasks use FollowUp instead
// (the until:agent case).
type RepeatPolicy struct {
	// Until is a condition expression (see evalCondition). Empty = no repeat.
	Until string `json:"until,omitempty"`
	// Max caps iterations (a required guardrail; <=0 treated as 1).
	Max int `json:"max,omitempty"`
	// Iteration is the runtime 0-based iteration counter.
	Iteration int `json:"iteration,omitempty"`
}

// evalCondition evaluates a when/until expression against step outcomes keyed
// by step ID. Grammar (v1, deliberately tight — no LLM judgment, 守则 1):
//
//	expr   := term ("&&" term)*
//	term   := "steps." <stepID> "." ("succeeded" | "failed" | "skipped")
//
// An empty expression is true. A term referencing a step with no recorded
// outcome is false (it has not run). Unparseable input is an error (fail
// closed — never silently treat a malformed condition as satisfied).
func evalCondition(expr string, outcomes map[string]StepOutcome) (bool, error) {
	expr = strings.TrimSpace(expr)
	if expr == "" {
		return true, nil
	}
	for _, raw := range strings.Split(expr, "&&") {
		term := strings.TrimSpace(raw)
		ok, err := evalTerm(term, outcomes)
		if err != nil {
			return false, err
		}
		if !ok {
			return false, nil
		}
	}
	return true, nil
}

func evalTerm(term string, outcomes map[string]StepOutcome) (bool, error) {
	rest, ok := strings.CutPrefix(term, "steps.")
	if !ok {
		return false, fmt.Errorf("condition term %q must start with steps.", term)
	}
	dot := strings.LastIndex(rest, ".")
	if dot <= 0 || dot == len(rest)-1 {
		return false, fmt.Errorf("condition term %q must be steps.<id>.<predicate>", term)
	}
	stepID, predicate := rest[:dot], rest[dot+1:]
	outcome, ran := outcomes[stepID]
	switch predicate {
	case "succeeded":
		return ran && outcomeSucceeded(outcome), nil
	case "failed":
		return ran && outcomeFailed(outcome), nil
	case "skipped":
		return ran && outcome.Result.State == "skipped", nil
	default:
		return false, fmt.Errorf("unknown predicate %q in %q (want succeeded|failed|skipped)", predicate, term)
	}
}

func outcomeSucceeded(o StepOutcome) bool {
	return o.Result.State == "done" && (o.Result.ExitCode == nil || *o.Result.ExitCode == 0)
}

func outcomeFailed(o StepOutcome) bool {
	if o.Result.State == "failed" {
		return true
	}
	return o.Result.ExitCode != nil && *o.Result.ExitCode != 0
}

// outcomesByID indexes completed step outcomes by their step ID.
func outcomesByID(outcomes []StepOutcome) map[string]StepOutcome {
	m := make(map[string]StepOutcome, len(outcomes))
	for _, o := range outcomes {
		m[o.StepID] = o
	}
	return m
}

// validateConditionRefs checks that every step ID referenced by a condition
// exists in the step set (typo guard at creation time).
func validateConditionRefs(expr string, stepIDs map[string]bool) error {
	expr = strings.TrimSpace(expr)
	if expr == "" || expr == "agent" {
		return nil
	}
	for _, raw := range strings.Split(expr, "&&") {
		term := strings.TrimSpace(raw)
		rest, ok := strings.CutPrefix(term, "steps.")
		if !ok {
			return fmt.Errorf("condition term %q must start with steps.", term)
		}
		dot := strings.LastIndex(rest, ".")
		if dot <= 0 {
			return fmt.Errorf("condition term %q must be steps.<id>.<predicate>", term)
		}
		if id := rest[:dot]; !stepIDs[id] {
			return fmt.Errorf("condition references unknown step %q", id)
		}
	}
	return nil
}
