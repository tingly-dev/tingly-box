package agentrun_test

// Full-stack smoke test: the real tingly-box server (StoreManager, event
// log directories, the Launcher wired in UseManagedAgentEndpoints, TBClient
// gateway routing), driven over HTTP exactly as the web UI drives it, with
// the real `claude` CLI and the virtual upstream. Where real_cli_test.go
// validates the Launcher, this validates everything around it.
//
// Opt-in, same switch as the CLI test:
//
//	TB_MANAGED_AGENT_E2E=1 go test ./internal/managedagent/agentrun/ -run FullStack -v

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"os"
	"os/exec"
	"path/filepath"
	"strconv"
	"strings"
	"testing"
	"time"

	"github.com/tingly-dev/tingly-box/agentboot/claude"
	"github.com/tingly-dev/tingly-box/internal/constant"
	"github.com/tingly-dev/tingly-box/internal/managedagent"
	"github.com/tingly-dev/tingly-box/internal/protocoltest"
)

type apiClient struct {
	t     *testing.T
	base  string
	token string
}

func (c *apiClient) do(method, path string, body any, out any) int {
	c.t.Helper()
	var rd io.Reader
	if body != nil {
		b, _ := json.Marshal(body)
		rd = bytes.NewReader(b)
	}
	req, err := http.NewRequest(method, c.base+path, rd)
	if err != nil {
		c.t.Fatal(err)
	}
	req.Header.Set("Authorization", "Bearer "+c.token)
	req.Header.Set("Content-Type", "application/json")
	res, err := http.DefaultClient.Do(req)
	if err != nil {
		c.t.Fatalf("%s %s: %v", method, path, err)
	}
	defer res.Body.Close()
	raw, _ := io.ReadAll(res.Body)
	if out != nil && res.StatusCode < 300 && len(raw) > 0 {
		if err := json.Unmarshal(raw, out); err != nil {
			c.t.Fatalf("%s %s: decode %s: %v", method, path, raw, err)
		}
	}
	if res.StatusCode >= 300 {
		c.t.Logf("%s %s → %d %s", method, path, res.StatusCode, raw)
	}
	return res.StatusCode
}

type sessionDetail struct {
	Session   managedagent.Session   `json:"session"`
	Workspace managedagent.Workspace `json:"workspace"`
}

