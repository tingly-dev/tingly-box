package runtime

import (
	"sync"
	"time"
)

// SessionContext holds session-persistent heavy data.
type SessionContext struct {
	SessionID      string
	WorkspaceTree  map[string]any
	BuildLogs      []string
	LastWorkerResp string
	CreatedAt      time.Time
}

// SessionStore is an in-memory KV with TTL sweeper.
type SessionStore struct {
	mu       sync.RWMutex
	sessions map[string]*SessionContext
	ttl      time.Duration
}

func NewSessionStore(ttl time.Duration) *SessionStore {
	if ttl <= 0 {
		ttl = 10 * time.Minute
	}
	return &SessionStore{
		sessions: make(map[string]*SessionContext),
		ttl:      ttl,
	}
}

func (s *SessionStore) Put(ctx *SessionContext) {
	if s == nil || ctx == nil {
		return
	}
	ctx.CreatedAt = time.Now()
	s.mu.Lock()
	defer s.mu.Unlock()
	s.sessions[ctx.SessionID] = ctx
}

func (s *SessionStore) Get(sessionID string) (*SessionContext, bool) {
	if s == nil {
		return nil, false
	}
	s.mu.RLock()
	defer s.mu.RUnlock()
	ctx, ok := s.sessions[sessionID]
	if !ok || time.Since(ctx.CreatedAt) > s.ttl {
		return nil, false
	}
	return ctx, true
}

func (s *SessionStore) Destroy(sessionID string) {
	if s == nil {
		return
	}
	s.mu.Lock()
	defer s.mu.Unlock()
	delete(s.sessions, sessionID)
}

func (s *SessionStore) Sweep() {
	if s == nil {
		return
	}
	s.mu.Lock()
	defer s.mu.Unlock()
	now := time.Now()
	for id, ctx := range s.sessions {
		if now.Sub(ctx.CreatedAt) > s.ttl {
			delete(s.sessions, id)
		}
	}
}

func (s *SessionStore) StartSweeper(interval time.Duration) *time.Ticker {
	if interval <= 0 {
		interval = 1 * time.Minute
	}
	ticker := time.NewTicker(interval)
	go func() {
		for range ticker.C {
			s.Sweep()
		}
	}()
	return ticker
}
