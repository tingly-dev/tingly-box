package agentboot

import (
	"context"

	"github.com/tingly-dev/tingly-box/agentboot/common"
)

// SessionManager wraps the Claude session store
type SessionManager struct {
	store common.SessionStore
}

// NewSessionManager creates a new session manager
func NewSessionManager(projectsDir string) (*SessionManager, error) {
	// Import claude session store
	// This function is a bridge to avoid circular imports
	store, err := NewClaudeStore(projectsDir)
	if err != nil {
		return nil, err
	}

	return &SessionManager{
		store: store,
	}, nil
}

// GetStore returns the underlying session store
func (sm *SessionManager) GetStore() common.SessionStore {
	return sm.store
}

// ListRecentSessions returns recent sessions for a project
func (sm *SessionManager) ListRecentSessions(ctx context.Context, projectPath string, limit int) ([]common.SessionMetadata, error) {
	return sm.store.GetRecentSessions(ctx, projectPath, limit)
}

// GetSession returns a specific session's metadata
func (sm *SessionManager) GetSession(ctx context.Context, sessionID string) (*common.SessionMetadata, error) {
	return sm.store.GetSession(ctx, sessionID)
}

// GetSessionSummary returns a summary of a session
func (sm *SessionManager) GetSessionSummary(ctx context.Context, sessionID string, firstN, lastM int) (*common.SessionSummary, error) {
	return sm.store.GetSessionSummary(ctx, sessionID, firstN, lastM)
}

// GetSessionEvents retrieves events from a session
func (sm *SessionManager) GetSessionEvents(ctx context.Context, sessionID string, offset, limit int) ([]common.SessionEvent, error) {
	return sm.store.GetSessionEvents(ctx, sessionID, offset, limit)
}
