package client

import (
	"strings"
	"testing"
)

func TestRedactProxy(t *testing.T) {
	cases := []struct {
		raw  string
		want string
	}{
		{"", "direct"},
		{ProxyURLNone, "direct"},
		{"http://proxy.example.com:8080", "http://proxy.example.com:8080"},
		{"socks5://10.0.0.1:1080", "socks5://10.0.0.1:1080"},
		{"http://user:secret@proxy.example.com:8080", "http://proxy.example.com:8080"},
		{"://bad", "proxy(set)"},
	}
	for _, c := range cases {
		got := redactProxy(c.raw)
		if got != c.want {
			t.Errorf("redactProxy(%q) = %q, want %q", c.raw, got, c.want)
		}
		// Hard guarantee: credentials must never appear in the logged value.
		if strings.Contains(got, "secret") {
			t.Errorf("redactProxy(%q) leaked credentials: %q", c.raw, got)
		}
	}
}
