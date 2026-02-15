package oauth

import (
	"sync"
	"time"
)

// StateStorage defines the interface for storing OAuth state data
type StateStorage interface {
	// SaveState saves state data with expiration
	SaveState(state string, data *StateData) error

	// GetState retrieves and validates state data
	// Returns ErrStateExpired if state has expired
	GetState(state string) (*StateData, error)

	// DeleteState removes state data
	DeleteState(state string) error

	// CleanupExpired removes expired states
	CleanupExpired() error
}

// MemoryStateStorage is an in-memory implementation of StateStorage
type MemoryStateStorage struct {
	mu     sync.RWMutex
	states map[string]*StateData
}

// NewMemoryStateStorage creates a new in-memory state storage
func NewMemoryStateStorage() *MemoryStateStorage {
	return &MemoryStateStorage{
		states: make(map[string]*StateData),
	}
}

// SaveState saves state data with expiration
func (s *MemoryStateStorage) SaveState(state string, data *StateData) error {
	s.mu.Lock()
	defer s.mu.Unlock()

	// Set expiration timestamp if not already set
	if data.ExpiresAt.IsZero() {
		data.ExpiresAt = time.Now().Add(10 * time.Minute)
	}
	data.Timestamp = time.Now().Unix()
	data.ExpiresAtUnix = data.ExpiresAt.Unix()

	s.states[state] = data
	return nil
}

// GetState retrieves and validates state data
func (s *MemoryStateStorage) GetState(state string) (*StateData, error) {
	s.mu.RLock()
	defer s.mu.RUnlock()

	data, ok := s.states[state]
	if !ok {
		return nil, ErrInvalidState
	}

	// Check expiration
	if time.Now().After(data.ExpiresAt) {
		return nil, ErrStateExpired
	}

	return data, nil
}

// DeleteState removes state data
func (s *MemoryStateStorage) DeleteState(state string) error {
	s.mu.Lock()
	defer s.mu.Unlock()

	delete(s.states, state)
	return nil
}

// CleanupExpired removes all expired states from the storage
func (s *MemoryStateStorage) CleanupExpired() error {
	s.mu.Lock()
	defer s.mu.Unlock()

	now := time.Now()
	for key, data := range s.states {
		if now.After(data.ExpiresAt) {
			delete(s.states, key)
		}
	}
	return nil
}

// stateKey generates a key for storing state data (for compatibility with existing code)
func (s *MemoryStateStorage) stateKey(state string) string {
	return state
}

// Count returns the number of states currently stored (for testing)
func (s *MemoryStateStorage) Count() int {
	s.mu.RLock()
	defer s.mu.RUnlock()
	return len(s.states)
}

// GetStates returns a copy of all states (for testing/debugging)
func (s *MemoryStateStorage) GetStates() map[string]*StateData {
	s.mu.RLock()
	defer s.mu.RUnlock()

	result := make(map[string]*StateData, len(s.states))
	for k, v := range s.states {
		result[k] = v
	}
	return result
}
