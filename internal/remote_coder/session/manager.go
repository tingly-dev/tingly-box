package session

import (
	"sync"
	"time"

	"github.com/google/uuid"
	"github.com/sirupsen/logrus"
)

// Status represents the current state of a session
type Status string

const (
	StatusPending   Status = "pending"
	StatusRunning   Status = "running"
	StatusCompleted Status = "completed"
	StatusFailed    Status = "failed"
	StatusExpired   Status = "expired"
	StatusClosed    Status = "closed"
)

// Config holds session manager configuration
type Config struct {
	Timeout          time.Duration // Session timeout duration
	MessageRetention time.Duration // Message retention window
}

// Session represents an execution session
type Session struct {
	ID           string                 // Unique session identifier
	Status       Status                 // Current session status
	Request      string                 // User's request payload
	Response     string                 // Claude Code response summary
	Error        string                 // Error message if failed
	CreatedAt    time.Time              // Session creation timestamp
	LastActivity time.Time              // Last activity timestamp
	ExpiresAt    time.Time              // Session expiration timestamp
	Context      map[string]interface{} // Request context for continued communication
	Messages     []Message              // Chat message history
}

// Message represents a chat message within a session
type Message struct {
	Role      string    // "user" or "assistant"
	Content   string    // Full content
	Summary   string    // Optional summary for assistant responses
	Timestamp time.Time // When the message was created
}

// Manager handles session lifecycle
type Manager struct {
	mu        sync.RWMutex
	sessions  map[string]*Session
	config    Config
	stopCh    chan struct{}
	wg        sync.WaitGroup
	startTime time.Time
	store     *MessageStore
}

// NewManager creates a new session manager
func NewManager(cfg Config, store *MessageStore) *Manager {
	mgr := &Manager{
		sessions:  make(map[string]*Session),
		config:    cfg,
		stopCh:    make(chan struct{}),
		startTime: time.Now(),
		store:     store,
	}

	if store != nil {
		if sessions, err := store.LoadSessions(); err == nil {
			for _, s := range sessions {
				mgr.sessions[s.ID] = s
			}
			logrus.Debugf("Loaded %d remote-coder sessions from DB", len(sessions))
		} else {
			logrus.Warnf("Failed to load remote-cc sessions from DB: %v", err)
		}
	}

	// Start cleanup goroutine
	mgr.wg.Add(1)
	go mgr.cleanupLoop()
	mgr.wg.Add(1)
	go mgr.retentionLoop()

	return mgr
}

// Create creates a new session and returns it
func (m *Manager) Create() *Session {
	m.mu.Lock()
	defer m.mu.Unlock()

	now := time.Now()
	session := &Session{
		ID:           uuid.New().String(),
		Status:       StatusPending,
		CreatedAt:    now,
		LastActivity: now,
		ExpiresAt:    now.Add(m.config.Timeout),
		Context:      make(map[string]interface{}),
	}

	m.sessions[session.ID] = session
	logrus.Debugf("Session created: %s (expires at %s)", session.ID, session.ExpiresAt.Format(time.RFC3339))
	if m.store != nil {
		_ = m.store.UpsertSession(session)
	}

	return session
}

// Get retrieves a session by ID
func (m *Manager) Get(id string) (*Session, bool) {
	m.mu.RLock()
	defer m.mu.RUnlock()

	session, exists := m.sessions[id]
	return session, exists
}

// GetOrLoad retrieves a session by ID, falling back to the store if needed
func (m *Manager) GetOrLoad(id string) (*Session, bool) {
	m.mu.RLock()
	session, exists := m.sessions[id]
	m.mu.RUnlock()
	if exists {
		return session, true
	}
	if m.store == nil {
		return nil, false
	}
	sess, err := m.store.GetSession(id)
	if err != nil || sess == nil {
		return nil, false
	}
	m.mu.Lock()
	m.sessions[id] = sess
	m.mu.Unlock()
	return sess, true
}

