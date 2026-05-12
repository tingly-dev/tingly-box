package local

import (
	"context"
	"net/http"
	"net/http/httptest"
	"sync/atomic"
	"testing"
)

// stubHandler is a minimal MCPConnectionHandler for lifecycle tests.
type stubHandler struct {
	listCalls atomic.Int32
}

func (h *stubHandler) ListTools(_ context.Context) ([]MCPTool, error) {
	h.listCalls.Add(1)
	return []MCPTool{{Name: "stub_tool", Description: "stub"}}, nil
}

func (h *stubHandler) CallTool(_ context.Context, _ string, _ map[string]any) (string, error) {
	return "ok", nil
}

func TestMCPServer_StartAndStop(t *testing.T) {
	s := NewMCPServer("test", &stubHandler{}, nil)

	s.serverMu.RLock()
	nilBefore := s.httpServer == nil
	s.serverMu.RUnlock()
	if !nilBefore {
		t.Fatal("httpServer should be nil before Start")
	}

	if err := s.Start(); err != nil {
		t.Fatalf("Start() error: %v", err)
	}

	s.serverMu.RLock()
	nilAfterStart := s.httpServer == nil
	s.serverMu.RUnlock()
	if nilAfterStart {
		t.Fatal("httpServer should be non-nil after Start")
	}

	if err := s.Stop(); err != nil {
		t.Fatalf("Stop() error: %v", err)
	}

	s.serverMu.RLock()
	nilAfterStop := s.httpServer == nil
	s.serverMu.RUnlock()
	if !nilAfterStop {
		t.Fatal("httpServer should be nil after Stop")
	}
}

func TestMCPServer_StartIdempotent(t *testing.T) {
	h := &stubHandler{}
	s := NewMCPServer("test", h, nil)

	_ = s.Start()
	_ = s.Start() // second call must be a no-op

	if got := int(h.listCalls.Load()); got != 1 {
		t.Errorf("ListTools called %d times, want 1 (Start must be idempotent)", got)
	}
	_ = s.Stop()
}

func TestMCPServer_Reset(t *testing.T) {
	h := &stubHandler{}
	s := NewMCPServer("test", h, nil)

	_ = s.Start()

	s.serverMu.RLock()
	notNil := s.httpServer != nil
	s.serverMu.RUnlock()
	if !notNil {
		t.Fatal("httpServer should be non-nil after Start")
	}

	s.Reset()

	s.serverMu.RLock()
	nilAfterReset := s.httpServer == nil
	s.serverMu.RUnlock()
	if !nilAfterReset {
		t.Fatal("httpServer should be nil after Reset")
	}

	// Next Start after Reset should list tools again.
	_ = s.Start()
	if got := int(h.listCalls.Load()); got != 2 {
		t.Errorf("ListTools called %d times after Reset+Start, want 2", got)
	}
	_ = s.Stop()
}

// TestMCPServer_ServeHTTP_PersistsAcrossRequests verifies that ServeHTTP does NOT
// shut down the inner server after each request (Fix 2 regression guard).
func TestMCPServer_ServeHTTP_PersistsAcrossRequests(t *testing.T) {
	h := &stubHandler{}
	s := NewMCPServer("test", h, nil)

	makeReq := func() {
		req := httptest.NewRequest(http.MethodPost, "/", nil)
		w := httptest.NewRecorder()
		s.ServeHTTP(w, req)
	}

	makeReq()

	s.serverMu.RLock()
	aliveAfterFirst := s.httpServer != nil
	s.serverMu.RUnlock()
	if !aliveAfterFirst {
		t.Fatal("httpServer must stay alive after first ServeHTTP (Fix 2)")
	}

	makeReq()

	s.serverMu.RLock()
	aliveAfterSecond := s.httpServer != nil
	s.serverMu.RUnlock()
	if !aliveAfterSecond {
		t.Fatal("httpServer must stay alive after second ServeHTTP")
	}

	// ListTools should be called exactly once — server reused, not rebuilt.
	if got := int(h.listCalls.Load()); got != 1 {
		t.Errorf("ListTools called %d times across 2 requests, want 1", got)
	}

	_ = s.Stop()
}
