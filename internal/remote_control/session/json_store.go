package session

import (
	"fmt"
	"time"

	"github.com/sirupsen/logrus"
	"github.com/tingly-dev/tingly-box/pkg/jsonstore"
)

// SessionStoreJSON handles session persistence using JSON file storage
// This provides a simpler, more portable alternative to SQLite MessageStore
type SessionStoreJSON struct {
	store *jsonstore.Store[Session]
}

// NewSessionStoreJSON creates a new JSON-based session store
func NewSessionStoreJSON(filePath string) (*SessionStoreJSON, error) {
	if filePath == "" {
		return nil, fmt.Errorf("file path is required")
	}

	store, err := jsonstore.New[Session](filePath)
	if err != nil {
		return nil, fmt.Errorf("failed to create JSON store: %w", err)
	}

	return &SessionStoreJSON{store: store}, nil
}

// Close ensures data is persisted before closing
func (s *SessionStoreJSON) Close() error {
	if s == nil || s.store == nil {
		return nil
	}
	return s.store.Close()
}

// Get retrieves a session by ID
func (s *SessionStoreJSON) Get(sessionID string) (*Session, error) {
	if s == nil || s.store == nil {
		return nil, nil
	}
	return s.store.Get(sessionID), nil
}

// Set stores a session
func (s *SessionStoreJSON) Set(sessionID string, sess *Session) error {
	if s == nil || s.store == nil || sess == nil {
		return nil
	}

	// Update timestamp
	sess.LastActivity = time.Now().UTC()

	return s.store.Set(sessionID, sess)
}

// Delete removes a session
func (s *SessionStoreJSON) Delete(sessionID string) error {
	if s == nil || s.store == nil {
		return nil
	}
	s.store.Delete(sessionID)
	return nil
}

// List returns all sessions
func (s *SessionStoreJSON) List() []*Session {
	if s == nil || s.store == nil {
		return []*Session{}
	}

	items := s.store.List()
	result := make([]*Session, 0, len(items))
	for _, sess := range items {
		result = append(result, sess)
	}
	return result
}

// FindByChatAgentProject finds a session by (chatID, agent, project) tuple
// Returns the most recent session matching the criteria
func (s *SessionStoreJSON) FindByChatAgentProject(chatID, agent, project string) (*Session, error) {
	if s == nil || s.store == nil {
		return nil, nil
	}

	sessions := s.store.List()
	var mostRecent *Session
	var mostRecentTime time.Time

	for _, sess := range sessions {
		if sess.ChatID == chatID && sess.Agent == agent && sess.Project == project {
			// Skip closed or expired sessions
			if sess.Status == StatusClosed || sess.Status == StatusExpired {
				continue
			}
			// Track the most recently active session
			if sess.LastActivity.After(mostRecentTime) {
				mostRecent = sess
				mostRecentTime = sess.LastActivity
			}
		}
	}

	return mostRecent, nil
}

// ListByChat lists all sessions for a given chat ID
func (s *SessionStoreJSON) ListByChat(chatID string) ([]*Session, error) {
	if s == nil || s.store == nil {
		return []*Session{}, nil
	}

	sessions := s.store.List()
	var result []*Session

	for _, sess := range sessions {
		if sess.ChatID == chatID {
			result = append(result, sess)
		}
	}

	return result, nil
}

// ForceSave ensures data is written to disk immediately
func (s *SessionStoreJSON) ForceSave() error {
	if s == nil || s.store == nil {
		return nil
	}
	s.store.ForceSave()
	return nil
}

// CleanupOldSessions removes sessions older than the specified duration
// Only removes closed/completed/failed sessions, not running ones
func (s *SessionStoreJSON) CleanupOldSessions(olderThan time.Duration) int {
	if s == nil || s.store == nil {
		return 0
	}

	cutoff := time.Now().UTC().Add(-olderThan)
	sessions := s.store.List()
	removed := 0

	for _, sess := range sessions {
		// Skip running sessions
		if sess.Status == StatusRunning || sess.Status == StatusPending {
			continue
		}
		// Skip persistent sessions (zero ExpiresAt)
		if sess.ExpiresAt.IsZero() {
			continue
		}
		// Remove old inactive sessions
		if sess.LastActivity.Before(cutoff) {
			s.store.Delete(sess.ID)
			removed++
		}
	}

	if removed > 0 {
		logrus.Infof("Cleaned up %d old sessions from JSON store", removed)
		_ = s.store.ForceSave()
	}

	return removed
}
