package runtime

import (
	"testing"
	"time"
)

func TestSessionStore_PutGetDestroy(t *testing.T) {
	ss := NewSessionStore(5 * time.Minute)
	ctx := &SessionContext{
		SessionID:     "sess-1",
		WorkspaceTree: map[string]any{"file.go": "package main"},
	}
	ss.Put(ctx)

	got, ok := ss.Get("sess-1")
	if !ok {
		t.Fatal("expected session to exist")
	}
	if got.SessionID != "sess-1" {
		t.Fatalf("expected sess-1, got %s", got.SessionID)
	}

	ss.Destroy("sess-1")
	_, ok = ss.Get("sess-1")
	if ok {
		t.Fatal("expected session to be destroyed")
	}
}

func TestSessionStore_TTLSweep(t *testing.T) {
	ss := NewSessionStore(1 * time.Millisecond)
	ss.Put(&SessionContext{SessionID: "old"})
	time.Sleep(50 * time.Millisecond)
	ss.Sweep()
	_, ok := ss.Get("old")
	if ok {
		t.Fatal("expected expired session to be swept")
	}
}
