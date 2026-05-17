package server

import (
	"testing"

	"github.com/tingly-dev/tingly-box/ai"
	"github.com/tingly-dev/tingly-box/internal/protocol"
	"github.com/tingly-dev/tingly-box/internal/typ"
)

func TestResolveOpenAIEndpoint(t *testing.T) {
	plain := &typ.Provider{UUID: "p-plain"}
	responsesOnly := &typ.Provider{UUID: "p-ronly", ResponsesOnly: true}

	tests := []struct {
		name     string
		provider *typ.Provider
		flags    typ.RuleFlags
		opts     OpenAIEndpointOptions
		want     protocol.APIType
		wantErr  bool
	}{
		{
			name:     "default mirrors chat",
			provider: plain,
			opts:     OpenAIEndpointOptions{Incoming: IncomingAPIChat},
			want:     protocol.TypeOpenAIChat,
		},
		{
			name:     "default mirrors responses",
			provider: plain,
			opts:     OpenAIEndpointOptions{Incoming: IncomingAPIResponses},
			want:     protocol.TypeOpenAIResponses,
		},
		{
			name:     "provider responses_only forces responses (chat in)",
			provider: responsesOnly,
			opts:     OpenAIEndpointOptions{Incoming: IncomingAPIChat},
			want:     protocol.TypeOpenAIResponses,
		},
		{
			name:     "provider responses_only forces responses (responses in)",
			provider: responsesOnly,
			opts:     OpenAIEndpointOptions{Incoming: IncomingAPIResponses},
			want:     protocol.TypeOpenAIResponses,
		},
		{
			name:     "rule override=chat on plain provider",
			provider: plain,
			flags:    typ.RuleFlags{OpenAIEndpointOverride: "chat"},
			opts:     OpenAIEndpointOptions{Incoming: IncomingAPIResponses},
			want:     protocol.TypeOpenAIChat,
		},
		{
			name:     "rule override=chat on responses_only ignored, stays responses",
			provider: responsesOnly,
			flags:    typ.RuleFlags{OpenAIEndpointOverride: "chat"},
			opts:     OpenAIEndpointOptions{Incoming: IncomingAPIChat},
			want:     protocol.TypeOpenAIResponses,
		},
		{
			name:     "rule override=responses",
			provider: plain,
			flags:    typ.RuleFlags{OpenAIEndpointOverride: "responses"},
			opts:     OpenAIEndpointOptions{Incoming: IncomingAPIChat},
			want:     protocol.TypeOpenAIResponses,
		},
		{
			name:     "rule override=chat with RequireResponses errors",
			provider: plain,
			flags:    typ.RuleFlags{OpenAIEndpointOverride: "chat"},
			opts:     OpenAIEndpointOptions{Incoming: IncomingAPIResponses, RequireResponses: true},
			wantErr:  true,
		},
		{
			name:     "auto flag treated as default",
			provider: plain,
			flags:    typ.RuleFlags{OpenAIEndpointOverride: "auto"},
			opts:     OpenAIEndpointOptions{Incoming: IncomingAPIChat},
			want:     protocol.TypeOpenAIChat,
		},
		{
			name:     "unknown flag value treated as default",
			provider: plain,
			flags:    typ.RuleFlags{OpenAIEndpointOverride: "bogus"},
			opts:     OpenAIEndpointOptions{Incoming: IncomingAPIChat},
			want:     protocol.TypeOpenAIChat,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got, err := ResolveOpenAIEndpoint(tt.provider, tt.flags, tt.opts)
			if tt.wantErr {
				if err == nil {
					t.Fatalf("expected error, got selection %+v", got)
				}
				return
			}
			if err != nil {
				t.Fatalf("unexpected error: %v", err)
			}
			if got.Target != tt.want {
				t.Errorf("Target = %v, want %v (reason: %s)", got.Target, tt.want, got.Reason)
			}
		})
	}
}

func TestResolveOpenAIEndpointNilProviderErrors(t *testing.T) {
	if _, err := ResolveOpenAIEndpoint(nil, typ.RuleFlags{}, OpenAIEndpointOptions{Incoming: IncomingAPIChat}); err == nil {
		t.Error("expected error for nil provider")
	}
}

// TestResolveOpenAIEndpointCodexOAuthSnapshot documents the design assumption
// that Codex providers carry ResponsesOnly=true by the time they reach
// routing. The OAuth handler is responsible for setting this on
// instantiation; this test pins the resolver behavior against such providers.
func TestResolveOpenAIEndpointCodexOAuthSnapshot(t *testing.T) {
	codex := &typ.Provider{
		UUID:          "codex-1",
		OAuthDetail:   &typ.OAuthDetail{Issuer: ai.IssuerCodex},
		ResponsesOnly: true,
	}
	got, err := ResolveOpenAIEndpoint(codex, typ.RuleFlags{}, OpenAIEndpointOptions{Incoming: IncomingAPIChat})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if got.Target != protocol.TypeOpenAIResponses {
		t.Errorf("Codex with ResponsesOnly should route to Responses, got %v", got.Target)
	}
}
