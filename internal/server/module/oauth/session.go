package oauth

import (
	"sync"
	"time"

	"github.com/google/uuid"
)

// SessionManager manages OAuth session state
type SessionManager struct {
	sessions map[string]*Session
	mu       sync.RWMutex
}

// Session represents an OAuth session state
type Session struct {
	SessionID    string
	Provider     string
	UserID       string
	Redirect     string
	ResponseType string
	Name         string
	ProxyURL     string
	Status       string // "pending", "success", "failed", "cancelled"
	ProviderUUID string
	Error        string
	CreatedAt    time.Time
	ExpiresAt    time.Time
}

// NewSessionManager creates a new session manager
func NewSessionManager() *SessionManager {
	sm := &SessionManager{
		sessions: make(map[string]*Session),
	}
	go sm.cleanupExpiredSessions()
	return sm
}

// CreateSession creates a new OAuth session
func (sm *SessionManager) CreateSession(provider, userID, redirect, responseType, name, proxyURL string) string {
	sm.mu.Lock()
	defer sm.mu.Unlock()

	sessionID := uuid.New().String()
	sm.sessions[sessionID] = &Session{
		SessionID:    sessionID,
		Provider:     provider,
		UserID:       userID,
		Redirect:     redirect,
		ResponseType: responseType,
		Name:         name,
		ProxyURL:     proxyURL,
		Status:       "pending",
		CreatedAt:    time.Now(),
		ExpiresAt:    time.Now().Add(30 * time.Minute),
	}
	return sessionID
}

// GetSession retrieves a session
func (sm *SessionManager) GetSession(sessionID string) *Session {
	sm.mu.RLock()
	defer sm.mu.RUnlock()

	if session, ok := sm.sessions[sessionID]; ok {
		// Check if expired
		if time.Now().Before(session.ExpiresAt) {
			return session
		}
	}
	return nil
}

// CompleteSession marks session as complete
func (sm *SessionManager) CompleteSession(sessionID, providerUUID string) {
	sm.mu.Lock()
	defer sm.mu.Unlock()
	if session, ok := sm.sessions[sessionID]; ok {
		session.Status = "success"
		session.ProviderUUID = providerUUID
	}
}

// FailSession marks session as failed
func (sm *SessionManager) FailSession(sessionID, errMsg string) {
	sm.mu.Lock()
	defer sm.mu.Unlock()
	if session, ok := sm.sessions[sessionID]; ok {
		session.Status = "failed"
		session.Error = errMsg
	}
}

// GetStatus returns session status
func (sm *SessionManager) GetStatus(sessionID string) SessionStatus {
	sm.mu.RLock()
	defer sm.mu.RUnlock()

	if session, ok := sm.sessions[sessionID]; ok {
		// Check if expired
		if time.Now().After(session.ExpiresAt) {
			return SessionStatus{
				SessionID: sessionID,
				Status:    "expired",
			}
		}
		return SessionStatus{
			SessionID:    session.SessionID,
			Status:       session.Status,
			ProviderUUID: session.ProviderUUID,
			Error:        session.Error,
		}
	}
	return SessionStatus{
		SessionID: sessionID,
		Status:    "not_found",
	}
}

// CancelSession cancels a session
func (sm *SessionManager) CancelSession(sessionID string) bool {
	sm.mu.Lock()
	defer sm.mu.Unlock()
	if session, ok := sm.sessions[sessionID]; ok {
		session.Status = "cancelled"
		return true
	}
	return false
}

// cleanupExpiredSessions removes expired sessions periodically
func (sm *SessionManager) cleanupExpiredSessions() {
	ticker := time.NewTicker(5 * time.Minute)
	defer ticker.Stop()

	for range ticker.C {
		sm.mu.Lock()
		now := time.Now()
		for id, session := range sm.sessions {
			if session.ExpiresAt.Before(now) {
				delete(sm.sessions, id)
			}
		}
		sm.mu.Unlock()
	}
}

// SessionStatus represents the status of an OAuth session
type SessionStatus struct {
	SessionID    string `json:"session_id"`
	Status       string `json:"status"`
	ProviderUUID string `json:"provider_uuid,omitempty"`
	Error        string `json:"error,omitempty"`
}
