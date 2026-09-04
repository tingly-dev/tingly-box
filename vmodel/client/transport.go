package vmodelclient

import (
	"context"
	"net"
	"net/http"
)

// Dialer opens a connection to a virtual-model server, typically
// virtualserver.Server.DialContext. network and addr are ignored by the
// in-memory listener; there is only one possible peer.
type Dialer func(ctx context.Context, network, addr string) (net.Conn, error)

// NewTransport returns the base transport for vmodel providers served by the
// given dialer. It carries real HTTP — status codes, headers, SSE framing —
// and is meant to sit under the gateway's generic provider chain (rule flags,
// logging, ...). The owner of the virtualserver hands it to its ClientPool
// (ClientPool.SetVModelTransport) so each server instance dials its own
// registry.
func NewTransport(d Dialer) *http.Transport {
	return &http.Transport{
		DialContext:         d,
		MaxIdleConns:        100,
		MaxIdleConnsPerHost: 100,
	}
}
