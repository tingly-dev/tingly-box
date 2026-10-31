package peertransport

import (
	"context"
	"crypto/tls"
	"crypto/x509"
	"errors"
	"io"
	"net"
	"net/http"
	"net/http/httptest"
	"net/url"
	"strings"
	"sync/atomic"
	"testing"
	"time"

	"github.com/quic-go/quic-go/http3"
)

func testPeer(t *testing.T, handler http.Handler, withQUIC bool) (string, *Transport, *http3.Server) {
	t.Helper()
	return testPeerWithMiddleware(t, handler, withQUIC, nil)
}

func testPeerWithMiddleware(t *testing.T, handler http.Handler, withQUIC bool, middleware func(http.Handler) http.Handler) (string, *Transport, *http3.Server) {
	t.Helper()
	s := httptest.NewUnstartedServer(nil)
	_, port, _ := net.SplitHostPort(s.Listener.Addr().String())
	s.Config.Handler = http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.TLS != nil {
			w.Header().Set("Alt-Svc", `h3=":`+port+`"; ma=300`)
		}
		ProbeHandler(handler).ServeHTTP(w, r)
	})
	if middleware != nil {
		s.Config.Handler = middleware(s.Config.Handler)
	}
	s.StartTLS()
	t.Cleanup(s.Close)
	roots := x509.NewCertPool()
	roots.AddCert(s.Certificate())
	tr := New(&http.Transport{TLSClientConfig: &tls.Config{RootCAs: roots}})
	t.Cleanup(func() { _ = tr.Close() })
	var h3 *http3.Server
	if withQUIC {
		addr, err := net.ResolveUDPAddr("udp", s.Listener.Addr().String())
		if err != nil {
			t.Fatal(err)
		}
		conn, err := net.ListenUDP("udp", addr)
		if err != nil {
			t.Fatal(err)
		}
		h3 = &http3.Server{Handler: s.Config.Handler, TLSConfig: s.TLS.Clone()}
		go func() { _ = h3.Serve(conn) }()
		t.Cleanup(func() { _ = h3.Close(); _ = conn.Close() })
	}
	return s.URL, tr, h3
}

func peerState(t *testing.T, tr *Transport, origin string, predicate func(*peer) bool) {
	t.Helper()
	deadline := time.Now().Add(3 * time.Second)
	for time.Now().Before(deadline) {
		tr.mu.Lock()
		p := tr.peers[origin]
		ok := p != nil && predicate(p)
		tr.mu.Unlock()
		if ok {
			return
		}
		time.Sleep(5 * time.Millisecond)
	}
	t.Fatal("peer did not reach expected state")
}

func doRequest(t *testing.T, c *http.Client, method, target string) *http.Response {
	t.Helper()
	req, _ := http.NewRequest(method, target, strings.NewReader(`{"synthetic":true}`))
	resp, err := c.Do(req)
	if err != nil {
		t.Fatal(err)
	}
	return resp
}

func TestAutomaticHTTP3(t *testing.T) {
	var tcp, quic atomic.Int32
	origin, tr, _ := testPeer(t, http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.ProtoMajor == 3 {
			quic.Add(1)
		} else {
			tcp.Add(1)
		}
		w.Header().Set("Content-Type", "text/event-stream")
		io.WriteString(w, "data: hello\n\n")
		w.(http.Flusher).Flush()
	}), true)
	c := &http.Client{Transport: tr}
	resp := doRequest(t, c, http.MethodPost, origin+"/generate")
	io.Copy(io.Discard, resp.Body)
	resp.Body.Close()
	if resp.ProtoMajor != 1 {
		t.Fatalf("first request: %s", resp.Proto)
	}
	peerState(t, tr, origin+"/generate", func(p *peer) bool { return p.ready })
	resp = doRequest(t, c, http.MethodPost, origin+"/generate")
	data, err := io.ReadAll(resp.Body)
	resp.Body.Close()
	if err != nil || string(data) != "data: hello\n\n" || resp.ProtoMajor != 3 {
		t.Fatalf("H3 response proto=%s body=%q err=%v", resp.Proto, data, err)
	}
	if tcp.Load() != 1 || quic.Load() != 1 {
		t.Fatalf("business counts TCP=%d H3=%d", tcp.Load(), quic.Load())
	}
	t.Logf("first request HTTP/1.1; probe succeeded; next POST %s; one business execution per request", resp.Proto)
}

