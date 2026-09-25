package stage

import (
	"fmt"
	"strings"
)

// Compose wraps terminal with stages written in request order, from outermost
// to innermost. For example:
//
//	Compose(provider, guardrails, tools)
//
// produces guardrails(tools(provider)). Responses and stream events naturally
// return through tools and then guardrails.
//
// Compose performs structural validation only. It never invokes Complete or
// Stream and it never inserts an implicit protocol conversion.
func Compose(terminal Endpoint, stages ...Stage) (Endpoint, error) {
	if terminal == nil {
		return nil, fmt.Errorf("compose protocol stages: terminal endpoint is nil")
	}

	currentProtocol := terminal.Protocol()
	if err := checkChainProtocol(currentProtocol); err != nil {
		return nil, fmt.Errorf("compose protocol stages: terminal endpoint: %w", err)
	}

	current := terminal
	for i := len(stages) - 1; i >= 0; i-- {
		stage := stages[i]
		if stage == nil {
			return nil, fmt.Errorf("compose protocol stages: stage at index %d is nil", i)
		}

		name := strings.TrimSpace(stage.Name())
		if name == "" {
			return nil, fmt.Errorf("compose protocol stages: stage at index %d has empty name", i)
		}

		stageProtocol := stage.Protocol()
		if err := checkChainProtocol(stageProtocol); err != nil {
			return nil, fmt.Errorf("compose protocol stages: stage %q: %w", name, err)
		}
		if stageProtocol != currentProtocol {
			return nil, fmt.Errorf(
				"compose protocol stages: stage %q speaks %q and cannot wrap endpoint speaking %q",
				name,
				stageProtocol,
				currentProtocol,
			)
		}

		wrapped := stage.Wrap(current)
		if wrapped == nil {
			return nil, fmt.Errorf("compose protocol stages: stage %q returned a nil endpoint", name)
		}
		if wrapped.Protocol() != stageProtocol {
			return nil, fmt.Errorf(
				"compose protocol stages: stage %q returned endpoint speaking %q, want %q",
				name,
				wrapped.Protocol(),
				stageProtocol,
			)
		}

		current = wrapped
		currentProtocol = stageProtocol
	}

	return current, nil
}
