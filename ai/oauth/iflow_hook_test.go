package oauth

import (
	"context"
	"io"
	"net/http"
	"net/http/httptest"
	"testing"
)

func TestIFlowHook_BeforeAuth(t *testing.T) {
	hook := &IFlowHook{ClientID: "cid", ClientSecret: "secret"}
	params := map[string]string{}
	if err := hook.BeforeAuth(params); err != nil {
		t.Fatalf("BeforeAuth returned error: %v", err)
	}
	if params["loginMethod"] != "phone" {
		t.Errorf("expected loginMethod=phone, got %q", params["loginMethod"])
	}
	if params["type"] != "phone" {
		t.Errorf("expected type=phone, got %q", params["type"])
	}
}

func TestIFlowHook_BeforeToken_BasicAuth(t *testing.T) {
	hook := &IFlowHook{ClientID: "myclient", ClientSecret: "mysecret"}
	header := http.Header{}
	if err := hook.BeforeToken(map[string]string{}, header); err != nil {
		t.Fatalf("BeforeToken returned error: %v", err)
	}

	req := &http.Request{Header: header}
	user, pass, ok := req.BasicAuth()
	if !ok {
		t.Fatal("expected a valid Basic Authorization header")
	}
	if user != "myclient" || pass != "mysecret" {
		t.Errorf("expected client_id/secret myclient/mysecret, got %s/%s", user, pass)
	}
	if got := header.Get("Accept"); got != "application/json" {
		t.Errorf("expected Accept=application/json, got %q", got)
	}
}

func TestIFlowHook_AfterToken_Success(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if got := r.URL.Query().Get("accessToken"); got != "test-token" {
			t.Errorf("expected accessToken=test-token, got %q", got)
		}
		_, _ = io.WriteString(w, `{"success":true,"data":{"apiKey":"sk-iflow-123","email":"user@example.com"}}`)
	}))
	defer srv.Close()

	httpClient := &http.Client{Transport: &hostRewriteTransport{
		base:   http.DefaultTransport,
		target: srv.URL,
		hosts:  map[string]bool{"iflow.cn": true},
	}}

	hook := &IFlowHook{}
	meta, err := hook.AfterToken(context.Background(), "test-token", httpClient)
	if err != nil {
		t.Fatalf("AfterToken returned error: %v", err)
	}
	if meta["api_key"] != "sk-iflow-123" {
		t.Errorf("expected api_key=sk-iflow-123, got %v", meta["api_key"])
	}
	if meta["email"] != "user@example.com" {
		t.Errorf("expected email=user@example.com, got %v", meta["email"])
	}
}

func TestIFlowHook_AfterToken_FallsBackToPhone(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		_, _ = io.WriteString(w, `{"success":true,"data":{"apiKey":"sk-iflow-123","phone":"13800000000"}}`)
	}))
	defer srv.Close()

	httpClient := &http.Client{Transport: &hostRewriteTransport{
		base:   http.DefaultTransport,
		target: srv.URL,
		hosts:  map[string]bool{"iflow.cn": true},
	}}

	hook := &IFlowHook{}
	meta, err := hook.AfterToken(context.Background(), "test-token", httpClient)
	if err != nil {
		t.Fatalf("AfterToken returned error: %v", err)
	}
	if meta["email"] != "13800000000" {
		t.Errorf("expected email fallback to phone 13800000000, got %v", meta["email"])
	}
}

func TestIFlowHook_AfterToken_NotSuccessful(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		_, _ = io.WriteString(w, `{"success":false}`)
	}))
	defer srv.Close()

	httpClient := &http.Client{Transport: &hostRewriteTransport{
		base:   http.DefaultTransport,
		target: srv.URL,
		hosts:  map[string]bool{"iflow.cn": true},
	}}

	hook := &IFlowHook{}
	meta, err := hook.AfterToken(context.Background(), "test-token", httpClient)
	if err == nil {
		t.Fatal("expected error when success=false")
	}
	if meta != nil {
		t.Errorf("expected nil metadata on failure, got %v", meta)
	}
}

func TestIFlowHook_AfterToken_NonOKStatus(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusForbidden)
	}))
	defer srv.Close()

	httpClient := &http.Client{Transport: &hostRewriteTransport{
		base:   http.DefaultTransport,
		target: srv.URL,
		hosts:  map[string]bool{"iflow.cn": true},
	}}

	hook := &IFlowHook{}
	meta, err := hook.AfterToken(context.Background(), "test-token", httpClient)
	if err == nil {
		t.Fatal("expected error on non-OK status")
	}
	if meta != nil {
		t.Errorf("expected nil metadata on non-OK status, got %v", meta)
	}
}
