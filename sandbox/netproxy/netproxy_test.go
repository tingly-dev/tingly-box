package netproxy

import (
	"context"
	"io"
	"net"
	"net/http"
	"net/http/httptest"
	"net/url"
	"path/filepath"
	"strings"
	"testing"
)

func TestAllowed(t *testing.T) {
	allow := []string{"api.anthropic.com", "*.github.com", "127.0.0.1:8080", "[::1]:9000"}
	for _, tc := range []struct {
		host string
		port int
		want bool
	}{
		{"api.anthropic.com", 443, true},
		{"API.Anthropic.com.", 80, true},
		{"api.anthropic.com", 22, false}, // no port named: 80 and 443 only
		{"evil-api.anthropic.com", 443, false},
		{"codeload.github.com", 443, true},
		{"github.com", 443, false}, // "*." is subdomains only
		{"notgithub.com", 443, false},
		{"127.0.0.1", 8080, true},
		{"127.0.0.1", 443, false},
		{"::1", 9000, true},
	} {
		if got := Allowed(allow, tc.host, tc.port); got != tc.want {
			t.Errorf("Allowed(%s:%d) = %v, want %v", tc.host, tc.port, got, tc.want)
		}
	}
}

// A plain http:// request through the proxy reaches an allowed host and is
// refused for any other.
func TestProxyForwardsPlainRequestsToAllowedHostsOnly(t *testing.T) {
	up := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) { _, _ = io.WriteString(w, "upstream") }))
	defer up.Close()
	u, _ := url.Parse(up.URL)
	p, err := Listen([]string{u.Host})
	if err != nil {
		t.Fatal(err)
	}
	defer p.Close()
	client := &http.Client{Transport: &http.Transport{
		Proxy: http.ProxyURL(&url.URL{Scheme: "http", Host: "proxy"}),
		DialContext: func(ctx context.Context, _, _ string) (net.Conn, error) {
			return (&net.Dialer{}).DialContext(ctx, "unix", filepath.Join(p.Dir(), SocketName))
		},
	}}
	get := func(target string) (int, string) {
		resp, err := client.Get(target)
		if err != nil {
			t.Fatal(err)
		}
		defer resp.Body.Close()
		b, _ := io.ReadAll(resp.Body)
		return resp.StatusCode, string(b)
	}
	if code, body := get(up.URL); code != 200 || body != "upstream" {
		t.Errorf("allowed: %d %q", code, body)
	}
	other := strings.Replace(up.URL, "127.0.0.1", "localhost", 1)
	if code, body := get(other); code != http.StatusForbidden || !strings.Contains(body, "allowlist") {
		t.Errorf("not allowed: %d %q", code, body)
	}
	_ = p.Close()
	if _, err := net.Dial("unix", filepath.Join(p.Dir(), SocketName)); err == nil {
		t.Error("the socket outlives Close")
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
