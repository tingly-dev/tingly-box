// Package e2e holds the Managed Agent end-to-end journeys: the real
// tingly-box server (StoreManager, event logs, Launcher wiring, gateway
// routing) driven over HTTP exactly as the web UI and IM drive it, with the
// real `claude` CLI, real git, and a scripted or virtual upstream in place of
// Anthropic. Where the unit tests in ../ validate pieces in isolation, these
// validate the user journeys the feature promises.
//
// Opt-in, because they need the CLI and take a few minutes:
//
//	task test:e2e:agent
//	TB_MANAGED_AGENT_E2E=1 go test -count=1 -v ./internal/managedagent/e2e/ -timeout 30m
//
// Each journey boots its own isolated stack (temp config dir, own ports), so
// they can run in any order and never share state. See
// .claude/skills/managed-agent-e2e/SKILL.md for the plan and how to extend it.
package e2e_test

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/http/httptest"
	"net/url"
	"os"
	"os/exec"
	"path/filepath"
	"strconv"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/tingly-dev/tingly-box/agentboot/claude"
	"github.com/tingly-dev/tingly-box/internal/managedagent"
	"github.com/tingly-dev/tingly-box/internal/protocoltest"
)

const turnTimeout = 3 * time.Minute

// requireE2E skips unless the journey switch and its tools are present.
func requireE2E(t *testing.T) {
	t.Helper()
	if os.Getenv("TB_MANAGED_AGENT_E2E") == "" {
		t.Skip("set TB_MANAGED_AGENT_E2E=1 to run the managed-agent journeys")
	}
	if _, err := exec.LookPath("git"); err != nil {
		t.Skip("git not installed")
	}
	if _, err := claude.FindClaudeCLI(context.Background()); err != nil {
		t.Skipf("claude CLI not found: %v", err)
	}
}

// TestMain: the CLI refuses bypassPermissions as root unless IS_SANDBOX is
// exactly "1" (other truthy spellings do not count). CI containers are root;
// the journeys are the sandbox. Set before anything snapshots the environment.
func TestMain(m *testing.M) {
	if os.Getenv("TB_MANAGED_AGENT_E2E") != "" && os.Geteuid() == 0 {
		os.Setenv("IS_SANDBOX", "1")
	}
	os.Exit(m.Run())
}

// stack is one booted tingly-box with an API client bound to its user token.
type stack struct {
	t        *testing.T
	env      *protocoltest.AgentTestEnv
	base     string
	token    string
	upstream *scriptedUpstream // nil when the virtual server is used
}

// bootStack starts an isolated server. With script == nil the CLI is routed
// to the harness's virtual upstream (fixed text answer containing
// protocoltest.VirtualMockAnswerMarker); with a scriptedUpstream the
// journey controls every upstream turn.
func bootStack(t *testing.T, script *scriptedUpstream) *stack {
	t.Helper()
	env, err := protocoltest.NewAgentTestEnv(protocoltest.AgentTypeClaudeCode)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { env.Close(false) })
	if script == nil {
		err = env.SetupAgent(protocoltest.AgentTypeClaudeCode, "virtual-claude", "tingly/cc")
	} else {
		err = env.SetupRealAgent(protocoltest.AgentTypeClaudeCode, "scripted", "claude-scripted", script.URL(), "scripted-key", "anthropic")
	}
	if err != nil {
		t.Fatal(err)
	}
	// TBClient derives the Claude Code base URL from the configured server
	// port; align it with the httptest listener. This is the only thing the
	// journeys do that a running tb does not.
	u, _ := url.Parse(env.BaseURL())
	port, _ := strconv.Atoi(u.Port())
	if err := env.AppConfig().SetServerPort(port); err != nil {
		t.Fatal(err)
	}
	return &stack{t: t, env: env, base: env.BaseURL(), token: env.AppConfig().GetUserToken(), upstream: script}
}

func (s *stack) do(method, path string, body any, out any) int {
	s.t.Helper()
	var rd io.Reader
	if body != nil {
		b, _ := json.Marshal(body)
		rd = bytes.NewReader(b)
	}
	req, err := http.NewRequest(method, s.base+path, rd)
	if err != nil {
		s.t.Fatal(err)
	}
	req.Header.Set("Authorization", "Bearer "+s.token)
	req.Header.Set("Content-Type", "application/json")
	res, err := http.DefaultClient.Do(req)
	if err != nil {
		s.t.Fatalf("%s %s: %v", method, path, err)
	}
	defer res.Body.Close()
	raw, _ := io.ReadAll(res.Body)
	if out != nil && res.StatusCode < 300 && len(raw) > 0 {
		if err := json.Unmarshal(raw, out); err != nil {
			s.t.Fatalf("%s %s: decode %s: %v", method, path, raw, err)
		}
	}
	if res.StatusCode >= 300 {
		s.t.Logf("%s %s → %d %s", method, path, res.StatusCode, raw)
	}
	return res.StatusCode
}

