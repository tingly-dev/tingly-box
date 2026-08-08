package stage

import (
	"fmt"
	"reflect"
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
	if isNil(terminal) {
		return nil, fmt.Errorf("compose protocol stages: terminal endpoint is nil")
	}

	currentProtocol := terminal.Protocol()
	if currentProtocol == "" {
		return nil, fmt.Errorf("compose protocol stages: terminal endpoint has empty protocol")
	}

	current := terminal
	for i := len(stages) - 1; i >= 0; i-- {
		stage := stages[i]
		if isNil(stage) {
			return nil, fmt.Errorf("compose protocol stages: stage at index %d is nil", i)
		}

		name := strings.TrimSpace(stage.Name())
		if name == "" {
			return nil, fmt.Errorf("compose protocol stages: stage at index %d has empty name", i)
		}

		stageProtocol := stage.Protocol()
		if stageProtocol == "" {
			return nil, fmt.Errorf("compose protocol stages: stage %q has empty protocol", name)
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
		if isNil(wrapped) {
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

// isNil recognizes typed nil pointers stored in an interface so Compose can
// report a validation error instead of panicking while calling their methods.
func isNil(value any) bool {
	if value == nil {
		return true
	}

	v := reflect.ValueOf(value)
	switch v.Kind() {
	case reflect.Chan, reflect.Func, reflect.Interface, reflect.Map, reflect.Pointer, reflect.Slice:
		return v.IsNil()
	default:
		return false
	}
}
