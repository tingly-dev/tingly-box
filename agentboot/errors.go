package agentboot

import (
	"errors"
	"fmt"
	"strings"
)

// ErrNoTerminalResult means a stream-json process ended cleanly without the
// result event that defines the outcome of an agent turn.
var ErrNoTerminalResult = errors.New("agentboot: stream ended without a terminal result")

// ProcessError reports an unsuccessful agent subprocess exit.
type ProcessError struct {
	AgentType AgentType
	ExitCode  int
	Err       error
}

func (e *ProcessError) Error() string {
	agent := e.AgentType.String()
	if agent == "" {
		agent = "agent"
	}
	if e.ExitCode >= 0 {
		return fmt.Sprintf("agentboot: %s process exited with code %d: %v", agent, e.ExitCode, e.Err)
	}
	return fmt.Sprintf("agentboot: %s process failed: %v", agent, e.Err)
}

// Unwrap exposes the underlying process error.
func (e *ProcessError) Unwrap() error { return e.Err }

// ProtocolError reports a fatal stream protocol failure.
type ProtocolError struct {
	AgentType AgentType
	Err       error
}

func (e *ProtocolError) Error() string {
	agent := e.AgentType.String()
	if agent == "" {
		agent = "agent"
	}
	return fmt.Sprintf("agentboot: %s protocol error: %v", agent, e.Err)
}

// Unwrap exposes the decoder or protocol error. In particular,
// errors.Is(err, ErrNoTerminalResult) remains supported.
func (e *ProtocolError) Unwrap() error { return e.Err }

// ResultError reports a structured terminal error emitted by the agent.
type ResultError struct {
	AgentType AgentType
	Subtype   string
	Details   []string
}

func (e *ResultError) Error() string {
	agent := e.AgentType.String()
	if agent == "" {
		agent = "agent"
	}
	message := fmt.Sprintf("agentboot: %s returned an error result", agent)
	if e.Subtype != "" {
		message += " (" + e.Subtype + ")"
	}
	if len(e.Details) > 0 {
		message += ": " + strings.Join(e.Details, "; ")
	}
	return message
}
