package oauth

import (
	"context"
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
