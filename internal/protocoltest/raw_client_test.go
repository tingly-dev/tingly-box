package protocoltest

import (
	"bytes"
	"io"
	"net/http"
	"testing"
)

// sendRaw posts body to the gateway over real HTTP and returns the status and
// the full response body (JSON or the raw SSE stream).
func sendRaw(t *testing.T, env *TestEnv, path string, body []byte) (int, string) {
	t.Helper()
	return sendRawWithHeaders(t, env, path, body, nil)
}

// sendRawWithHeaders is sendRaw with extra request headers (e.g. a session id).
func sendRawWithHeaders(t *testing.T, env *TestEnv, path string, body []byte, headers map[string]string) (int, string) {
	t.Helper()
	req, err := http.NewRequest(http.MethodPost, env.GatewayURL()+path, bytes.NewReader(body))
	if err != nil {
		t.Fatalf("new request: %v", err)
	}
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("Authorization", "Bearer "+env.ModelToken())
	for k, v := range headers {
		req.Header.Set(k, v)
	}
	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		t.Fatalf("do request: %v", err)
	}
	defer resp.Body.Close()
	raw, err := io.ReadAll(resp.Body)
	if err != nil {
		t.Fatalf("read response: %v", err)
	}
	return resp.StatusCode, string(raw)
}
