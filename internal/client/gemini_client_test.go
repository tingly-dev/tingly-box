package client

import (
	"testing"

	"github.com/tingly-dev/tingly-box/ai"
	"github.com/tingly-dev/tingly-box/internal/typ"
)

// TestNewGeminiClient_AppliesRoundTripper verifies that NewGeminiClient builds
// a GoogleClient whose http.Client is wrapped with geminiRoundTripper carrying
// the project_id from OAuth metadata.
func TestNewGeminiClient_AppliesRoundTripper(t *testing.T) {
	provider := &typ.Provider{
		UUID:     "test-provider-gemini",
		Name:     "Gemini Provider",
		APIBase:  "https://cloudcode-pa.googleapis.com",
		AuthType: typ.AuthTypeOAuth,
		OAuthDetail: &typ.OAuthDetail{
			Issuer:       ai.IssuerGemini,
			ProviderType: string(ai.IssuerGemini),
			AccessToken:  "token",
			ExtraFields:  map[string]interface{}{"project_id": "test-project"},
		},
	}
	sessionID := typ.SessionID{Source: typ.SessionSourceUser, Value: "gemini-client-test"}

	c, err := NewGeminiClient(provider, "gemini-2.5-pro", sessionID)
	if err != nil {
		t.Fatalf("NewGeminiClient: %v", err)
	}
	if c == nil || c.GoogleClient == nil {
		t.Fatal("expected GeminiClient wrapping a GoogleClient")
	}

	if c.httpClient == nil {
		t.Fatal("expected embedded http.Client to be set")
	}
	rt, ok := c.httpClient.Transport.(*geminiRoundTripper)
	if !ok {
		t.Fatalf("expected transport to be *geminiRoundTripper, got %T", c.httpClient.Transport)
	}
	if rt.project != "test-project" {
		t.Errorf("expected project_id=test-project on round tripper, got %q", rt.project)
	}
}

// TestNewGeminiClient_RejectsWrongIssuer guards against constructing a
// GeminiClient for a non-Gemini provider — the upstream wire format is wrong
// for any other issuer.
func TestNewGeminiClient_RejectsWrongIssuer(t *testing.T) {
	provider := &typ.Provider{
		UUID:     "test-provider-wrong",
		APIBase:  "https://cloudcode-pa.googleapis.com",
		AuthType: typ.AuthTypeOAuth,
		OAuthDetail: &typ.OAuthDetail{
			Issuer: ai.IssuerAntigravity,
		},
	}
	if _, err := NewGeminiClient(provider, "x", typ.SessionID{}); err == nil {
		t.Error("expected NewGeminiClient to reject a non-gemini provider")
	}
}

// Compile-time check: GeminiClient embeds *GoogleClient so callers expecting
// *GoogleClient (e.g. forwarding.ForwardGoogle) continue to work after dispatch.
var _ = (*GeminiClient)(nil)
