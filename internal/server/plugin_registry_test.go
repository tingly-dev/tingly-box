package server

import (
	"testing"
	"time"
)

func TestPluginRegistry_RegisterResolveExpire(t *testing.T) {
	r := NewPluginRegistry()
	reg := r.Register("my-rag", "http://127.0.0.1:8765/v1", "plugin/my-rag", "experiment", "", 50*time.Millisecond)

	// stable id from name
	if reg.ID != PluginID("my-rag") {
		t.Fatalf("id not derived from name: %s", reg.ID)
	}

	// resolves to a live plugin-kind provider
	p, ok := r.Resolve(reg.ID)
	if !ok {
		t.Fatalf("expected live resolution")
	}
	if !p.IsPlugin() || p.APIBase != "http://127.0.0.1:8765/v1" || p.PluginDetail.ModelID != "plugin/my-rag" {
		t.Fatalf("synthesized provider wrong: %+v", p)
	}

	// after TTL it is gone (auto-expire on resolve)
	time.Sleep(70 * time.Millisecond)
	if _, ok := r.Resolve(reg.ID); ok {
		t.Fatalf("expected expiry after TTL")
	}
}

func TestPluginRegistry_HeartbeatKeepsAlive(t *testing.T) {
	r := NewPluginRegistry()
	reg := r.Register("p", "http://x/v1", "", "", "", 60*time.Millisecond)

	time.Sleep(40 * time.Millisecond)
	if !r.Heartbeat(reg.LeaseID, 60*time.Millisecond) {
		t.Fatalf("heartbeat should succeed before expiry")
	}
	time.Sleep(40 * time.Millisecond) // 80ms since register, but heartbeat reset it
	if _, ok := r.Resolve(reg.ID); !ok {
		t.Fatalf("heartbeat should have kept the instance alive")
	}

	// unknown lease
	if r.Heartbeat("nope", 0) {
		t.Fatalf("unknown lease must not heartbeat")
	}
}

func TestPluginRegistry_Deregister(t *testing.T) {
	r := NewPluginRegistry()
	reg := r.Register("p", "http://x/v1", "", "", "", time.Minute)
	if !r.Deregister(reg.LeaseID) {
		t.Fatalf("deregister should remove the instance")
	}
	if _, ok := r.Resolve(reg.ID); ok {
		t.Fatalf("resolve should miss after deregister")
	}
	if r.Deregister(reg.LeaseID) {
		t.Fatalf("second deregister should be a no-op")
	}
}

func TestPluginRegistry_ReRegisterReusesID(t *testing.T) {
	r := NewPluginRegistry()
	a := r.Register("same", "http://a/v1", "", "", "", time.Minute)
	b := r.Register("same", "http://b/v1", "", "", "", time.Minute)
	if a.ID != b.ID {
		t.Fatalf("re-register must reuse the stable id")
	}
	if a.LeaseID == b.LeaseID {
		t.Fatalf("lease must rotate on re-register")
	}
	// latest endpoint wins
	p, _ := r.Resolve(b.ID)
	if p.APIBase != "http://b/v1" {
		t.Fatalf("latest registration endpoint should win, got %s", p.APIBase)
	}
}
