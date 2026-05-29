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

