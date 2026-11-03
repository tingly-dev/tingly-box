package agentboot

import (
	"context"
	"errors"
	"fmt"
	"strings"
)

// ErrNoTerminalResult means a stream-json process ended cleanly without the
// result event that defines the outcome of an agent turn.
var ErrNoTerminalResult = errors.New("agentboot: stream ended without a terminal result")

// PreOutputFailure reports whether err is the kind of failure that happens
// before the agent produced any output: a process launch failure, an early
// crash, or a stream that ended without a result. Only such failures may be
// auto-retried (see [RetryConfig]).
//
// The predicate alone is NOT sufficient to decide a retry — a crash mid-turn
// also yields a *ProcessError — callers must additionally verify that the
// failed attempt produced no output (an empty Result.Events on the one-shot
// path, a turn-output flag on the persistent path).
func PreOutputFailure(err error) bool {
	if err == nil {
		return false
	}
	if errors.Is(err, context.Canceled) || errors.Is(err, context.DeadlineExceeded) {
		return false
	}
	var resultErr *ResultError
	if errors.As(err, &resultErr) {
		// The agent completed the turn with an error result: side effects
		// already happened; re-running the prompt is not safe.
		return false
	}
	var termErr *SessionTerminatedError
	if errors.As(err, &termErr) {
		return termErr.ProcessFailure
	}
	var protoErr *ProtocolError
	if errors.As(err, &protoErr) {
		return errors.Is(protoErr.Err, ErrNoTerminalResult)
	}
	var procErr *ProcessError
	return errors.As(err, &procErr)
}

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

// SessionTerminatedError reports a [PersistentSession] terminating before the
// turn it was driving completed. ProcessFailure distinguishes a crash or
// fatal protocol error (true) from an explicit Close (false): a session
// closed on purpose is not a failure, and never a retryable one.
type SessionTerminatedError struct {
	Reason         string
	ProcessFailure bool
}

func (e *SessionTerminatedError) Error() string {
	return "agentboot: persistent session terminated before this turn completed: " + e.Reason
}

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
