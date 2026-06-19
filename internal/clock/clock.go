package clock

import "time"

// nowFn is the time source for all time-based logic in tingly-box that needs
// deterministic control in tests/simulation. Production uses time.Now.
// This includes: circuit breaker timing, health monitor recovery windows,
// and affinity TTL bookkeeping. A single shared source ensures all three
// advance together in the simulator.
var nowFn = time.Now

// Now returns the current time. In production this is time.Now; in tests
// and the LB harness simulator it can be overridden via SetClock.
func Now() time.Time {
	return nowFn()
}

// SetClock installs fn as the time source and returns a restore function
// that reinstates the previous source. Intended for tests and the LB harness
// simulator only; callers must defer the returned restore to avoid leaking
// the fake clock into production paths.
//
// Usage in lbsim:
//
//	restore := clock.SetClock(fakeClock.Now)
//	defer restore()
func SetClock(fn func() time.Time) (restore func()) {
	prev := nowFn
	nowFn = fn
	return func() { nowFn = prev }
}
