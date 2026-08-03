package oauth

import (
	"context"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
)

func TestAntigravityHook_BeforeAuth(t *testing.T) {
	hook := &AntigravityHook{}
	params := map[string]string{}
	if err := hook.BeforeAuth(params); err != nil {
		t.Fatalf("BeforeAuth returned error: %v", err)
	}
	if params["access_type"] != "offline" {
		t.Errorf("expected access_type=offline, got %q", params["access_type"])
	}
	if params["prompt"] != "consent" {
		t.Errorf("expected prompt=consent, got %q", params["prompt"])
	}
	if params["include_granted_scopes"] != "true" {
		t.Errorf("expected include_granted_scopes=true, got %q", params["include_granted_scopes"])
	}
}

func TestAntigravityHook_BeforeToken_NoOp(t *testing.T) {
	hook := &AntigravityHook{}
	header := http.Header{}
	if err := hook.BeforeToken(map[string]string{}, header); err != nil {
		t.Fatalf("BeforeToken returned error: %v", err)
	}
	if len(header) != 0 {
		t.Errorf("expected BeforeToken to be a no-op, got header %v", header)
	}
}

func TestAntigravityHook_AfterToken_LoadCodeAssistDirect(t *testing.T) {
	var loadCalled, userinfoCalled bool

	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch {
		case strings.HasSuffix(r.URL.Path, ":loadCodeAssist"):
			loadCalled = true
			if got := r.Header.Get("Authorization"); got != "Bearer test-token" {
				t.Errorf("expected Bearer auth, got %q", got)
			}
			_, _ = io.WriteString(w, `{"cloudaicompanionProject":"discovered-project"}`)
		case strings.HasSuffix(r.URL.Path, "/oauth2/v1/userinfo"):
			userinfoCalled = true
			_, _ = io.WriteString(w, `{"email":"user@example.com"}`)
		default:
			http.NotFound(w, r)
		}
	}))
	defer srv.Close()

	httpClient := &http.Client{Transport: &hostRewriteTransport{
		base:   http.DefaultTransport,
		target: srv.URL,
		hosts:  map[string]bool{"cloudcode-pa.googleapis.com": true, "www.googleapis.com": true},
	}}

	hook := &AntigravityHook{}
	meta, err := hook.AfterToken(context.Background(), "test-token", httpClient)
	if err != nil {
		t.Fatalf("AfterToken returned error: %v", err)
	}
	if !loadCalled {
		t.Error("expected loadCodeAssist to be called")
	}
	if !userinfoCalled {
		t.Error("expected userinfo to be called")
	}
	if meta["project_id"] != "discovered-project" {
		t.Errorf("expected project_id=discovered-project, got %v", meta["project_id"])
	}
	if meta["email"] != "user@example.com" {
		t.Errorf("expected email=user@example.com, got %v", meta["email"])
	}
}

func TestAntigravityHook_AfterToken_ProjectDiscoveryFails(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch {
		case strings.HasSuffix(r.URL.Path, ":loadCodeAssist"):
			w.WriteHeader(http.StatusInternalServerError)
		case strings.HasSuffix(r.URL.Path, "/oauth2/v1/userinfo"):
			_, _ = io.WriteString(w, `{"email":"user@example.com"}`)
		default:
			http.NotFound(w, r)
		}
	}))
	defer srv.Close()

	httpClient := &http.Client{Transport: &hostRewriteTransport{
		base:   http.DefaultTransport,
		target: srv.URL,
		hosts:  map[string]bool{"cloudcode-pa.googleapis.com": true, "www.googleapis.com": true},
	}}

	hook := &AntigravityHook{}
	meta, err := hook.AfterToken(context.Background(), "test-token", httpClient)
	if err != nil {
		t.Fatalf("expected no error when project discovery fails (best-effort), got %v", err)
	}
	if _, ok := meta["project_id"]; ok {
		t.Errorf("expected no project_id when discovery fails, got %v", meta["project_id"])
	}
	if meta["email"] != "user@example.com" {
		t.Errorf("expected email to still be populated, got %v", meta["email"])
	}
}
