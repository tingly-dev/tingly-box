package oauth

import (
	"context"
	"encoding/base64"
	"encoding/json"
	"github.com/tingly-dev/tingly-box/ai"
	"io"
	"net/http"
	"net/http/httptest"
	"net/url"
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
	if got := header.Get("Accept"); got != "application/json, text/plain, */*" {
		t.Errorf("unexpected Accept %q", got)
	}
	if got := header.Get("User-Agent"); got != "axios/1.15.2" {
		t.Errorf("unexpected OAuth User-Agent %q", got)
	}
	for _, key := range []string{"Origin", "Referer", "Accept-Language"} {
		if got := header.Get(key); got != "" {
			t.Errorf("unexpected browser header %s=%q", key, got)
		}
	}
}

func TestAnthropicHook_AfterToken_Success(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/v1/account" {
			http.NotFound(w, r)
			return
		}
		if got := r.Header.Get("Authorization"); got != "Bearer test-token" {
			t.Errorf("expected Bearer auth, got %q", got)
		}
		_, _ = io.WriteString(w, `{"uuid":"acct-123","email_address":"user@example.com"}`)
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

// ensure the accountResponse decode path tolerates unrelated extra fields.
func TestAnthropicHook_AfterToken_ExtraFields(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		payload, _ := json.Marshal(map[string]any{
			"uuid":          "acct-456",
			"email_address": "other@example.com",
			"unrelated":     "field",
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

func TestClaudeAuthorizationURL(t *testing.T) {
	config := DefaultConfig()
	config.BaseURL = "http://localhost:54545"
	manager := NewManager(WithConfig(config))
	authURL, state, err := manager.GetAuthURL("user", ai.IssuerClaudeCode, "", "", "")
	if err != nil {
		t.Fatal(err)
	}
	u, err := url.Parse(authURL)
	if err != nil {
		t.Fatal(err)
	}
	if u.Scheme != "https" || u.Host != "claude.com" || u.Path != "/cai/oauth/authorize" {
		t.Fatalf("unexpected authorization endpoint %s", u)
	}
	rawState, err := base64.RawURLEncoding.DecodeString(state)
	if err != nil || len(rawState) != 32 || len(state) != 43 {
		t.Fatalf("unexpected state format, length %d", len(state))
	}
	stored, err := manager.GetStateData(state)
	if err != nil {
		t.Fatal(err)
	}
	query := u.Query()
	want := map[string]string{
		"code":                  "true",
		"response_type":         "code",
		"client_id":             "9d1c250a-e61b-44d9-88ed-5944d1962f5e",
		"redirect_uri":          "http://localhost:54545/callback",
		"state":                 state,
		"scope":                 "org:create_api_key user:profile user:inference user:sessions:claude_code user:mcp_servers user:file_upload",
		"code_challenge_method": "S256",
		"code_challenge":        manager.generateCodeChallenge(stored.CodeVerifier),
	}
	for key, value := range want {
		if query.Get(key) != value {
			t.Errorf("%s = %q, want %q", key, query.Get(key), value)
		}
	}
	if stored.RedirectURI != query.Get("redirect_uri") {
		t.Fatal("stored callback differs from authorization URL")
	}
}
