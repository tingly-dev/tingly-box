package server

import (
	"sync"
	"time"
)

// ModelRequestState represents the current request being processed
type ModelRequestState struct {
	ProviderName string    `json:"provider_name"`
	ProviderUUID string    `json:"provider_uuid"`
	Model        string    `json:"model"`
	RequestModel string    `json:"request_model"`
	Scenario     string    `json:"scenario"`
	StartTime    time.Time `json:"start_time"`
	Streamed     bool      `json:"streamed"`
}

// ModelRequestTracker tracks the currently active request in real-time
// It provides status information for monitoring and status line display
type ModelRequestTracker struct {
	mu      sync.RWMutex
	current *ModelRequestState
}

// NewModelRequestTracker creates a new tracker
func NewModelRequestTracker() *ModelRequestTracker {
	return &ModelRequestTracker{}
}

// SetCurrent updates the current request state
func (t *ModelRequestTracker) SetCurrent(state ModelRequestState) {
	t.mu.Lock()
	defer t.mu.Unlock()

	state.StartTime = time.Now()
	t.current = &state
}

// GetCurrent returns the current request state
func (t *ModelRequestTracker) GetCurrent() *ModelRequestState {
	t.mu.RLock()
	defer t.mu.RUnlock()

	if t.current == nil {
		return nil
	}

	// Return a copy
	state := *t.current
	return &state
}

// Clear clears the current request state (call when request completes)
func (t *ModelRequestTracker) Clear() {
	t.mu.Lock()
	defer t.mu.Unlock()
	t.current = nil
}

// globalModelRequestTracker is the global instance
var globalModelRequestTracker = NewModelRequestTracker()

// GetGlobalModelRequestTracker returns the global tracker instance
func GetGlobalModelRequestTracker() *ModelRequestTracker {
	return globalModelRequestTracker
}
