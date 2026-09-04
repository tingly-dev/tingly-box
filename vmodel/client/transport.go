package vmodelclient

import (
	"context"
	"errors"
	"net"
	"net/http"
	"sync"
)

// Dialer opens a connection to the virtual-model server. network and addr
// are ignored by the in-memory listener; there is only one possible peer.
type Dialer func(ctx context.Context, network, addr string) (net.Conn, error)

var (
	mu        sync.RWMutex
	transport *http.Transport
)

var errNotConnected = errors.New("vmodel: no virtual-model server connected in this process")

// Connect points Transport at a server, typically
// virtualserver.Server.DialContext. Call it once at startup; Connect(nil)
// disconnects (used on shutdown and in tests).
func Connect(d Dialer) {
	mu.Lock()
	defer mu.Unlock()
	if transport != nil {
		transport.CloseIdleConnections()
		transport = nil
	}
	if d == nil {
		return
	}
	transport = &http.Transport{
		DialContext:         d,
		MaxIdleConns:        100,
		MaxIdleConnsPerHost: 100,
	}
}

// Transport returns the RoundTripper to use as the base of a vmodel
// provider's transport chain. It carries real HTTP — status codes, headers,
// SSE framing — to the connected server. When nothing is connected every
// request fails fast with a clear error instead of leaking a DNS lookup for
// the placeholder Host.
func Transport() http.RoundTripper {
	mu.RLock()
	t := transport
	mu.RUnlock()
	if t != nil {
		return t
	}
	return notConnectedTransport{}
}

type notConnectedTransport struct{}

func (notConnectedTransport) RoundTrip(*http.Request) (*http.Response, error) {
	return nil, &net.OpError{Op: "dial", Net: Scheme, Err: errNotConnected}
}
