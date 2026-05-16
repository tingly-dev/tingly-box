package server

import (
	"testing"

	"github.com/tingly-dev/tingly-box/ai"
	"github.com/tingly-dev/tingly-box/internal/typ"
)

func TestParseEndpointOverride(t *testing.T) {
	cases := []struct {
		in   string
		want EndpointOverride
	}{
		{"", OverrideAuto},
		{"auto", OverrideAuto},
		{"chat", OverrideChat},
		{"responses", OverrideResponses},
		{"unknown", OverrideAuto},
		{"CHAT", OverrideAuto}, // case-sensitive on purpose; the registry emits lowercase
	}
	for _, c := range cases {
		got := ParseEndpointOverride(c.in)
		if got != c.want {
			t.Errorf("ParseEndpointOverride(%q) = %q, want %q", c.in, got, c.want)
		}
	}
}

func TestIsCodexProvider(t *testing.T) {
	if isCodexProvider(nil) {
		t.Error("nil provider should not be Codex")
	}
	if isCodexProvider(&typ.Provider{UUID: "p-1"}) {
		t.Error("provider without OAuthDetail should not be Codex")
	}
	codex := &typ.Provider{
		UUID:        "codex-1",
		OAuthDetail: &typ.OAuthDetail{Issuer: ai.IssuerCodex},
	}
	if !isCodexProvider(codex) {
		t.Error("Codex-issuer OAuth provider should be Codex")
	}
}
