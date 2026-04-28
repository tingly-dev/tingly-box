package task

import (
	"context"
	"sync"
)

// cancelRegistry tracks the cancel function for every running task and
// which serialization key each running task holds.
type cancelRegistry struct {
	mu      sync.Mutex
	running map[string]context.CancelFunc // taskID → cancelFn
	locks   map[string]string             // serializationKey → running taskID
}

func newCancelRegistry() *cancelRegistry {
	return &cancelRegistry{
		running: make(map[string]context.CancelFunc),
		locks:   make(map[string]string),
	}
}

// register associates taskID with cancelFn and acquires key (if non-empty).
// Returns false if key is already locked by another task.
func (r *cancelRegistry) register(taskID, key string, cancelFn context.CancelFunc) bool {
	r.mu.Lock()
	defer r.mu.Unlock()
	if key != "" {
		if _, locked := r.locks[key]; locked {
			return false
		}
		r.locks[key] = taskID
	}
	r.running[taskID] = cancelFn
	return true
}

// unregister removes taskID and releases its key lock.
func (r *cancelRegistry) unregister(taskID, key string) {
	r.mu.Lock()
	defer r.mu.Unlock()
	delete(r.running, taskID)
	if key != "" && r.locks[key] == taskID {
		delete(r.locks, key)
	}
}

// cancel calls the cancel function for taskID. Returns true if found.
func (r *cancelRegistry) cancel(taskID string) bool {
	r.mu.Lock()
	fn, ok := r.running[taskID]
	r.mu.Unlock()
	if ok {
		fn()
	}
	return ok
}

// isLocked reports whether key is currently held by a running task.
func (r *cancelRegistry) isLocked(key string) bool {
	r.mu.Lock()
	defer r.mu.Unlock()
	_, ok := r.locks[key]
	return ok
}

// lockedBy returns the taskID holding key, or "" if unlocked.
func (r *cancelRegistry) lockedBy(key string) string {
	r.mu.Lock()
	defer r.mu.Unlock()
	return r.locks[key]
}
