package server

import (
	"testing"

	"github.com/tingly-dev/tingly-box/internal/data/db"
	"github.com/tingly-dev/tingly-box/internal/protocol"
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

func TestResolveOpenAIEndpoint_OverridesSkipProbe(t *testing.T) {
	// Constructing a Server with a nil adaptive probe is fine because the
	// override branches return before calling GetPreferredEndpointForModel.
	s := &Server{}
	provider := &typ.Provider{UUID: "p-1", APIBase: "https://api.openai.example"}

	if got := s.ResolveOpenAIEndpoint(provider, "gpt-4o", OverrideChat); got != string(db.EndpointTypeChat) {
		t.Errorf("override=chat: got %q want %q", got, db.EndpointTypeChat)
	}
	if got := s.ResolveOpenAIEndpoint(provider, "gpt-4o", OverrideResponses); got != string(db.EndpointTypeResponses) {
		t.Errorf("override=responses: got %q want %q", got, db.EndpointTypeResponses)
	}
}

func TestResolveOpenAIEndpoint_CodexAlwaysResponses(t *testing.T) {
	s := &Server{}
	codex := &typ.Provider{UUID: "codex-1", APIBase: protocol.CodexAPIBase}

	// Even an explicit "chat" override must be ignored for Codex.
	for _, override := range []EndpointOverride{OverrideAuto, OverrideChat, OverrideResponses} {
		got := s.ResolveOpenAIEndpoint(codex, "codex-mini", override)
		if got != string(db.EndpointTypeResponses) {
			t.Errorf("codex override=%q: got %q want %q", override, got, db.EndpointTypeResponses)
		}
	}
}
