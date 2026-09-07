package client

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/openai/openai-go/v3"
	"github.com/tingly-dev/tingly-box/ai"
	"github.com/tingly-dev/tingly-box/internal/typ"
)

func openCodeProvider() *typ.Provider {
	return &typ.Provider{
		UUID:     "opencode-1",
		AuthType: ai.AuthTypeAPIKey,
		Token:    "sk-x",
		APIBase:  "https://opencode.ai/zen/go/v1",
	}
}

func sessionCtx(value string) context.Context {
	return typ.WithSessionID(context.Background(), typ.SessionID{
		Source: typ.SessionSourceHeader,
		Value:  value,
	})
}

func TestIsOpenCodeZen(t *testing.T) {
	tests := []struct {
		apiBase string
		want    bool
	}{
		{"https://opencode.ai/zen/go/v1", true},
		{"https://opencode.ai/zen/v1", true},
		{"https://opencode.ai/zen/go", true}, // Anthropic-style base
		{"opencode.ai/zen/go/v1", true},      // stored without a scheme
		{"https://opencode.ai/v1", false},
		{"https://api.openai.com/v1", false},
		// A relay that merely mentions the host in its path is not OpenCode.
		{"https://gateway.example.com/relay/opencode.ai/zen/v1", false},
		{"", false},
	}
	for _, tt := range tests {
		if got := IsOpenCodeZen(tt.apiBase); got != tt.want {
			t.Errorf("IsOpenCodeZen(%q) = %v, want %v", tt.apiBase, got, tt.want)
		}
	}
}

func TestNewOpenCodeClient_RejectsForeignProvider(t *testing.T) {
	provider := openCodeProvider()
	provider.APIBase = "https://api.openai.com/v1"

	if _, err := NewOpenCodeClient(provider, "gpt-5", typ.SessionID{}); err == nil {
		t.Fatal("expected an error for a non-OpenCode provider")
	}
	if _, err := NewOpenCodeAnthropicClient(provider, "claude-sonnet-5", typ.SessionID{}); err == nil {
		t.Fatal("expected an error for a non-OpenCode provider (Anthropic shape)")
	}
}

// ── the vendor layer ────────────────────────────────────────────────────────

func TestOpenCodeRoundTripper_StampsHeaderFromContextSession(t *testing.T) {
	cap := &captureTransport{}
	wrapped := &openCodeRoundTripper{RoundTripper: cap}

	req := newReq(t, sessionCtx("conversation-a"), "")
	if _, err := wrapped.RoundTrip(req); err != nil {
		t.Fatalf("RoundTrip: %v", err)
	}

	got := cap.lastReq.Header.Get(OpenCodeSessionHeader)
	if got == "" {
		t.Fatal("no x-opencode-session header on the outbound request")
	}
	if got == "conversation-a" {
		t.Error("session value forwarded verbatim; it must be hashed so IPs and user ids never reach the upstream")
	}
	// Clone-before-mutate: the caller's request stays untouched.
	if req.Header.Get(OpenCodeSessionHeader) != "" {
		t.Error("original request was mutated")
	}
}

func TestOpenCodeRoundTripper_StableAcrossRequestsOfOneConversation(t *testing.T) {
	cap := &captureTransport{}
	wrapped := &openCodeRoundTripper{RoundTripper: cap}

	values := make([]string, 0, 2)
	for range 2 {
		if _, err := wrapped.RoundTrip(newReq(t, sessionCtx("conversation-a"), "")); err != nil {
			t.Fatalf("RoundTrip: %v", err)
		}
		values = append(values, cap.lastReq.Header.Get(OpenCodeSessionHeader))
	}
	if values[0] != values[1] {
		t.Errorf("session header changed between requests of one conversation: %q vs %q", values[0], values[1])
	}

	if _, err := wrapped.RoundTrip(newReq(t, sessionCtx("conversation-b"), "")); err != nil {
		t.Fatalf("RoundTrip: %v", err)
	}
	if other := cap.lastReq.Header.Get(OpenCodeSessionHeader); other == values[0] {
		t.Error("two conversations collapsed onto one session value")
	}
}

func TestOpenCodeRoundTripper_MintsValueWhenNoSessionResolved(t *testing.T) {
	cap := &captureTransport{}
	wrapped := &openCodeRoundTripper{RoundTripper: cap}

	if _, err := wrapped.RoundTrip(newReq(t, context.Background(), "")); err != nil {
		t.Fatalf("RoundTrip: %v", err)
	}
	first := cap.lastReq.Header.Get(OpenCodeSessionHeader)
	if first == "" {
		t.Fatal("session-less request went out without the header; the upstream would 400")
	}

	if _, err := wrapped.RoundTrip(newReq(t, context.Background(), "")); err != nil {
		t.Fatalf("RoundTrip: %v", err)
	}
	if second := cap.lastReq.Header.Get(OpenCodeSessionHeader); second == first {
		t.Error("session-less requests share one value; unrelated conversations must not collapse onto one affinity scope")
	}
}

