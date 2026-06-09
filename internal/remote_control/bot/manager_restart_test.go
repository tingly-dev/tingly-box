package bot

import (
	"testing"
	"time"
)

// TestNextRestartDelay_Backoff locks in the crash-restart backoff schedule that
// replaced the old 30s reconciliation poll: exponential growth from the base
// delay, capped at restartMaxDelay, with the attempt counter advancing on every
// consecutive failure.
func TestNextRestartDelay_Backoff(t *testing.T) {
	m := &Manager{restartAttempts: make(map[string]int)}
	const uuid = "bot-x"

	want := []time.Duration{
		3 * time.Second,  // attempt 1: base
		6 * time.Second,  // attempt 2
		12 * time.Second, // attempt 3
		24 * time.Second, // attempt 4
		48 * time.Second, // attempt 5
		60 * time.Second, // attempt 6: 96s capped to max
		60 * time.Second, // attempt 7: still capped
	}

	for i, w := range want {
		got, attempt := m.nextRestartDelay(uuid, 0)
		if attempt != i+1 {
			t.Fatalf("step %d: attempt = %d, want %d", i, attempt, i+1)
		}
		if got != w {
			t.Fatalf("attempt %d: delay = %v, want %v", attempt, got, w)
		}
	}
}

// TestNextRestartDelay_HealthyRunResets verifies that a bot which ran healthily
// before dying is treated as a fresh failure (immediate base-delay restart)
// rather than inheriting a stale crash-loop backoff.
func TestNextRestartDelay_HealthyRunResets(t *testing.T) {
	m := &Manager{restartAttempts: make(map[string]int)}
	const uuid = "bot-y"

	// Drive the backoff up a few steps with rapid (unhealthy) crashes.
	for i := 0; i < 4; i++ {
		m.nextRestartDelay(uuid, 0)
	}

	// A subsequent exit after a healthy run resets to attempt 1 / base delay.
	got, attempt := m.nextRestartDelay(uuid, restartHealthyRun)
	if attempt != 1 {
		t.Fatalf("after healthy run: attempt = %d, want 1", attempt)
	}
	if got != restartBaseDelay {
		t.Fatalf("after healthy run: delay = %v, want %v", got, restartBaseDelay)
	}
}

// TestClearRestartAttempts ensures backoff state is forgotten once a bot is no
// longer being recovered (intentional stop / disabled), so a later restart
// starts from the base delay.
func TestClearRestartAttempts(t *testing.T) {
	m := &Manager{restartAttempts: make(map[string]int)}
	const uuid = "bot-z"

	m.nextRestartDelay(uuid, 0)
	m.nextRestartDelay(uuid, 0)
	m.clearRestartAttempts(uuid)

	got, attempt := m.nextRestartDelay(uuid, 0)
	if attempt != 1 || got != restartBaseDelay {
		t.Fatalf("after clear: attempt = %d delay = %v, want 1 and %v", attempt, got, restartBaseDelay)
	}
}
