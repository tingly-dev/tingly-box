package loadbalance

import (
	"sync"
	"time"
)

// BreakerState represents the state of a circuit breaker.
type BreakerState int

const (
	BreakerClosed   BreakerState = iota // Normal operation; requests pass through
	BreakerOpen                         // Tripped; requests are blocked
	BreakerHalfOpen                     // Recovery probe; one request is allowed
)

// Default circuit breaker tunables.
const (
	DefaultBreakerFailureThreshold = 3
	DefaultBreakerOpenDuration     = 30 * time.Second
)

// Breaker is a simple three-state circuit breaker for a single service.
//
// State transitions:
//   - Closed → Open when consecutive failures hit FailureThreshold.
//   - Open → HalfOpen lazily, once OpenDuration has elapsed since the trip.
//     The next Allow() call observes the elapsed time and flips state.
//   - HalfOpen → Closed on RecordSuccess.
//   - HalfOpen → Open on RecordFailure (open timer restarts).
//
// While HalfOpen, Allow() returns true for the first caller and false for
// concurrent callers, so exactly one probe request goes through at a time.
type Breaker struct {
	mu               sync.Mutex
	state            BreakerState
	consecFails      int
	openedAt         time.Time
	halfOpenInFlight bool

	FailureThreshold int
	OpenDuration     time.Duration
}

// NewBreaker creates a breaker with the supplied thresholds. Zero values
// fall back to defaults.
func NewBreaker(failureThreshold int, openDuration time.Duration) *Breaker {
	if failureThreshold <= 0 {
		failureThreshold = DefaultBreakerFailureThreshold
	}
	if openDuration <= 0 {
		openDuration = DefaultBreakerOpenDuration
	}
	return &Breaker{
		state:            BreakerClosed,
		FailureThreshold: failureThreshold,
		OpenDuration:     openDuration,
	}
}

// Allow reports whether a new request is permitted right now. It also
// performs the Open→HalfOpen transition when the open timer has expired
// and arbitrates which caller gets the probe slot.
func (b *Breaker) Allow() bool {
	b.mu.Lock()
	defer b.mu.Unlock()

	switch b.state {
	case BreakerClosed:
		return true
	case BreakerOpen:
		if time.Since(b.openedAt) >= b.OpenDuration {
			b.state = BreakerHalfOpen
			b.halfOpenInFlight = true
			return true
		}
		return false
	case BreakerHalfOpen:
		if b.halfOpenInFlight {
			return false
		}
		b.halfOpenInFlight = true
		return true
	}
	return true
}

// RecordSuccess closes the breaker and resets failure tracking.
func (b *Breaker) RecordSuccess() {
	b.mu.Lock()
	defer b.mu.Unlock()
	b.state = BreakerClosed
	b.consecFails = 0
	b.halfOpenInFlight = false
}

// RecordFailure increments failure tracking and trips the breaker when
// the threshold is reached. A failure during HalfOpen immediately re-opens.
func (b *Breaker) RecordFailure() {
	b.mu.Lock()
	defer b.mu.Unlock()

	if b.state == BreakerHalfOpen {
		b.state = BreakerOpen
		b.openedAt = time.Now()
		b.halfOpenInFlight = false
		return
	}
	b.consecFails++
	if b.consecFails >= b.FailureThreshold {
		b.state = BreakerOpen
		b.openedAt = time.Now()
	}
}

// State returns the current breaker state. Intended for introspection / UI.
func (b *Breaker) State() BreakerState {
	b.mu.Lock()
	defer b.mu.Unlock()
	// Apply the lazy Open→HalfOpen transition for read consistency.
	if b.state == BreakerOpen && time.Since(b.openedAt) >= b.OpenDuration {
		return BreakerHalfOpen
	}
	return b.state
}

// String renders the breaker state for logs/UI.
func (s BreakerState) String() string {
	switch s {
	case BreakerClosed:
		return "closed"
	case BreakerOpen:
		return "open"
	case BreakerHalfOpen:
		return "half_open"
	}
	return "unknown"
}

