package fetcher

import (
	"context"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/tingly-dev/tingly-box/ai"
	"github.com/tingly-dev/tingly-box/ai/quota"
)

func TestTinglyBoxFetcher(t *testing.T) {
	t.Parallel()

	var gotPath, gotAuth string
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		gotPath, gotAuth = r.URL.Path, r.Header.Get("Authorization")
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(`{"provider_type":"tingly_box","fetched_at":"2026-09-26T11:00:00Z","windows":[
			{"key":"upstream_1/5h","type":"session","kind":"limit","label":"upstream 1 · 5h","used":40,"limit":100,"used_percent":40,"unit":"percent","window_minutes":300}
		]}`))
	}))
	defer srv.Close()

	provider := &ai.Provider{
		UUID: "p1", Name: "central", APIBase: srv.URL + "/tingly/team/v1",
		AuthType: ai.AuthTypeAPIKey, Token: "tb-share-abc",
	}
	usage, err := NewTinglyBoxFetcher().Fetch(context.Background(), provider)
	if err != nil {
		t.Fatalf("Fetch: %v", err)
	}
	if gotPath != "/tingly/team/quota" || gotAuth != "Bearer tb-share-abc" {
		t.Errorf("request = %s (auth %q), want /tingly/team/quota with the provider key", gotPath, gotAuth)
	}
	if usage.ProviderType != quota.ProviderTypeTinglyBox || usage.ProviderUUID != "p1" || usage.ProviderName != "central" {
		t.Errorf("usage identity = %s/%s/%s", usage.ProviderType, usage.ProviderUUID, usage.ProviderName)
	}
	if pct, ok := usage.Pct(); !ok || pct != 40 || usage.Windows[0].Label != "upstream 1 · 5h" {
		t.Errorf("windows = %+v", usage.Windows)
	}
}

func TestTinglyBoxFetcherExplainsRefusal(t *testing.T) {
	t.Parallel()

	for status, want := range map[int]string{
		http.StatusForbidden: "does not share quota",
		http.StatusNotFound:  "does not serve quota",
	} {
		srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			w.WriteHeader(status)
		}))
		provider := &ai.Provider{APIBase: srv.URL + "/tingly/team", AuthType: ai.AuthTypeAPIKey, Token: "k"}
		_, err := NewTinglyBoxFetcher().Fetch(context.Background(), provider)
		srv.Close()
		if err == nil || !strings.Contains(err.Error(), want) {
			t.Errorf("status %d: err = %v, want %q", status, err, want)
		}
	}
}
