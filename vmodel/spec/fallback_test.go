package spec_test

import (
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
)

// serve runs r behind a real listener: streaming handlers need a connection
// that httptest.ResponseRecorder does not provide.
func serve(t *testing.T, r http.Handler) func(path, body string) (int, []byte) {
	t.Helper()
	srv := httptest.NewServer(r)
	t.Cleanup(srv.Close)
	return func(path, body string) (int, []byte) {
		t.Helper()
		resp, err := http.Post(srv.URL+path, "application/json", strings.NewReader(body))
		if err != nil {
			t.Fatal(err)
		}
		defer resp.Body.Close()
		b, _ := io.ReadAll(resp.Body)
		return resp.StatusCode, b
	}
}

// claudeCodeBody has the shape of a Claude Code 2.1.328 `-p` request
// (.research/claude-code/2.1.328.md §9.4): adaptive thinking with display,
// output_config, context_management, diagnostics, scoped 1h cache_control and
// a billing-header system block. The Anthropic handler binds the body into an
// SDK-typed struct, so this checks that new client fields do not become a 400.
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

const fallbackDefs = `
- {id: known, kind: mock, mock: {content: known answer}}
- {id: catchall, kind: mock, mock: {content: fallback answer}}
- id: chat-only
  kind: mock
  protocols: [openai_chat]
  mock: {content: chat only}
`

func TestFallbackServesUnknownModels(t *testing.T) {
	r := compile(t, fallbackDefs)
	if err := r.SetFallback("catchall"); err != nil {
		t.Fatal(err)
	}
	do := serve(t, r)
	for _, tc := range []struct{ name, path, body, want string }{
		{"claude code body", "/v1/messages?beta=true", claudeCodeBody, "fallback answer"},
		{"anthropic non-stream", "/v1/messages", `{"model":"claude-haiku-4-5","max_tokens":8,"messages":[{"role":"user","content":"hi"}]}`, "fallback answer"},
		{"openai chat", "/v1/chat/completions", `{"model":"gpt-x","messages":[{"role":"user","content":"hi"}]}`, "fallback answer"},
		{"openai responses", "/v1/responses", `{"model":"gpt-x","input":"hi"}`, "fallback answer"},
		{"defined model is not shadowed", "/v1/messages", `{"model":"known","max_tokens":8,"messages":[{"role":"user","content":"hi"}]}`, "known answer"},
	} {
		t.Run(tc.name, func(t *testing.T) {
			code, body := do(tc.path, tc.body)
			if code != 200 || !strings.Contains(string(body), tc.want) {
				t.Fatalf("%d %s", code, body)
			}
		})
	}

	// The reply echoes the id the client asked for, not the fallback's.
	_, body := do("/v1/messages", `{"model":"claude-haiku-4-5","max_tokens":8,"messages":[{"role":"user","content":"hi"}]}`)
	if !strings.Contains(string(body), `"model":"claude-haiku-4-5"`) {
		t.Fatalf("model not echoed: %s", body)
	}
}

func TestFallbackIsOptInAndNotAdvertised(t *testing.T) {
	r := compile(t, fallbackDefs)
	do := serve(t, r)
	if code, _ := do("/v1/chat/completions", `{"model":"gpt-x","messages":[]}`); code != http.StatusNotFound {
		t.Fatalf("unknown model without fallback: %d", code)
	}
	if err := r.SetFallback("catchall"); err != nil {
		t.Fatal(err)
	}
	// Fallback is not a listed model of its own accord: ids stay the defined ones.
	if got := strings.Join(r.IDs(), ","); got != "catchall,chat-only,known" {
		t.Fatalf("ids: %s", got)
	}
}

func TestFallbackRespectsProtocolsAndDefinitions(t *testing.T) {
	r := compile(t, fallbackDefs)
	do := serve(t, r)
	if err := r.SetFallback("nope"); err == nil {
		t.Fatal("undefined fallback accepted")
	}
	if err := r.SetFallback("chat-only"); err != nil {
		t.Fatal(err)
	}
	if code, body := do("/v1/chat/completions", `{"model":"gpt-x","messages":[]}`); code != 200 || !strings.Contains(string(body), "chat only") {
		t.Fatalf("chat: %d %s", code, body)
	}
	// A fallback that does not speak the protocol is a 404, not another model.
	if code, _ := do("/v1/messages", `{"model":"claude-x","max_tokens":8,"messages":[{"role":"user","content":"hi"}]}`); code != http.StatusNotFound {
		t.Fatalf("anthropic via chat-only fallback: %d", code)
	}
}
