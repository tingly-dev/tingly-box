package server

import (
	"bytes"
	"encoding/json"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/gin-gonic/gin"

	"github.com/tingly-dev/tingly-box/internal/config"
	"github.com/tingly-dev/tingly-box/vmodel/harnessupstream"
)

// newHarnessUpstreamTestServer boots a real Server with model auth stubbed out
// and serves its router over HTTP, so requests take the production /virtual
// route wiring.
func newHarnessUpstreamTestServer(t *testing.T, enabled bool) string {
	t.Helper()
	cfg, err := config.NewConfig(config.WithConfigDir(t.TempDir()))
	if err != nil {
		t.Fatalf("new config: %v", err)
	}
	s := NewServer(cfg,
		WithUI(false),
		WithModelAuthMiddleware(func(c *gin.Context) { c.Next() }),
		WithHarnessUpstream(enabled),
	)
	t.Cleanup(s.CloseVirtualModelServer)
	ts := httptest.NewServer(s.GetRouter())
	t.Cleanup(ts.Close)
	return ts.URL
}

func postJSON(t *testing.T, url string, body map[string]any, headers map[string]string) (int, string) {
	t.Helper()
	raw, _ := json.Marshal(body)
	req, _ := http.NewRequest(http.MethodPost, url, bytes.NewReader(raw))
	req.Header.Set("Content-Type", "application/json")
	for k, v := range headers {
		req.Header.Set(k, v)
	}
	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		t.Fatalf("POST %s: %v", url, err)
	}
	defer resp.Body.Close()
	out, _ := io.ReadAll(resp.Body)
	return resp.StatusCode, string(out)
}

func getCaptures(t *testing.T, url string) (int, harnessupstream.CapturesResponse) {
	t.Helper()
	resp, err := http.Get(url)
	if err != nil {
		t.Fatalf("GET %s: %v", url, err)
	}
	defer resp.Body.Close()
	var out harnessupstream.CapturesResponse
	if resp.StatusCode == http.StatusOK {
		if err := json.NewDecoder(resp.Body).Decode(&out); err != nil {
			t.Fatalf("decode captures: %v", err)
		}
	}
	return resp.StatusCode, out
}

func chatBody(model string, stream bool) map[string]any {
	return map[string]any{
		"model":    model,
		"stream":   stream,
		"messages": []map[string]any{{"role": "user", "content": "What is the capital of France?"}},
	}
}

func anthropicBody(model string, stream bool) map[string]any {
	return map[string]any{
		"model":      model,
		"max_tokens": 64,
		"stream":     stream,
		"messages":   []map[string]any{{"role": "user", "content": "What is the capital of France?"}},
	}
}

func TestHarnessUpstream_ServesScenarioFixtures(t *testing.T) {
	base := newHarnessUpstreamTestServer(t, true)
	text := harnessupstream.ModelID("text")

	cases := []struct {
		name, path string
		body       map[string]any
		want       []string
	}{
		{"openai chat", "/virtual/openai/v1/chat/completions", chatBody(text, false), []string{"Paris", `"chat.completion"`}},
		{"openai chat stream", "/virtual/openai/v1/chat/completions", chatBody(text, true), []string{"data:", "Paris"}},
		{"openai responses", "/virtual/openai/v1/responses", map[string]any{"model": text, "input": "hi"}, []string{"Paris", `"output_text"`}},
		{"anthropic", "/virtual/anthropic/v1/messages", anthropicBody(text, false), []string{"Paris", `"message"`}},
		{"anthropic stream", "/virtual/anthropic/v1/messages", anthropicBody(text, true), []string{"event: message_start", "Paris"}},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			status, body := postJSON(t, base+tc.path, tc.body, nil)
			if status != http.StatusOK {
				t.Fatalf("status = %d, body = %s", status, body)
			}
			for _, w := range tc.want {
				if !strings.Contains(body, w) {
					t.Errorf("response missing %q:\n%s", w, body)
				}
			}
		})
	}
}

