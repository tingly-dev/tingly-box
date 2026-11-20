package oauth

import (
	"context"
	"encoding/json"
	"io"
	"net/http"
	"net/http/httptest"
	"net/url"
	"slices"
	"strings"
	"sync"
	"testing"

	"github.com/tingly-dev/tingly-box/ai"
)

// The Claude Code OAuth client follows the official CLI (2.1.328):
// .research/claude-code/2.1.328.md §4.

func TestClaudeRegistry_MatchesOfficialClient(t *testing.T) {
	cfg, ok := DefaultRegistry().Get(ai.IssuerClaudeCode)
	if !ok {
		t.Fatal("claude_code issuer not registered")
	}
	if cfg.AuthURL != "https://claude.com/cai/oauth/authorize" {
		t.Errorf("AuthURL = %q", cfg.AuthURL)
	}
	if cfg.TokenURL != "https://platform.claude.com/v1/oauth/token" {
		t.Errorf("TokenURL = %q", cfg.TokenURL)
	}
	if cfg.ClientID != "9d1c250a-e61b-44d9-88ed-5944d1962f5e" || cfg.ClientSecret != "" {
		t.Errorf("must stay the public CLI client: %q / %q", cfg.ClientID, cfg.ClientSecret)
	}
	for _, scope := range []string{"org:create_api_key", "user:profile", "user:inference", "user:sessions:claude_code", "user:mcp_servers", "user:file_upload", "user:plugins"} {
		if !slices.Contains(cfg.Scopes, scope) {
			t.Errorf("login scopes miss %s: %v", scope, cfg.Scopes)
		}
	}
	if slices.Contains(cfg.Scopes, "user:privacy_consents:write") {
		t.Error("privacy-consent write is not needed and must not be requested")
	}
	// Refresh restates the CLI's default set, which has no login-only scope.
	if slices.Contains(cfg.RefreshScopes, "org:create_api_key") || !slices.Contains(cfg.RefreshScopes, "user:inference") {
		t.Errorf("refresh scopes = %v", cfg.RefreshScopes)
	}
}

func TestClaudeAuthURL_UsesOfficialParameters(t *testing.T) {
	m := NewManager(WithRegistry(DefaultRegistry()))
	authURL, _, err := m.GetAuthURL("u", ai.IssuerClaudeCode, "", "", "", WithBaseURL("http://localhost:54545"))
	if err != nil {
		t.Fatal(err)
	}
	u, err := url.Parse(authURL)
	if err != nil {
		t.Fatal(err)
	}
	if u.Host != "claude.com" || u.Path != "/cai/oauth/authorize" {
		t.Errorf("authorize endpoint = %s%s", u.Host, u.Path)
	}
	q := u.Query()
	if q.Get("code") != "true" || q.Get("response_type") != "code" || q.Get("code_challenge_method") != "S256" || q.Get("code_challenge") == "" {
		t.Errorf("query = %v", q)
	}
	if !strings.Contains(q.Get("scope"), "org:create_api_key") || !strings.Contains(q.Get("scope"), "user:plugins") {
		t.Errorf("scope = %q", q.Get("scope"))
	}
}

// tokenServer records the refresh requests it gets and answers from handler.
type tokenServer struct {
	mu   sync.Mutex
	reqs []map[string]any
	hdrs []http.Header
}

func (s *tokenServer) serve(t *testing.T, handler func(call int, body map[string]any, w http.ResponseWriter)) *httptest.Server {
	t.Helper()
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		raw, _ := io.ReadAll(r.Body)
		var body map[string]any
		_ = json.Unmarshal(raw, &body)
		s.mu.Lock()
		s.reqs = append(s.reqs, body)
		s.hdrs = append(s.hdrs, r.Header.Clone())
		call := len(s.reqs)
		s.mu.Unlock()
		w.Header().Set("Content-Type", "application/json")
		handler(call, body, w)
	}))
	t.Cleanup(srv.Close)
	return srv
}

func claudeRefreshManager(tokenURL string) *Manager {
	reg := DefaultRegistry()
	cfg, _ := reg.Get(ai.IssuerClaudeCode)
	cfg.TokenURL = tokenURL
	return NewManager(WithRegistry(reg))
}

const refreshOK = `{"access_token":"new-access","refresh_token":"new-refresh","expires_in":28800}`

