package obs

import (
	"strings"
	"testing"
)

// TestRequestBodyStore_CountEviction verifies the count cap evicts oldest.
func TestRequestBodyStore_CountEviction(t *testing.T) {
	s := NewRequestBodyStore(2, 0) // 2 entries, unlimited bytes

	id1 := s.Store("POST", "/a", "body-a")
	id2 := s.Store("POST", "/b", "body-b")
	id3 := s.Store("POST", "/c", "body-c")

	if s.Size() != 2 {
		t.Fatalf("expected size 2, got %d", s.Size())
	}
	if s.Get(id1) != nil {
		t.Errorf("expected oldest entry %s to be evicted", id1)
	}
	if s.Get(id2) == nil || s.Get(id3) == nil {
		t.Errorf("expected the two newest entries to be retained")
	}
}

// TestRequestBodyStore_ByteBudgetEviction verifies the byte budget evicts oldest
// even when the count cap is not reached.
func TestRequestBodyStore_ByteBudgetEviction(t *testing.T) {
	// Each body is 100 bytes; budget 250 bytes => at most 2 retained.
	s := NewRequestBodyStore(100, 250)
	body := strings.Repeat("x", 100)

	id1 := s.Store("POST", "/1", body)
	id2 := s.Store("POST", "/2", body)
	id3 := s.Store("POST", "/3", body)

	if s.Get(id1) != nil {
		t.Errorf("expected oldest entry to be evicted under byte budget")
	}
	if s.Get(id2) == nil || s.Get(id3) == nil {
		t.Errorf("expected the two newest entries to be retained")
	}
	if s.Bytes() > 250 {
		t.Errorf("expected bytes <= 250, got %d", s.Bytes())
	}
}

// TestRequestBodyStore_LastResortTruncation verifies a single body larger than
// the whole byte budget is truncated (and still retained).
func TestRequestBodyStore_LastResortTruncation(t *testing.T) {
	s := NewRequestBodyStore(10, 16) // 16-byte budget

	id := s.Store("POST", "/big", strings.Repeat("y", 100))
	e := s.Get(id)
	if e == nil {
		t.Fatal("expected oversized body to be retained (truncated)")
	}
	if !e.Truncated {
		t.Error("expected Truncated=true for body exceeding the budget")
	}
	if len(e.Body) > 16 {
		t.Errorf("expected truncated body <= 16 bytes, got %d", len(e.Body))
	}
}

// TestRequestBodyStore_NoTruncationWhenFits verifies bodies within budget are
// kept whole.
func TestRequestBodyStore_NoTruncationWhenFits(t *testing.T) {
	s := NewRequestBodyStore(10, 1024)
	body := `{"model":"gpt-4"}`

	id := s.Store("POST", "/ok", body)
	e := s.Get(id)
	if e == nil {
		t.Fatal("expected entry to be retained")
	}
	if e.Truncated {
		t.Error("expected Truncated=false when body fits the budget")
	}
	if e.Body != body {
		t.Errorf("expected body kept whole, got %q", e.Body)
	}
}
