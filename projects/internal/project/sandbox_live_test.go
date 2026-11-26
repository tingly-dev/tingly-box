package project

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/http/httptest"
	"os"
	"os/exec"
	"path/filepath"
	"strconv"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"github.com/tingly-dev/tingly-box/agentboot"
	"github.com/tingly-dev/tingly-box/agentboot/claude"
	"github.com/tingly-dev/tingly-box/agentboot/claude/session"
	"github.com/tingly-dev/tingly-box/agentboot/engine/enginetest"
	"github.com/tingly-dev/tingly-box/sandbox"
	// Not shipped yet (Projects threads need network open); here to run the
	// CLI in a microVM with TB_SANDBOX_LIVE_RUNTIME=microvm.
	_ "github.com/tingly-dev/tingly-box/sandbox/microvm"
)

// scriptedAnthropic answers Claude Code's requests: the first turn with tools
// asks for one Bash call, the next one (carrying its result) ends the turn;
// every other request (titles and the like) gets plain text. It keeps what the
// Bash call returned.
type scriptedAnthropic struct {
	command string
	mu      sync.Mutex
	result  string
}

func (s *scriptedAnthropic) ServeHTTP(w http.ResponseWriter, r *http.Request) {
	body, _ := io.ReadAll(r.Body)
	var req struct {
		Stream   bool              `json:"stream"`
		Tools    []json.RawMessage `json:"tools"`
		Messages []struct {
			Content json.RawMessage `json:"content"`
		} `json:"messages"`
	}
	_ = json.Unmarshal(body, &req)
	if !strings.Contains(r.URL.Path, "/messages") || strings.Contains(r.URL.Path, "count_tokens") {
		_, _ = w.Write([]byte(`{"input_tokens":1}`))
		return
	}
	toolUse := false
	if len(req.Tools) > 0 && strings.Contains(string(body), `"name":"Bash"`) {
		toolUse = true
		// Once any tool result is in the conversation, the turn ends.
		for _, m := range req.Messages {
			if strings.Contains(string(m.Content), `"tool_result"`) {
				s.mu.Lock()
				if s.result == "" {
					s.result = string(m.Content)
				}
				s.mu.Unlock()
				toolUse = false
			}
		}
	}
	w.Header().Set("Content-Type", "text/event-stream")
	ev := func(name string, data any) {
		b, _ := json.Marshal(data)
		fmt.Fprintf(w, "event: %s\ndata: %s\n\n", name, b)
	}
	ev("message_start", map[string]any{"type": "message_start", "message": map[string]any{"id": "msg_1", "type": "message", "role": "assistant", "model": "claude-sonnet-4-5", "content": []any{}, "stop_reason": nil, "stop_sequence": nil, "usage": map[string]any{"input_tokens": 1, "output_tokens": 1}}})
	stop := "end_turn"
	if toolUse {
		stop = "tool_use"
		in, _ := json.Marshal(map[string]string{"command": s.command, "description": "probe"})
		ev("content_block_start", map[string]any{"type": "content_block_start", "index": 0, "content_block": map[string]any{"type": "tool_use", "id": "toolu_1", "name": "Bash", "input": map[string]any{}}})
		ev("content_block_delta", map[string]any{"type": "content_block_delta", "index": 0, "delta": map[string]any{"type": "input_json_delta", "partial_json": string(in)}})
	} else {
		ev("content_block_start", map[string]any{"type": "content_block_start", "index": 0, "content_block": map[string]any{"type": "text", "text": ""}})
		ev("content_block_delta", map[string]any{"type": "content_block_delta", "index": 0, "delta": map[string]any{"type": "text_delta", "text": "done"}})
	}
	ev("content_block_stop", map[string]any{"type": "content_block_stop", "index": 0})
	ev("message_delta", map[string]any{"type": "message_delta", "delta": map[string]any{"stop_reason": stop, "stop_sequence": nil}, "usage": map[string]any{"output_tokens": 1}})
	ev("message_stop", map[string]any{"type": "message_stop"})
}

