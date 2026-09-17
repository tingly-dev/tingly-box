// Package pool holds at most one [agentboot.PersistentSession] per
// caller-defined key, evicting by capacity (LRU) and by idle timeout.
//
// It lives apart from the agentboot root package because pooling is a
// distinct concern from the process/protocol lifecycle Runner and
// PersistentSession own: a Pool only ever calls the public
// PersistentSession interface (Status/Close), never anything about how the
// underlying process runs. See .design/claude-code.md §5.3.
package pool

import (
	"context"
	"errors"
	"sync"
	"time"

	"github.com/tingly-dev/tingly-box/agentboot"
)

// Sentinel errors returned by [Pool.Open].
var (
	// ErrKeyAlreadyOpen is returned when key already has a live
	// (non-terminated) session registered. Callers must Acquire first and
	// reuse it instead of calling Open again.
	ErrKeyAlreadyOpen = errors.New("agentboot/pool: key already open")

	// ErrFull is returned when the pool is at MaxSessions capacity and
	// every resident session is Running (mid-turn), so none can be safely
	// evicted to make room. The caller decides the fallback (e.g. run this
	// turn as a one-shot agentboot.Runner.Execute instead).
	ErrFull = errors.New("agentboot/pool: at capacity, no idle session to evict")
)

// Config controls a [Pool]'s capacity and idle-eviction policy.
type Config struct {
	// MaxSessions caps how many PersistentSessions the pool holds
	// concurrently. Open evicts the least-recently-touched Idle entry to
	// make room when at capacity. Zero or negative means unlimited.
	MaxSessions int

	// IdleTimeout closes and evicts a session that has been Idle (per
	// Status, not merely un-Touch-ed) for at least this long since its last
	// Touch. Zero or negative disables idle eviction.
	IdleTimeout time.Duration

	// SweepInterval is how often the idle sweep runs. Left zero with
	// IdleTimeout > 0, it defaults to IdleTimeout/4 (floored at 1s).
	SweepInterval time.Duration

	// CloseTimeout bounds how long a single eviction/close waits for the
	// session to shut down. The underlying process's own shutdown-grace/
	// Kill sequence still runs to completion independently of this
	// deadline. Defaults to 10s.
	CloseTimeout time.Duration
}

func normalizeConfig(cfg Config) Config {
	if cfg.CloseTimeout <= 0 {
		cfg.CloseTimeout = 10 * time.Second
	}
	if cfg.IdleTimeout > 0 && cfg.SweepInterval <= 0 {
		cfg.SweepInterval = cfg.IdleTimeout / 4
		if cfg.SweepInterval < time.Second {
			cfg.SweepInterval = time.Second
		}
	}
	return cfg
}

type entry struct {
	session      agentboot.PersistentSession
	lastActivity time.Time
}

// Pool holds at most one [agentboot.PersistentSession] per caller-defined
// key. The caller decides what a key means (e.g. tingly-box keys @cc's pool
// by chatID+agent+project, the same tuple remote/session.Manager.FindBy
// already uses).
//
// Pool never evicts a Running session (a turn in flight): capacity eviction
// only considers Idle entries, and the idle sweep only ever inspects Idle
// entries by construction. A pool at capacity with every entry Running
// returns ErrFull from Open rather than force-closing an active turn.
//
// Pool does not itself watch a session's Events() or drive Touch — callers
// call Touch when a turn completes (TurnCompleteEvent), and should
// Acquire/CloseAndRemove/Remove in response to a SessionStateEvent so the
// pool's bookkeeping never gets out of sync with a session that terminated
// on its own (e.g. a crash).
type Pool struct {
	cfg Config

	mu      sync.Mutex
	entries map[string]*entry

	stopOnce sync.Once
	stopCh   chan struct{}
	wg       sync.WaitGroup
}

// New creates a Pool and, if cfg.IdleTimeout > 0, starts its background
// idle sweep. Call Shutdown to stop the sweep and close every resident
// session.
func New(cfg Config) *Pool {
	cfg = normalizeConfig(cfg)
	p := &Pool{
		cfg:     cfg,
		entries: make(map[string]*entry),
		stopCh:  make(chan struct{}),
	}
	if cfg.IdleTimeout > 0 {
		p.wg.Add(1)
		go p.sweepLoop()
	}
	return p
}

// Acquire returns the live session registered for key, if any. A session
// observed to have reached SessionStateTerminated is treated as absent (and
// its bookkeeping dropped) — the caller should Open a fresh one.
func (p *Pool) Acquire(key string) (agentboot.PersistentSession, bool) {
	p.mu.Lock()
	defer p.mu.Unlock()
	e, ok := p.entries[key]
	if !ok {
		return nil, false
	}
	if e.session.Status() == agentboot.SessionStateTerminated {
		delete(p.entries, key)
		return nil, false
	}
	return e.session, true
}