func TestUnavailableQUICKeepsTCPWithoutRepeatedProbe(t *testing.T) {
	origin, tr, _ := testPeer(t, http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) { w.WriteHeader(http.StatusOK) }), false)
	tr.probeTimeout = 40 * time.Millisecond
	c := &http.Client{Transport: tr}
	resp := doRequest(t, c, http.MethodPost, origin)
	resp.Body.Close()
	peerState(t, tr, origin, func(p *peer) bool { return !p.probing && !p.retryAfter.IsZero() })
	tr.mu.Lock()
	retry := tr.peers[origin].retryAfter
	tr.mu.Unlock()
	for i := 0; i < 3; i++ {
		resp = doRequest(t, c, http.MethodPost, origin)
		resp.Body.Close()
		if resp.ProtoMajor != 1 {
			t.Fatal(resp.Proto)
		}
	}
	tr.mu.Lock()
	changed := !tr.peers[origin].retryAfter.Equal(retry) || tr.peers[origin].probing
	tr.mu.Unlock()
	if changed {
		t.Fatal("failed peer was probed during cooldown")
	}
}

func TestSubmittedPOSTIsNotReplayedOnTCP(t *testing.T) {
	var tcp, quic atomic.Int32
	entered := make(chan struct{})
	origin, tr, h3 := testPeer(t, http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.ProtoMajor != 3 {
			tcp.Add(1)
			w.WriteHeader(200)
			return
		}
		quic.Add(1)
		close(entered)
		<-r.Context().Done()
	}), true)
	c := &http.Client{Transport: tr}
	resp := doRequest(t, c, http.MethodPost, origin)
	resp.Body.Close()
	peerState(t, tr, origin, func(p *peer) bool { return p.ready })
	done := make(chan error, 1)
	go func() {
		req, _ := http.NewRequest(http.MethodPost, origin, strings.NewReader("body"))
		resp, err := c.Do(req)
		if resp != nil {
			resp.Body.Close()
		}
		done <- err
	}()
	select {
	case <-entered:
	case <-time.After(3 * time.Second):
		t.Fatal("H3 request not received")
	}
	_ = h3.Close()
	select {
	case err := <-done:
		if err == nil {
			t.Fatal("expected interrupted request error")
		}
	case <-time.After(3 * time.Second):
		t.Fatal("request did not fail")
	}
	if tcp.Load() != 1 || quic.Load() != 1 {
		t.Fatalf("POST replayed TCP=%d H3=%d", tcp.Load(), quic.Load())
	}
	resp = doRequest(t, c, http.MethodPost, origin)
	resp.Body.Close()
	if resp.ProtoMajor != 1 || tcp.Load() != 2 {
		t.Fatal("subsequent request did not return to TCP")
	}
}

func TestRetirementDoesNotInterruptActiveStream(t *testing.T) {
	origin, tr, _ := testPeer(t, http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		io.WriteString(w, "event\n")
		w.(http.Flusher).Flush()
		if r.ProtoMajor == 3 {
			<-r.Context().Done()
		}
	}), true)
	c := &http.Client{Transport: tr}
	resp := doRequest(t, c, http.MethodPost, origin)
	resp.Body.Close()
	peerState(t, tr, origin, func(p *peer) bool { return p.ready })
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	req, _ := http.NewRequestWithContext(ctx, http.MethodPost, origin, nil)
	resp, err := c.Do(req)
	if err != nil {
		t.Fatal(err)
	}
	if err = tr.Close(); err != nil {
		t.Fatal(err)
	}
	buf := make([]byte, 6)
	if _, err = io.ReadFull(resp.Body, buf); err != nil {
		t.Fatalf("retirement killed active stream: %v", err)
	}
	cancel()
	resp.Body.Close()
	tr.mu.Lock()
	active := tr.active
	tr.mu.Unlock()
	if active != 0 {
		t.Fatal("active stream reference leaked")
	}
}

func TestEligibilityAndAdvertisement(t *testing.T) {
	u, _ := url.Parse("https://tb.example:8443/v1")
	for _, value := range []string{`h3="other.example:8443"`, `h3=":443"`, `h3=":8443"; ma=0`, `h3=":8443"; ma=bad`, `clear`} {
		if _, ok := advertisement([]string{value}, u); ok {
			t.Fatalf("accepted %s", value)
		}
	}
	if age, ok := advertisement([]string{`h3=":8443"; ma=99999`}, u); !ok || age != 300*time.Second {
		t.Fatalf("lifetime %s %v", age, ok)
	}
	proxy, _ := url.Parse("http://proxy.example:8080")
	tr := New(&http.Transport{Proxy: http.ProxyURL(proxy)})
	defer tr.Close()
	req, _ := http.NewRequest(http.MethodPost, u.String(), nil)
	if tr.eligible(req) {
		t.Fatal("proxy was bypassed")
	}
	tr.base.Proxy = nil
	tr.base.TLSClientConfig = &tls.Config{InsecureSkipVerify: true}
	if tr.eligible(req) {
		t.Fatal("insecure TLS discovered a peer")
	}
}

