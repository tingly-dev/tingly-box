package agentboot

import (
	"errors"
	"sync"
	"time"

	"github.com/tingly-dev/tingly-box/agentboot/common"
)

// runState is the shared accumulator driven by the pump goroutine.
type runState struct {
	mu sync.Mutex

	opts         ExecutionOptions
	startTime    time.Time
	events       []common.Event
	pendingInput map[string]map[string]any
	terminalSeen bool
	terminalErr  *ResultError
	processErr   *ProcessError
	protocolErr  *ProtocolError
	exitCode     int
}

func newProcessError(agentType AgentType, err error) *ProcessError {
	exitCode := -1
	type exitCoder interface {
		ExitCode() int
	}
	var withExitCode exitCoder
	if errors.As(err, &withExitCode) {
		exitCode = withExitCode.ExitCode()
	}
	return &ProcessError{
		AgentType: agentType,
		ExitCode:  exitCode,
		Err:       err,
	}
}

func resultErrorFromEvent(agentType AgentType, event common.Event) *ResultError {
	resultErr := &ResultError{
		AgentType: agentType,
		Subtype:   stringValue(event.Data["subtype"]),
	}
	if values, ok := event.Data["errors"].([]any); ok {
		for _, value := range values {
			if detail := stringValue(value); detail != "" {
				resultErr.Details = append(resultErr.Details, detail)
			}
		}
	}
	if len(resultErr.Details) == 0 {
		for _, key := range []string{"error", "result"} {
			if detail := stringValue(event.Data[key]); detail != "" {
				resultErr.Details = append(resultErr.Details, detail)
				break
			}
		}
	}
	return resultErr
}

func stringValue(value any) string {
	text, _ := value.(string)
	return text
}
