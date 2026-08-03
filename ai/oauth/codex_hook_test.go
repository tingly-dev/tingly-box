package oauth

import (
	"context"
	"io"
	"net/http"
	"net/http/httptest"
	"testing"
)

func TestCodexHook_BeforeAuth(t *testing.T) {
	hook := &CodexHook{}
	params := map[string]string{}
	if err := hook.BeforeAuth(params); err != nil {
		t.Fatalf("BeforeAuth returned error: %v", err)
	}
	if params["id_token_add_organizations"] != "true" {
		t.Errorf("expected id_token_add_organizations=true, got %q", params["id_token_add_organizations"])
	}
	if params["codex_cli_simplified_flow"] != "true" {
		t.Errorf("expected codex_cli_simplified_flow=true, got %q", params["codex_cli_simplified_flow"])
	}
	if params["originator"] != "codex_cli_rs" {
		t.Errorf("expected originator=codex_cli_rs, got %q", params["originator"])
	}
}

func TestCodexHook_BeforeToken(t *testing.T) {
	hook := &CodexHook{}
	header := http.Header{}
	if err := hook.BeforeToken(map[string]string{}, header); err != nil {
		t.Fatalf("BeforeToken returned error: %v", err)
	}
	if got := header.Get("Content-Type"); got != "application/x-www-form-urlencoded" {
		t.Errorf("expected Content-Type=application/x-www-form-urlencoded, got %q", got)
	}
	if got := header.Get("Accept"); got != "application/json" {
		t.Errorf("expected Accept=application/json, got %q", got)
	}
}

func TestCodexHook_AfterToken_Success(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/v1/user" {
			http.NotFound(w, r)
			return
		}
		if got := r.Header.Get("Authorization"); got != "Bearer test-token" {
			t.Errorf("expected Bearer auth, got %q", got)
		}
		_, _ = io.WriteString(w, `{"email":"user@example.com","name":"Test User"}`)
	}))
	defer srv.Close()

	httpClient := &http.Client{Transport: &hostRewriteTransport{
		base:   http.DefaultTransport,
		target: srv.URL,
		hosts:  map[string]bool{"api.openai.com": true},
	}}

	hook := &CodexHook{}
	meta, err := hook.AfterToken(context.Background(), "test-token", httpClient)
	if err != nil {
		t.Fatalf("AfterToken returned error: %v", err)
	}
	if meta["email"] != "user@example.com" {
		t.Errorf("expected email=user@example.com, got %v", meta["email"])
	}
	if meta["name"] != "Test User" {
		t.Errorf("expected name=Test User, got %v", meta["name"])
	}
}

func TestCodexHook_AfterToken_NonOKStatus(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusNotFound)
	}))
	defer srv.Close()

	httpClient := &http.Client{Transport: &hostRewriteTransport{
		base:   http.DefaultTransport,
		target: srv.URL,
		hosts:  map[string]bool{"api.openai.com": true},
	}}

	hook := &CodexHook{}
	meta, err := hook.AfterToken(context.Background(), "test-token", httpClient)
	if err != nil {
		t.Fatalf("expected no error on non-OK status (best-effort), got %v", err)
	}
	if meta != nil {
		t.Errorf("expected nil metadata on non-OK status, got %v", meta)
	}
}

func TestCodexHook_AfterToken_MalformedJSON(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		_, _ = io.WriteString(w, `not-json`)
	}))
	defer srv.Close()

	httpClient := &http.Client{Transport: &hostRewriteTransport{
		base:   http.DefaultTransport,
		target: srv.URL,
		hosts:  map[string]bool{"api.openai.com": true},
	}}

	hook := &CodexHook{}
	meta, err := hook.AfterToken(context.Background(), "test-token", httpClient)
	if err != nil {
		t.Fatalf("expected no error on malformed JSON (best-effort), got %v", err)
	}
	if meta != nil {
		t.Errorf("expected nil metadata on malformed JSON, got %v", meta)
	}
}
