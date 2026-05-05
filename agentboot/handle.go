package agentboot

import (
	"context"
	"errors"
	"sync"
	"sync/atomic"
)

// Sentinel errors returned by [ExecutionHandle.Respond].
var (
	// ErrHandleClosed is returned when the handle's event stream has already
	// closed (process exited, Cancel called, or ctx canceled).
	ErrHandleClosed = errors.New("agentboot: execution handle closed")

	// ErrUnknownRequestID is returned when reqID does not correspond to a
	// pending control request, either because the ID is wrong or because
	// the runner already received a response for it.
	ErrUnknownRequestID = errors.New("agentboot: unknown request id")
)

// ExecutionHandle is the per-execution result of [Agent.Execute].
//
// Lifecycle:
//  1. Caller iterates Events() to consume the totally-ordered event stream.
//  2. For ApprovalRequestEvent / AskRequestEvent, caller computes a response
//     and calls Respond(req.ID, response). The runner encodes the response
//     and forwards it to the agent process's stdin.
//  3. The channel closes after the underlying process has exited AND every
//     decoded event has been delivered to the channel.
//  4. After the channel closes, Wait() returns immediately with the
//     aggregated Result.
//
// Cancellation:
//   - Cancel() requests cooperative shutdown; idempotent. The runner kills
//     the process and joins all goroutines; the channel then closes.
//   - The Context passed to Execute can also be canceled to the same effect.
//
// Concurrency:
//   - Events() may be consumed by exactly one goroutine.
//   - Respond / Cancel / Wait are safe to call concurrently from any goroutine.
type ExecutionHandle interface {
	Events() <-chan StreamEvent
	Respond(reqID string, resp ControlResponse) error
	Wait() (*Result, error)
	Cancel()
}

// ControlResponse is the sum type passed to [ExecutionHandle.Respond].
//
// The interface is sealed; agentboot owns the closed set of response shapes.
type ControlResponse interface {
	isControlResponse()
}

// ApprovalResponse responds to an [ApprovalRequestEvent].
type ApprovalResponse struct {
	Approved     bool
	UpdatedInput map[string]any
	Reason       string
}

func (ApprovalResponse) isControlResponse() {}

// AskResponse responds to an [AskRequestEvent].
type AskResponse struct {
	Approved     bool
	UpdatedInput map[string]any
	Reason       string
	Response     string
	Selection    map[string]any
}

func (AskResponse) isControlResponse() {}

// --- runnerHandle: concrete ExecutionHandle implementation -------------------

// runnerHandle is the concrete ExecutionHandle returned by Runner.Execute.
//
// It is intentionally designed as a thin façade with closure-based hooks
// (responseFn, cancelFn, waitFn) so that tests can drive it without the full
// runner+process+protocol stack. The runner wires the closures to the
// response router, ctx cancel, and goroutine-join code paths respectively.
type runnerHandle struct {
	events     chan StreamEvent
	responseFn func(reqID string, resp ControlResponse) error
	cancelFn   func()
	waitFn     func() (*Result, error)

	pendingMu sync.Mutex
	pending   map[string]struct{}

	closed    atomic.Bool
	closeOnce sync.Once
	cancelOne sync.Once
}

// newRunnerHandle constructs a runnerHandle. The events channel is owned by
// the handle; the runner's pump goroutine pushes events via emit() and calls
// closeStream() exactly once when the process has exited and decoding is
// finished.
//
// All closures are required.
func newRunnerHandle(
	bufferSize int,
	responseFn func(reqID string, resp ControlResponse) error,
	cancelFn func(),
	waitFn func() (*Result, error),
) *runnerHandle {
	return &runnerHandle{
		events:     make(chan StreamEvent, bufferSize),
		responseFn: responseFn,
		cancelFn:   cancelFn,
		waitFn:     waitFn,
		pending:    make(map[string]struct{}),
	}
}

