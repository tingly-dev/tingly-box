package client

import (
	"context"
	"net"
	"net/http"
	"sync"

	"github.com/sirupsen/logrus"

	"github.com/tingly-dev/tingly-box/ai"
	"github.com/tingly-dev/tingly-box/internal/typ"
)

// VModelDialer connects to the private virtualserver listener. The network
// and address arguments are ignored; there is only one possible peer.
type VModelDialer func(ctx context.Context, network, addr string) (net.Conn, error)

var (
	vmodelMu        sync.RWMutex
	vmodelDialer    VModelDialer
	vmodelTransport *http.Transport
)

// SetVModelDialer installs the dialer used for providers whose APIBase carries
// the vmodel:// scheme. Call it once at server start with
// virtualserver.Server.DialContext. Passing nil uninstalls it.
func SetVModelDialer(d VModelDialer) {
	vmodelMu.Lock()
	defer vmodelMu.Unlock()
	if vmodelTransport != nil {
		vmodelTransport.CloseIdleConnections()
		vmodelTransport = nil
	}
	vmodelDialer = d
	if d == nil {
		return
	}
	vmodelTransport = &http.Transport{
		DialContext: d,
		// Everything else is the default transport's behaviour; proxies never
		// apply because the dialer does not go through the network.
		MaxIdleConns:        100,
		MaxIdleConnsPerHost: 100,
		DisableCompression:  true,
	}
}

// providerBaseTransport returns the base *http.Transport for provider: the
// vmodel transport when the provider's APIBase is vmodel://, otherwise the
// pooled session-bound transport every real provider uses.
func providerBaseTransport(provider *typ.Provider, model string, sessionID typ.SessionID) http.RoundTripper {
	if ai.IsVModelAPIBase(provider.APIBase) {
		vmodelMu.RLock()
		t := vmodelTransport
		vmodelMu.RUnlock()
		if t != nil {
			return t
		}
		logrus.Warnf("vmodel provider %q used before SetVModelDialer; requests will fail to dial %s", provider.Name, ai.VModelHost)
		return vmodelUnavailableTransport{}
	}
	return GetGlobalTransportPool().GetTransport(provider.UUID, model, provider.ProxyURL, ai.Issuer(""), sessionID)
}

// providerBaseURL returns the base URL to hand to the SDK: the vmodel:// APIBase
// rewritten to its http form, or the APIBase unchanged for everything else.
func providerBaseURL(provider *typ.Provider) string {
	if ai.IsVModelAPIBase(provider.APIBase) {
		return ai.VModelHTTPBase(provider.APIBase, provider.APIStyle)
	}
	return provider.APIBase
}

// vmodelUnavailableTransport fails every request with a clear error instead of
// leaking a DNS lookup for the placeholder host.
type vmodelUnavailableTransport struct{}

func (vmodelUnavailableTransport) RoundTrip(req *http.Request) (*http.Response, error) {
	return nil, &net.OpError{Op: "dial", Net: "vmodel", Err: errVModelNotServed}
}

var errVModelNotServed = &vmodelError{"virtual-model server is not running in this process"}

type vmodelError struct{ msg string }

func (e *vmodelError) Error() string { return e.msg }
