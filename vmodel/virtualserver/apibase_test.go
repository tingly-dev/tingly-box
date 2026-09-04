package virtualserver

import (
	"testing"

	"github.com/tingly-dev/tingly-box/internal/protocol"
)

func TestAPIBase(t *testing.T) {
	if got := APIBase(protocol.APIStyleOpenAI); got != "vmodel://openai" {
		t.Fatalf("got %q", got)
	}
	if !IsAPIBase("vmodel://anthropic") || IsAPIBase("https://api.openai.com/v1") || IsAPIBase("") {
		t.Fatal("IsAPIBase mismatch")
	}
}

func TestHTTPBase(t *testing.T) {
	cases := []struct{ base, style, want string }{
		{"vmodel://openai", "anthropic", "http://vmodel.internal/openai/v1"},
		{"vmodel://anthropic", "openai", "http://vmodel.internal/anthropic/v1"},
		// legacy sentinel and unknown host fall back to the provider style
		{"vmodel://local", "anthropic", "http://vmodel.internal/anthropic/v1"},
		{"vmodel://whatever", "openai", "http://vmodel.internal/openai/v1"},
		{"vmodel://local", "", "http://vmodel.internal/openai/v1"},
	}
	for _, c := range cases {
		if got := HTTPBase(c.base, protocol.APIStyle(c.style)); got != c.want {
			t.Errorf("HTTPBase(%q,%q) = %q, want %q", c.base, c.style, got, c.want)
		}
	}
}
