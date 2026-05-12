package local

import (
	"testing"
)

func TestTransportHandler_ResetAll_ResetsAllServers(t *testing.T) {
	h := &stubHandler{}

	th := NewTransportHandler(nil, nil, "", nil)

	s1 := NewMCPServer("c1", h, nil)
	s2 := NewMCPServer("c2", h, nil)

	if err := s1.Start(); err != nil {
		t.Fatalf("s1.Start: %v", err)
	}
	if err := s2.Start(); err != nil {
		t.Fatalf("s2.Start: %v", err)
	}

	th.serversMu.Lock()
	th.servers["c1"] = s1
	th.servers["c2"] = s2
	th.serversMu.Unlock()

	th.ResetAll()

	s1.serverMu.RLock()
	s1nil := s1.httpServer == nil
	s1.serverMu.RUnlock()

	s2.serverMu.RLock()
	s2nil := s2.httpServer == nil
	s2.serverMu.RUnlock()

	if !s1nil {
		t.Error("s1.httpServer should be nil after ResetAll")
	}
	if !s2nil {
		t.Error("s2.httpServer should be nil after ResetAll")
	}

	// ResetAll must not remove entries from the map — only clear the inner server.
	th.serversMu.RLock()
	_, has1 := th.servers["c1"]
	_, has2 := th.servers["c2"]
	th.serversMu.RUnlock()

	if !has1 || !has2 {
		t.Error("ResetAll must keep server entries in the map; only the inner httpServer is cleared")
	}
}

func TestTransportHandler_ResetAll_EmptyMapIsNoOp(t *testing.T) {
	th := NewTransportHandler(nil, nil, "", nil)
	// Must not panic on empty map.
	th.ResetAll()
}
