package client

import (
	"testing"

	"github.com/tingly-dev/tingly-box/ai"
	"github.com/tingly-dev/tingly-box/internal/typ"
)

// TestNewAntigravityClient_AppliesRoundTripper verifies that NewAntigravityClient
// builds a GoogleClient whose http.Client is wrapped with antigravityRoundTripper
// carrying the project_id from OAuth metadata.
func TestNewAntigravityClient_AppliesRoundTripper(t *testing.T) {
	provider := &typ.Provider{
		UUID:     "test-provider-antigravity",
		Name:     "Antigravity Provider",
		APIBase:  "https://cloudcode-pa.googleapis.com",
		AuthType: typ.AuthTypeOAuth,
		OAuthDetail: &typ.OAuthDetail{
			Issuer:       ai.IssuerAntigravity,
			ProviderType: string(ai.IssuerAntigravity),
			AccessToken:  "token",
			ExtraFields:  map[string]interface{}{"project_id": "antigrav-proj"},
		},
	}
	sessionID := typ.SessionID{Source: typ.SessionSourceUser, Value: "antigrav-client-test"}

	c, err := NewAntigravityClient(provider, "gemini-2.5-pro", sessionID)
	if err != nil {
		t.Fatalf("NewAntigravityClient: %v", err)
	}
	if c == nil || c.GoogleClient == nil {
		t.Fatal("expected AntigravityClient wrapping a GoogleClient")
	}
	if c.httpClient == nil {
		t.Fatal("expected embedded http.Client to be set")
	}

	lrt, ok := c.httpClient.Transport.(*loggingRoundTripper)
	if !ok {
		t.Fatalf("expected transport to be *loggingRoundTripper, got %T", c.httpClient.Transport)
	}
	rt, ok := lrt.inner.(*antigravityRoundTripper)
	if !ok {
		t.Fatalf("expected wrapped transport to be *antigravityRoundTripper, got %T", lrt.inner)
	}
	if rt.project != "antigrav-proj" {
		t.Errorf("expected project_id=antigrav-proj on round tripper, got %q", rt.project)
	}
}

// TestNewAntigravityClient_RejectsWrongIssuer guards against misuse.
func TestNewAntigravityClient_RejectsWrongIssuer(t *testing.T) {
	provider := &typ.Provider{
		AuthType: typ.AuthTypeOAuth,
		OAuthDetail: &typ.OAuthDetail{
			Issuer: ai.IssuerGemini,
		},
	}
	if _, err := NewAntigravityClient(provider, "x", typ.SessionID{}); err == nil {
		t.Error("expected NewAntigravityClient to reject a non-antigravity provider")
	}
}
