package server

import (
	"testing"

	"github.com/tingly-dev/tingly-box/ai"
	"github.com/tingly-dev/tingly-box/internal/protocol"
	"github.com/tingly-dev/tingly-box/internal/typ"
)

func TestResolveOpenAIEndpoint(t *testing.T) {
	chatOnly := &typ.Provider{UUID: "p-chat", OpenAIEndpointMode: ai.EndpointModeChat}
	autoDefault := &typ.Provider{UUID: "p-default"} // zero value = auto
	responsesOnly := &typ.Provider{UUID: "p-resp", OpenAIEndpointMode: ai.EndpointModeResponses}
	both := &typ.Provider{UUID: "p-both", OpenAIEndpointMode: ai.EndpointModeBoth}

	tests := []struct {
		name     string
		provider *typ.Provider
		flags    typ.RuleFlags
		incoming IncomingAPIType
		want     protocol.APIType
	}{
		// Explicit chat mode — forces chat regardless of incoming
		{
			name:     "chat mode forces chat (chat incoming)",
			provider: chatOnly,
			incoming: IncomingAPIChat,
			want:     protocol.TypeOpenAIChat,
		},
		{
			name:     "chat mode forces chat (responses incoming, downgrade)",
			provider: chatOnly,
			incoming: IncomingAPIResponses,
			want:     protocol.TypeOpenAIChat,
		},

		// Default (zero value) = auto — mirrors incoming
		{
			name:     "default mode mirrors chat",
			provider: autoDefault,
			incoming: IncomingAPIChat,
			want:     protocol.TypeOpenAIChat,
		},
		{
			name:     "default mode mirrors responses",
			provider: autoDefault,
			incoming: IncomingAPIResponses,
			want:     protocol.TypeOpenAIResponses,
		},

		// Responses-only (Codex)
		{
			name:     "responses mode forces responses (chat incoming)",
			provider: responsesOnly,
			incoming: IncomingAPIChat,
			want:     protocol.TypeOpenAIResponses,
		},
		{
			name:     "responses mode forces responses (responses incoming)",
			provider: responsesOnly,
			incoming: IncomingAPIResponses,
			want:     protocol.TypeOpenAIResponses,
		},

		// Both (OpenAI proper) — mirror
		{
			name:     "both mode mirrors chat",
			provider: both,
			incoming: IncomingAPIChat,
			want:     protocol.TypeOpenAIChat,
		},
		{
			name:     "both mode mirrors responses",
			provider: both,
			incoming: IncomingAPIResponses,
			want:     protocol.TypeOpenAIResponses,
		},

		// Rule overrides (override takes priority over provider mode)
		{
			name:     "override=responses on default chat-mode provider forces responses",
			provider: chatOnly,
			flags:    typ.RuleFlags{OpenAIEndpointOverride: "responses"},
			incoming: IncomingAPIChat,
			want:     protocol.TypeOpenAIResponses,
		},
		{
			name:     "override=chat on responses-only provider forces chat",
			provider: responsesOnly,
			flags:    typ.RuleFlags{OpenAIEndpointOverride: "chat"},
			incoming: IncomingAPIChat,
			want:     protocol.TypeOpenAIChat,
		},
		{
			name:     "override=chat on both-mode provider forces chat",
			provider: both,
			flags:    typ.RuleFlags{OpenAIEndpointOverride: "chat"},
			incoming: IncomingAPIResponses,
			want:     protocol.TypeOpenAIChat,
		},
		{
			name:     "override=responses on both-mode provider forces responses",
			provider: both,
			flags:    typ.RuleFlags{OpenAIEndpointOverride: "responses"},
			incoming: IncomingAPIChat,
			want:     protocol.TypeOpenAIResponses,
		},

		// Auto / unknown override values
		{
			name:     "auto flag treated as no override",
			provider: chatOnly,
			flags:    typ.RuleFlags{OpenAIEndpointOverride: "auto"},
			incoming: IncomingAPIResponses,
			want:     protocol.TypeOpenAIChat,
		},
		{
			name:     "unknown flag value treated as no override",
			provider: both,
			flags:    typ.RuleFlags{OpenAIEndpointOverride: "bogus"},
			incoming: IncomingAPIResponses,
			want:     protocol.TypeOpenAIResponses,
		},

		// Explicit auto mode — same as zero value
		{
			name:     "explicit auto mode mirrors chat",
			provider: &typ.Provider{UUID: "p-auto", OpenAIEndpointMode: ai.EndpointModeAuto},
			incoming: IncomingAPIChat,
			want:     protocol.TypeOpenAIChat,
		},
		{
			name:     "explicit auto mode mirrors responses",
			provider: &typ.Provider{UUID: "p-auto", OpenAIEndpointMode: ai.EndpointModeAuto},
			incoming: IncomingAPIResponses,
			want:     protocol.TypeOpenAIResponses,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got, err := ResolveOpenAIEndpoint(tt.provider, tt.flags, tt.incoming)
			if err != nil {
				t.Fatalf("unexpected error: %v", err)
			}
			if got != tt.want {
				t.Errorf("got %v, want %v", got, tt.want)
			}
		})
	}
}

func TestResolveOpenAIEndpointNilProviderErrors(t *testing.T) {
	if _, err := ResolveOpenAIEndpoint(nil, typ.RuleFlags{}, IncomingAPIChat); err == nil {
		t.Error("expected error for nil provider")
	}
}

// TestResolveOpenAIEndpointCodexOAuthSnapshot documents the design assumption
// that Codex providers carry OpenAIEndpointMode=responses by the time they
// reach routing. The OAuth handler is responsible for setting this on
// instantiation; this test pins the resolver behavior against such providers.
func TestResolveOpenAIEndpointCodexOAuthSnapshot(t *testing.T) {
	codex := &typ.Provider{
		UUID:               "codex-1",
		OAuthDetail:        &typ.OAuthDetail{Issuer: ai.IssuerCodex},
		OpenAIEndpointMode: ai.EndpointModeResponses,
	}
	got, err := ResolveOpenAIEndpoint(codex, typ.RuleFlags{}, IncomingAPIChat)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if got != protocol.TypeOpenAIResponses {
		t.Errorf("Codex with EndpointModeResponses should route to Responses, got %v", got)
	}
}
