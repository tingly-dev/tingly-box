package oauth

import (
	"context"
	"encoding/json"
	"errors"
	"io"
	"net/http"
	"strings"
	"testing"

	"github.com/tingly-dev/tingly-box/ai"
)

type tokenResponseTransport func(*http.Request) (*http.Response, error)

func (f tokenResponseTransport) RoundTrip(r *http.Request) (*http.Response, error) { return f(r) }

func TestTokenResponseRequiresAccessToken(t *testing.T) {
	for _, body := range []string{`{}`, `{"access_token":""}`, `{"access_token":"  "}`} {
		t.Run(body, func(t *testing.T) {
			client := &http.Client{Transport: tokenResponseTransport(func(r *http.Request) (*http.Response, error) {
				return &http.Response{StatusCode: http.StatusOK, Body: io.NopCloser(strings.NewReader(body)), Header: make(http.Header)}, nil
			})}
			opts := &Options{HTTPClient: client}
			registry := NewRegistry()
			provider := &ProviderConfig{Type: ai.IssuerMock, ClientID: "test", TokenURL: "https://example.test/token"}
			registry.Register(provider)
			manager := NewManager(WithRegistry(registry))
			ctx := context.Background()
			if _, err := manager.exchangeCodeForToken(ctx, provider, "state", "code", "", "http://localhost/callback", opts); !errors.Is(err, ErrTokenExchangeFailed) {
				t.Errorf("exchange error = %v, want ErrTokenExchangeFailed", err)
			}
			if _, err := manager.refreshToken(ctx, ai.IssuerMock, "refresh", opts); !errors.Is(err, ErrTokenExchangeFailed) {
				t.Errorf("refresh error = %v, want ErrTokenExchangeFailed", err)
			}
		})
	}
}

func TestExchangeRateLimited(t *testing.T) {
	for header, want := range map[string]string{
		"":                              "wait before starting a new login",
		"60":                            "wait 60 seconds",
		"Sun, 06 Nov 1994 08:49:37 GMT": "1994-11-06T08:49:37Z",
		"invalid":                       "wait before starting a new login",
	} {
		t.Run(header, func(t *testing.T) {
			calls := 0
			opts := &Options{HTTPClient: &http.Client{Transport: tokenResponseTransport(func(r *http.Request) (*http.Response, error) {
				calls++
				return &http.Response{StatusCode: http.StatusTooManyRequests, Header: http.Header{"Retry-After": {header}}, Body: http.NoBody}, nil
			})}}
			config := &ProviderConfig{Type: ai.IssuerClaudeCode, TokenURL: "https://example.test/token"}
			_, err := NewManager().exchangeCodeForToken(context.Background(), config, "state", "code", "verifier", "http://localhost/callback", opts)
			if calls != 1 || !errors.Is(err, ErrTokenExchangeFailed) || !strings.Contains(err.Error(), "429") || !strings.Contains(err.Error(), want) {
				t.Fatalf("calls = %d, error = %v, want one request and %q", calls, err, want)
			}
		})
	}
}

func TestClaudeRefreshRequest(t *testing.T) {
	opts := &Options{HTTPClient: &http.Client{Transport: tokenResponseTransport(func(r *http.Request) (*http.Response, error) {
		if r.URL.String() != "https://platform.claude.com/v1/oauth/token" {
			t.Errorf("unexpected refresh URL %s", r.URL)
		}
		if r.Header.Get("User-Agent") != "axios/1.15.2" {
			t.Error("unexpected refresh User-Agent")
		}
		var body map[string]string
		if err := json.NewDecoder(r.Body).Decode(&body); err != nil {
			t.Fatal(err)
		}
		if _, exists := body["client_secret"]; exists {
			t.Error("public client refresh must omit client_secret")
		}
		if body["grant_type"] != "refresh_token" || body["refresh_token"] != "test-refresh" || body["client_id"] != "9d1c250a-e61b-44d9-88ed-5944d1962f5e" {
			t.Error("unexpected refresh parameters")
		}
		return &http.Response{StatusCode: http.StatusOK, Header: make(http.Header), Body: io.NopCloser(strings.NewReader(`{"access_token":"test-access","expires_in":3600}`))}, nil
	})}}
	if _, err := NewManager().refreshToken(context.Background(), ai.IssuerClaudeCode, "test-refresh", opts); err != nil {
		t.Fatal(err)
	}
}
