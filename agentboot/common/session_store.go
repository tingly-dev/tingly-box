package common

import "context"

// SessionStore defines the session storage interface
type SessionStore interface {
	// ListProjects returns all project paths that have at least one session
	ListProjects(ctx context.Context) ([]string, error)

	// ListSessions returns all sessions for a project, ordered by start time (newest first)
	ListSessions(ctx context.Context, projectPath string) ([]SessionMetadata, error)

	// GetSession retrieves a specific session's metadata
	GetSession(ctx context.Context, sessionID string) (*SessionMetadata, error)

	// GetRecentSessions returns the N most recent sessions
	GetRecentSessions(ctx context.Context, projectPath string, limit int) ([]SessionMetadata, error)

	// GetSessionEvents retrieves events from a session
	// offset: number of events to skip
	// limit: maximum number of events to return (0 = all)
	GetSessionEvents(ctx context.Context, sessionID string, offset, limit int) ([]SessionEvent, error)

	// GetSessionSummary returns a summary (first N and last M events)
	GetSessionSummary(ctx context.Context, sessionID string, firstN, lastM int) (*SessionSummary, error)
}

// SessionSummary contains head and tail events
type SessionSummary struct {
	Metadata SessionMetadata `json:"metadata"`
	Head     []SessionEvent  `json:"head"` // First N events
	Tail     []SessionEvent  `json:"tail"` // Last M events
}
