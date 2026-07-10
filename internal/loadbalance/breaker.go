package loadbalance

import (
	"sync"
	"time"

	"github.com/tingly-dev/tingly-box/internal/clock"
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
	DefaultBreakerFailureThreshold  = 3
	DefaultBreakerOpenDuration      = 30 * time.Second
	DefaultBreakerRecoveryThreshold = 3 // consecutive half-open successes required to close

	// DefaultPromotionHold is how long a recovered (Closed) primary tier must
	// stay recovered before it can reclaim sessions pinned to a lower tier.
	// It de-jitters batch return-to-primary: a freshly-recovered primary only
	// takes NEW sessions during the hold; existing fallback-tier pins migrate
	// back once the primary has proven stable. Zero disables the hold.
	DefaultPromotionHold = 60 * time.Second
)

// Breaker is a three-state circuit breaker for a single service, with
// hysteresis on recovery to avoid tier oscillation.
//
//   - Closed → Open when consecutive failures hit FailureThreshold.
//   - Open → HalfOpen lazily, once OpenDuration has elapsed since the trip.
//   - HalfOpen → Closed only after RecoveryThreshold consecutive probe
//     successes. A single success is not enough — it just releases the probe
//     slot so the next probe can go through.
//   - HalfOpen → Open on any probe failure (timer restarts from scratch).
//
// While HalfOpen, Allow() returns true for at most one caller at a time, so
// exactly one probe request goes through per probe round.
type Breaker struct {
	mu                sync.Mutex
	state             BreakerState
	consecFails       int
	openedAt          time.Time
	halfOpenInFlight  bool
	halfOpenClaimedAt time.Time // when the current half-open probe slot was claimed
	halfOpenSuccess   int       // consecutive successes in the current HalfOpen run
	closedSince       time.Time // when recovery completed (HalfOpen→Closed); zero while never recovered or currently open

	FailureThreshold  int
	OpenDuration      time.Duration
	RecoveryThreshold int // N_up: consecutive successes to recover
}

// NewBreaker creates a breaker with the supplied thresholds. Zero values
// fall back to defaults. RecoveryThreshold defaults to
// DefaultBreakerRecoveryThreshold.
func NewBreaker(failureThreshold int, openDuration time.Duration) *Breaker {
	if failureThreshold <= 0 {
		failureThreshold = DefaultBreakerFailureThreshold
	}
	if openDuration <= 0 {
		openDuration = DefaultBreakerOpenDuration
	}
	return &Breaker{
		state:             BreakerClosed,
		FailureThreshold:  failureThreshold,
		OpenDuration:      openDuration,
		RecoveryThreshold: DefaultBreakerRecoveryThreshold,
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
		if clock.Now().Sub(b.openedAt) >= b.OpenDuration {
			b.state = BreakerHalfOpen
			b.claimProbeLocked()
			b.halfOpenSuccess = 0
			return true
		}
		return false
	case BreakerHalfOpen:
		if b.halfOpenInFlight {
			// Reclaim a stale probe slot: if the claimer never reported an
			// outcome (e.g. it was selected but not dispatched), the slot
			// would otherwise stay consumed forever and the service could
			// never recover. After OpenDuration without an outcome, treat
			// the probe as lost and hand the slot to this caller.
			if clock.Now().Sub(b.halfOpenClaimedAt) < b.OpenDuration {
				return false
			}
		}
		b.claimProbeLocked()
		return true
	}
	return true
}

// claimProbeLocked marks the half-open probe slot as taken. Caller holds b.mu.
func (b *Breaker) claimProbeLocked() {
	b.halfOpenInFlight = true
	b.halfOpenClaimedAt = clock.Now()
}

// recoveryThreshold guards against a misconfigured zero value.
func (b *Breaker) recoveryThreshold() int {
	if b.RecoveryThreshold <= 0 {
		return DefaultBreakerRecoveryThreshold
	}
	return b.RecoveryThreshold
}

// RecordSuccess closes the breaker once RecoveryThreshold consecutive probe
// successes have been observed in HalfOpen. Before the threshold is reached it
// merely releases the probe slot so the next probe can go through. A success
// from a Closed breaker resets failure tracking.
func (b *Breaker) RecordSuccess() {
	b.mu.Lock()
	defer b.mu.Unlock()

	if b.state == BreakerHalfOpen {
		b.halfOpenSuccess++
		b.halfOpenInFlight = false
		if b.halfOpenSuccess >= b.recoveryThreshold() {
			b.state = BreakerClosed
			b.consecFails = 0
			b.halfOpenSuccess = 0
			b.closedSince = clock.Now()
		}
		return
	}
	b.consecFails = 0
}

// RecordFailure increments failure tracking and trips the breaker when the
// threshold is reached. A failure during HalfOpen immediately re-opens and
// restarts the open timer from scratch.
func (b *Breaker) RecordFailure() {
	b.mu.Lock()
	defer b.mu.Unlock()

	if b.state == BreakerHalfOpen {
		b.openLocked()
		b.halfOpenInFlight = false
		return
	}
	b.consecFails++
	if b.state == BreakerClosed && b.consecFails >= b.FailureThreshold {
		b.openLocked()
	}
}

