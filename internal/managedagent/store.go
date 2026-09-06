package managedagent

import "context"

// The store interfaces are split per entity so a test can fake one without
// the others, and so the SQLite implementation in internal/db can be wired
// through StoreManager like every other store. A missing row is ErrNotFound.

// SourceStore persists Sources.
type SourceStore interface {
	CreateSource(ctx context.Context, s *Source) error
	GetSource(ctx context.Context, id string) (*Source, error)
	ListSources(ctx context.Context) ([]Source, error)
	UpdateSource(ctx context.Context, s *Source) error
	DeleteSource(ctx context.Context, id string) error
}

// EnvironmentStore persists Environments.
type EnvironmentStore interface {
	CreateEnvironment(ctx context.Context, e *Environment) error
	GetEnvironment(ctx context.Context, id string) (*Environment, error)
	ListEnvironments(ctx context.Context) ([]Environment, error)
	UpdateEnvironment(ctx context.Context, e *Environment) error
	DeleteEnvironment(ctx context.Context, id string) error
}

// WorkspaceStore persists Workspaces.
type WorkspaceStore interface {
	CreateWorkspace(ctx context.Context, w *Workspace) error
	GetWorkspace(ctx context.Context, id string) (*Workspace, error)
	ListWorkspaces(ctx context.Context, f WorkspaceFilter) ([]Workspace, error)
	UpdateWorkspace(ctx context.Context, w *Workspace) error
}

// WorkspaceFilter narrows ListWorkspaces. Zero values match everything.
type WorkspaceFilter struct {
	SourceID      string
	EnvironmentID string
	State         WorkspaceState
}

// SessionStore persists the session INDEX. The conversation itself is in
// the EventStore.
type SessionStore interface {
	CreateSession(ctx context.Context, s *Session) error
	GetSession(ctx context.Context, id string) (*Session, error)
	ListSessions(ctx context.Context, f SessionFilter) ([]Session, error)
	UpdateSession(ctx context.Context, s *Session) error
}

// SessionFilter narrows ListSessions. Zero values match everything; the
// result is ordered by last_active_at DESC and capped at Limit (0 = default).
type SessionFilter struct {
	WorkspaceID string
	Status      SessionStatus
	Active      bool // only statuses for which IsActive() is true
	Limit       int
}

// EventStore is the append-only per-session log. Append assigns Seq.
type EventStore interface {
	AppendEvent(ctx context.Context, e *Event) error
	// ListEvents returns events with Seq > after, oldest first, at most limit
	// (0 = no cap).
	ListEvents(ctx context.Context, sessionID string, after int64, limit int) ([]Event, error)
}

// Stores bundles everything the Service needs.
type Stores struct {
	Sources      SourceStore
	Environments EnvironmentStore
	Workspaces   WorkspaceStore
	Sessions     SessionStore
	Events       EventStore
}
