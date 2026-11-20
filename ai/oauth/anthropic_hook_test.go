package oauth

import (
	"context"
	"encoding/json"
	"io"
	"net/http"
	"net/http/httptest"
	"testing"
)

func TestAnthropicHook_BeforeAuth(t *testing.T) {
	hook := &AnthropicHook{}
	params := map[string]string{}
	if err := hook.BeforeAuth(params); err != nil {
		t.Fatalf("BeforeAuth returned error: %v", err)
	}
	if params["code"] != "true" {
		t.Errorf("expected code=true, got %q", params["code"])
	}
	if params["response_type"] != "code" {
		t.Errorf("expected response_type=code, got %q", params["response_type"])
	}
}

func TestAnthropicHook_BeforeToken(t *testing.T) {
	hook := &AnthropicHook{}
	header := http.Header{}
	if err := hook.BeforeToken(map[string]string{}, header); err != nil {
		t.Fatalf("BeforeToken returned error: %v", err)
	}
	if got := header.Get("Content-Type"); got != "application/json" {
		t.Errorf("expected Content-Type=application/json, got %q", got)
	}
	// The official CLI posts plain JSON as itself; it never pretends to be a
	// browser on claude.ai.
	if got := header.Get("User-Agent"); got != ClaudeCLIOAuthUserAgent {
		t.Errorf("User-Agent = %q, want %q", got, ClaudeCLIOAuthUserAgent)
	}
	for _, h := range []string{"Origin", "Referer", "Accept-Language"} {
		if got := header.Get(h); got != "" {
			t.Errorf("%s must not be sent, got %q", h, got)
		}
	}
}

func TestAnthropicHook_AfterToken_Success(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/api/oauth/profile" {
			http.NotFound(w, r)
			return
		}
		if got := r.Header.Get("Authorization"); got != "Bearer test-token" {
			t.Errorf("expected Bearer auth, got %q", got)
		}
		_, _ = io.WriteString(w, `{"account":{"uuid":"acct-123","email":"user@example.com","display_name":"U"},
			"organization":{"uuid":"org-9","organization_type":"claude_max","rate_limit_tier":"default_claude_max_5x","billing_type":"stripe_subscription"}}`)
	}))
	defer srv.Close()

	httpClient := &http.Client{Transport: &hostRewriteTransport{
		base:   http.DefaultTransport,
		target: srv.URL,
		hosts:  map[string]bool{"api.anthropic.com": true},
	}}

	hook := &AnthropicHook{}
	meta, err := hook.AfterToken(context.Background(), "test-token", httpClient)
	if err != nil {
		t.Fatalf("AfterToken returned error: %v", err)
	}
	if meta["email"] != "user@example.com" {
		t.Errorf("expected email=user@example.com, got %v", meta["email"])
	}
	if meta["account_id"] != "acct-123" {
		t.Errorf("expected account_id=acct-123, got %v", meta["account_id"])
	}
	if meta["organization_id"] != "org-9" {
		t.Errorf("expected organization_id=org-9, got %v", meta["organization_id"])
	}
	if meta["subscription_type"] != "max" {
		t.Errorf("expected subscription_type=max, got %v", meta["subscription_type"])
	}
	if meta["rate_limit_tier"] != "default_claude_max_5x" {
		t.Errorf("expected rate_limit_tier, got %v", meta["rate_limit_tier"])
	}
}

// An organization type this client does not know is left out, not guessed.
func TestAnthropicHook_AfterToken_UnknownPlan(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		_, _ = io.WriteString(w, `{"account":{"uuid":"a"},"organization":{"organization_type":"claude_future"}}`)
	}))
	defer srv.Close()
	httpClient := &http.Client{Transport: &hostRewriteTransport{base: http.DefaultTransport, target: srv.URL, hosts: map[string]bool{"api.anthropic.com": true}}}

	meta, err := (&AnthropicHook{}).AfterToken(context.Background(), "t", httpClient)
	if err != nil {
		t.Fatal(err)
	}
	if _, ok := meta["subscription_type"]; ok {
		t.Errorf("unknown organization_type must not produce a plan: %v", meta)
	}
	if meta["account_id"] != "a" {
		t.Errorf("identity still read: %v", meta)
	}
}

// The profile of an account with nothing we read adds no metadata at all.
func TestAnthropicHook_AfterToken_EmptyProfile(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		_, _ = io.WriteString(w, `{"account":{},"organization":{}}`)
	}))
	defer srv.Close()
	httpClient := &http.Client{Transport: &hostRewriteTransport{base: http.DefaultTransport, target: srv.URL, hosts: map[string]bool{"api.anthropic.com": true}}}

	meta, err := (&AnthropicHook{}).AfterToken(context.Background(), "t", httpClient)
	if err != nil || meta != nil {
		t.Errorf("want nil metadata, got %v (err %v)", meta, err)
	}
}

func TestAnthropicHook_AfterToken_NonOKStatus(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusUnauthorized)
	}))
	defer srv.Close()

	httpClient := &http.Client{Transport: &hostRewriteTransport{
		base:   http.DefaultTransport,
		target: srv.URL,
		hosts:  map[string]bool{"api.anthropic.com": true},
	}}

	hook := &AnthropicHook{}
	meta, err := hook.AfterToken(context.Background(), "bad-token", httpClient)
	if err != nil {
		t.Fatalf("expected no error on non-OK status (best-effort), got %v", err)
	}
	if meta != nil {
		t.Errorf("expected nil metadata on non-OK status, got %v", meta)
	}
}

func TestAnthropicHook_AfterToken_MalformedJSON(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		_, _ = io.WriteString(w, `not-json`)
	}))
	defer srv.Close()

	httpClient := &http.Client{Transport: &hostRewriteTransport{
		base:   http.DefaultTransport,
		target: srv.URL,
		hosts:  map[string]bool{"api.anthropic.com": true},
	}}

	hook := &AnthropicHook{}
	meta, err := hook.AfterToken(context.Background(), "test-token", httpClient)
	if err != nil {
		t.Fatalf("expected no error on malformed JSON (best-effort), got %v", err)
	}
	if meta != nil {
		t.Errorf("expected nil metadata on malformed JSON, got %v", meta)
	}
}

// ensure the profile decode path tolerates unrelated extra fields.
func TestAnthropicHook_AfterToken_ExtraFields(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		payload, _ := json.Marshal(map[string]any{
			"account":   map[string]any{"uuid": "acct-456", "email": "other@example.com", "unrelated": "field"},
			"unrelated": "field",
		})
		_, _ = w.Write(payload)
	}))
	defer srv.Close()

	httpClient := &http.Client{Transport: &hostRewriteTransport{
		base:   http.DefaultTransport,
		target: srv.URL,
		hosts:  map[string]bool{"api.anthropic.com": true},
	}}

	hook := &AnthropicHook{}
	meta, err := hook.AfterToken(context.Background(), "test-token", httpClient)
	if err != nil {
		t.Fatalf("AfterToken returned error: %v", err)
	}
	if meta["account_id"] != "acct-456" {
		t.Errorf("expected account_id=acct-456, got %v", meta["account_id"])
	}
}