func TestStreamInterruptionReturnsSubsequentRequestsToTCP(t *testing.T) {
	var tcp, quic atomic.Int32
	origin, tr, h3 := testPeer(t, http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.ProtoMajor != 3 {
			tcp.Add(1)
			w.WriteHeader(200)
			return
		}
		quic.Add(1)
		io.WriteString(w, "data: partial\n\n")
		w.(http.Flusher).Flush()
		<-r.Context().Done()
	}), true)
	c := &http.Client{Transport: tr}
	resp := doRequest(t, c, http.MethodPost, origin)
	resp.Body.Close()
	peerState(t, tr, origin, func(p *peer) bool { return p.ready })
	resp = doRequest(t, c, http.MethodPost, origin)
	buf := make([]byte, len("data: partial\n\n"))
	if _, err := io.ReadFull(resp.Body, buf); err != nil {
		t.Fatal(err)
	}
	_ = h3.Close()
	if _, err := io.ReadAll(resp.Body); err == nil {
		t.Fatal("expected interrupted stream error")
	}
	resp.Body.Close()
	resp = doRequest(t, c, http.MethodPost, origin)
	resp.Body.Close()
	if resp.ProtoMajor != 1 || tcp.Load() != 2 || quic.Load() != 1 {
		t.Fatalf("stream failure did not cool down: %s TCP=%d H3=%d", resp.Proto, tcp.Load(), quic.Load())
	}
}

func TestHTTP3DeadlineFailureCoolsPeerWithoutReplay(t *testing.T) {
	var tcp, quic atomic.Int32
	origin, tr, _ := testPeer(t, http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.ProtoMajor == 3 {
			quic.Add(1)
			<-r.Context().Done()
			return
		}
		tcp.Add(1)
		w.WriteHeader(http.StatusOK)
	}), true)
	client := &http.Client{Transport: tr}
	resp := doRequest(t, client, http.MethodPost, origin)
	resp.Body.Close()
	peerState(t, tr, origin, func(p *peer) bool { return p.ready })
	ctx, cancel := context.WithTimeout(context.Background(), 100*time.Millisecond)
	defer cancel()
	req, _ := http.NewRequestWithContext(ctx, http.MethodPost, origin, strings.NewReader("synthetic"))
	resp, err := client.Do(req)
	if resp != nil {
		resp.Body.Close()
	}
	if !errors.Is(err, context.DeadlineExceeded) {
		t.Fatalf("expected deadline failure, got %v", err)
	}
	if tcp.Load() != 1 || quic.Load() != 1 {
		t.Fatalf("timed-out POST replayed: TCP=%d H3=%d", tcp.Load(), quic.Load())
	}
	resp = doRequest(t, client, http.MethodPost, origin)
	resp.Body.Close()
	if resp.ProtoMajor != 1 || tcp.Load() != 2 || quic.Load() != 1 {
		t.Fatalf("deadline did not trigger TCP fallback: %s TCP=%d H3=%d", resp.Proto, tcp.Load(), quic.Load())
	}
}

func TestProbePreservesPathAndIsolatesRoutes(t *testing.T) {
	var probes atomic.Int32
	middleware := func(next http.Handler) http.Handler {
		return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			if !strings.HasPrefix(r.URL.Path, "/prefix/") {
				http.NotFound(w, r)
				return
			}
			if r.Method == http.MethodHead && r.Header.Get(ProbeHeader) == Capability && strings.HasPrefix(r.URL.Path, "/prefix/tingly/") {
				if r.URL.Path != "/prefix/tingly/model" || r.URL.RawQuery != "route=one" || r.Header.Get("Authorization") != "Bearer synthetic" {
					t.Errorf("probe lost routing or auth: %s", r.URL.String())
				}
				probes.Add(1)
			}
			r.URL.Path = strings.TrimPrefix(r.URL.Path, "/prefix")
			next.ServeHTTP(w, r)
		})
	}
	origin, tr, _ := testPeerWithMiddleware(t, http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) { w.WriteHeader(http.StatusOK) }), true, middleware)
	c := &http.Client{Transport: tr}
	target := origin + "/prefix/tingly/model?route=one"
	request := func(target string) *http.Response {
		req, _ := http.NewRequest(http.MethodPost, target, nil)
		req.Header.Set("Authorization", "Bearer synthetic")
		res, err := c.Do(req)
		if err != nil {
			t.Fatal(err)
		}
		return res
	}
	res := request(target)
	res.Body.Close()
	peerState(t, tr, target, func(p *peer) bool { return p.ready })
	res = request(target)
	res.Body.Close()
	if res.ProtoMajor != 3 || probes.Load() != 1 {
		t.Fatalf("prefixed route used %s, probes %d", res.Proto, probes.Load())
	}
	other := origin + "/prefix/other/model"
	res = request(other)
	res.Body.Close()
	if res.ProtoMajor == 3 {
		t.Fatal("discovery leaked between routes")
	}
}
