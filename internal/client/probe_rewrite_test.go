package client

import (
	"context"
	"encoding/json"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestApplyBodyOverrides(t *testing.T) {
	body := []byte(`{"model":"m","messages":[{"role":"user","content":"hi"}],"stream":true}`)

	out, err := ApplyBodyOverrides(body, map[string]json.RawMessage{
		"temperature":        json.RawMessage(`0.2`),
		"messages.0.content": json.RawMessage(`"edited"`),
		"stream":             json.RawMessage(`null`),
		"metadata":           json.RawMessage(`{"user_id":"u1"}`),
	})
	require.NoError(t, err)
	assert.JSONEq(t, `{"model":"m","messages":[{"role":"user","content":"edited"}],"temperature":0.2,"metadata":{"user_id":"u1"}}`, string(out))

	_, err = ApplyBodyOverrides(body, map[string]json.RawMessage{"x": json.RawMessage(`{not json`)})
	assert.Error(t, err)

	same, err := ApplyBodyOverrides(body, nil)
	require.NoError(t, err)
	assert.Equal(t, body, same)
}

func TestProbeRewriteRoundTripper(t *testing.T) {
	var gotBody string
	var gotHeader http.Header
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		b, _ := io.ReadAll(r.Body)
		gotBody = string(b)
		gotHeader = r.Header.Clone()
		w.WriteHeader(200)
	}))
	defer srv.Close()

	hc := &http.Client{Transport: &probeRewriteRoundTripper{inner: http.DefaultTransport}}
	rw := &ProbeRewrite{
		Body:    map[string]json.RawMessage{"temperature": json.RawMessage(`1`), "drop": json.RawMessage(`null`)},
		Headers: map[string]string{"X-Extra": "yes", "X-Gone": ""},
	}

	req, _ := http.NewRequestWithContext(WithProbeRewrite(context.Background(), rw), "POST", srv.URL, strings.NewReader(`{"model":"m","drop":1}`))
	req.Header.Set("X-Gone", "was-here")
	req.Header.Set("Content-Type", "application/json")
	resp, err := hc.Do(req)
	require.NoError(t, err)
	resp.Body.Close()

	assert.JSONEq(t, `{"model":"m","temperature":1}`, gotBody)
	assert.Equal(t, "yes", gotHeader.Get("X-Extra"))
	assert.Empty(t, gotHeader.Get("X-Gone"), "empty override removes the header")
	assert.Equal(t, "application/json", gotHeader.Get("Content-Type"))

	// No rewrite in ctx → untouched.
	req2, _ := http.NewRequest("POST", srv.URL, strings.NewReader(`{"model":"m","drop":1}`))
	resp2, err := hc.Do(req2)
	require.NoError(t, err)
	resp2.Body.Close()
	assert.JSONEq(t, `{"model":"m","drop":1}`, gotBody)
}
