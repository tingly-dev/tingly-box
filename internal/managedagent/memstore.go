package managedagent

import (
	"context"
	"sort"
	"sync"
)

// MemStores is an in-memory Stores implementation for tests and for callers
// that have no database (harness, examples). It applies the same ordering
// rules as the SQLite stores so handler tests see production-shaped output.
type MemStores struct {
	mu           sync.Mutex
	sources      map[string]Source
	environments map[string]Environment
	workspaces   map[string]Workspace
	sessions     map[string]Session
	events       map[string][]Event
	seq          map[string]int64
}

// NewMemStores returns an empty MemStores bundled as Stores.
func NewMemStores() (*MemStores, Stores) {
	m := &MemStores{
		sources:      map[string]Source{},
		environments: map[string]Environment{},
		workspaces:   map[string]Workspace{},
		sessions:     map[string]Session{},
		events:       map[string][]Event{},
		seq:          map[string]int64{},
	}
	return m, Stores{Sources: m, Environments: m, Workspaces: m, Sessions: m, Events: m}
}

func (m *MemStores) CreateSource(_ context.Context, s *Source) error {
	m.mu.Lock()
	defer m.mu.Unlock()
	m.sources[s.ID] = *s
	return nil
}

func (m *MemStores) GetSource(_ context.Context, id string) (*Source, error) {
	m.mu.Lock()
	defer m.mu.Unlock()
	s, ok := m.sources[id]
	if !ok {
		return nil, notFound("source", id)
	}
	return &s, nil
}

func (m *MemStores) ListSources(_ context.Context) ([]Source, error) {
	m.mu.Lock()
	defer m.mu.Unlock()
	out := make([]Source, 0, len(m.sources))
	for _, s := range m.sources {
		out = append(out, s)
	}
	sort.Slice(out, func(i, j int) bool { return out[i].CreatedAt.Before(out[j].CreatedAt) })
	return out, nil
}

func (m *MemStores) UpdateSource(_ context.Context, s *Source) error {
	m.mu.Lock()
	defer m.mu.Unlock()
	if _, ok := m.sources[s.ID]; !ok {
		return notFound("source", s.ID)
	}
	m.sources[s.ID] = *s
	return nil
}

func (m *MemStores) DeleteSource(_ context.Context, id string) error {
	m.mu.Lock()
	defer m.mu.Unlock()
	if _, ok := m.sources[id]; !ok {
		return notFound("source", id)
	}
	delete(m.sources, id)
	return nil
}

func (m *MemStores) CreateEnvironment(_ context.Context, e *Environment) error {
	m.mu.Lock()
	defer m.mu.Unlock()
	m.environments[e.ID] = *e
	return nil
}

func (m *MemStores) GetEnvironment(_ context.Context, id string) (*Environment, error) {
	m.mu.Lock()
	defer m.mu.Unlock()
	e, ok := m.environments[id]
	if !ok {
		return nil, notFound("environment", id)
	}
	return &e, nil
}

func (m *MemStores) ListEnvironments(_ context.Context) ([]Environment, error) {
	m.mu.Lock()
	defer m.mu.Unlock()
	out := make([]Environment, 0, len(m.environments))
	for _, e := range m.environments {
		out = append(out, e)
	}
	sort.Slice(out, func(i, j int) bool { return out[i].CreatedAt.Before(out[j].CreatedAt) })
	return out, nil
}

func (m *MemStores) UpdateEnvironment(_ context.Context, e *Environment) error {
	m.mu.Lock()
	defer m.mu.Unlock()
	if _, ok := m.environments[e.ID]; !ok {
		return notFound("environment", e.ID)
	}
	m.environments[e.ID] = *e
	return nil
}

func (m *MemStores) DeleteEnvironment(_ context.Context, id string) error {
	m.mu.Lock()
	defer m.mu.Unlock()
	if _, ok := m.environments[id]; !ok {
		return notFound("environment", id)
	}
	delete(m.environments, id)
	return nil
}

func (m *MemStores) CreateWorkspace(_ context.Context, w *Workspace) error {
	m.mu.Lock()
	defer m.mu.Unlock()
	m.workspaces[w.ID] = *w
	return nil
}

func (m *MemStores) GetWorkspace(_ context.Context, id string) (*Workspace, error) {
	m.mu.Lock()
	defer m.mu.Unlock()
	w, ok := m.workspaces[id]
	if !ok {
		return nil, notFound("workspace", id)
	}
	return &w, nil
}

func (m *MemStores) ListWorkspaces(_ context.Context, f WorkspaceFilter) ([]Workspace, error) {
	m.mu.Lock()
	defer m.mu.Unlock()
	var out []Workspace
	for _, w := range m.workspaces {
		if f.SourceID != "" && w.SourceID != f.SourceID {
			continue
		}
		if f.EnvironmentID != "" && w.EnvironmentID != f.EnvironmentID {
			continue
		}
		if f.State != "" && w.State != f.State {
			continue
		}
		out = append(out, w)
	}
	sort.Slice(out, func(i, j int) bool { return out[i].LastActiveAt.After(out[j].LastActiveAt) })
	return out, nil
}

func (m *MemStores) UpdateWorkspace(_ context.Context, w *Workspace) error {
	m.mu.Lock()
	defer m.mu.Unlock()
	if _, ok := m.workspaces[w.ID]; !ok {
		return notFound("workspace", w.ID)
	}
	m.workspaces[w.ID] = *w
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
		if f.WorkspaceID != "" && s.WorkspaceID != f.WorkspaceID {
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
