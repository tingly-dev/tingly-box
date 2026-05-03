package interaction

import (
	"sync"
	"time"
)

// Registry owns the pending / await / resolve / cache lifecycle for
// long-running interactions whose result is delivered out-of-band.
//
// The registry is generic over the result type so different scenarios
// can keep their own native types (e.g. Result for the wait HTTP
// endpoint; an internal value type for fully in-process flows). Phase 1
// uses Registry[Result] for the Claude Code wait endpoint.
//
// Concurrency model:
//   - Producer side calls Begin (mark inflight) then eventually Resolve.
//   - Consumer side (long-poll) calls Await to obtain the receive
//     channel and either picks up the cached answer or blocks until
//     Resolve fires.
//   - Recently-resolved values are cached for answerTTL so a script
//     reconnect that races with the producer still gets a deterministic
//     answer instead of an "unknown id" 404.
type Registry[T any] struct {
	mu       sync.Mutex
	waiters  map[string]chan T
	inflight map[string]struct{}
	answers  map[string]answeredEntry[T]
	answerTL time.Duration
}

type answeredEntry[T any] struct {
	value     T
	expiresAt time.Time
}

// New creates a registry. answerTTL controls how long resolved values
// remain in the cache for late reconnects (default 30s if <=0).
func New[T any](answerTTL time.Duration) *Registry[T] {
	if answerTTL <= 0 {
		answerTTL = 30 * time.Second
	}
	return &Registry[T]{
		waiters:  make(map[string]chan T),
		inflight: make(map[string]struct{}),
		answers:  make(map[string]answeredEntry[T]),
		answerTL: answerTTL,
	}
}

// Begin marks id as inflight. Returns true if the caller should run
// the producer (newly added) or false if id is already inflight or
// already cached. Idempotent — duplicate triggers reuse the existing
// producer instead of double-prompting the human.
func (r *Registry[T]) Begin(id string) bool {
	r.mu.Lock()
	defer r.mu.Unlock()
	r.gcLocked()
	if _, ok := r.inflight[id]; ok {
		return false
	}
	if _, ok := r.answers[id]; ok {
		return false
	}
	r.inflight[id] = struct{}{}
	return true
}

// Resolve removes id from inflight, delivers value to any waiter
// (non-blocking), and caches value for answerTTL.
func (r *Registry[T]) Resolve(id string, value T) {
	r.mu.Lock()
	delete(r.inflight, id)
	ch, ok := r.waiters[id]
	if ok {
		delete(r.waiters, id)
	}
	r.gcLocked()
	r.answers[id] = answeredEntry[T]{
		value:     value,
		expiresAt: time.Now().Add(r.answerTL),
	}
	r.mu.Unlock()
	if ok {
		select {
		case ch <- value:
		default:
		}
	}
}

// Await returns the receive channel for id. ok is true if id is
// inflight or has a cached answer (the caller may safely block on the
// channel); false means the id is unknown or already evicted and the
// channel is nil.
//
// If a cached answer exists, the returned channel is buffered and
// pre-loaded so a select returns immediately.
func (r *Registry[T]) Await(id string) (<-chan T, bool) {
	r.mu.Lock()
	defer r.mu.Unlock()
	r.gcLocked()
	if e, ok := r.answers[id]; ok {
		ch := make(chan T, 1)
		ch <- e.value
		return ch, true
	}
	if _, ok := r.inflight[id]; ok {
		if ch, ok := r.waiters[id]; ok {
			return ch, true
		}
		ch := make(chan T, 1)
		r.waiters[id] = ch
		return ch, true
	}
	return nil, false
}

// Cancel removes id from inflight and drops any waiter channel without
// delivering. Existing select-blocked goroutines fall through to their
// own ctx / timeout path.
func (r *Registry[T]) Cancel(id string) {
	r.mu.Lock()
	defer r.mu.Unlock()
	delete(r.inflight, id)
	delete(r.waiters, id)
}

// Forget removes any cached answer.
func (r *Registry[T]) Forget(id string) {
	r.mu.Lock()
	defer r.mu.Unlock()
	delete(r.answers, id)
}

// IsInflight reports whether id has an active producer.
func (r *Registry[T]) IsInflight(id string) bool {
	r.mu.Lock()
	defer r.mu.Unlock()
	_, ok := r.inflight[id]
	return ok
}

func (r *Registry[T]) gcLocked() {
	now := time.Now()
	for id, e := range r.answers {
		if now.After(e.expiresAt) {
			delete(r.answers, id)
		}
	}
}