// Touch refreshes key's last-activity time for LRU/idle-timeout purposes.
// Callers call this when a turn completes (a TurnCompleteEvent), not on
// every Send — idle time is measured from when a session actually went
// idle, not from when it was last asked to do something.
func (p *Pool) Touch(key string) {
	p.mu.Lock()
	defer p.mu.Unlock()
	if e, ok := p.entries[key]; ok {
		e.lastActivity = time.Now()
	}
}

// Open registers session under key. Returns ErrKeyAlreadyOpen if key
// already has a live session. If the pool is at MaxSessions capacity, the
// least-recently-touched Idle entry (excluding key) is closed and evicted
// first to make room; if no entry is safely evictable (every resident
// session is Running), Open returns ErrFull and session is not registered
// — closing session in that case is the caller's responsibility.
func (p *Pool) Open(ctx context.Context, key string, session agentboot.PersistentSession) error {
	p.mu.Lock()
	if existing, ok := p.entries[key]; ok && existing.session.Status() != agentboot.SessionStateTerminated {
		p.mu.Unlock()
		return ErrKeyAlreadyOpen
	}

	var evictKey string
	var evict *entry
	if p.cfg.MaxSessions > 0 && len(p.entries) >= p.cfg.MaxSessions {
		evictKey, evict = p.lruIdleLocked(key)
		if evict == nil {
			p.mu.Unlock()
			return ErrFull
		}
		delete(p.entries, evictKey)
	}
	p.entries[key] = &entry{session: session, lastActivity: time.Now()}
	p.mu.Unlock()

	if evict != nil {
		_ = p.closeEntry(ctx, evict)
	}
	return nil
}

// lruIdleLocked finds the least-recently-touched Idle entry other than
// excludeKey. Caller holds p.mu.
func (p *Pool) lruIdleLocked(excludeKey string) (string, *entry) {
	var (
		oldestKey string
		oldest    *entry
	)
	for k, e := range p.entries {
		if k == excludeKey || e.session.Status() != agentboot.SessionStateIdle {
			continue
		}
		if oldest == nil || e.lastActivity.Before(oldest.lastActivity) {
			oldestKey, oldest = k, e
		}
	}
	return oldestKey, oldest
}

// CloseAndRemove closes key's session (if any) and removes it from the
// pool. A no-op if key isn't registered.
func (p *Pool) CloseAndRemove(ctx context.Context, key string) error {
	p.mu.Lock()
	e, ok := p.entries[key]
	if ok {
		delete(p.entries, key)
	}
	p.mu.Unlock()
	if !ok {
		return nil
	}
	return p.closeEntry(ctx, e)
}

// Remove drops key's bookkeeping without calling Close — for a session the
// caller already knows has terminated on its own (e.g. it observed
// SessionStateEvent{Terminated} from a crash) and does not want re-closed.
func (p *Pool) Remove(key string) {
	p.mu.Lock()
	defer p.mu.Unlock()
	delete(p.entries, key)
}

// Len reports how many sessions are currently resident.
func (p *Pool) Len() int {
	p.mu.Lock()
	defer p.mu.Unlock()
	return len(p.entries)
}

func (p *Pool) closeEntry(ctx context.Context, e *entry) error {
	closeCtx, cancel := context.WithTimeout(ctx, p.cfg.CloseTimeout)
	defer cancel()
	return e.session.Close(closeCtx)
}

func (p *Pool) sweepLoop() {
	defer p.wg.Done()
	ticker := time.NewTicker(p.cfg.SweepInterval)
	defer ticker.Stop()
	for {
		select {
		case <-p.stopCh:
			return
		case <-ticker.C:
			p.sweepIdle()
		}
	}
}

func (p *Pool) sweepIdle() {
	cutoff := time.Now().Add(-p.cfg.IdleTimeout)

	type victim struct {
		key string
		e   *entry
	}
	var victims []victim

	p.mu.Lock()
	for k, e := range p.entries {
		if e.session.Status() == agentboot.SessionStateIdle && e.lastActivity.Before(cutoff) {
			victims = append(victims, victim{k, e})
		}
	}
	for _, v := range victims {
		delete(p.entries, v.key)
	}
	p.mu.Unlock()

	for _, v := range victims {
		_ = p.closeEntry(context.Background(), v.e)
	}
}

// Shutdown stops the idle sweep and closes every resident session,
// concurrently, waiting for all of them to finish or ctx to be canceled.
func (p *Pool) Shutdown(ctx context.Context) {
	p.stopOnce.Do(func() { close(p.stopCh) })
	p.wg.Wait()

	p.mu.Lock()
	entries := p.entries
	p.entries = make(map[string]*entry)
	p.mu.Unlock()

	var wg sync.WaitGroup
	for _, e := range entries {
		wg.Add(1)
		go func(e *entry) {
			defer wg.Done()
			_ = p.closeEntry(ctx, e)
		}(e)
	}
	wg.Wait()
}
