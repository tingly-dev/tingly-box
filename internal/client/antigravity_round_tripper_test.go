package client

import (
	"encoding/json"
	"io"
	"net/http"
	"strings"
	"testing"

	"github.com/tingly-dev/tingly-box/internal/typ"
)

func TestAntigravityRoundTripper_Envelope(t *testing.T) {
	for _, tc := range []struct{ name, body, requestType string }{
		{"text", `{"model":"ignored","contents":[{"parts":[{"text":"hi"}]}]}`, ""},
		{"tools", `{"tools":[{"functionDeclarations":[{"name":"lookup"}]}]}`, "agent"},
		{"call history", `{"contents":[{"parts":[{"functionCall":{"name":"lookup"}}]}]}`, "agent"},
		{"result history", `{"contents":[{"parts":[{"functionResponse":{"name":"lookup","response":{"id":9007199254740993}}}]}]}`, "agent"},
		{"empty tools", `{"tools":[]}`, ""},
		{"image", `{"generationConfig":{"responseModalities":["TEXT","IMAGE"]},"tools":[{}]}`, "image_gen"},
	} {
		t.Run(tc.name, func(t *testing.T) {
			req, err := http.NewRequest("POST", "https://cloudcode-pa.googleapis.com/v1beta/models/gemini-test:generateContent?custom=1", strings.NewReader(tc.body))
			if err != nil {
				t.Fatal(err)
			}
			req.Header.Set("X-Goog-Api-Key", "token")
			req.Header.Set("X-Goog-Api-Client", "sdk")
			req.Header.Set("X-Goog-User-Project", "quota-project")
			req.Header.Set("User-Agent", "sdk-agent")
			req.Header.Set("Anthropic-Beta", "feature")
			req.Header.Set("Accept", "application/json")
			originalURL := req.URL.String()
			fake := &fakeRoundTripper{}
			rt := &antigravityRoundTripper{RoundTripper: fake, project: "project"}
			resp, err := rt.RoundTrip(req)
			if err != nil {
				t.Fatal(err)
			}
			defer resp.Body.Close()
			if req.URL.String() != originalURL || req.Header.Get("User-Agent") != "sdk-agent" || req.Header.Get("X-Goog-Api-Key") != "token" {
				t.Fatal("mutated caller URL or headers")
			}
			out := fake.captured
			if out.URL.Path != "/v1internal:generateContent" || out.URL.RawQuery != "custom=1" {
				t.Fatalf("URL = %s", out.URL)
			}
			for key, want := range map[string]string{"Authorization": "Bearer token", "X-Goog-Api-Key": "", "X-Goog-Api-Client": "", "X-Goog-User-Project": "", "Anthropic-Beta": "feature", "Accept": "application/json"} {
				if got := out.Header.Get(key); got != want {
					t.Errorf("%s = %q, want %q", key, got, want)
				}
			}
			var envelope struct {
				Project     string                     `json:"project"`
				Model       string                     `json:"model"`
				RequestID   string                     `json:"requestId"`
				RequestType string                     `json:"requestType"`
				Request     map[string]json.RawMessage `json:"request"`
			}
			if err := json.Unmarshal(fake.body, &envelope); err != nil {
				t.Fatal(err)
			}
			if envelope.Project != "project" || envelope.Model != "gemini-test" || !strings.HasPrefix(envelope.RequestID, "agent/") || envelope.RequestType != tc.requestType {
				t.Fatalf("envelope = %+v", envelope)
			}
			if parts := strings.Split(envelope.RequestID, "/"); len(parts) != 5 {
				t.Fatalf("expected five request ID segments: %q", envelope.RequestID)
			}
			if _, ok := envelope.Request["model"]; ok {
				t.Fatal("inner model was not removed")
			}
			if strings.Contains(tc.body, "9007199254740993") && !strings.Contains(string(fake.body), "9007199254740993") {
				t.Fatal("tool result number lost precision")
			}
			replay, err := out.GetBody()
			if err != nil {
				t.Fatal(err)
			}
			defer replay.Close()
			raw, _ := io.ReadAll(replay)
			if string(raw) != string(fake.body) || out.ContentLength != int64(len(raw)) {
				t.Fatal("replay body or content length mismatch")
			}
			body, _ := io.ReadAll(resp.Body)
			if string(body) != `{"candidates":[]}` || resp.Header.Get("Content-Length") != "17" {
				t.Fatalf("response = %s, length = %s", body, resp.Header.Get("Content-Length"))
			}
		})
	}
}

