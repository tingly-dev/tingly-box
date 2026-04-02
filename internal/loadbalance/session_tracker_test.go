package loadbalance

import (
	"fmt"
	"testing"
	"time"
)

func TestSessionTracker_TryAcquire(t *testing.T) {
	tracker := NewSessionTracker(30 * time.Minute)

	// Initialize capacities
	tracker.InitializeCapacities([]*Service{
		{Provider: "prov-1", Model: "gpt-4", ModelCapacity: intPtr(1)},
	}, map[string]int64{"prov-1:gpt-4": 1, "prov-1": 100})

	// Should succeed
	ok, err := tracker.TryAcquire("session-1", "prov-1", "gpt-4")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if !ok {
		t.Fatal("expected acquire to succeed")
	}

	// Should fail when at capacity
	ok, _ = tracker.TryAcquire("session-2", "prov-1", "gpt-4")
	if ok {
		t.Fatal("expected acquire to fail at capacity")
	}
}

func TestSessionTracker_ProviderCapacity(t *testing.T) {
	tracker := NewSessionTracker(30 * time.Minute)

	// Initialize with provider total capacity
	tracker.InitializeCapacities([]*Service{
		{Provider: "prov-1", Model: "gpt-4", ModelCapacity: intPtr(100)},
		{Provider: "prov-1", Model: "gpt-3.5", ModelCapacity: intPtr(100)},
	}, map[string]int64{
		"prov-1:gpt-4": 100,
		"prov-1:gpt-3.5": 100,
		"prov-1": 100, // Provider total
	})

	// Fill provider capacity
	for i := 0; i < 100; i++ {
		tracker.TryAcquire(fmt.Sprintf("session-%d", i), "prov-1", "gpt-4")
	}

	// Should fail for both models (provider at total capacity)
	ok, _ := tracker.TryAcquire("session-x", "prov-1", "gpt-4")
	if ok {
		t.Fatal("expected acquire to fail (provider at total capacity)")
	}

	ok, _ = tracker.TryAcquire("session-y", "prov-1", "gpt-3.5")
	if ok {
		t.Fatal("expected acquire to fail (provider at total capacity)")
	}
}

func TestSessionTracker_Release(t *testing.T) {
	tracker := NewSessionTracker(30 * time.Minute)
	tracker.InitializeCapacities([]*Service{
		{Provider: "prov-1", Model: "gpt-4", ModelCapacity: intPtr(10)},
	}, map[string]int64{"prov-1:gpt-4": 10, "prov-1": 100})

	tracker.TryAcquire("session-1", "prov-1", "gpt-4")
	tracker.Release("session-1")

	// Should be able to acquire again
	ok, _ := tracker.TryAcquire("session-2", "prov-1", "gpt-4")
	if !ok {
		t.Fatal("expected acquire to succeed after release")
	}
}

func TestSessionTracker_IdleTimeout(t *testing.T) {
	tracker := NewSessionTracker(1 * time.Millisecond) // Very short for testing

	tracker.TryAcquire("session-1", "prov-1", "gpt-4")
	time.Sleep(2 * time.Millisecond)

	tracker.CleanupIdleSessions()

	// Session should be cleaned up
	ok, _ := tracker.TryAcquire("session-1", "prov-1", "gpt-4")
	if !ok {
		t.Fatal("expected acquire to succeed (session was cleaned up)")
	}
}

func intPtr(i int) *int {
	return &i
}
