package netproxy

import (
	"bufio"
	"context"
	"io"
	"net"
	"net/http"
	"net/http/httptest"
	"net/netip"
	"net/url"
	"path/filepath"
	"strings"
	"testing"

	"github.com/tingly-dev/tingly-box/sandbox"
)

// publicIP stands for a public host; the test dialer sends it to a local
// server, so the grant logic runs on a real address without a network.
var publicIP = netip.MustParseAddr("93.184.216.34")

func fakeNet(t *testing.T, names map[string]netip.Addr, server string) {
	oldLookup, oldDial := lookup, dialOut
	lookup = func(_ context.Context, name string) ([]netip.Addr, error) {
		if ip, ok := names[name]; ok {
			return []netip.Addr{ip}, nil
		}
		return nil, &net.DNSError{Err: "no such host", Name: name, IsNotFound: true}
	}
	dialOut = func(ctx context.Context, network, addr string) (net.Conn, error) {
		if strings.HasPrefix(addr, publicIP.String()+":") {
			addr = server
		}
		return (&net.Dialer{}).DialContext(ctx, network, addr)
	}
	t.Cleanup(func() { lookup, dialOut = oldLookup, oldDial })
}

func listenTest(t *testing.T, n sandbox.Network) *Host {
	t.Helper()
	h, err := ListenDir(n)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = h.Close() })
	return h
}

func viaProxy(h *Host) *http.Client {
	return &http.Client{Transport: &http.Transport{
		Proxy: http.ProxyURL(&url.URL{Scheme: "http", Host: "proxy"}),
		DialContext: func(ctx context.Context, _, _ string) (net.Conn, error) {
			return (&net.Dialer{}).DialContext(ctx, "unix", filepath.Join(h.Dir(), ProxyName+".sock"))
		},
	}}
}

func connect(t *testing.T, h *Host, target string) string {
	t.Helper()
	c, err := net.Dial("unix", filepath.Join(h.Dir(), ProxyName+".sock"))
	if err != nil {
		t.Fatal(err)
	}
	defer c.Close()
	_, _ = io.WriteString(c, "CONNECT "+target+" HTTP/1.1\r\nHost: "+target+"\r\n\r\n")
	resp, err := http.ReadResponse(bufio.NewReader(c), nil)
	if err != nil {
		t.Fatal(err)
	}
	if resp.StatusCode == http.StatusOK {
		return resp.Status // a tunnel: nothing to read until we send
	}
	body, _ := io.ReadAll(io.LimitReader(resp.Body, 200))
	return resp.Status + " " + string(body)
}

func TestProxyGrantsWhatAllowGrantsAndNeverTheHost(t *testing.T) {
	up := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) { _, _ = io.WriteString(w, "upstream") }))
	defer up.Close()
	upURL, _ := url.Parse(up.URL)
	fakeNet(t, map[string]netip.Addr{
		"example.test":          publicIP,
		"api.example.test":      publicIP,
		"rebind.example.test":   netip.MustParseAddr("127.0.0.1"), // a granted name that resolves to the host
		"intranet.example.test": netip.MustParseAddr("10.0.0.9"),
	}, upURL.Host)
	h := listenTest(t, sandbox.Network{Level: sandbox.NetworkAllowlist, Allow: []string{"example.test", "*.example.test", "rebind.example.test"}})

	resp, err := viaProxy(h).Get("http://example.test/")
	if err != nil {
		t.Fatal(err)
	}
	b, _ := io.ReadAll(resp.Body)
	_ = resp.Body.Close()
	if resp.StatusCode != 200 || string(b) != "upstream" {
		t.Errorf("granted plain request: %d %q", resp.StatusCode, b)
	}
	if got := connect(t, h, "api.example.test:443"); !strings.HasPrefix(got, "200") {
		t.Errorf("granted CONNECT: %s", got)
	}
	for target, why := range map[string]string{
		"other.test:443":            "not in the network allowlist",
		"example.test:22":           "not in the network allowlist",
		"rebind.example.test:443":   "the host itself",
		"intranet.example.test:443": "not in the network allowlist", // a wildcard never grants private
		upURL.Host:                  "the host itself",              // the host's loopback, named by address
	} {
		if got := connect(t, h, target); !strings.HasPrefix(got, "403") || !strings.Contains(got, why) {
			t.Errorf("CONNECT %s = %s, want 403 %q", target, got, why)
		}
	}
	resp, err = viaProxy(h).Get(up.URL)
	if err != nil {
		t.Fatal(err)
	}
	_ = resp.Body.Close()
	if resp.StatusCode != http.StatusForbidden {
		t.Errorf("a plain request to the host's loopback: %d", resp.StatusCode)
	}
	_ = h.Close()
	if _, err := net.Dial("unix", filepath.Join(h.Dir(), ProxyName+".sock")); err == nil {
		t.Error("the socket outlives Close")
	}
}

func TestExposeForwardsToTheHostsLoopback(t *testing.T) {
	ln, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatal(err)
	}
	defer ln.Close()
	go func() {
		for {
			c, err := ln.Accept()
			if err != nil {
				return
			}
			_, _ = io.WriteString(c, "HELLO")
			_ = c.Close()
		}
	}()
	port := ln.Addr().(*net.TCPAddr).Port
	h := listenTest(t, sandbox.Network{Level: sandbox.NetworkNone, Expose: []sandbox.Endpoint{{Port: port}}})
	c, err := net.Dial("unix", filepath.Join(h.Dir(), ExposeName(port)+".sock"))
	if err != nil {
		t.Fatal(err)
	}
	b, _ := io.ReadAll(c)
	if string(b) != "HELLO" {
		t.Errorf("got %q", b)
	}
}

func TestPlanRoundTrips(t *testing.T) {
	pl := Plan{Proxy: true, Expose: []int{12580, 8080}}
	got, err := parsePlan(strings.Join(pl.Names(), ","))
	if err != nil || !got.Proxy || len(got.Expose) != 2 || got.Expose[1] != 8080 {
		t.Fatalf("%+v %v", got, err)
	}
	if _, err := parsePlan("nope"); err == nil {
		t.Error("an unknown endpoint is an error")
	}
}

func TestProxyEnvReplacesEveryProxyVariable(t *testing.T) {
	env := proxyEnv([]string{"PATH=/bin", "https_proxy=http://corp:3128", "ALL_PROXY=socks5://x", "No_Proxy=*"}, "http://127.0.0.1:1")
	got := strings.Join(env, " ")
	for _, gone := range []string{"corp", "socks5", "No_Proxy"} {
		if strings.Contains(got, gone) {
			t.Errorf("%s kept: %s", gone, got)
		}
	}
	for _, want := range []string{"PATH=/bin", "HTTPS_PROXY=http://127.0.0.1:1", "https_proxy=http://127.0.0.1:1", "NO_PROXY=localhost,127.0.0.1,::1"} {
		if !strings.Contains(got, want) {
			t.Errorf("missing %s: %s", want, got)
		}
	}
}
