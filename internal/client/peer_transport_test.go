package client

import (
	"net/http"
	"testing"

	"github.com/tingly-dev/tingly-box/ai"
	"github.com/tingly-dev/tingly-box/internal/peertransport"
	"github.com/tingly-dev/tingly-box/internal/typ"
)

func TestPeerTransportSharesPoolAndRespectsProxy(t *testing.T) {
	t.Setenv("TINGLY_HTTP3", "auto")
	pool := &TransportPool{transports: make(map[string]*pooledTransport)}
	defer pool.Clear()
	first := pool.GetPeerTransport("peer", "", "", ai.Issuer(""), typ.SessionID{})
	if _, ok := first.(*peertransport.Transport); !ok {
		t.Fatalf("direct peer transport %T", first)
	}
	if next := pool.GetPeerTransport("peer", "", "", ai.Issuer(""), typ.SessionID{}); next != first {
		t.Fatal("new SDK clients allocated distinct peer pools")
	}
	for _, proxy := range []string{"http://localhost:8080", "https://localhost:8080", "socks5://localhost:1080"} {
		got := pool.GetPeerTransport(proxy, "", proxy, ai.Issuer(""), typ.SessionID{})
		if _, ok := got.(*http.Transport); !ok {
			t.Fatalf("explicit proxy %s enabled H3: %T", proxy, got)
		}
	}
	t.Setenv("TINGLY_HTTP3", "off")
	if got := pool.GetPeerTransport("off", "", "", ai.Issuer(""), typ.SessionID{}); got == nil {
		t.Fatal("nil disabled transport")
	} else if _, ok := got.(*http.Transport); !ok {
		t.Fatalf("off enabled H3: %T", got)
	}
}
