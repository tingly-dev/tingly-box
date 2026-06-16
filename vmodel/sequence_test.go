package vmodel

import (
	"sync"
	"testing"
)

// TestSequence_CyclesStatuses verifies the canonical 200,200,429 program walks
// in order and wraps around (loops) by default.
func TestSequence_CyclesStatuses(t *testing.T) {
	seq := NewSequence(SequenceConfig{
		DefaultContent: "ok",
		Steps: []SequenceStep{
			{Status: 200},
			{Status: 200},
			{Status: 429},
		},
	})

	want := []int{200, 200, 429, 200, 200, 429, 200}
	for i, w := range want {
		got := seq.Next()
		if got.Status != w {
			t.Fatalf("request %d: status = %d, want %d", i, got.Status, w)
		}
		if w == 200 {
			if got.Error != nil {
				t.Fatalf("request %d: success step must have nil Error", i)
			}
			if got.Content != "ok" {
				t.Fatalf("request %d: content = %q, want default %q", i, got.Content, "ok")
			}
		} else {
			if got.Error == nil {
				t.Fatalf("request %d: error step must have non-nil Error", i)
			}
			if got.Error.Stage != ErrorStagePreContent {
				t.Fatalf("request %d: error stage = %v, want pre-content", i, got.Error.Stage)
			}
			if got.Error.Status != 429 || got.Error.Type != "rate_limit_error" {
				t.Fatalf("request %d: bad error meta: %+v", i, got.Error)
			}
		}
	}
}

// TestSequence_Repeat expands the Repeat count into consecutive steps.
func TestSequence_Repeat(t *testing.T) {
	seq := NewSequence(SequenceConfig{
		DefaultContent: "ok",
		Steps: []SequenceStep{
			{Status: 200, Repeat: 3},
			{Status: 500},
		},
	})
	if seq.Len() != 4 {
		t.Fatalf("Len = %d, want 4", seq.Len())
	}
	want := []int{200, 200, 200, 500, 200}
	for i, w := range want {
		if got := seq.Next().Status; got != w {
			t.Fatalf("request %d: status = %d, want %d", i, got, w)
		}
	}
}

// TestSequence_NoLoop clamps to the last step once the program is exhausted.
func TestSequence_NoLoop(t *testing.T) {
	seq := NewSequence(SequenceConfig{
		DefaultContent: "ok",
		NoLoop:         true,
		Steps: []SequenceStep{
			{Status: 200},
			{Status: 503},
		},
	})
	want := []int{200, 503, 503, 503}
	for i, w := range want {
		if got := seq.Next().Status; got != w {
			t.Fatalf("request %d: status = %d, want %d", i, got, w)
		}
	}
}

// TestSequence_EmptyStepsIsUsable guards the "no steps configured" path: the
// model degrades to a single success step rather than panicking on modulo-by-0.
func TestSequence_EmptyStepsIsUsable(t *testing.T) {
	seq := NewSequence(SequenceConfig{DefaultContent: "fallback"})
	got := seq.Next()
	if got.Status != 200 || got.Content != "fallback" || got.Error != nil {
		t.Fatalf("empty program: got %+v, want success with default content", got)
	}
}

// TestSequence_PerStepContentAndMessage verifies explicit content / message /
// type overrides win over the defaults.
func TestSequence_PerStepContentAndMessage(t *testing.T) {
	seq := NewSequence(SequenceConfig{
		DefaultContent: "default",
		Steps: []SequenceStep{
			{Status: 200, Content: "custom-ok"},
			{Status: 418, Message: "i am a teapot", Type: "teapot_error"},
		},
	})
	ok := seq.Next()
	if ok.Content != "custom-ok" {
		t.Fatalf("content = %q, want custom-ok", ok.Content)
	}
	fail := seq.Next()
	if fail.Error == nil || fail.Error.Status != 418 ||
		fail.Error.Message != "i am a teapot" || fail.Error.Type != "teapot_error" {
		t.Fatalf("error step overrides not honored: %+v", fail.Error)
	}
}

// TestSequence_ConcurrentNextIsAtomic verifies the atomic cursor hands out each
// position exactly once under concurrency (no duplicates, no gaps).
func TestSequence_ConcurrentNextIsAtomic(t *testing.T) {
	const n = 300
	// A program as long as the number of calls so each call maps to a unique
	// index; status encodes the index (200 + idx) so we can detect duplicates.
	steps := make([]SequenceStep, n)
	for i := range steps {
		steps[i] = SequenceStep{Status: 1000 + i}
	}
	seq := NewSequence(SequenceConfig{Steps: steps})

	var wg sync.WaitGroup
	results := make([]int, n)
	for i := 0; i < n; i++ {
		wg.Add(1)
		go func(i int) {
			defer wg.Done()
			results[i] = seq.Next().Status
		}(i)
	}
	wg.Wait()

	seen := make(map[int]bool, n)
	for _, s := range results {
		if seen[s] {
			t.Fatalf("status %d handed out more than once — cursor not atomic", s)
		}
		seen[s] = true
	}
	if len(seen) != n {
		t.Fatalf("got %d distinct steps, want %d", len(seen), n)
	}
}