// emit pushes a stream event to the user-facing channel.
//
// If the event is a control request (Approval/Ask), its ID is registered as
// pending before the event becomes visible to the consumer, so that an
// immediate Respond from the consumer will find it. If ctx is canceled
// before the consumer reads the event, emit drops it and returns; in that
// case the pending entry is cleaned up.
func (h *runnerHandle) emit(ctx context.Context, ev StreamEvent) {
	var pendingID string
	switch e := ev.(type) {
	case ApprovalRequestEvent:
		pendingID = e.ID
	case AskRequestEvent:
		pendingID = e.ID
	}
	if pendingID != "" {
		h.pendingMu.Lock()
		h.pending[pendingID] = struct{}{}
		h.pendingMu.Unlock()
	}

	select {
	case h.events <- ev:
	case <-ctx.Done():
		if pendingID != "" {
			h.pendingMu.Lock()
			delete(h.pending, pendingID)
			h.pendingMu.Unlock()
		}
	}
}

// closeStream closes the user-facing event channel exactly once.
// After calling this, Respond returns ErrHandleClosed.
func (h *runnerHandle) closeStream() {
	h.closeOnce.Do(func() {
		h.closed.Store(true)
		close(h.events)
	})
}

func (h *runnerHandle) Events() <-chan StreamEvent { return h.events }

func (h *runnerHandle) Respond(reqID string, resp ControlResponse) error {
	if h.closed.Load() {
		return ErrHandleClosed
	}
	h.pendingMu.Lock()
	if _, ok := h.pending[reqID]; !ok {
		h.pendingMu.Unlock()
		return ErrUnknownRequestID
	}
	delete(h.pending, reqID)
	h.pendingMu.Unlock()

	if h.closed.Load() {
		return ErrHandleClosed
	}
	return h.responseFn(reqID, resp)
}

func (h *runnerHandle) Wait() (*Result, error) { return h.waitFn() }

func (h *runnerHandle) Cancel() {
	h.cancelOne.Do(h.cancelFn)
}

// HandleControls bundles the operations a custom Agent implementation uses
// to drive an [ExecutionHandle] it created via [NewControlledHandle].
//
// Emit and Close are the inverse of Events() and channel-close that the
// consumer observes: the producer pushes events with Emit and signals
// completion with Close.
type HandleControls struct {
	h *runnerHandle
}

// Emit pushes a stream event to the handle's Events channel. It blocks
// until the consumer reads the event, or until ctx is canceled — in which
// case the event is dropped and any pending control registration is
// cleaned up automatically.
func (c *HandleControls) Emit(ctx context.Context, ev StreamEvent) {
	c.h.emit(ctx, ev)
}

// Close closes the handle's Events channel exactly once. After Close,
// [ExecutionHandle.Respond] returns [ErrHandleClosed].
func (c *HandleControls) Close() {
	c.h.closeStream()
}

// NewControlledHandle builds an [ExecutionHandle] whose lifecycle is driven
// directly by the caller via the returned [HandleControls].
//
// Use this from Agent implementations that do not go through the
// process+protocol pipeline (e.g. in-process mocks).
//
// The supplied closures define:
//   - responseFn: how [ExecutionHandle.Respond] routes a [ControlResponse]
//     back to the in-flight execution. For an in-process agent this is
//     typically a channel send to a goroutine waiting for the response.
//   - cancelFn: how [ExecutionHandle.Cancel] requests cooperative shutdown.
//   - waitFn: how [ExecutionHandle.Wait] gathers the final [Result] and error.
func NewControlledHandle(
	bufferSize int,
	responseFn func(reqID string, resp ControlResponse) error,
	cancelFn func(),
	waitFn func() (*Result, error),
) (ExecutionHandle, *HandleControls) {
	h := newRunnerHandle(bufferSize, responseFn, cancelFn, waitFn)
	return h, &HandleControls{h: h}
}
