package vmodel

import (
	"testing"
)

func TestEmitGate_Disabled(t *testing.T) {
	g := NewEmitGate(0)
	for i := 0; i < 5; i++ {
		if !g.Allow() {
			t.Fatalf("disabled gate denied at call %d", i)
		}
	}
	if g.Tripped() {
		t.Fatalf("disabled gate reports tripped")
	}
	g2 := NewEmitGate(-1)
	if !g2.Allow() || g2.Tripped() {
		t.Fatalf("negative cutoff should disable the gate")
	}
}

func TestEmitGate_AllowAndTrip(t *testing.T) {
	g := NewEmitGate(2)
	if !g.Allow() {
		t.Fatalf("first Allow should succeed")
	}
	if g.Tripped() {
		t.Fatalf("Tripped after 1 of 2 allowed")
	}
	if !g.Allow() {
		t.Fatalf("second Allow should succeed")
	}
	if !g.Tripped() {
		t.Fatalf("Tripped should be true after reaching cutoff")
	}
	if g.Allow() {
		t.Fatalf("Allow past cutoff must return false")
	}
	if g.Allow() {
		t.Fatalf("Allow remains false after tripping")
	}
}

func TestEmitChunksGated_EarlyTrip(t *testing.T) {
	g := NewEmitGate(2)
	chunks := []string{"a", "b", "c", "d"}
	var emitted []string
	tripped := EmitChunksGated(chunks, 0, g, func(_ int, c string) {
		emitted = append(emitted, c)
	})
	if !tripped {
		t.Fatalf("expected gate to trip")
	}
	if len(emitted) != 2 || emitted[0] != "a" || emitted[1] != "b" {
		t.Fatalf("expected exactly [a b], got %v", emitted)
	}
}

func TestEmitChunksGated_NoTrip(t *testing.T) {
	g := NewEmitGate(0)
	chunks := []string{"x", "y"}
	var emitted []string
	tripped := EmitChunksGated(chunks, 0, g, func(_ int, c string) {
		emitted = append(emitted, c)
	})
	if tripped {
		t.Fatalf("disabled gate should not trip")
	}
	if len(emitted) != 2 {
		t.Fatalf("expected all chunks emitted, got %v", emitted)
	}
}

type stubModel struct {
	ei *ErrorInjection
}

func (m *stubModel) ErrorInjection() *ErrorInjection { return m.ei }

type nonModel struct{}

func TestExtractErrorInjection(t *testing.T) {
	if got := ExtractErrorInjection(&nonModel{}); got != nil {
		t.Fatalf("non-implementer should return nil, got %+v", got)
	}
	if got := ExtractErrorInjection(&stubModel{ei: nil}); got != nil {
		t.Fatalf("model with nil injection should return nil, got %+v", got)
	}
	want := &ErrorInjection{Stage: ErrorStagePreContent, Status: 429}
	if got := ExtractErrorInjection(&stubModel{ei: want}); got != want {
		t.Fatalf("expected exact pointer back, got %+v", got)
	}
}

func TestMidStreamCutoff(t *testing.T) {
	cases := []struct {
		name string
		vm   any
		want int
	}{
		{"non-implementer", &nonModel{}, -1},
		{"nil injection", &stubModel{ei: nil}, -1},
		{"pre-content injection", &stubModel{ei: &ErrorInjection{Stage: ErrorStagePreContent}}, -1},
		{"mid-stream default", &stubModel{ei: &ErrorInjection{Stage: ErrorStageMidStream}}, 1},
		{"mid-stream zero events", &stubModel{ei: &ErrorInjection{Stage: ErrorStageMidStream, AfterEvents: 0}}, 1},
		{"mid-stream negative events", &stubModel{ei: &ErrorInjection{Stage: ErrorStageMidStream, AfterEvents: -3}}, 1},
		{"mid-stream explicit cutoff", &stubModel{ei: &ErrorInjection{Stage: ErrorStageMidStream, AfterEvents: 4}}, 4},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			if got := MidStreamCutoff(tc.vm); got != tc.want {
				t.Fatalf("MidStreamCutoff = %d, want %d", got, tc.want)
			}
		})
	}
}
