package client

import (
	"context"
	"errors"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/openai/openai-go/v3"
	"github.com/openai/openai-go/v3/option"
	"github.com/stretchr/testify/require"

	"github.com/tingly-dev/tingly-box/internal/protocol"
	"github.com/tingly-dev/tingly-box/internal/typ"
)

func newDecisionTestClient(t *testing.T, base string) *OpenAIClient {
	t.Helper()
	c, err := NewOpenAIClient(&typ.Provider{
		Name:     "decision-test",
		APIBase:  base,
		APIStyle: protocol.APIStyleOpenAI,
		AuthType: typ.AuthTypeAPIKey,
		Token:    "sk-test",
	}, "m", typ.SessionID{})
	require.NoError(t, err)
	t.Cleanup(func() { _ = c.Close() })
	return c
}

// The body travels byte-for-byte to {APIBase}/decisions with the provider's
// bearer token, and the raw JSON answer comes back untouched.
func TestDecisionsNew_Passthrough(t *testing.T) {
	var gotPath, gotAuth, gotCT string
	var gotBody []byte
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		gotPath, gotAuth, gotCT = r.URL.Path, r.Header.Get("Authorization"), r.Header.Get("Content-Type")
		gotBody, _ = io.ReadAll(r.Body)
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(`{"answer":"b",  "usage":{"prompt_tokens":3}}`))
	}))
	defer srv.Close()

	req := []byte(`{"model":"luna","options":["a","b"], "context":"x"}`)
	out, err := newDecisionTestClient(t, srv.URL+"/v1").DecisionsNew(context.Background(), req)
	require.NoError(t, err)
	require.Equal(t, "/v1/decisions", gotPath)
	require.Equal(t, "Bearer sk-test", gotAuth)
	require.Equal(t, "application/json", gotCT)
	require.Equal(t, req, gotBody)
	require.Equal(t, `{"answer":"b",  "usage":{"prompt_tokens":3}}`, string(out))
}

// Upstream failures surface as *openai.Error carrying the HTTP status, which
// the gateway maps back to the caller.
func TestDecisionsNew_UpstreamError(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusForbidden)
		_, _ = w.Write([]byte(`{"error":{"message":"not enabled","type":"invalid_request_error"}}`))
	}))
	defer srv.Close()

	_, err := newDecisionTestClient(t, srv.URL+"/v1").DecisionsNew(context.Background(), []byte(`{"model":"luna"}`))
	var apiErr *openai.Error
	require.True(t, errors.As(err, &apiErr), "got %T: %v", err, err)
	require.Equal(t, http.StatusForbidden, apiErr.StatusCode)
}

// The SDK must actually send to the vendor-specific URL (resolved through the
// real request path, not just the helper).
func TestDecisionsTarget_SDKURL(t *testing.T) {
	for base, want := range map[string]string{
		"https://api.openai.com/v1":    "https://api.openai.com/v1/decisions",
		"https://api.typesafe.ai":      "https://api.typesafe.ai/v1/systemone",
		"https://api.typesafe.ai/v1":   "https://api.typesafe.ai/v1/systemone",
		"https://openrouter.ai/api/v1": "https://openrouter.ai/api/alpha/decisions",
		"::bad":                        "",
	} {
		path, override := decisionsTarget(base)
		var got string
		c := openai.NewClient(
			option.WithBaseURL(base), option.WithAPIKey("k"),
			option.WithHTTPClient(&http.Client{Transport: rtPtr(func(r *http.Request) (*http.Response, error) {
				got = r.URL.String()
				return &http.Response{StatusCode: 200, Header: http.Header{"Content-Type": {"application/json"}}, Body: io.NopCloser(strings.NewReader(`{}`))}, nil
			})}))
		var out []byte
		_ = c.Post(context.Background(), path, nil, &out, override...)
		if base == "::bad" {
			require.Equal(t, "decisions", path)
			continue
		}
		require.Equal(t, want, got, base)
	}
}

func rtPtr(f func(*http.Request) (*http.Response, error)) http.RoundTripper {
	rt := roundTripFunc(f)
	return &rt
}