type sessionDetail struct {
	Session   managedagent.Session   `json:"session"`
	Workspace managedagent.Workspace `json:"workspace"`
}

// createSession posts the composer request and fails on anything but 201.
func (s *stack) createSession(body map[string]any) sessionDetail {
	s.t.Helper()
	var d sessionDetail
	if code := s.do(http.MethodPost, "/api/v1/agent/sessions", body, &d); code != 201 {
		s.t.Fatalf("create session %v: %d", body, code)
	}
	return d
}

func (s *stack) session(id string) sessionDetail {
	s.t.Helper()
	var d sessionDetail
	if code := s.do(http.MethodGet, "/api/v1/agent/sessions/"+id, nil, &d); code != 200 {
		s.t.Fatalf("get session: %d", code)
	}
	return d
}

func (s *stack) events(id string) []managedagent.Event {
	s.t.Helper()
	var page struct {
		Events []managedagent.Event `json:"events"`
	}
	s.do(http.MethodGet, "/api/v1/agent/sessions/"+id+"/events?after=0", nil, &page)
	return page.Events
}

func (s *stack) send(id, text string) {
	s.t.Helper()
	if code := s.do(http.MethodPost, "/api/v1/agent/sessions/"+id+"/messages", map[string]any{"text": text}, nil); code != 202 {
		s.t.Fatalf("send message: %d", code)
	}
}

// waitFor polls the session and its events until pred holds, dumping the
// log on timeout so a failure explains itself.
func (s *stack) waitFor(id string, timeout time.Duration, pred func(d sessionDetail, ev []managedagent.Event) bool) (sessionDetail, []managedagent.Event) {
	s.t.Helper()
	deadline := time.Now().Add(timeout)
	var d sessionDetail
	var ev []managedagent.Event
	for time.Now().Before(deadline) {
		d, ev = s.session(id), s.events(id)
		if pred(d, ev) {
			return d, ev
		}
		time.Sleep(250 * time.Millisecond)
	}
	s.t.Fatalf("timed out waiting on %s: status=%s err=%q\n%s", id, d.Session.Status, d.Session.Error, eventsDump(ev))
	return d, ev
}

// turnEnded reports whether a turn that started after event index `before`
// has finished: the launcher's closing status event (idle… or failed…) has
// landed. Polling the status field alone races with the next turn starting
// (a message sent while idle keeps the old status for a beat), so the log is
// the authority.
func turnEnded(ev []managedagent.Event, before int) bool {
	for i := before; i < len(ev); i++ {
		if ev[i].Kind == managedagent.EventStatus &&
			(strings.HasPrefix(ev[i].Text, string(managedagent.SessionIdle)) || strings.HasPrefix(ev[i].Text, string(managedagent.SessionFailed))) {
			return true
		}
	}
	return false
}

// waitSettled waits for the first turn to end (idle or failed).
func (s *stack) waitSettled(id string) (sessionDetail, []managedagent.Event) {
	s.t.Helper()
	return s.waitNextTurnSettled(id, 0)
}

// waitIdle waits for the first turn to end and fails unless it ended idle.
func (s *stack) waitIdle(id string) (sessionDetail, []managedagent.Event) {
	s.t.Helper()
	d, ev := s.waitSettled(id)
	if d.Session.Status != managedagent.SessionIdle {
		s.t.Fatalf("turn ended %s: %q\n%s", d.Session.Status, d.Session.Error, eventsDump(ev))
	}
	return d, ev
}

// waitNextTurnSettled waits for the turn started after `before` events to end.
func (s *stack) waitNextTurnSettled(id string, before int) (sessionDetail, []managedagent.Event) {
	s.t.Helper()
	return s.waitFor(id, turnTimeout, func(_ sessionDetail, ev []managedagent.Event) bool {
		return turnEnded(ev, before)
	})
}

func (s *stack) waitStatus(id string, want managedagent.SessionStatus) (sessionDetail, []managedagent.Event) {
	s.t.Helper()
	return s.waitFor(id, turnTimeout, func(d sessionDetail, _ []managedagent.Event) bool {
		return d.Session.Status == want
	})
}