func TestHarnessUpstream_ErrorScenarioKeepsStatus(t *testing.T) {
	base := newHarnessUpstreamTestServer(t, true)
	status, body := postJSON(t, base+"/virtual/openai/v1/chat/completions", chatBody(harnessupstream.ModelID("error-500"), false), nil)
	if status != http.StatusInternalServerError {
		t.Fatalf("status = %d, want 500; body = %s", status, body)
	}
}

func TestHarnessUpstream_OtherModelsFallThrough(t *testing.T) {
	base := newHarnessUpstreamTestServer(t, true)
	status, body := postJSON(t, base+"/virtual/openai/v1/chat/completions", chatBody("virtual-gpt-4", false), nil)
	if status != http.StatusOK {
		t.Fatalf("status = %d, body = %s", status, body)
	}
	if strings.Contains(body, "Paris") {
		t.Fatalf("virtual-gpt-4 was answered by a harness fixture: %s", body)
	}
}

func TestHarnessUpstream_Captures(t *testing.T) {
	base := newHarnessUpstreamTestServer(t, true)
	text := harnessupstream.ModelID("text")

	postJSON(t, base+"/virtual/anthropic/v1/messages", anthropicBody(text, false), map[string]string{
		harnessupstream.RequestIDHeader: "req-1",
		"X-Api-Key":                     "secret",
	})
	postJSON(t, base+"/virtual/openai/v1/chat/completions", chatBody("virtual-gpt-4", false), map[string]string{
		harnessupstream.RequestIDHeader: "req-2",
	})

	status, got := getCaptures(t, base+"/virtual/captures?request_id=req-1")
	if status != http.StatusOK {
		t.Fatalf("captures status = %d", status)
	}
	if len(got.Captures) != 1 {
		t.Fatalf("captures for req-1 = %d, want 1", len(got.Captures))
	}
	c := got.Captures[0]
	if c.Path != "/virtual/anthropic/v1/messages" || c.Model != text {
		t.Errorf("capture = %s %s, want /virtual/anthropic/v1/messages %s", c.Path, c.Model, text)
	}
	if v := c.Headers.Get("X-Api-Key"); v != "[redacted]" {
		t.Errorf("X-Api-Key = %q, want redacted", v)
	}
	var body map[string]any
	if err := json.Unmarshal(c.Body, &body); err != nil || body["max_tokens"] != float64(64) {
		t.Errorf("captured body = %s (err %v)", c.Body, err)
	}

	// Requests to non-harness models are recorded too: a gateway that rewrote
	// the model name is exactly what the capture should reveal.
	if _, got := getCaptures(t, base+"/virtual/captures?model=virtual-gpt-4"); len(got.Captures) != 1 {
		t.Errorf("captures for virtual-gpt-4 = %d, want 1", len(got.Captures))
	}
	if _, got := getCaptures(t, base+"/virtual/captures?limit=1"); len(got.Captures) != 1 || got.Captures[0].RequestID != "req-2" {
		t.Errorf("limit=1 should return only the newest capture, got %+v", got.Captures)
	}

	req, _ := http.NewRequest(http.MethodDelete, base+"/virtual/captures", nil)
	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		t.Fatalf("DELETE captures: %v", err)
	}
	resp.Body.Close()
	if _, got := getCaptures(t, base+"/virtual/captures"); len(got.Captures) != 0 {
		t.Errorf("captures after reset = %d, want 0", len(got.Captures))
	}
}

func TestHarnessUpstream_OffByDefault(t *testing.T) {
	base := newHarnessUpstreamTestServer(t, false)
	if status, _ := getCaptures(t, base+"/virtual/captures"); status != http.StatusNotFound {
		t.Errorf("/virtual/captures status = %d, want 404 when disabled", status)
	}
	_, body := postJSON(t, base+"/virtual/openai/v1/chat/completions", chatBody(harnessupstream.ModelID("text"), false), nil)
	if strings.Contains(body, "Paris") {
		t.Errorf("harness-text answered with the scenario fixture while disabled: %s", body)
	}
}
