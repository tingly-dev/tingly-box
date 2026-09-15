package managedagent

import "context"

// The store interfaces are split per entity so a test can fake one without
// the others, and so the SQLite implementation in internal/db can be wired
// through StoreManager like every other store. A missing row is ErrNotFound.

// FolderStore persists the folders the agent may work in.
type FolderStore interface {
	CreateFolder(ctx context.Context, f *Folder) error
	GetFolder(ctx context.Context, id string) (*Folder, error)
	// GetFolderByPath finds a folder by its cleaned absolute path;
	// ErrNotFound when the path was never handed over.
	GetFolderByPath(ctx context.Context, path string) (*Folder, error)
	ListFolders(ctx context.Context) ([]Folder, error)
	UpdateFolder(ctx context.Context, f *Folder) error
	DeleteFolder(ctx context.Context, id string) error
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
	FolderID string
	Status   SessionStatus
	Active   bool // only statuses for which IsActive() is true
	Limit    int
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
	Folders  FolderStore
	Sessions SessionStore
	Events   EventStore
}