// BreakerStore is a concurrent-safe registry of breakers keyed by service
// identifier. Breakers are created lazily on first access.
//
// The store is process-wide: two rules that reference the same
// provider:model share breaker state. This is intentional — if a service
// is failing, it is generally failing for every rule that uses it.
type BreakerStore struct {
	mu       sync.RWMutex
	breakers map[string]*Breaker
	failures int
	openFor  time.Duration
}

// NewBreakerStore returns a BreakerStore. Defaults are applied for zero
// values, matching the package defaults.
func NewBreakerStore(failureThreshold int, openDuration time.Duration) *BreakerStore {
	if failureThreshold <= 0 {
		failureThreshold = DefaultBreakerFailureThreshold
	}
	if openDuration <= 0 {
		openDuration = DefaultBreakerOpenDuration
	}
	return &BreakerStore{
		breakers: make(map[string]*Breaker),
		failures: failureThreshold,
		openFor:  openDuration,
	}
}

// Get returns the breaker for the given serviceID, creating one with the
// store's default thresholds if needed.
func (s *BreakerStore) Get(serviceID string) *Breaker {
	s.mu.RLock()
	if b, ok := s.breakers[serviceID]; ok {
		s.mu.RUnlock()
		return b
	}
	s.mu.RUnlock()

	s.mu.Lock()
	defer s.mu.Unlock()
	if b, ok := s.breakers[serviceID]; ok {
		return b
	}
	b := NewBreaker(s.failures, s.openFor)
	s.breakers[serviceID] = b
	return b
}

// Allow is a convenience over Get(serviceID).Allow().
func (s *BreakerStore) Allow(serviceID string) bool {
	return s.Get(serviceID).Allow()
}

// IsAvailable reports whether the service's breaker currently permits traffic,
// without consuming a half-open probe slot. Closed and HalfOpen are available;
// Open is not. Unlike Allow(), this is a side-effect-free read — callers that
// only need to know "is this service usable right now" (e.g. affinity scoping)
// should use it so they don't steal the single half-open probe from the
// selection path.
func (s *BreakerStore) IsAvailable(serviceID string) bool {
	return s.Get(serviceID).State() != BreakerOpen
}

// RecordSuccess is a convenience over Get(serviceID).RecordSuccess().
func (s *BreakerStore) RecordSuccess(serviceID string) {
	s.Get(serviceID).RecordSuccess()
}

// RecordFailure is a convenience over Get(serviceID).RecordFailure().
func (s *BreakerStore) RecordFailure(serviceID string) {
	s.Get(serviceID).RecordFailure()
}

// Snapshot returns a copy of current breaker states for introspection.
func (s *BreakerStore) Snapshot() map[string]BreakerState {
	s.mu.RLock()
	defer s.mu.RUnlock()
	out := make(map[string]BreakerState, len(s.breakers))
	for id, b := range s.breakers {
		out[id] = b.State()
	}
	return out
}

// defaultStore is the process-wide breaker registry used by tactics and
// the recorder integration. Tests may replace it; production code should
// use the package-level helpers below.
var defaultStore = NewBreakerStore(DefaultBreakerFailureThreshold, DefaultBreakerOpenDuration)

// DefaultBreakerStore returns the process-wide breaker store.
func DefaultBreakerStore() *BreakerStore {
	return defaultStore
}

// AllowService is a package-level convenience used by selection logic.
func AllowService(serviceID string) bool {
	return defaultStore.Allow(serviceID)
}

// RecordServiceSuccess is a package-level convenience used by dispatch.
func RecordServiceSuccess(serviceID string) {
	defaultStore.RecordSuccess(serviceID)
}

// RecordServiceFailure is a package-level convenience used by dispatch.
func RecordServiceFailure(serviceID string) {
	defaultStore.RecordFailure(serviceID)
}