func TestAntigravityConversationID_Isolation(t *testing.T) {
	session := typ.SessionID{Source: typ.SessionSourceUser, Value: "private-user-session", IPBackup: "192.0.2.1"}
	a := antigravityConversationID("provider-a", session)
	if a != antigravityConversationID("provider-a", session) {
		t.Fatal("conversation changed for the same provider and session")
	}
	if a == antigravityConversationID("provider-b", session) || strings.Contains(a, session.Value) {
		t.Fatal("conversation does not isolate providers or conceal the session")
	}
	session.Value = "other-session"
	if a == antigravityConversationID("provider-a", session) {
		t.Fatal("different sessions share a conversation")
	}
	for _, session := range []typ.SessionID{{}, {Source: typ.SessionSourceIP, Value: "192.0.2.1"}} {
		if antigravityConversationID("p", session) == antigravityConversationID("p", session) {
			t.Fatal("requests without a real session must not share a conversation")
		}
	}
}

func TestAntigravityRoundTripper_InvalidRequests(t *testing.T) {
	for _, tc := range []struct{ name, project, body string }{
		{"missing project", "", `{}`}, {"invalid JSON", "p", `{`}, {"null", "p", `null`}, {"array", "p", `[]`},
	} {
		t.Run(tc.name, func(t *testing.T) {
			req, _ := http.NewRequest("POST", "https://cloudcode-pa.googleapis.com/v1beta/models/test:generateContent", strings.NewReader(tc.body))
			fake := &fakeRoundTripper{}
			rt := &antigravityRoundTripper{RoundTripper: fake, project: tc.project}
			if _, err := rt.RoundTrip(req); err == nil {
				t.Fatal("expected error")
			}
			if fake.captured != nil {
				t.Fatal("invalid request reached upstream")
			}
		})
	}
}

func TestAntigravityRoundTripper_StreamAndErrors(t *testing.T) {
	for _, status := range []int{200, 401, 403, 429, 503} {
		input := "data: {\"response\":{\"text\":\"hi\"}}\n\n"
		if status != 200 {
			input = `{"error":{"code":429,"message":"retry later"}}`
		}
		fake := &fakeRoundTripper{resp: &http.Response{StatusCode: status, Header: http.Header{"Content-Length": {"999"}, "Retry-After": {"30"}}, ContentLength: 999, Body: io.NopCloser(strings.NewReader(input))}}
		req, _ := http.NewRequest("POST", "https://cloudcode-pa.googleapis.com/v1beta/models/test:streamGenerateContent?alt=sse", strings.NewReader(`{}`))
		req.Header.Set("Authorization", "Bearer existing")
		rt := &antigravityRoundTripper{RoundTripper: fake, project: "p"}
		resp, err := rt.RoundTrip(req)
		if err != nil {
			t.Fatal(err)
		}
		body, err := io.ReadAll(resp.Body)
		resp.Body.Close()
		if err != nil {
			t.Fatal(err)
		}
		want := input
		if status == 200 {
			want = "data: {\"text\":\"hi\"}\n\n"
			if resp.ContentLength != -1 || resp.Header.Get("Content-Length") != "" {
				t.Fatal("stale stream content length")
			}
		}
		if string(body) != want || resp.Header.Get("Retry-After") != "30" || fake.captured.Header.Get("Authorization") != "Bearer existing" {
			t.Fatalf("status %d: body %q, headers %v", status, body, resp.Header)
		}
	}
}