// Update updates a session
func (m *Manager) Update(id string, fn func(*Session)) bool {
	m.mu.Lock()
	defer m.mu.Unlock()

	session, exists := m.sessions[id]
	if !exists {
		return false
	}

	fn(session)
	session.LastActivity = time.Now()
	if m.store != nil {
		_ = m.store.UpsertSession(session)
	}

	return true
}

// Delete removes a session
func (m *Manager) Delete(id string) bool {
	m.mu.Lock()
	defer m.mu.Unlock()

	_, exists := m.sessions[id]
	if !exists {
		return false
	}

	delete(m.sessions, id)
	logrus.Debugf("Session deleted: %s", id)
	if m.store != nil {
		_ = m.store.DeleteMessagesForSession(id)
		_ = m.store.DeleteSession(id)
	}

	return true
}

// Close terminates a session gracefully
func (m *Manager) Close(id string) bool {
	m.mu.Lock()
	defer m.mu.Unlock()

	session, exists := m.sessions[id]
	if !exists {
		return false
	}

	session.Status = StatusClosed
	session.LastActivity = time.Now()

	delete(m.sessions, id)
	logrus.Debugf("Session closed: %s", id)
	if m.store != nil {
		_ = m.store.DeleteMessagesForSession(id)
		_ = m.store.DeleteSession(id)
	}

	return true
}

// SetRunning marks a session as running
func (m *Manager) SetRunning(id string) bool {
	return m.Update(id, func(s *Session) {
		s.Status = StatusRunning
	})
}

// SetCompleted marks a session as completed with response
func (m *Manager) SetCompleted(id string, response string) bool {
	return m.Update(id, func(s *Session) {
		s.Status = StatusCompleted
		s.Response = response
	})
}

// SetFailed marks a session as failed with error
func (m *Manager) SetFailed(id string, err string) bool {
	return m.Update(id, func(s *Session) {
		s.Status = StatusFailed
		s.Error = err
	})
}

// SetRequest stores the request for a session
func (m *Manager) SetRequest(id string, request string) bool {
	return m.Update(id, func(s *Session) {
		s.Request = request
	})
}

// GetRequest retrieves the request for a session
func (m *Manager) GetRequest(id string) (string, bool) {
	m.mu.RLock()
	defer m.mu.RUnlock()

	session, exists := m.sessions[id]
	if !exists {
		return "", false
	}

	return session.Request, true
}

// SetContext stores context data for a session
func (m *Manager) SetContext(id string, key string, value interface{}) bool {
	return m.Update(id, func(s *Session) {
		s.Context[key] = value
	})
}

// AppendMessage adds a message to a session
func (m *Manager) AppendMessage(id string, msg Message) bool {
	ok := m.Update(id, func(s *Session) {
		if m.store == nil {
			s.Messages = append(s.Messages, msg)
		}
	})
	if ok && m.store != nil {
		_ = m.store.InsertMessage(id, msg)
	}
	return ok
}

// GetMessages retrieves messages for a session
func (m *Manager) GetMessages(id string) ([]Message, bool) {
	m.mu.RLock()
	defer m.mu.RUnlock()

	session, exists := m.sessions[id]
	if !exists {
		return nil, false
	}

	if m.store != nil {
		if msgs, err := m.store.GetMessages(id); err == nil {
			return msgs, true
		}
	}
	return append([]Message{}, session.Messages...), true
}

// GetContext retrieves context data for a session
func (m *Manager) GetContext(id string, key string) (interface{}, bool) {
	m.mu.RLock()
	defer m.mu.RUnlock()

	session, exists := m.sessions[id]
	if !exists {
		return nil, false
	}

	value, exists := session.Context[key]
	return value, exists
}

