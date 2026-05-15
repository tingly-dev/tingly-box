package client

import (
	"bytes"
	"encoding/json"
	"io"
	"net/http"
	"strings"
	"testing"
)

// fakeRoundTripper captures the outbound request and returns a canned response.
type fakeRoundTripper struct {
	captured *http.Request
	body     []byte
	resp     *http.Response
}

func (f *fakeRoundTripper) RoundTrip(req *http.Request) (*http.Response, error) {
	f.captured = req
	if req.Body != nil {
		b, err := io.ReadAll(req.Body)
		if err != nil {
			return nil, err
		}
		f.body = b
	}
	resp := f.resp
	if resp == nil {
		resp = &http.Response{
			StatusCode: http.StatusOK,
			Header:     http.Header{},
			Body:       io.NopCloser(strings.NewReader(`{"response":{"candidates":[]}}`)),
		}
	}
	return resp, nil
}

// TestGeminiRoundTripper_WrapsGenerateContent verifies that requests to the
// standard Google genai path get rewritten to /v1internal:<op>, that the body
// is wrapped with project/model/user_prompt_id, and that the Code Assist
// "response" envelope is unwrapped on the way back.
func TestGeminiRoundTripper_WrapsGenerateContent(t *testing.T) {
	innerBody := map[string]any{
		"model": "gemini-2.5-pro",
		"contents": []map[string]any{
			{"role": "user", "parts": []map[string]any{{"text": "hi"}}},
		},
	}
	rawInner, _ := json.Marshal(innerBody)

	req, err := http.NewRequest("POST",
		"https://cloudcode-pa.googleapis.com/v1beta/models/gemini-2.5-pro:generateContent",
		bytes.NewReader(rawInner))
	if err != nil {
		t.Fatalf("build request: %v", err)
	}
	req.Header.Set("X-Goog-Api-Key", "ya29.test-access-token")
	req.ContentLength = int64(len(rawInner))

	fake := &fakeRoundTripper{}
	rt := &geminiRoundTripper{RoundTripper: fake, project: "my-project"}

	resp, err := rt.RoundTrip(req)
	if err != nil {
		t.Fatalf("RoundTrip returned error: %v", err)
	}
	defer resp.Body.Close()

	if fake.captured.URL.Path != "/v1internal:generateContent" {
		t.Errorf("expected path rewrite to /v1internal:generateContent, got %s", fake.captured.URL.Path)
	}

	if got := fake.captured.Header.Get("Authorization"); got != "Bearer ya29.test-access-token" {
		t.Errorf("expected Bearer auth from X-Goog-Api-Key swap, got %q", got)
	}
	if got := fake.captured.Header.Get("X-Goog-Api-Key"); got != "" {
		t.Errorf("expected X-Goog-Api-Key stripped, got %q", got)
	}
	if got := fake.captured.Header.Get("Content-Type"); got != "application/json" {
		t.Errorf("expected Content-Type application/json, got %q", got)
	}

	var wrapped map[string]any
	if err := json.Unmarshal(fake.body, &wrapped); err != nil {
		t.Fatalf("body is not valid JSON: %v\nbody=%s", err, string(fake.body))
	}
	if wrapped["model"] != "gemini-2.5-pro" {
		t.Errorf("expected wrapped.model=gemini-2.5-pro, got %v", wrapped["model"])
	}
	if wrapped["project"] != "my-project" {
		t.Errorf("expected wrapped.project=my-project, got %v", wrapped["project"])
	}
	if _, ok := wrapped["user_prompt_id"].(string); !ok {
		t.Error("expected wrapped.user_prompt_id to be a string uuid")
	}
	inner, ok := wrapped["request"].(map[string]any)
	if !ok {
		t.Fatalf("expected wrapped.request to be a JSON object, got %T", wrapped["request"])
	}
	if _, has := inner["model"]; has {
		t.Error("expected inner request to have model stripped (lifted to top level)")
	}
	if _, has := inner["contents"]; !has {
		t.Error("expected inner request to retain contents")
	}

	out, err := io.ReadAll(resp.Body)
	if err != nil {
		t.Fatalf("read response body: %v", err)
	}
	var unwrapped map[string]any
	if err := json.Unmarshal(out, &unwrapped); err != nil {
		t.Fatalf("response not JSON: %v\nbody=%s", err, string(out))
	}
	if _, has := unwrapped["response"]; has {
		t.Errorf("expected Code Assist response wrapper to be stripped, got %s", string(out))
	}
	if _, has := unwrapped["candidates"]; !has {
		t.Errorf("expected unwrapped response to include candidates, got %s", string(out))
	}
}

// TestGeminiRoundTripper_NoProjectIDSkipsWrap ensures that when project_id
// hasn't been discovered yet, we don't malform the body — we still rewrite the
// URL and swap auth, but leave the payload alone so the caller sees a clean
// upstream error rather than a partially-formed envelope.
func TestGeminiRoundTripper_NoProjectIDSkipsWrap(t *testing.T) {
	raw := []byte(`{"model":"gemini-2.5-pro","contents":[]}`)
	req, _ := http.NewRequest("POST",
		"https://cloudcode-pa.googleapis.com/v1beta/models/gemini-2.5-pro:generateContent",
		bytes.NewReader(raw))
	req.Header.Set("X-Goog-Api-Key", "tok")
	req.ContentLength = int64(len(raw))

	fake := &fakeRoundTripper{}
	rt := &geminiRoundTripper{RoundTripper: fake, project: ""}

	if _, err := rt.RoundTrip(req); err != nil {
		t.Fatalf("RoundTrip returned error: %v", err)
	}

	if !bytes.Equal(fake.body, raw) {
		t.Errorf("expected body unchanged when project is empty, got %s", string(fake.body))
	}
	if fake.captured.URL.Path != "/v1internal:generateContent" {
		t.Errorf("expected path rewrite even without project, got %s", fake.captured.URL.Path)
	}
}