func TestClaudeRefresh_SendsScopeAndNoSecret(t *testing.T) {
	ts := &tokenServer{}
	srv := ts.serve(t, func(_ int, _ map[string]any, w http.ResponseWriter) { _, _ = io.WriteString(w, refreshOK) })

	tok, err := claudeRefreshManager(srv.URL).RefreshToken(context.Background(), "u", ai.IssuerClaudeCode, "old-refresh")
	if err != nil {
		t.Fatal(err)
	}
	if tok.AccessToken != "new-access" {
		t.Errorf("access token = %q", tok.AccessToken)
	}
	if len(ts.reqs) != 1 {
		t.Fatalf("requests = %d", len(ts.reqs))
	}
	body := ts.reqs[0]
	if body["grant_type"] != "refresh_token" || body["refresh_token"] != "old-refresh" || body["client_id"] != "9d1c250a-e61b-44d9-88ed-5944d1962f5e" {
		t.Errorf("body = %v", body)
	}
	if _, has := body["client_secret"]; has {
		t.Errorf("a public client sends no client_secret: %v", body)
	}
	scope, _ := body["scope"].(string)
	if !strings.Contains(scope, "user:inference") || strings.Contains(scope, "org:create_api_key") {
		t.Errorf("scope = %q", scope)
	}
	if got := ts.hdrs[0].Get("User-Agent"); got != ClaudeCLIOAuthUserAgent {
		t.Errorf("User-Agent = %q", got)
	}
	if got := ts.hdrs[0].Get("Origin"); got != "" {
		t.Errorf("Origin = %q", got)
	}
}

func TestClaudeRefresh_InvalidScopeRetriesOnceWithoutScope(t *testing.T) {
	ts := &tokenServer{}
	srv := ts.serve(t, func(call int, body map[string]any, w http.ResponseWriter) {
		if _, scoped := body["scope"]; scoped {
			w.WriteHeader(http.StatusBadRequest)
			_, _ = io.WriteString(w, `{"error":"invalid_scope","error_description":"scope not granted"}`)
			return
		}
		_, _ = io.WriteString(w, refreshOK)
	})

	tok, err := claudeRefreshManager(srv.URL).RefreshToken(context.Background(), "u", ai.IssuerClaudeCode, "old-refresh")
	if err != nil {
		t.Fatalf("refresh must recover from invalid_scope: %v", err)
	}
	if tok.AccessToken != "new-access" {
		t.Errorf("access token = %q", tok.AccessToken)
	}
	if len(ts.reqs) != 2 {
		t.Fatalf("requests = %d, want the scoped one and one retry", len(ts.reqs))
	}
	if _, scoped := ts.reqs[1]["scope"]; scoped {
		t.Errorf("the retry must drop scope: %v", ts.reqs[1])
	}
}

func TestClaudeRefresh_OtherErrorsAreNotRetried(t *testing.T) {
	for name, tc := range map[string]struct {
		status int
		body   string
	}{
		"invalid_grant": {http.StatusBadRequest, `{"error":"invalid_grant","error_description":"Refresh token not found or invalid"}`},
		"server error":  {http.StatusInternalServerError, `oops`},
	} {
		t.Run(name, func(t *testing.T) {
			ts := &tokenServer{}
			srv := ts.serve(t, func(_ int, _ map[string]any, w http.ResponseWriter) {
				w.WriteHeader(tc.status)
				_, _ = io.WriteString(w, tc.body)
			})
			_, err := claudeRefreshManager(srv.URL).RefreshToken(context.Background(), "u", ai.IssuerClaudeCode, "old")
			if err == nil {
				t.Fatal("expected an error")
			}
			if len(ts.reqs) != 1 {
				t.Errorf("requests = %d, want 1 (no retry)", len(ts.reqs))
			}
			if !strings.Contains(err.Error(), tc.body) {
				t.Errorf("the issuer's own words must reach the error: %v", err)
			}
		})
	}
}

func TestRefreshErrorCode(t *testing.T) {
	cases := map[string]string{
		`{"error":"invalid_scope"}`:                             "invalid_scope",
		`{"error":{"type":"invalid_scope","message":"x"}}`:      "invalid_scope",
		`{"error":{"code":"invalid_grant","type":"api_error"}}`: "invalid_grant",
		`not json`: "",
		`{}`:       "",
	}
	for in, want := range cases {
		if got := refreshErrorCode([]byte(in)); got != want {
			t.Errorf("refreshErrorCode(%s) = %q, want %q", in, got, want)
		}
	}
}

// Other issuers keep their refresh exactly as before: secret sent, no scope.
func TestRefresh_OtherIssuersUnchanged(t *testing.T) {
	ts := &tokenServer{}
	srv := ts.serve(t, func(_ int, _ map[string]any, w http.ResponseWriter) { _, _ = io.WriteString(w, refreshOK) })
	reg := NewRegistry()
	reg.Register(&ProviderConfig{
		Type: ai.IssuerGoogle, ClientID: "cid", ClientSecret: "sec", TokenURL: srv.URL,
		TokenRequestFormat: TokenRequestFormatJSON, Hook: &NoopHook{},
	})
	if _, err := NewManager(WithRegistry(reg)).RefreshToken(context.Background(), "u", ai.IssuerGoogle, "r"); err != nil {
		t.Fatal(err)
	}
	body := ts.reqs[0]
	if body["client_secret"] != "sec" {
		t.Errorf("other issuers still send their secret: %v", body)
	}
	if _, has := body["scope"]; has {
		t.Errorf("no scope unless the issuer asks for it: %v", body)
	}
}
