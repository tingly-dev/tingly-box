package managedagent

import (
	"context"
	"path/filepath"
	"sort"
	"sync"
)

// MemStores is an in-memory Stores implementation for tests and for callers
// that have no database (harness, examples). It applies the same ordering
// rules as the SQLite stores so handler tests see production-shaped output.
type MemStores struct {
	mu       sync.Mutex
	folders  map[string]Folder
	sessions map[string]Session
	events   map[string][]Event
	seq      map[string]int64
}

// NewMemStores returns an empty MemStores bundled as Stores.
func NewMemStores() (*MemStores, Stores) {
	m := &MemStores{
		folders:  map[string]Folder{},
		sessions: map[string]Session{},
		events:   map[string][]Event{},
		seq:      map[string]int64{},
	}
	return m, Stores{Folders: m, Sessions: m, Events: m}
}

func (m *MemStores) CreateFolder(_ context.Context, f *Folder) error {
	m.mu.Lock()
	defer m.mu.Unlock()
	m.folders[f.ID] = *f
	return nil
}

func (m *MemStores) GetFolder(_ context.Context, id string) (*Folder, error) {
	m.mu.Lock()
	defer m.mu.Unlock()
	f, ok := m.folders[id]
	if !ok {
		return nil, notFound("folder", id)
	}
	return &f, nil
}

func (m *MemStores) GetFolderByPath(_ context.Context, path string) (*Folder, error) {
	m.mu.Lock()
	defer m.mu.Unlock()
	clean := filepath.Clean(path)
	for _, f := range m.folders {
		if f.Path == clean {
			return &f, nil
		}
	}
	return nil, notFound("folder", path)
}

// ListFolders orders by most recently used, which is the order the picker
// and the composer offer them in.
func (m *MemStores) ListFolders(_ context.Context) ([]Folder, error) {
	m.mu.Lock()
	defer m.mu.Unlock()
	out := make([]Folder, 0, len(m.folders))
	for _, f := range m.folders {
		out = append(out, f)
	}
	sort.Slice(out, func(i, j int) bool { return out[i].LastUsedAt.After(out[j].LastUsedAt) })
	return out, nil
}

func (m *MemStores) UpdateFolder(_ context.Context, f *Folder) error {
	m.mu.Lock()
	defer m.mu.Unlock()
	if _, ok := m.folders[f.ID]; !ok {
		return notFound("folder", f.ID)
	}
	m.folders[f.ID] = *f
	return nil
}

func (m *MemStores) DeleteFolder(_ context.Context, id string) error {
	m.mu.Lock()
	defer m.mu.Unlock()
	if _, ok := m.folders[id]; !ok {
		return notFound("folder", id)
	}
	delete(m.folders, id)
	return nil
}

func (m *MemStores) CreateSession(_ context.Context, s *Session) error {
	m.mu.Lock()
	defer m.mu.Unlock()
	m.sessions[s.ID] = *s
	return nil
}

func (m *MemStores) GetSession(_ context.Context, id string) (*Session, error) {
	m.mu.Lock()
	defer m.mu.Unlock()
	s, ok := m.sessions[id]
	if !ok {
		return nil, notFound("session", id)
	}
	return &s, nil
}

func (m *MemStores) ListSessions(_ context.Context, f SessionFilter) ([]Session, error) {
	m.mu.Lock()
	defer m.mu.Unlock()
	var out []Session
	for _, s := range m.sessions {
		if f.FolderID != "" && s.FolderID != f.FolderID {
			continue
		}
		if f.Status != "" && s.Status != f.Status {
			continue
		}
		if f.Active && !s.Status.IsActive() {
			continue
		}
		out = append(out, s)
	}
	sort.Slice(out, func(i, j int) bool { return out[i].LastActiveAt.After(out[j].LastActiveAt) })
	if f.Limit > 0 && len(out) > f.Limit {
		out = out[:f.Limit]
	}
	return out, nil
}

func (m *MemStores) UpdateSession(_ context.Context, s *Session) error {
	m.mu.Lock()
	defer m.mu.Unlock()
	if _, ok := m.sessions[s.ID]; !ok {
		return notFound("session", s.ID)
	}
	m.sessions[s.ID] = *s
	return nil
}

func (m *MemStores) AppendEvent(_ context.Context, e *Event) error {
	m.mu.Lock()
	defer m.mu.Unlock()
	m.seq[e.SessionID]++
	e.Seq = m.seq[e.SessionID]
	m.events[e.SessionID] = append(m.events[e.SessionID], *e)
	return nil
}

func (m *MemStores) ListEvents(_ context.Context, sessionID string, after int64, limit int) ([]Event, error) {
	m.mu.Lock()
	defer m.mu.Unlock()
	var out []Event
	for _, e := range m.events[sessionID] {
		if e.Seq <= after {
			continue
		}
		out = append(out, e)
		if limit > 0 && len(out) >= limit {
			break
		}
	}
	return out, nil
}
