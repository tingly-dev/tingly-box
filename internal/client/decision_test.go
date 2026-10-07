package client

import (
	"context"
	"errors"
	"io"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/openai/openai-go/v3"
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
