package client

import (
	"net/http"
	"sync/atomic"

	"github.com/tingly-dev/tingly-box/internal/typ"
)

// ClaudeResponseObserver receives the response headers of a Claude OAuth
// upstream call that carries the subscription rate-limit state
// (anthropic-ratelimit-unified-*). It runs on the request path, so it must
// return quickly; the header map is a private copy.
type ClaudeResponseObserver func(provider *typ.Provider, header http.Header)

var claudeResponseObserver atomic.Pointer[ClaudeResponseObserver]

// SetClaudeResponseObserver installs (or, with nil, removes) the observer.
func SetClaudeResponseObserver(fn ClaudeResponseObserver) {
	if fn == nil {
		claudeResponseObserver.Store(nil)
		return
	}
	claudeResponseObserver.Store(&fn)
}

// claudeObservingTransport reports the rate-limit headers of every response.
type claudeObservingTransport struct {
	next     http.RoundTripper
	provider *typ.Provider
}

func (t *claudeObservingTransport) RoundTrip(req *http.Request) (*http.Response, error) {
	resp, err := t.next.RoundTrip(req)
	if err != nil || resp == nil {
		return resp, err
	}
	if obs := claudeResponseObserver.Load(); obs != nil && resp.Header.Get("anthropic-ratelimit-unified-status") != "" {
		(*obs)(t.provider, resp.Header.Clone())
	}
	return resp, nil
}
