package engine

import (
	"context"
	"os"
	"path/filepath"
	"sync"
	"testing"

	"github.com/tingly-dev/tingly-box/agentboot"
)

// memStore records what the engine writes.
type memStore struct {
	mu      sync.Mutex
	entries map[string][]Entry
}

func newMemStore() *memStore { return &memStore{entries: map[string][]Entry{}} }

func (m *memStore) Append(id string, e Entry) {
	m.mu.Lock()
	defer m.mu.Unlock()
	m.entries[id] = append(m.entries[id], e)
}
func (m *memStore) SetRunning(string) bool           { return true }
func (m *memStore) SetCompleted(string, string) bool { return true }
func (m *memStore) SetFailed(string, string) bool    { return true }
func (m *memStore) SetInterrupted(string)            {}

func (m *memStore) count(id string) int {
	m.mu.Lock()
	defer m.mu.Unlock()
	return len(m.entries[id])
}

func TestStart_LosingClaimRecordsNothing(t *testing.T) {
	store := newMemStore()
	e := New(Config{Store: store})
	t.Cleanup(func() { e.Shutdown(context.Background()) })

	// Simulate a concurrent send that already won the claim.
	e.mu.Lock()
	e.runs["s1"] = &run{cancel: func() {}, done: make(chan struct{})}
	e.mu.Unlock()

	if e.Start("s1", t.TempDir(), "second", true, nil) {
		t.Fatal("Start succeeded while another turn held the claim")
	}
	if n := store.count("s1"); n != 0 {
		t.Fatalf("losing Start wrote %d transcript entries, want none", n)
	}
	if err := e.Release("s1"); err != ErrBusy {
		t.Fatalf("Release during a turn = %v, want ErrBusy", err)
	}
}

// An edited profile keeps its settings path, so the path alone would let a
// resident process keep running on the settings it was launched with.
func TestLaunchSignature_ChangesWhenTheSettingsFileChanges(t *testing.T) {
	path := filepath.Join(t.TempDir(), "settings.json")
	if err := os.WriteFile(path, []byte(`{"model":"a"}`), 0o600); err != nil {
		t.Fatal(err)
	}
	opts := agentboot.ExecutionOptions{SettingsPath: path}
	before := launchSignature(opts)
	if err := os.WriteFile(path, []byte(`{"model":"b"}`), 0o600); err != nil {
		t.Fatal(err)
	}
	if launchSignature(opts) == before {
		t.Fatal("signature unchanged after the profile's settings changed; the resident process would not restart")
	}
}
