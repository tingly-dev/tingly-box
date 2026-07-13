package oauth

import (
	"crypto/rand"
	"encoding/hex"
	"fmt"
	"time"

	"github.com/sirupsen/logrus"

	"github.com/tingly-dev/tingly-box/ai"
)

// =============================================
// Session Management for OAuth Status Tracking
// =============================================

// SessionStatus represents the status of an OAuth session
type SessionStatus string

const (
	SessionStatusPending SessionStatus = "pending" // Authorization initiated
	SessionStatusSuccess SessionStatus = "success" // Provider created successfully
	SessionStatusFailed  SessionStatus = "failed"  // Authorization failed
)

// SessionState holds information about an OAuth session
type SessionState struct {
	SessionID    string        `json:"session_id"`
	Status       SessionStatus `json:"status"`
	Issuer       ai.Issuer     `json:"issuer"`
	UserID       string        `json:"user_id"`
	CreatedAt    time.Time     `json:"created_at"`
	ExpiresAt    time.Time     `json:"expires_at"`
	ProviderUUID string        `json:"provider_uuid,omitempty"` // Set when success
	Error        string        `json:"error,omitempty"`         // Set when failed
	ProxyURL     string        `json:"proxy_url,omitempty"`     // Proxy URL used for this session
	// TargetProviderUUID, when set, marks this flow as a re-authentication of an
	// existing provider: on success the credentials are overwritten in place on
	// this UUID instead of creating a new provider.
	TargetProviderUUID string `json:"target_provider_uuid,omitempty"`
}

// generateSessionID generates a unique session ID
func (m *Manager) generateSessionID() (string, error) {
	b := make([]byte, 16)
	if _, err := rand.Read(b); err != nil {
		return "", fmt.Errorf("failed to generate session ID: %w", err)
	}
	return hex.EncodeToString(b), nil
}

// CreateSession creates a new OAuth session with pending status
func (m *Manager) CreateSession(userID string, issuer ai.Issuer) (*SessionState, error) {
	sessionID, err := m.generateSessionID()
	if err != nil {
		return nil, err
	}

	now := time.Now()
	session := &SessionState{
		SessionID: sessionID,
		Status:    SessionStatusPending,
		Issuer:    issuer,
		UserID:    userID,
		CreatedAt: now,
		ExpiresAt: now.Add(10 * time.Minute), // Session expires after 10 minutes
	}

	if err := m.sessionStorage.SaveSession(sessionID, session); err != nil {
		return nil, err
	}

	logrus.WithFields(logrus.Fields{
		"session_id": sessionID,
		"issuer":     issuer,
		"user_id":    userID,
		"status":     SessionStatusPending,
	}).Info("[OAuth] Session created")

	return session, nil
}

// GetSession retrieves a session by ID
func (m *Manager) GetSession(sessionID string) (*SessionState, error) {
	session, err := m.sessionStorage.GetSession(sessionID)
	if err != nil {
		return nil, err
	}

	// Check expiration
	if !session.ExpiresAt.IsZero() && time.Now().After(session.ExpiresAt) {
		return nil, ErrSessionNotFound
	}

	return session, nil
}

// StoreSession stores or updates a session
func (m *Manager) StoreSession(session *SessionState) {
	_ = m.sessionStorage.SaveSession(session.SessionID, session)
}

// UpdateSessionStatus updates the status of a session
func (m *Manager) UpdateSessionStatus(sessionID string, status SessionStatus, providerUUID string, errMsg string) error {
	// First get the session to log issuer info
	session, err := m.sessionStorage.GetSession(sessionID)
	if err != nil {
		logrus.WithField("session_id", sessionID).Warn("[OAuth] Failed to update session: not found")
		return err
	}

	// Update the status
	if err := m.sessionStorage.UpdateSessionStatus(sessionID, status, providerUUID, errMsg); err != nil {
		return err
	}

	// Log session status change
	logEntry := logrus.WithFields(logrus.Fields{
		"session_id":    sessionID,
		"issuer":        session.Issuer,
		"new_status":    status,
		"provider_uuid": providerUUID,
	})

	if status == SessionStatusSuccess {
		logEntry.Info("[OAuth] Session completed successfully")
	} else if status == SessionStatusFailed {
		logEntry.WithField("error", errMsg).Error("[OAuth] Session failed")
	} else {
		logEntry.Debug("[OAuth] Session status updated")
	}

	return nil
}
