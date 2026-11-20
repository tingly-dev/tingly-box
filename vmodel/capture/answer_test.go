package capture_test

import (
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/tingly-dev/tingly-box/vmodel/capture"
	"github.com/tingly-dev/tingly-box/vmodel/spec"
)

// claudeCodeBody has the shape of a Claude Code 2.1.328 `-p` request
// (.research/claude-code/2.1.328.md §9.4): adaptive thinking with display,
// output_config, context_management, diagnostics, scoped 1h cache_control and
// a billing-header system block. The Anthropic handler binds the body into an
// SDK-typed struct, so this is the check that new client fields do not turn
// into a 400 for a capture.
const claudeCodeBody = `{
 "model":"claude-sonnet-4-6",
 "messages":[{"role":"user","content":[
   {"type":"text","text":"<system-reminder>env</system-reminder>"},
   {"type":"text","text":"say hi"}]}],
 "system":[
   {"type":"text","text":"x-anthropic-billing-header: cc_version=2.1.328.593; cc_entrypoint=sdk-cli; cch=00000;"},
   {"type":"text","text":"You are a Claude agent, built on Anthropic's Claude Agent SDK.","cache_control":{"type":"ephemeral","ttl":"1h"}},
   {"type":"text","text":"main prompt","cache_control":{"type":"ephemeral","ttl":"1h","scope":"global"}}],
 "tools":[{"name":"Bash","description":"run","input_schema":{"type":"object","properties":{"command":{"type":"string"}},"required":["command"]}},
          {"name":"DeferredToolPlaceholder","description":"","input_schema":{"type":"object","properties":{}}}],
 "metadata":{"user_id":"{\"device_id\":\"abc\",\"account_uuid\":\"\",\"session_id\":\"s\"}"},
 "max_tokens":32000,
 "thinking":{"type":"adaptive","display":"omitted"},
 "context_management":{"edits":[{"type":"clear_thinking_20251015","keep":"all"}]},
 "output_config":{"effort":"high"},
 "diagnostics":{"previous_message_id":null},
 "stream":true}`

func captureServer(t *testing.T) (*httptest.Server, func(string) bool) {
	t.Helper()
	defs, err := spec.Decode([]byte("- {id: known, kind: mock, mock: {content: known answer}}\n- {id: " + capture.DefaultModel + ", kind: mock, mock: {content: fallback answer}}\n"))
	if err != nil {
		t.Fatal(err)
	}
	rt, err := spec.New(defs)
	if err != nil {
		t.Fatal(err)
	}
	ids := map[string]bool{}
	for _, id := range rt.IDs() {
		ids[id] = true
	}
	known := func(id string) bool { return id != capture.DefaultModel && ids[id] }
	srv := httptest.NewServer(capture.Answer(rt, known))
	t.Cleanup(srv.Close)
	return srv, known
}

func post(t *testing.T, url, body string) (int, string) {
	t.Helper()
	resp, err := http.Post(url, "application/json", strings.NewReader(body))
	if err != nil {
		t.Fatal(err)
	}
	defer resp.Body.Close()
	b, _ := io.ReadAll(resp.Body)
	return resp.StatusCode, string(b)
}

func TestUnknownModelsAreAnsweredByDefault(t *testing.T) {
	srv, _ := captureServer(t)
	for _, tc := range []struct{ name, path, body, want string }{
		{"claude code body, stream", "/v1/messages?beta=true", claudeCodeBody, "fallback answer"},
		{"anthropic plain", "/v1/messages", `{"model":"claude-haiku-4-5","max_tokens":8,"messages":[{"role":"user","content":"hi"}]}`, "fallback answer"},
		{"openai chat", "/v1/chat/completions", `{"model":"gpt-x","messages":[{"role":"user","content":"hi"}]}`, "fallback answer"},
		{"openai responses", "/v1/responses", `{"model":"gpt-x","input":"hi"}`, "fallback answer"},
		{"known model untouched", "/v1/messages", `{"model":"known","max_tokens":8,"messages":[{"role":"user","content":"hi"}]}`, "known answer"},
	} {
		t.Run(tc.name, func(t *testing.T) {
			code, body := post(t, srv.URL+tc.path, tc.body)
			if code != 200 || !strings.Contains(body, tc.want) {
				t.Fatalf("%d %s", code, body)
			}
		})
	}
}

func TestAuxiliaryEndpoints(t *testing.T) {
	srv, _ := captureServer(t)
	code, body := post(t, srv.URL+"/v1/messages/count_tokens", `{"model":"m","messages":[{"role":"user","content":"hello world, how are you today"}]}`)
	if code != 200 || !strings.Contains(body, `"input_tokens"`) {
		t.Fatalf("count_tokens: %d %s", code, body)
	}
	for _, p := range []string{"/api/event_logging/v2/batch", "/api/claude_code/metrics", "/api/eval/sdk-key", "/v1/messages/cache_touch"} {
		if code, body := post(t, srv.URL+p, `{}`); code != 200 || strings.TrimSpace(body) != "{}" {
			t.Fatalf("%s: %d %s", p, code, body)
		}
	}
	// /api/oauth/* is not simulated: it keeps the protocol-shaped 501.
	if code, _ := post(t, srv.URL+"/api/oauth/org", `{}`); code != http.StatusNotImplemented {
		t.Fatalf("oauth: %d", code)
	}
}