// TestOpenCodeClient_RuleExtraHeaderWins proves the vendor layer sits *below*
// the rule-flag layer: OpenCode providers are api_key providers, so a session
// pinned through the rule's extra_headers is still decisive.
func TestOpenCodeClient_RuleExtraHeaderWins(t *testing.T) {
	cap := &captureTransport{}
	provider := openCodeProvider()
	chain := providerTransportChain(&openCodeRoundTripper{RoundTripper: cap}, provider)

	ctx := typ.WithRuleFlags(sessionCtx("conversation-a"), typ.RuleFlags{
		ExtraHeaders: map[string]string{OpenCodeSessionHeader: "pinned-by-rule"},
	})
	if _, err := chain.RoundTrip(newReq(t, ctx, "")); err != nil {
		t.Fatalf("RoundTrip: %v", err)
	}
	if got := cap.lastReq.Header.Get(OpenCodeSessionHeader); got != "pinned-by-rule" {
		t.Errorf("session header = %q, want the rule-configured value", got)
	}
}

// TestOpenCodeClient_ChatCompletionSucceeds reproduces #1713 against a
// stand-in for OpenCode Zen: the upstream rejects any request without
// x-opencode-session exactly as the real one does, so this fails with the
// reported 400 MissingSessionID unless the client supplies the header.
func TestOpenCodeClient_ChatCompletionSucceeds(t *testing.T) {
	var gotSession string
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		gotSession = r.Header.Get(OpenCodeSessionHeader)
		if gotSession == "" {
			w.WriteHeader(http.StatusBadRequest)
			json.NewEncoder(w).Encode(map[string]any{
				"type":    "MissingSessionID",
				"message": "Error from provider (Console Go): Request is missing x-opencode-session and cannot be routed efficiently.",
			})
			return
		}
		w.Header().Set("Content-Type", "application/json")
		json.NewEncoder(w).Encode(map[string]any{
			"id": "chatcmpl-1", "object": "chat.completion", "created": 1, "model": "gpt-luna",
			"choices": []map[string]any{{
				"index": 0, "finish_reason": "stop",
				"message": map[string]any{"role": "assistant", "content": "ok"},
			}},
		})
	}))
	defer server.Close()

	provider := openCodeProvider()
	// The client is gated on the provider's API base (OpenCode Zen) while the
	// request is dispatched at the local stand-in.
	base, err := newOpenAIClientWithTransport(provider, server.URL, openCodeTransport(provider, "gpt-luna", typ.SessionID{}))
	if err != nil {
		t.Fatalf("newOpenAIClientWithTransport: %v", err)
	}
	oc := &OpenCodeClient{OpenAIClient: base}

	resp, err := oc.ChatCompletionsNew(sessionCtx("conversation-a"), openai.ChatCompletionNewParams{
		Model:    "gpt-luna",
		Messages: []openai.ChatCompletionMessageParamUnion{openai.UserMessage("say ok")},
	})
	if err != nil {
		t.Fatalf("chat completion through OpenCode Zen: %v", err)
	}
	if gotSession == "" {
		t.Fatal("upstream saw no x-opencode-session")
	}
	if len(resp.Choices) != 1 {
		t.Fatalf("choices = %d, want 1", len(resp.Choices))
	}
}

// TestClientPool_DispatchesOpenCodeProviders guards the wiring: without the
// pool branch the vendor layer is never mounted and every Zen request 400s,
// which is exactly the shape of #1713.
func TestClientPool_DispatchesOpenCodeProviders(t *testing.T) {
	pool := NewClientPool()
	ctx := sessionCtx("conversation-a")

	if _, ok := pool.GetOpenAIClient(ctx, openCodeProvider(), "gpt-luna").(*OpenCodeClient); !ok {
		t.Error("OpenAI-shape Zen provider did not dispatch to the OpenCode client")
	}

	anthropicBase := openCodeProvider()
	anthropicBase.APIBase = "https://opencode.ai/zen/go"
	if pool.GetAnthropicClient(ctx, anthropicBase, "claude-sonnet-5") == nil {
		t.Error("Anthropic-shape Zen provider produced no client")
	}

	other := openCodeProvider()
	other.APIBase = "https://api.openai.com/v1"
	if _, ok := pool.GetOpenAIClient(ctx, other, "gpt-5").(*OpenCodeClient); ok {
		t.Error("a non-OpenCode provider was routed through the OpenCode client")
	}
}
