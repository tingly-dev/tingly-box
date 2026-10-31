// Package peertransport adds opportunistic HTTP/3 between TB peers without
// changing their HTTP API or replaying business requests across transports.
package peertransport

import (
	"context"
	"crypto/tls"
	"errors"
	"io"
	"net/http"
	"net/url"
	"strconv"
	"strings"
	"sync"
	"time"

	"github.com/quic-go/quic-go"
	"github.com/quic-go/quic-go/http3"
)

const ProbeHeader = "Tingly-Transport-Probe"
const CapabilityHeader = "Tingly-Transport"

// Capability identifies a TB peer that supports this probe, not a direct HTTP/3 server.
const Capability = "tb-peer-v1"

type peer struct {
	ready      bool
	probing    bool
	expires    time.Time
	retryAfter time.Time
}

// Transport retains the caller's TCP transport, proxy and TLS policy. Only
// same-origin, same-port alternatives advertised over verified HTTPS are used.
// A failed business request is never replayed over TCP by this wrapper.
type Transport struct {
	base         *http.Transport
	h3           *http3.Transport
	mu           sync.Mutex
	peers        map[string]*peer
	ctx          context.Context
	cancel       context.CancelFunc
	closed       bool
	active       int
	probeTimeout time.Duration
	backoff      time.Duration
}

func New(base *http.Transport) *Transport {
	var conf *tls.Config
	if base.TLSClientConfig != nil {
		conf = base.TLSClientConfig.Clone()
	}
	ctx, cancel := context.WithCancel(context.Background())
	return &Transport{base: base, h3: &http3.Transport{TLSClientConfig: conf, QUICConfig: &quic.Config{MaxIdleTimeout: 120 * time.Second, KeepAlivePeriod: 15 * time.Second}}, peers: make(map[string]*peer), ctx: ctx, cancel: cancel, probeTimeout: 2 * time.Second, backoff: time.Minute}
}

func (t *Transport) eligible(req *http.Request) bool {
	if req.URL.Scheme != "https" || req.URL.User != nil {
		return false
	}
	if t.base.TLSClientConfig != nil && t.base.TLSClientConfig.InsecureSkipVerify {
		return false
	}
	if t.base.DialTLS != nil || t.base.DialTLSContext != nil {
		return false
	}
	if t.base.Proxy != nil {
		p, err := t.base.Proxy(req)
		if err != nil || p != nil {
			return false
		}
	}
	return true
}

func (t *Transport) RoundTrip(req *http.Request) (*http.Response, error) {
	if !t.eligible(req) {
		return t.base.RoundTrip(req)
	}
	key := req.URL.String()
	now := time.Now()
	t.mu.Lock()
	p := t.peers[key]
	ready := !t.closed && p != nil && p.ready && now.Before(p.expires)
	if ready {
		t.active++
	}
	t.mu.Unlock()
	if ready {
		resp, err := t.h3.RoundTrip(req)
		failed := func() {
			// An explicit cancellation says nothing about peer reachability.
			// A deadline can be how an unreachable QUIC path finally fails;
			// it must cool down so the next request can use the TCP path.
			if errors.Is(req.Context().Err(), context.Canceled) {
				return
			}
			t.mu.Lock()
			p.ready = false
			p.retryAfter = time.Now().Add(t.backoff)
			t.mu.Unlock()
		}
		if err != nil {
			failed()
		}
		if err != nil || resp == nil || resp.Body == nil {
			t.release()
		} else {
			resp.Body = &trackedBody{ReadCloser: resp.Body, release: t.release, failed: failed}
		}
		return resp, err
	}
	resp, err := t.base.RoundTrip(req)
	if err == nil && resp.TLS != nil && resp.TLS.HandshakeComplete && len(resp.TLS.VerifiedChains) > 0 {
		if lifetime, ok := advertisement(resp.Header.Values("Alt-Svc"), req.URL); ok {
			t.discover(key, req, lifetime)
		}
	}
	return resp, err
}

