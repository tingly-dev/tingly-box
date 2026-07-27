package remoteagent

import (
	"context"
	"errors"
	"sync"
)

// errExecutionBusy is returned by executionRegistry.begin when the chat
// already has a running execution. Callers detect it with errors.Is to render
// the guided "Session Busy" message instead of a raw error dump.
var errExecutionBusy = errors.New("another execution is already in progress for this chat. Please wait for it to complete or use /stop to cancel it")

// executionRegistry tracks the one running execution per chat and owns the
// cancel functions. It is the single mechanism behind both the duplicate-
// execution guard (begin) and /stop (cancel) — the two used to be separate
// map mutations with divergent locking discipline.
type executionRegistry struct {
	mu      sync.Mutex
	cancels map[string]context.CancelFunc
}

func newExecutionRegistry() *executionRegistry {
	return &executionRegistry{cancels: make(map[string]context.CancelFunc)}
}

// begin registers cancel as the chat's running execution. It fails with
// errExecutionBusy when one is already registered — check-and-set happens
// atomically so an execution can never trip over its own entry.
func (r *executionRegistry) begin(chatID string, cancel context.CancelFunc) error {
	r.mu.Lock()
	defer r.mu.Unlock()
	if _, exists := r.cancels[chatID]; exists {
		return errExecutionBusy
	}
	r.cancels[chatID] = cancel
	return nil
}

// end removes the chat's entry without cancelling. Call when the execution
// finishes; a no-op if /stop already removed it.
func (r *executionRegistry) end(chatID string) {
	r.mu.Lock()
	defer r.mu.Unlock()
	delete(r.cancels, chatID)
}

// cancel stops the chat's running execution, reporting whether one was
// running. The entry is removed here so a second /stop reports "nothing to
// stop" instead of double-cancelling.
func (r *executionRegistry) cancel(chatID string) bool {
	r.mu.Lock()
	cancel, exists := r.cancels[chatID]
	if exists {
		delete(r.cancels, chatID)
	}
	r.mu.Unlock()

	if exists && cancel != nil {
		cancel()
		return true
	}
	return false
}