func hasEvent(ev []managedagent.Event, kind managedagent.EventKind, contains string) bool {
	for _, e := range ev {
		if e.Kind == kind && (contains == "" || strings.Contains(e.Text, contains) || strings.Contains(string(e.Payload), contains)) {
			return true
		}
	}
	return false
}

func findEvent(ev []managedagent.Event, kind managedagent.EventKind) *managedagent.Event {
	for i := range ev {
		if ev[i].Kind == kind {
			return &ev[i]
		}
	}
	return nil
}

func eventsDump(ev []managedagent.Event) string {
	var b strings.Builder
	for _, e := range ev {
		text := e.Text
		if len(text) > 160 {
			text = text[:160] + "…"
		}
		fmt.Fprintf(&b, "  #%d %-18s %s\n", e.Seq, e.Kind, strings.ReplaceAll(text, "\n", "⏎"))
	}
	return b.String()
}

// newGitDir creates a committed repository the agent can work in.
func newGitDir(t *testing.T, name string) string {
	t.Helper()
	dir := filepath.Join(t.TempDir(), name)
	if err := os.MkdirAll(dir, 0o755); err != nil {
		t.Fatal(err)
	}
	run := func(args ...string) {
		t.Helper()
		cmd := exec.Command("git", append([]string{"-C", dir}, args...)...)
		cmd.Env = append(os.Environ(), "GIT_AUTHOR_NAME=e2e", "GIT_AUTHOR_EMAIL=e2e@test", "GIT_COMMITTER_NAME=e2e", "GIT_COMMITTER_EMAIL=e2e@test")
		if out, err := cmd.CombinedOutput(); err != nil {
			t.Fatalf("git %v: %v %s", args, err, out)
		}
	}
	run("init", "-q", "-b", "main")
	if err := os.WriteFile(filepath.Join(dir, "README.md"), []byte("# "+name+"\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	run("add", ".")
	run("commit", "-q", "-m", "init")
	return dir
}

// newOriginRepo creates a bare repository with one commit, usable as a
// file:// source.
func newOriginRepo(t *testing.T) string {
	t.Helper()
	work := newGitDir(t, "origin-work")
	bare := filepath.Join(t.TempDir(), "origin.git")
	if out, err := exec.Command("git", "clone", "-q", "--bare", work, bare).CombinedOutput(); err != nil {
		t.Fatalf("bare clone: %v %s", err, out)
	}
	return bare
}

// ─── scripted upstream ──────────────────────────────────────────────────

// upstreamTurn is what the scripted upstream answers to one request.
type upstreamTurn struct {
	Text   string        // plain assistant text (end_turn)
	Bash   string        // when set, a Bash tool_use with this command
	Status int           // when >= 400, an error response with this status
	Delay  time.Duration // sleep before answering (interruptible)
}

// scriptedUpstream speaks just enough of the Anthropic Messages API for the
// claude CLI: it pops one turn per request and serves it as SSE or JSON.
// When the script is exhausted it answers a fixed text so the CLI always
// finishes. Every request body is kept for assertions.
type scriptedUpstream struct {
	srv   *httptest.Server
	mu    sync.Mutex
	turns []upstreamTurn
	reqs  []map[string]any
}

const scriptedDefaultText = "Done (scripted)."

func newScriptedUpstream(t *testing.T, turns ...upstreamTurn) *scriptedUpstream {
	t.Helper()
	u := &scriptedUpstream{turns: turns}
	u.srv = httptest.NewServer(http.HandlerFunc(u.serve))
	t.Cleanup(u.srv.Close)
	return u
}

func (u *scriptedUpstream) URL() string { return u.srv.URL }

func (u *scriptedUpstream) Queue(turns ...upstreamTurn) {
	u.mu.Lock()
	defer u.mu.Unlock()
	u.turns = append(u.turns, turns...)
}

func (u *scriptedUpstream) Requests() []map[string]any {
	u.mu.Lock()
	defer u.mu.Unlock()
	return append([]map[string]any(nil), u.reqs...)
}

// LastRequestJSON returns the last request body as text, for substring
// assertions (e.g. that a tool_result made it back to the model).
func (u *scriptedUpstream) LastRequestJSON() string {
	u.mu.Lock()
	defer u.mu.Unlock()
	if len(u.reqs) == 0 {
		return ""
	}
	b, _ := json.Marshal(u.reqs[len(u.reqs)-1])
	return string(b)
}

func (u *scriptedUpstream) serve(w http.ResponseWriter, r *http.Request) {
	if !strings.HasSuffix(r.URL.Path, "/messages") {
		http.Error(w, `{"type":"error","error":{"type":"not_found_error","message":"scripted upstream: only /v1/messages"}}`, http.StatusNotFound)
		return
	}
	raw, _ := io.ReadAll(r.Body)
	var body map[string]any
	_ = json.Unmarshal(raw, &body)

	u.mu.Lock()
	u.reqs = append(u.reqs, body)
	turn := upstreamTurn{Text: scriptedDefaultText}
	if len(u.turns) > 0 {
		turn, u.turns = u.turns[0], u.turns[1:]
	}
	u.mu.Unlock()

	if turn.Delay > 0 {
		select {
		case <-time.After(turn.Delay):
		case <-r.Context().Done():
			return
		}
	}
	if turn.Status >= 400 {
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(turn.Status)
		fmt.Fprintf(w, `{"type":"error","error":{"type":"invalid_request_error","message":"scripted failure %d"}}`, turn.Status)
		return
	}
	stream, _ := body["stream"].(bool)
	if stream {
		w.Header().Set("Content-Type", "text/event-stream")
		w.Header().Set("Cache-Control", "no-cache")
		w.WriteHeader(http.StatusOK)
		for _, line := range turn.sse() {
			fmt.Fprint(w, line, "\n")
		}
		fmt.Fprint(w, "\n")
		if f, ok := w.(http.Flusher); ok {
			f.Flush()
		}
		return
	}
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(http.StatusOK)
	_ = json.NewEncoder(w).Encode(turn.message())
}

func (tn upstreamTurn) blocks() ([]map[string]any, string) {
	if tn.Bash != "" {
		return []map[string]any{{
			"type": "tool_use", "id": "toolu_scripted_1", "name": "Bash",
			"input": map[string]any{"command": tn.Bash, "description": "scripted"},
		}}, "tool_use"
	}
	return []map[string]any{{"type": "text", "text": tn.Text}}, "end_turn"
}

func (tn upstreamTurn) message() map[string]any {
	blocks, stop := tn.blocks()
	return map[string]any{
		"id": "msg_scripted", "type": "message", "role": "assistant", "model": "claude-scripted",
		"content": blocks, "stop_reason": stop, "stop_sequence": nil,
		"usage": map[string]any{"input_tokens": 12, "output_tokens": 7},
	}
}

func (tn upstreamTurn) sse() []string {
	ev := func(kind string, v map[string]any) []string {
		b, _ := json.Marshal(v)
		return []string{"event: " + kind, "data: " + string(b), ""}
	}
	var out []string
	out = append(out, ev("message_start", map[string]any{"type": "message_start", "message": map[string]any{
		"id": "msg_scripted", "type": "message", "role": "assistant", "model": "claude-scripted",
		"content": []any{}, "stop_reason": nil, "usage": map[string]any{"input_tokens": 12, "output_tokens": 0},
	}})...)
	if tn.Bash != "" {
		input, _ := json.Marshal(map[string]any{"command": tn.Bash, "description": "scripted"})
		out = append(out, ev("content_block_start", map[string]any{"type": "content_block_start", "index": 0,
			"content_block": map[string]any{"type": "tool_use", "id": "toolu_scripted_1", "name": "Bash", "input": map[string]any{}}})...)
		out = append(out, ev("content_block_delta", map[string]any{"type": "content_block_delta", "index": 0,
			"delta": map[string]any{"type": "input_json_delta", "partial_json": string(input)}})...)
		out = append(out, ev("content_block_stop", map[string]any{"type": "content_block_stop", "index": 0})...)
		out = append(out, ev("message_delta", map[string]any{"type": "message_delta",
			"delta": map[string]any{"stop_reason": "tool_use", "stop_sequence": nil}, "usage": map[string]any{"output_tokens": 7}})...)
	} else {
		out = append(out, ev("content_block_start", map[string]any{"type": "content_block_start", "index": 0,
			"content_block": map[string]any{"type": "text", "text": ""}})...)
		out = append(out, ev("content_block_delta", map[string]any{"type": "content_block_delta", "index": 0,
			"delta": map[string]any{"type": "text_delta", "text": tn.Text}})...)
		out = append(out, ev("content_block_stop", map[string]any{"type": "content_block_stop", "index": 0})...)
		out = append(out, ev("message_delta", map[string]any{"type": "message_delta",
			"delta": map[string]any{"stop_reason": "end_turn", "stop_sequence": nil}, "usage": map[string]any{"output_tokens": 7}})...)
	}
	out = append(out, ev("message_stop", map[string]any{"type": "message_stop"})...)
	return out
}