// Support the narrow same-origin form produced by TB; other alternatives are
// ignored, rather than rewriting URLs or weakening the SDK's origin guard.
func advertisement(values []string, u *url.URL) (time.Duration, bool) {
	port := u.Port()
	if port == "" {
		port = "443"
	}
	for _, v := range values {
		for _, item := range strings.Split(v, ",") {
			parts := strings.Split(strings.TrimSpace(item), ";")
			if strings.TrimSpace(parts[0]) != "h3=\":"+port+"\"" {
				continue
			}
			age := 300
			for _, part := range parts[1:] {
				key, val, ok := strings.Cut(strings.TrimSpace(part), "=")
				if ok && key == "ma" {
					n, err := strconv.Atoi(val)
					if err != nil || n <= 0 {
						return 0, false
					}
					age = min(n, 300)
				}
			}
			return time.Duration(age) * time.Second, true
		}
	}
	return 0, false
}

func (t *Transport) discover(key string, original *http.Request, lifetime time.Duration) {
	t.mu.Lock()
	p := t.peers[key]
	if p == nil {
		if len(t.peers) >= 64 {
			t.mu.Unlock()
			return
		}
		p = &peer{}
		t.peers[key] = p
	}
	if t.closed || p.probing || time.Now().Before(p.retryAfter) {
		t.mu.Unlock()
		return
	}
	p.probing = true
	t.mu.Unlock()
	probeURL := *original.URL
	probeHost := original.Host
	probeHeaders := original.Header.Clone()
	probeHeaders.Del("Content-Length")
	probeHeaders.Del("Transfer-Encoding")
	probeHeaders.Set(ProbeHeader, Capability)
	go func() {
		ctx, cancel := context.WithTimeout(t.ctx, t.probeTimeout)
		defer cancel()
		req, _ := http.NewRequestWithContext(ctx, http.MethodHead, probeURL.String(), nil)
		req.Header = probeHeaders
		req.Host = probeHost
		resp, err := t.h3.RoundTrip(req)
		ok := err == nil && resp.StatusCode == http.StatusNoContent && resp.Header.Get(CapabilityHeader) == Capability
		if resp != nil && resp.Body != nil {
			io.Copy(io.Discard, io.LimitReader(resp.Body, 1024))
			resp.Body.Close()
		}
		t.mu.Lock()
		defer t.mu.Unlock()
		p.probing = false
		if t.closed {
			return
		}
		p.ready = ok
		if ok {
			p.expires = time.Now().Add(lifetime)
		} else {
			p.retryAfter = time.Now().Add(t.backoff)
		}
	}()
}

func (t *Transport) CloseIdleConnections() {
	t.base.CloseIdleConnections()
	t.mu.Lock()
	defer t.mu.Unlock()
	if t.active != 0 {
		return
	}
	for _, p := range t.peers {
		if p.probing {
			return
		}
	}
	t.h3.CloseIdleConnections()
}

func (t *Transport) Close() error {
	t.mu.Lock()
	t.closed = true
	t.cancel()
	idle := t.active == 0
	t.mu.Unlock()
	t.base.CloseIdleConnections()
	if idle {
		return t.h3.Close()
	}
	return nil
}

func (t *Transport) release() {
	t.mu.Lock()
	t.active--
	closeNow := t.closed && t.active == 0
	t.mu.Unlock()
	if closeNow {
		_ = t.h3.Close()
	}
}

type trackedBody struct {
	io.ReadCloser
	once        sync.Once
	failureOnce sync.Once
	release     func()
	failed      func()
}

func (b *trackedBody) Read(p []byte) (int, error) {
	n, err := b.ReadCloser.Read(p)
	if err != nil && !errors.Is(err, io.EOF) {
		b.failureOnce.Do(b.failed)
	}
	return n, err
}

func (b *trackedBody) Close() error { err := b.ReadCloser.Close(); b.once.Do(b.release); return err }
