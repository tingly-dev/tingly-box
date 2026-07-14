package toolloop

import (
	"context"
	"errors"
	"fmt"
)

// ToolDefinition is the protocol-neutral description injected into the
// ToolLoop Stage's native request protocol.
type ToolDefinition struct {
	Name        string
	Description string
	Parameters  map[string]any
}

// ToolCall is one model-requested invocation after protocol-native assembly.
// Arguments contains the model's JSON text unchanged; the execution backend is
// responsible for schema validation.
type ToolCall struct {
	ID        string
	Name      string
	Arguments string
}

// ToolResult is the protocol-neutral value appended to the next model round.
type ToolResult struct {
	ToolCallID string
	Content    string
	IsError    bool
}

// ToolCatalog lists the server-visible tools for one request. The returned
// definitions also form the ownership set: calls whose names are absent remain
// client/external tool calls and are returned outward unchanged.
type ToolCatalog interface {
	ListTools(ctx context.Context) ([]ToolDefinition, error)
}

// ToolPolicy authorizes a server-owned call immediately before execution.
type ToolPolicy interface {
	Authorize(ctx context.Context, call ToolCall) error
}

// ToolExecutor invokes one server-owned tool. It may return an updated context
// for request-scoped state such as advisor depth or credentials.
type ToolExecutor interface {
	Execute(ctx context.Context, call ToolCall) (context.Context, ToolResult, error)
}

// AllowAllPolicy is the explicit default used when no additional authorization
// layer is configured.
type AllowAllPolicy struct{}

func (AllowAllPolicy) Authorize(context.Context, ToolCall) error { return nil }

var ErrMaxRounds = errors.New("tool loop reached the maximum number of rounds")

// ExecutionError preserves the irreversible-side-effect boundary when a later
// provider round or policy check fails. Failover code can inspect it without
// importing a concrete ToolLoop implementation.
type ExecutionError struct {
	Err                  error
	SideEffectsCommitted bool
}

func (e *ExecutionError) Error() string {
	if e == nil || e.Err == nil {
		return "tool loop failed"
	}
	return e.Err.Error()
}

func (e *ExecutionError) Unwrap() error {
	if e == nil {
		return nil
	}
	return e.Err
}

// WrapError annotates err only when a successful tool execution has already
// committed side effects. Before that boundary the original error is retained
// so existing retry classification remains unchanged.
func WrapError(err error, sideEffectsCommitted bool) error {
	if err == nil || !sideEffectsCommitted {
		return err
	}
	return &ExecutionError{Err: err, SideEffectsCommitted: true}
}

// HasCommittedSideEffects reports whether retrying the whole provider attempt
// could replay an already successful tool action.
func HasCommittedSideEffects(err error) bool {
	var executionErr *ExecutionError
	return errors.As(err, &executionErr) && executionErr.SideEffectsCommitted
}

func validateDependencies(catalog ToolCatalog, executor ToolExecutor) error {
	if catalog == nil {
		return errors.New("tool loop catalog is nil")
	}
	if executor == nil {
		return errors.New("tool loop executor is nil")
	}
	return nil
}

func validateDefinitions(definitions []ToolDefinition) error {
	seen := make(map[string]struct{}, len(definitions))
	for i, definition := range definitions {
		if definition.Name == "" {
			return fmt.Errorf("tool definition at index %d has an empty name", i)
		}
		if _, exists := seen[definition.Name]; exists {
			return fmt.Errorf("tool definition %q is duplicated", definition.Name)
		}
		seen[definition.Name] = struct{}{}
	}
	return nil
}
