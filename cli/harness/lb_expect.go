package main

import (
	"fmt"
	"slices"

	"github.com/tingly-dev/tingly-box/internal/server"
)

// lbExpect is the optional self-check block. All fields optional; a missing
// field is not asserted. Evaluated against the LAST request's trace plus the
// FINAL snapshots after the program runs.
type lbExpect struct {
	// FinalStatus is the last request's FinalStatus.
	FinalStatus *int `yaml:"final_status"`
	// Attempts is the last request's Attempts (exact, in order).
	Attempts []string `yaml:"attempts"`
	// AttemptsContain is the last request's Attempts must include each (set membership, order-independent).
	AttemptsContain []string `yaml:"attempts_contain"`
	// AttemptsExclude is the last request's Attempts must NOT include any.
	AttemptsExclude []string `yaml:"attempts_exclude"`
	// Pin is the final affinity pin for the last request's session ("" = no pin).
	Pin *string `yaml:"pin"`
	// Pins is final pin per named session (multi-session).
	Pins map[string]string `yaml:"pins"`
	// Breaker is final breaker snapshot subset (serviceID -> closed|open|half_open).
	Breaker map[string]string `yaml:"breaker"`
	// Health is final health snapshot subset (serviceID -> healthy|unhealthy).
	Health map[string]string `yaml:"health"`
	// DistinctFirstAttempts is the set of distinct serviceIDs that ever appeared as
	// the FIRST attempt across ALL request steps (within-tier load sharing).
	DistinctFirstAttempts []string `yaml:"distinct_first_attempts"`
}

// lbOutcome is what the expect block is evaluated against: the LAST request's
// trace plus the FINAL snapshots after the program ran.
type lbOutcome struct {
	lastTrace     *server.LBTrace
	finalPin      string            // affinity pin of the last request's session
	finalPins     map[string]string // final pin per named session
	finalBreakers map[string]string
	finalHealth   map[string]string
	firstAttempts map[string]bool // every serviceID seen as a request's first attempt
}

// check evaluates the expect block after the program runs. On mismatch,
// returns an error → non-zero exit (CI leg fails).
func (e *lbExpect) check(o lbOutcome) error {
	if e == nil {
		return nil
	}
	lastTrace, finalPin, finalPins := o.lastTrace, o.finalPin, o.finalPins
	finalBreakers, finalHealth, firstAttempts := o.finalBreakers, o.finalHealth, o.firstAttempts

	// Helper: string pointer to string (or "" if nil).
	ptrStr := func(p *string) string {
		if p == nil {
			return ""
		}
		return *p
	}
	// Helper: int pointer to int (or 0 if nil).
	ptrInt := func(p *int) int {
		if p == nil {
			return 0
		}
		return *p
	}

	// FinalStatus: last request's FinalStatus.
	if e.FinalStatus != nil {
		got := 0
		if lastTrace != nil {
			got = lastTrace.FinalStatus
		}
		if want := ptrInt(e.FinalStatus); got != want {
			return fmt.Errorf("expect: final_status: want %d, got %d", want, got)
		}
	}

	// Attempts: last request's Attempts (exact, in order).
	if len(e.Attempts) > 0 {
		got := []string{}
		if lastTrace != nil {
			got = lastTrace.Attempts
		}
		if !equalSlices(got, e.Attempts) {
			return fmt.Errorf("expect: attempts: want %v, got %v", e.Attempts, got)
		}
	}

	// AttemptsContain: last request must include each serviceID (set membership).
	for _, wantID := range e.AttemptsContain {
		found := false
		if lastTrace != nil {
			if slices.Contains(lastTrace.Attempts, wantID) {
				found = true
			}
		}
		if !found {
			return fmt.Errorf("expect: attempts_contain: service %q not found in attempts (got %v)", wantID, lastTrace.Attempts)
		}
	}

	// AttemptsExclude: last request must NOT include any serviceID.
	for _, banID := range e.AttemptsExclude {
		if lastTrace != nil {
			if slices.Contains(lastTrace.Attempts, banID) {
				return fmt.Errorf("expect: attempts_exclude: service %q should not appear in attempts (got %v)", banID, lastTrace.Attempts)
			}
		}
	}

	// Pin: final pin for the last request's session.
	if e.Pin != nil {
		wantPin := ptrStr(e.Pin)
		if finalPin != wantPin {
			return fmt.Errorf("expect: pin: want %q, got %q", wantPin, finalPin)
		}
	}

	// Pins: final pin per named session.
	for sess, wantPin := range e.Pins {
		gotPin, ok := finalPins[sess]
		if !ok {
			return fmt.Errorf("expect: pins: session %q not found in program", sess)
		}
		if gotPin != wantPin {
			return fmt.Errorf("expect: pins[%s]: want %q, got %q", sess, wantPin, gotPin)
		}
	}

	// Breaker: final breaker snapshot subset.
	for sid, wantState := range e.Breaker {
		gotState, ok := finalBreakers[sid]
		if !ok {
			return fmt.Errorf("expect: breaker: service %q not found in final breaker snapshot", sid)
		}
		if gotState != wantState {
			return fmt.Errorf("expect: breaker[%s]: want %s, got %s", sid, wantState, gotState)
		}
	}

	// Health: final health snapshot subset.
	for sid, wantState := range e.Health {
		gotState, ok := finalHealth[sid]
		if !ok {
			return fmt.Errorf("expect: health: service %q not found in final health snapshot", sid)
		}
		if gotState != wantState {
			return fmt.Errorf("expect: health[%s]: want %s, got %s", sid, wantState, gotState)
		}
	}

	// DistinctFirstAttempts: set of first-attempt serviceIDs across ALL request steps.
	if len(e.DistinctFirstAttempts) > 0 {
		wantSet := make(map[string]bool)
		for _, id := range e.DistinctFirstAttempts {
			wantSet[id] = true
		}
		for gotID := range firstAttempts {
			if !wantSet[gotID] {
				return fmt.Errorf("expect: distinct_first_attempts: unexpected service %q appeared (got set %v)", gotID, firstAttempts)
			}
		}
		for wantID := range wantSet {
			if !firstAttempts[wantID] {
				return fmt.Errorf("expect: distinct_first_attempts: expected service %q missing (got set %v)", wantID, firstAttempts)
			}
		}
	}

	return nil
}

// equalSlices compares two string slices for equality.
func equalSlices(a, b []string) bool {
	if len(a) != len(b) {
		return false
	}
	for i := range a {
		if a[i] != b[i] {
			return false
		}
	}
	return true
}