// openLocked transitions to Open and stamps the open time. Caller holds b.mu.
func (b *Breaker) openLocked() {
	b.state = BreakerOpen
	b.openedAt = clock.Now()
	b.halfOpenSuccess = 0
	b.closedSince = time.Time{}
}

// ClosedSince returns when recovery last completed (the HalfOpen→Closed
// transition after RecoveryThreshold successes). It is zero while the service
// has never recovered or is currently open/tripping. Tier affinity uses it for
// PromotionHold: a primary tier must stay recovered for a hold period before
// it can reclaim sessions pinned to a lower tier, so a freshly-recovered
// primary doesn't vacuum all fallback-tier sessions back at once.
func (b *Breaker) ClosedSince() time.Time {
	b.mu.Lock()
	defer b.mu.Unlock()
	return b.closedSince
}

// State returns the current breaker state. Intended for introspection / UI.
func (b *Breaker) State() BreakerState {
	b.mu.Lock()
	defer b.mu.Unlock()
	// Apply the lazy Open→HalfOpen transition for read consistency.
	if b.state == BreakerOpen && clock.Now().Sub(b.openedAt) >= b.OpenDuration {
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

// BreakerStore is a concurrent-safe registry of breakers keyed by
// (ruleUUID, serviceID). Breakers are created lazily on first access.
//
// Breakers are rule-scoped: each rule owns independent breaker state per
// service. A service failing for one rule does NOT fail it for another rule
// that uses the same provider:model — each rule's failover decisions are
// driven only by the traffic it observes. This mirrors the rule-scoped
// affinity store (internal/server/affinity/affinity.go). Key composition lives
// in FormatBreakerKey.
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

// Get returns the breaker for the given (ruleUUID, serviceID), creating one
// with the store's default thresholds if needed.
func (s *BreakerStore) Get(ruleUUID, serviceID string) *Breaker {
	key := FormatBreakerKey(ruleUUID, serviceID)
	s.mu.RLock()
	if b, ok := s.breakers[key]; ok {
		s.mu.RUnlock()
		return b
	}
	s.mu.RUnlock()

	s.mu.Lock()
	defer s.mu.Unlock()
	if b, ok := s.breakers[key]; ok {
		return b
	}
	b := NewBreaker(s.failures, s.openFor)
	s.breakers[key] = b
	return b
}

// Allow is a convenience over Get(ruleUUID, serviceID).Allow().
func (s *BreakerStore) Allow(ruleUUID, serviceID string) bool {
	return s.Get(ruleUUID, serviceID).Allow()
}

// IsAvailable reports whether the service's breaker currently permits traffic,
// without consuming a half-open probe slot. Closed and HalfOpen are available;
// Open is not. Unlike Allow(), this is a side-effect-free read — callers that
// only need to know "is this service usable right now" (e.g. affinity scoping)
// should use it so they don't steal the single half-open probe from the
// selection path.
func (s *BreakerStore) IsAvailable(ruleUUID, serviceID string) bool {
	return s.Get(ruleUUID, serviceID).State() != BreakerOpen
}

// WithinPromotionHold reports whether the service recovered (entered Closed
// from a HalfOpen recovery) within the given hold window. Tier affinity uses
// this to keep sessions pinned to a lower tier from being vacuumed back to a
// freshly-recovered primary until it has proven stable for the hold. A service
// that is Open, HalfOpen, never-tripped, or recovered longer ago than hold is
// NOT within the hold. hold <= 0 disables the check (always false).
func (s *BreakerStore) WithinPromotionHold(ruleUUID, serviceID string, hold time.Duration) bool {
	if hold <= 0 {
		return false
	}
	b := s.Get(ruleUUID, serviceID)
	since := b.ClosedSince()
	if since.IsZero() {
		return false
	}
	return clock.Now().Sub(since) < hold
}

// RecordSuccess is a convenience over Get(ruleUUID, serviceID).RecordSuccess().
func (s *BreakerStore) RecordSuccess(ruleUUID, serviceID string) {
	s.Get(ruleUUID, serviceID).RecordSuccess()
}

// RecordFailure is a convenience over Get(ruleUUID, serviceID).RecordFailure().
func (s *BreakerStore) RecordFailure(ruleUUID, serviceID string) {
	s.Get(ruleUUID, serviceID).RecordFailure()
}

// Reset clears all breaker entries. Useful for tests/harness to avoid state leakage
// between scenarios when reusing the global store.
func (s *BreakerStore) Reset() {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.breakers = make(map[string]*Breaker)
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
func AllowService(ruleUUID, serviceID string) bool {
	return defaultStore.Allow(ruleUUID, serviceID)
}

// RecordServiceSuccess is a package-level convenience used by dispatch.
func RecordServiceSuccess(ruleUUID, serviceID string) {
	defaultStore.RecordSuccess(ruleUUID, serviceID)
}

// RecordServiceFailure is a package-level convenience used by dispatch.
func RecordServiceFailure(ruleUUID, serviceID string) {
	defaultStore.RecordFailure(ruleUUID, serviceID)
}
