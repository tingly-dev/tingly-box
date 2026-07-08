package db

import (
	"testing"

	"github.com/tingly-dev/tingly-box/internal/loadbalance"
)

// newTestStatsStore creates a stats store backed by a temp database.
func newTestStatsStore(t *testing.T) *StatsStore {
	t.Helper()
	sm, err := NewStoreManager(t.TempDir())
	if err != nil {
		t.Fatalf("NewStoreManager error: %v", err)
	}
	t.Cleanup(func() { sm.Close() })
	return sm.Stats()
}

// TestStatsStore_ClearService verifies ClearService removes only the targeted
// provider:model, leaving other services' persisted stats intact. It fills the
// store with a couple of services, clears one, and asserts the survivor stays.
func TestStatsStore_ClearService(t *testing.T) {
	store := newTestStatsStore(t)

	// Seed two services.
	svcA := &loadbalance.Service{Provider: "prov-a", Model: "m"}
	svcB := &loadbalance.Service{Provider: "prov-b", Model: "m"}
	if _, err := store.RecordUsage(svcA, 10, 20); err != nil {
		t.Fatalf("RecordUsage A: %v", err)
	}
	if _, err := store.RecordUsage(svcB, 30, 40); err != nil {
		t.Fatalf("RecordUsage B: %v", err)
	}

	// Clear only A.
	if err := store.ClearService("prov-a", "m"); err != nil {
		t.Fatalf("ClearService: %v", err)
	}

	// A is gone.
	if _, ok := store.Get("prov-a", "m"); ok {
		t.Fatal("prov-a/m should be cleared")
	}
	// B survives.
	gotB, ok := store.Get("prov-b", "m")
	if !ok {
		t.Fatal("prov-b/m should still exist (ClearService must be scoped)")
	}
	if gotB.RequestCount != 1 {
		t.Errorf("prov-b/m RequestCount = %d, want 1 (untouched)", gotB.RequestCount)
	}

	// Clearing a non-existent service is a no-op (no error).
	if err := store.ClearService("never", "existed"); err != nil {
		t.Fatalf("ClearService on missing row should be a no-op, got: %v", err)
	}
}