// cleanupLoop periodically removes expired sessions
func (m *Manager) cleanupLoop() {
	defer m.wg.Done()

	ticker := time.NewTicker(5 * time.Minute)
	defer ticker.Stop()

	for {
		select {
		case <-m.stopCh:
			return
		case <-ticker.C:
			m.cleanupExpired()
		}
	}
}

// cleanupExpired removes all expired sessions
func (m *Manager) cleanupExpired() {
	now := time.Now()

	m.mu.Lock()
	defer m.mu.Unlock()

	for id, session := range m.sessions {
		if now.After(session.ExpiresAt) {
			session.Status = StatusExpired
			delete(m.sessions, id)
			logrus.Debugf("Session expired and cleaned up: %s", id)
			if m.store != nil {
				_ = m.store.DeleteMessagesForSession(id)
				_ = m.store.DeleteSession(id)
			}
		}
	}
}

// Stop stops the cleanup goroutine
func (m *Manager) Stop() {
	close(m.stopCh)
	m.wg.Wait()
	if m.store != nil {
		_ = m.store.Close()
	}
}

// Stats returns session statistics by status
func (m *Manager) Stats() map[string]int {
	m.mu.RLock()
	defer m.mu.RUnlock()

	stats := make(map[string]int)
	for _, session := range m.sessions {
		stats[string(session.Status)]++
	}
	return stats
}

// GetStats returns comprehensive session statistics
func (m *Manager) GetStats() map[string]interface{} {
	m.mu.RLock()
	defer m.mu.RUnlock()

	now := time.Now()
	stats := make(map[string]interface{})

	// Count by status
	statusCounts := make(map[string]int)
	total := 0
	recentActions := make(map[string]int)

	for _, session := range m.sessions {
		statusCounts[string(session.Status)]++
		total++

		// Count recent actions (last 24 hours)
		if now.Sub(session.CreatedAt) < 24*time.Hour {
			recentActions[string(session.Status)]++
		}
	}

	stats["total"] = total
	stats["active"] = statusCounts[string(StatusRunning)]
	stats["completed"] = statusCounts[string(StatusCompleted)]
	stats["failed"] = statusCounts[string(StatusFailed)]
	stats["closed"] = statusCounts[string(StatusClosed)]
	stats["pending"] = statusCounts[string(StatusPending)]
	stats["expired"] = statusCounts[string(StatusExpired)]
	stats["recent_actions"] = recentActions
	stats["uptime"] = now.Sub(m.startTime).String()

	return stats
}

// List returns all sessions
func (m *Manager) List() []*Session {
	m.mu.RLock()
	defer m.mu.RUnlock()

	sessions := make([]*Session, 0, len(m.sessions))
	for _, s := range m.sessions {
		sessions = append(sessions, s)
	}
	return sessions
}

// Clear removes all sessions
func (m *Manager) Clear() int {
	m.mu.Lock()
	defer m.mu.Unlock()

	count := len(m.sessions)
	m.sessions = make(map[string]*Session)
	logrus.Debugf("Cleared %d sessions", count)
	if m.store != nil {
		_ = m.store.ClearAllMessages()
		_ = m.store.ClearAllSessions()
	}
	return count
}

func (m *Manager) retentionLoop() {
	defer m.wg.Done()

	if m.config.MessageRetention <= 0 {
		return
	}

	ticker := time.NewTicker(5 * time.Minute)
	defer ticker.Stop()

	for {
		select {
		case <-m.stopCh:
			return
		case <-ticker.C:
			cutoff := time.Now().Add(-m.config.MessageRetention)
			if m.store != nil {
				_ = m.store.PurgeOlderThan(cutoff)
				_ = m.store.PurgeSessionsOlderThan(cutoff)
			}
			m.mu.Lock()
			for id, session := range m.sessions {
				if session.Status == StatusRunning {
					continue
				}
				if session.LastActivity.Before(cutoff) {
					delete(m.sessions, id)
				}
			}
			m.mu.Unlock()
		}
	}
}
