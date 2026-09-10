package client

import (
	"context"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestProbeHeaderOverridesRoundTripper(t *testing.T) {
	var gotHeader http.Header
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		gotHeader = r.Header.Clone()
		w.WriteHeader(200)
	}))
	defer srv.Close()

	hc := &http.Client{Transport: &probeHeaderOverridesRoundTripper{inner: http.DefaultTransport}}
	ctx := WithProbeHeaderOverrides(context.Background(), map[string]string{"X-Extra": "yes", "X-Gone": ""})

	req, _ := http.NewRequestWithContext(ctx, "POST", srv.URL, strings.NewReader(`{}`))
	req.Header.Set("X-Gone", "was-here")
	req.Header.Set("Content-Type", "application/json")
	resp, err := hc.Do(req)
	require.NoError(t, err)
	resp.Body.Close()
	assert.Equal(t, "yes", gotHeader.Get("X-Extra"))
	assert.Empty(t, gotHeader.Get("X-Gone"), "empty override removes the header")
	assert.Equal(t, "application/json", gotHeader.Get("Content-Type"))

	// No overrides in ctx → untouched.
	req2, _ := http.NewRequest("POST", srv.URL, strings.NewReader(`{}`))
	req2.Header.Set("X-Gone", "still-here")
	resp2, err := hc.Do(req2)
	require.NoError(t, err)
	resp2.Body.Close()
	assert.Equal(t, "still-here", gotHeader.Get("X-Gone"))
}
