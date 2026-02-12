package oauth

import (
	"sync"
	"time"
)

// SessionStorage defines the interface for storing OAuth session data
type SessionStorage interface {
	// SaveSession saves session data
	SaveSession(sessionID string, session *SessionState) error

	// GetSession retrieves session data
	GetSession(sessionID string) (*SessionState, error)

	// DeleteSession removes session data
	DeleteSession(sessionID string) error

	// UpdateSessionStatus updates session status and related fields
	UpdateSessionStatus(sessionID string, status SessionStatus, providerUUID, errorMsg string) error

	// CleanupExpired removes expired sessions
	CleanupExpired() error
}

// MemorySessionStorage is an in-memory implementation of SessionStorage
type MemorySessionStorage struct {
	mu        sync.RWMutex
	sessions  map[string]*SessionState
}

// NewMemorySessionStorage creates a new in-memory session storage
func NewMemorySessionStorage() *MemorySessionStorage {
	return &MemorySessionStorage{
		sessions: make(map[string]*SessionState),
	}
}

// SaveSession saves session data
func (s *MemorySessionStorage) SaveSession(sessionID string, session *SessionState) error {
	s.mu.Lock()
	defer s.mu.Unlock()

	// Set created timestamp if not already set
	if session.CreatedAt.IsZero() {
		session.CreatedAt = time.Now()
	}

	// Set default expiration if not set (1 hour)
	if session.ExpiresAt.IsZero() {
		session.ExpiresAt = session.CreatedAt.Add(time.Hour)
	}

	s.sessions[sessionID] = session
	return nil
}

// GetSession retrieves session data
func (s *MemorySessionStorage) GetSession(sessionID string) (*SessionState, error) {
	s.mu.RLock()
	defer s.mu.RUnlock()

	session, ok := s.sessions[sessionID]
	if !ok {
		return nil, ErrSessionNotFound
	}

	return session, nil
}

// DeleteSession removes session data
func (s *MemorySessionStorage) DeleteSession(sessionID string) error {
	s.mu.Lock()
	defer s.mu.Unlock()

	delete(s.sessions, sessionID)
	return nil
}

// UpdateSessionStatus updates session status and related fields
func (s *MemorySessionStorage) UpdateSessionStatus(sessionID string, status SessionStatus, providerUUID, errorMsg string) error {
	s.mu.Lock()
	defer s.mu.Unlock()

	session, ok := s.sessions[sessionID]
	if !ok {
		return ErrSessionNotFound
	}

	session.Status = status
	if providerUUID != "" {
		session.ProviderUUID = providerUUID
	}
	if errorMsg != "" {
		session.Error = errorMsg
	}

	return nil
}

// CleanupExpired removes all expired sessions from the storage
func (s *MemorySessionStorage) CleanupExpired() error {
	s.mu.Lock()
	defer s.mu.Unlock()

	now := time.Now()
	for key, session := range s.sessions {
		if !session.ExpiresAt.IsZero() && now.After(session.ExpiresAt) {
			delete(s.sessions, key)
		}
	}
	return nil
}

// Count returns the number of sessions currently stored (for testing)
func (s *MemorySessionStorage) Count() int {
	s.mu.RLock()
	defer s.mu.RUnlock()
	return len(s.sessions)
}

// GetSessions returns a copy of all sessions (for testing/debugging)
func (s *MemorySessionStorage) GetSessions() map[string]*SessionState {
	s.mu.RLock()
	defer s.mu.RUnlock()

	result := make(map[string]*SessionState, len(s.sessions))
	for k, v := range s.sessions {
		result[k] = v
	}
	return result
}
