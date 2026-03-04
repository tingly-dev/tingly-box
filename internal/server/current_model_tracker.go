package server

import (
	"sync"
	"time"
)

// CurrentRequestState represents the current request being processed
type CurrentRequestState struct {
	ProviderName string    `json:"provider_name"`
	ProviderUUID string    `json:"provider_uuid"`
	Model        string    `json:"model"`
	RequestModel string    `json:"request_model"`
	Scenario     string    `json:"scenario"`
	StartTime    time.Time `json:"start_time"`
	Streamed     bool      `json:"streamed"`
}

// CurrentRequestTracker tracks the currently active request in real-time
// It provides status information for monitoring and status line display
type CurrentRequestTracker struct {
	mu      sync.RWMutex
	current *CurrentRequestState
}

// NewCurrentRequestTracker creates a new tracker
func NewCurrentRequestTracker() *CurrentRequestTracker {
	return &CurrentRequestTracker{}
}

// SetCurrent updates the current request state
func (t *CurrentRequestTracker) SetCurrent(state CurrentRequestState) {
	t.mu.Lock()
	defer t.mu.Unlock()

	state.StartTime = time.Now()
	t.current = &state
}

// GetCurrent returns the current request state
func (t *CurrentRequestTracker) GetCurrent() *CurrentRequestState {
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
func (t *CurrentRequestTracker) Clear() {
	t.mu.Lock()
	defer t.mu.Unlock()
	t.current = nil
}

// globalCurrentRequestTracker is the global instance
var globalCurrentRequestTracker = NewCurrentRequestTracker()

// GetGlobalCurrentRequestTracker returns the global tracker instance
func GetGlobalCurrentRequestTracker() *CurrentRequestTracker {
	return globalCurrentRequestTracker
}