// The real Claude Code CLI, in yolo mode, inside a sandboxed thread's launch:
// what its Bash tool can and cannot reach. Opt in with TB_SANDBOX_LIVE=1; it
// needs claude and bwrap, and HOME must be the user's (toolchains live there).
// TB_SANDBOX_LIVE_RUNTIME names another runtime (microvm needs
// a network other than open). TB_SANDBOX_LIVE_NETWORK names the network level
// (allowlist, internet; TB_SANDBOX_LIVE_ALLOWLIST=1 is allowlist), with the
// model exposed.
func TestLiveSandboxedClaudeReachesOnlyItsWork(t *testing.T) {
	if os.Getenv("TB_SANDBOX_LIVE") != "1" {
		t.Skip("set TB_SANDBOX_LIVE=1 to run the real CLI in bwrap")
	}
	for _, bin := range []string{"claude", "bwrap"} {
		if _, err := exec.LookPath(bin); err != nil {
			t.Skipf("%s not installed", bin)
		}
	}
	config := t.TempDir()
	t.Setenv("CLAUDE_CONFIG_DIR", config)
	secretDir := t.TempDir()
	secret := filepath.Join(secretDir, "id_rsa")
	require.NoError(t, os.WriteFile(secret, []byte("PRIVATE"), 0o600))

	f := newFixture(t, func(turn *enginetest.Turn) error { turn.Say("done", 1, 1); return nil })
	f.project.ThreadSandbox = "bwrap"
	if rt := os.Getenv("TB_SANDBOX_LIVE_RUNTIME"); rt != "" {
		f.project.ThreadSandbox = rt
	}
	require.NoError(t, f.store.SaveProject(f.project))
	th, err := f.threads.Start(context.Background(), f.project, StartSpec{Title: "Probe", Task: "probe"})
	require.NoError(t, err)
	f.report(t)
	home, _ := os.UserHomeDir()

	probe := strings.Join([]string{
		"ls -A ~ | tr '\\n' ' '; echo",
		"[ -e " + home + "/.ssh ] && echo LEAK-ssh",
		"[ -e " + home + "/.tingly-box ] && echo LEAK-tb",
		"[ -e " + config + "/settings.json ] && echo LEAK-config",
		"cat " + secret + " 2>/dev/null && echo LEAK-secret",
		"[ -z \"$GH_TOKEN$AWS_SECRET_ACCESS_KEY\" ] || echo LEAK-env",
		"echo hi > " + filepath.Join(th.Cwd, "probe.txt") + " && echo WROTE",
		"git -C " + th.Cwd + " config core.fsmonitor x 2>/dev/null && echo LEAK-gitconfig",
		"ls /proc | grep -c '^[0-9]'",
		"echo PROBE-END",
	}, "; ")
	mock := &scriptedAnthropic{command: probe}
	srv := httptest.NewUnstartedServer(mock)
	srv.Start()
	defer srv.Close()
	require.NoError(t, os.WriteFile(filepath.Join(config, "settings.json"), []byte(`{}`), 0o600))

	t.Setenv("GH_TOKEN", "should-not-pass")
	_, opts, err := f.threads.turnLaunch(context.Background(), th.ID, th, f.project, false)
	require.NoError(t, err)
	require.NotNil(t, opts.Sandbox)
	opts.Env = append(opts.Env, "ANTHROPIC_BASE_URL="+srv.URL, "ANTHROPIC_AUTH_TOKEN=x", "ANTHROPIC_MODEL=claude-sonnet-4-5",
		"DISABLE_TELEMETRY=1", "DISABLE_AUTOUPDATER=1", "CLAUDE_CODE_DISABLE_NONESSENTIAL_TRAFFIC=1")
	opts.Sandbox.Policy.Credentials.Env = append(opts.Sandbox.Policy.Credentials.Env, "ANTHROPIC_BASE_URL", "ANTHROPIC_AUTH_TOKEN", "ANTHROPIC_MODEL")
	opts.PermissionMode = "bypassPermissions"
	opts.PermissionPromptTool = ""
	opts.ProjectPath = th.Cwd
	opts.OutputFormat = agentboot.OutputFormatStreamJSON
	opts.Timeout = 2 * time.Minute
	network := sandbox.Level(os.Getenv("TB_SANDBOX_LIVE_NETWORK"))
	if os.Getenv("TB_SANDBOX_LIVE_ALLOWLIST") == "1" {
		network = sandbox.NetworkAllowlist
	}
	if network != "" {
		// The model is on the host's loopback, reached only as an exposed
		// port; allowlist grants nothing else, internet the public internet.
		port, _ := strconv.Atoi(srv.URL[strings.LastIndex(srv.URL, ":")+1:])
		opts.Sandbox.Policy.Network = sandbox.Network{Level: network, Expose: []sandbox.Endpoint{{Port: port}}}
		if network == sandbox.NetworkAllowlist {
			opts.Sandbox.Policy.Network.Allow = []string{"example.org"}
		}
		mock.command += "; curl -sS -m 5 -o /dev/null -w 'PUBLIC=%{http_code}\\n' https://example.com 2>&1 | tail -1; curl -sS -m 5 " + srv.URL + "/v1/models >/dev/null && echo VIA-EXPOSE"
	}
	if os.Getenv("TB_SANDBOX_LIVE_HOST") == "1" {
		opts.Sandbox = nil // the same launch on the host, to compare
	}

	agent := claude.NewAgentWithConfig(claude.DefaultConfig())
	h, err := agent.Execute(context.Background(), "probe", opts)
	require.NoError(t, err)
	go func() {
		for range h.Events() {
		}
	}()
	res, err := h.Wait()
	require.NoError(t, err)
	require.NotNil(t, res)

	mock.mu.Lock()
	out := mock.result
	mock.mu.Unlock()
	t.Logf("probe output: %s", out)
	require.Contains(t, out, "PROBE-END", "the Bash call ran in the sandbox")
	assert.NotContains(t, out, "LEAK")
	switch network {
	case sandbox.NetworkAllowlist:
		assert.Contains(t, out, "PUBLIC=000", "a host off the list is refused")
	case sandbox.NetworkInternet:
		assert.Contains(t, out, "PUBLIC=200", "the public internet is reached")
	}
	if network != "" {
		assert.Contains(t, out, "VIA-EXPOSE", "the model on the host is reached as an exposed port")
	}
	assert.Contains(t, out, "WROTE")
	b, err := os.ReadFile(filepath.Join(th.Cwd, "probe.txt"))
	require.NoError(t, err)
	assert.Equal(t, "hi\n", string(b), "a write in the worktree lands on the host")
	entries, _ := os.ReadDir(filepath.Join(config, "projects", session.ProjectDirName(th.Cwd)))
	if len(entries) == 0 {
		all, _ := os.ReadDir(filepath.Join(config, "projects"))
		for _, e := range all {
			t.Logf("config/projects has %s (len %d); expected %s (len %d)", e.Name(), len(e.Name()), session.ProjectDirName(th.Cwd), len(session.ProjectDirName(th.Cwd)))
		}
	}
	assert.NotEmpty(t, entries, "the transcript is where the host reads it")
}