func TestFullStack_SessionOverHTTP(t *testing.T) {
	if os.Getenv("TB_MANAGED_AGENT_E2E") == "" {
		t.Skip("set TB_MANAGED_AGENT_E2E=1 to run the full-stack smoke test")
	}
	if _, err := exec.LookPath("git"); err != nil {
		t.Skip("git not installed")
	}
	if _, err := claude.FindClaudeCLI(context.Background()); err != nil {
		t.Skipf("claude CLI not found: %v", err)
	}

	env, err := protocoltest.NewAgentTestEnv(protocoltest.AgentTypeClaudeCode)
	if err != nil {
		t.Fatal(err)
	}
	defer env.Close(false)
	if err := env.SetupAgent(protocoltest.AgentTypeClaudeCode, "virtual-claude", "tingly/cc"); err != nil {
		t.Fatal(err)
	}
	// The gateway listens on an httptest port; TBClient derives the Claude
	// Code base URL from the configured server port, so align them. This is
	// the only thing the test does that a running tb does not.
	u, _ := url.Parse(env.BaseURL())
	port, _ := strconv.Atoi(u.Port())
	if err := env.AppConfig().SetServerPort(port); err != nil {
		t.Fatal(err)
	}
	api := &apiClient{t: t, base: env.BaseURL(), token: env.AppConfig().GetUserToken()}

	// Default environment exists without any setup.
	var envs struct {
		Environments      []managedagent.Environment `json:"environments"`
		SupportedRuntimes []string                   `json:"supported_runtimes"`
	}
	if code := api.do(http.MethodGet, "/api/v1/agent/environments", nil, &envs); code != 200 {
		t.Fatalf("list environments: %d", code)
	}
	if len(envs.Environments) != 1 || !envs.Environments[0].IsDefault || envs.SupportedRuntimes[0] != "local" {
		t.Fatalf("unexpected environments: %+v", envs)
	}

	origin := newOriginRepo(t)
	var src managedagent.Source
	if code := api.do(http.MethodPost, "/api/v1/agent/sources", map[string]any{"url": "file://" + origin}, &src); code != 201 {
		t.Fatalf("create source: %d", code)
	}

	var detail sessionDetail
	if code := api.do(http.MethodPost, "/api/v1/agent/sessions", map[string]any{
		"source_id": src.ID, "prompt": "What is the capital of France?",
	}, &detail); code != 201 {
		t.Fatalf("create session: %d", code)
	}
	id := detail.Session.ID

	waitHTTP := func(timeout time.Duration, pred func(d sessionDetail, events []managedagent.Event) bool) (sessionDetail, []managedagent.Event) {
		t.Helper()
		deadline := time.Now().Add(timeout)
		var d sessionDetail
		var page struct {
			Events []managedagent.Event `json:"events"`
		}
		for time.Now().Before(deadline) {
			api.do(http.MethodGet, "/api/v1/agent/sessions/"+id, nil, &d)
			api.do(http.MethodGet, "/api/v1/agent/sessions/"+id+"/events?after=0", nil, &page)
			if pred(d, page.Events) {
				return d, page.Events
			}
			time.Sleep(300 * time.Millisecond)
		}
		t.Fatalf("timed out; status=%s err=%q\n%s", d.Session.Status, d.Session.Error, eventsDump(page.Events))
		return d, nil
	}
	settled := func(d sessionDetail, _ []managedagent.Event) bool {
		return d.Session.Status == managedagent.SessionIdle || d.Session.Status == managedagent.SessionFailed
	}

	d, events := waitHTTP(3*time.Minute, settled)
	dump := eventsDump(events)
	if d.Session.Status != managedagent.SessionIdle || !strings.Contains(dump, protocoltest.VirtualMockAnswerMarker) {
		t.Fatalf("first turn: status=%s err=%q\n%s", d.Session.Status, d.Session.Error, dump)
	}
	if env.VirtualServer().CallCount() == 0 {
		t.Fatalf("the virtual upstream never saw a request; the CLI routed elsewhere\n%s", dump)
	}
	if d.Workspace.State != managedagent.WorkspaceReady || d.Workspace.Branch == "" {
		t.Fatalf("workspace: %+v", d.Workspace)
	}
	if !strings.HasPrefix(d.Workspace.Path, constant.GetAgentWorkspacesDir(env.ConfigDir())) {
		t.Fatalf("workspace not under the config dir: %s", d.Workspace.Path)
	}
	if _, err := os.Stat(filepath.Join(constant.GetAgentEventsDir(env.ConfigDir()), id+".jsonl")); err != nil {
		t.Fatalf("event log file missing: %v", err)
	}
	t.Logf("first turn over HTTP ok (usage %+v)\n%s", d.Session.Usage, dump)

	// Steer → resumed turn.
	before := len(events)
	if code := api.do(http.MethodPost, "/api/v1/agent/sessions/"+id+"/messages", map[string]any{"text": "And Germany?"}, nil); code != 202 {
		t.Fatalf("send message: %d", code)
	}
	d, events = waitHTTP(3*time.Minute, func(d sessionDetail, ev []managedagent.Event) bool {
		if !settled(d, ev) || len(ev) <= before {
			return false
		}
		for _, e := range ev[before:] {
			if e.Kind == managedagent.EventAssistantMessage {
				return true
			}
		}
		return d.Session.Status == managedagent.SessionFailed
	})
	if d.Session.Status != managedagent.SessionIdle {
		t.Fatalf("resumed turn: status=%s err=%q\n%s", d.Session.Status, d.Session.Error, eventsDump(events[before:]))
	}
	t.Logf("resumed turn ok\n%s", eventsDump(events[before:]))

	// Artifacts: a change in the checkout shows up in the diff; push lands
	// the branch on the origin; archive ends the run.
	if err := os.WriteFile(filepath.Join(d.Workspace.Path, "NOTES.md"), []byte("by agent\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	var diff managedagent.Diff
	if code := api.do(http.MethodGet, "/api/v1/agent/sessions/"+id+"/diff", nil, &diff); code != 200 || diff.ChangedFiles != 1 {
		t.Fatalf("diff: %d %+v", code, diff)
	}
	if code := api.do(http.MethodPost, "/api/v1/agent/sessions/"+id+"/push", nil, &d); code != 200 || !d.Session.Artifact.Pushed {
		t.Fatalf("push: %d %+v", code, d.Session.Artifact)
	}
	out, err := exec.Command("git", "-C", origin, "branch", "--list", d.Workspace.Branch).CombinedOutput()
	if err != nil || !strings.Contains(string(out), d.Workspace.Branch) {
		t.Fatalf("branch not on origin: %v %s", err, out)
	}
	if code := api.do(http.MethodPost, "/api/v1/agent/sessions/"+id+"/archive", nil, &d); code != 200 || d.Session.Status != managedagent.SessionArchived {
		t.Fatalf("archive: %d %s", code, d.Session.Status)
	}
	if code := api.do(http.MethodPost, "/api/v1/agent/sessions/"+id+"/messages", map[string]any{"text": "too late"}, nil); code != 409 {
		t.Fatalf("steer after archive: want 409, got %d", code)
	}

	var list struct {
		Sessions []struct {
			Session managedagent.Session `json:"session"`
			Source  *struct {
				Name string `json:"name"`
			} `json:"source"`
			Branch string `json:"branch"`
		} `json:"sessions"`
	}
	api.do(http.MethodGet, "/api/v1/agent/sessions", nil, &list)
	if len(list.Sessions) != 1 || list.Sessions[0].Source == nil || list.Sessions[0].Branch != d.Workspace.Branch {
		t.Fatalf("list: %+v", list)
	}
	fmt.Fprintln(os.Stderr, "full-stack ok:", id)
}
