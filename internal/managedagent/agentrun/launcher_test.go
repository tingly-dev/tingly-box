package agentrun

import (
	"bytes"
	"context"
	"encoding/json"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/tingly-dev/tingly-box/agentboot"
	"github.com/tingly-dev/tingly-box/agentboot/claude"
	"github.com/tingly-dev/tingly-box/agentboot/process"
	"github.com/tingly-dev/tingly-box/internal/managedagent"
	"github.com/tingly-dev/tingly-box/internal/managedagent/gitrepo"
)

func sh(t *testing.T, dir string, args ...string) {
	t.Helper()
	cmd := exec.Command("git", args...)
	cmd.Dir = dir
	cmd.Env = append(os.Environ(),
		"GIT_AUTHOR_NAME=t", "GIT_AUTHOR_EMAIL=t@x", "GIT_COMMITTER_NAME=t", "GIT_COMMITTER_EMAIL=t@x")
	if out, err := cmd.CombinedOutput(); err != nil {
		t.Fatalf("git %v: %v\n%s", args, err, out)
	}
}

func newOrigin(t *testing.T) string {
	t.Helper()
	root := t.TempDir()
	work := filepath.Join(root, "work")
	sh(t, root, "init", "-q", "-b", "main", work)
	os.WriteFile(filepath.Join(work, "README.md"), []byte("hi\n"), 0o644)
	sh(t, work, "add", ".")
	sh(t, work, "commit", "-q", "-m", "init")
	bare := filepath.Join(root, "origin.git")
	sh(t, root, "clone", "-q", "--bare", work, bare)
	return bare
}

func writeLine(t *testing.T, h *process.FakeHandle, v map[string]any) {
	t.Helper()
	b, _ := json.Marshal(v)
	if _, err := h.WriteOutput(append(b, '\n')); err != nil {
		t.Fatal(err)
	}
}

// waitFor polls until cond is true or the test times out.
func waitFor(t *testing.T, what string, cond func() bool) {
	t.Helper()
	deadline := time.Now().Add(5 * time.Second)
	for time.Now().Before(deadline) {
		if cond() {
			return
		}
		time.Sleep(10 * time.Millisecond)
	}
	t.Fatalf("timed out waiting for %s", what)
}

func TestLauncher_FullTurnWithApprovalAndSteer(t *testing.T) {
	if _, err := exec.LookPath("git"); err != nil {
		t.Skip("git not installed")
	}
	ctx := context.Background()
	origin := newOrigin(t)

	factory := process.NewFakeFactory()
	handles := make(chan *process.FakeHandle, 4)
	stdins := &stdinLog{buf: map[*process.FakeHandle]*bytes.Buffer{}}
	factory.OnStart = func(_ context.Context, _ process.LaunchSpec, h *process.FakeHandle) {
		// stdin is a synchronous pipe: the runner blocks on writing the
		// prompt until someone reads, so capture everything it sends.
		stdins.track(h)
		handles <- h
	}
	agent := claude.NewAgentWithFactory(claude.Config{}, factory)

	_, stores := managedagent.NewMemStores()
	git := &gitrepo.Git{MirrorsDir: filepath.Join(t.TempDir(), "mirrors")}
	launcher, err := New(Config{
		Stores: stores, Agent: agent, Git: git,
		Routing: RoutingFunc(func(context.Context, string) ([]string, string, error) {
			return []string{"ANTHROPIC_BASE_URL=http://gateway"}, "", nil
		}),
	})
	if err != nil {
		t.Fatal(err)
	}
	svc := managedagent.NewService(managedagent.Config{
		Stores: stores, Launcher: launcher, Git: GitAdapter{git}, WorkspacesDir: filepath.Join(t.TempDir(), "ws"),
	})
	if err := svc.EnsureDefaults(ctx); err != nil {
		t.Fatal(err)
	}
	src, _ := svc.CreateSource(ctx, managedagent.SourceInput{URL: "file://" + origin})
	sess, err := svc.CreateSession(ctx, managedagent.CreateSessionInput{SourceID: src.ID, Prompt: "do the thing"})
	if err != nil {
		t.Fatal(err)
	}

	// Provisioning happens first, then the agent process starts.
	var h *process.FakeHandle
	select {
	case h = <-handles:
	case <-time.After(10 * time.Second):
		t.Fatal("agent never started")
	}
	spec := h.Spec()
	ws, _ := svc.GetWorkspace(ctx, sess.WorkspaceID)
	if ws.State != managedagent.WorkspaceReady || spec.WorkDir != ws.AgentCwd {
		t.Fatalf("workspace %+v, workdir %q", ws, spec.WorkDir)
	}
	if !strings.Contains(strings.Join(spec.Env, "\n"), "ANTHROPIC_BASE_URL=http://gateway") {
		t.Fatalf("gateway env not injected: %v", spec.Env)
	}
	cur, _ := svc.GetSession(ctx, sess.ID)
	if cur.CCSessionID == "" || cur.Status != managedagent.SessionRunning {
		t.Fatalf("session before events: %+v", cur)
	}
	if !contains(spec.Command, "--session-id") || !contains(spec.Command, cur.CCSessionID) {
		t.Fatalf("first turn must create the cc session by id: %v", spec.Command)
	}
	waitFor(t, "prompt on stdin", func() bool { return strings.Contains(stdins.get(h), "do the thing") })

	// Assistant text + a tool call, then a permission request.
	writeLine(t, h, map[string]any{"type": claude.SDKSystemMessage, "subtype": claude.SystemSubtypeInit, "session_id": cur.CCSessionID})
	writeLine(t, h, map[string]any{"type": claude.SDKAssistantMessage, "message": map[string]any{
		"role": "assistant", "content": []any{
			map[string]any{"type": "text", "text": "Let me look."},
			map[string]any{"type": "tool_use", "id": "tu-1", "name": "Bash", "input": map[string]any{"command": "ls"}},
		}}})
	writeLine(t, h, map[string]any{"type": claude.SDKControlRequestMessage, "request_id": "req-1",
		"request": map[string]any{"subtype": claude.ControlRequestSubtypeCanUseTool, "tool_name": "Bash", "input": map[string]any{"command": "ls"}}})

	waitFor(t, "waiting_input", func() bool {
		s, _ := svc.GetSession(ctx, sess.ID)
		return s.Status == managedagent.SessionWaitingInput
	})
	events, _ := svc.ListEvents(ctx, sess.ID, 0, 0)
	kinds := map[managedagent.EventKind]int{}
	var reqID string
	for _, e := range events {
		kinds[e.Kind]++
		if e.Kind == managedagent.EventApprovalRequest {
			reqID = e.RequestID
		}
	}
	if kinds[managedagent.EventAssistantMessage] != 1 || kinds[managedagent.EventToolUse] != 1 || reqID != "req-1" {
		t.Fatalf("events before approval: %v", kinds)
	}

	// Approve through the Service; the runner writes the response to stdin.
	if err := svc.Respond(ctx, sess.ID, managedagent.Response{RequestID: reqID, Approved: true}); err != nil {
		t.Fatal(err)
	}
	waitFor(t, "approval on stdin", func() bool { return strings.Contains(stdins.get(h), "req-1") })
	waitFor(t, "running after approval", func() bool {
		s, _ := svc.GetSession(ctx, sess.ID)
		return s.Status == managedagent.SessionRunning
	})

	// Steer while running: queued for the next turn.
	if err := svc.SendMessage(ctx, sess.ID, "also update docs"); err != nil {
		t.Fatal(err)
	}

	// The agent commits a file and finishes the turn.
	os.WriteFile(filepath.Join(ws.Path, "a.txt"), []byte("a\n"), 0o644)
	writeLine(t, h, map[string]any{"type": claude.SDKResultMessage, "subtype": claude.ResultSubtypeSuccess,
		"is_error": false, "total_cost_usd": 0.25, "usage": map[string]any{"input_tokens": 100, "output_tokens": 20}})
	h.FinishOutput()
	h.SignalExit(nil)

	// Second turn starts automatically with the queued steer, resuming.
	var h2 *process.FakeHandle
	select {
	case h2 = <-handles:
	case <-time.After(10 * time.Second):
		t.Fatal("steered turn never started")
	}
	spec2 := h2.Spec()
	if !contains(spec2.Command, "--resume") || !contains(spec2.Command, cur.CCSessionID) {
		t.Fatalf("second turn must resume the cc session: %v", spec2.Command)
	}
	waitFor(t, "steer on stdin", func() bool { return strings.Contains(stdins.get(h2), "also update docs") })
	writeLine(t, h2, map[string]any{"type": claude.SDKResultMessage, "subtype": claude.ResultSubtypeSuccess, "is_error": false})
	h2.FinishOutput()
	h2.SignalExit(nil)

	waitFor(t, "idle", func() bool {
		s, _ := svc.GetSession(ctx, sess.ID)
		return s.Status == managedagent.SessionIdle
	})
	final, _ := svc.GetSession(ctx, sess.ID)
	if final.Usage.InputTokens != 100 || final.Usage.Cost != 0.25 || final.Artifact.Changed != 1 {
		t.Fatalf("usage/artifact not folded: %+v", final)
	}

	d, err := svc.Diff(ctx, sess.ID)
	if err != nil || d.ChangedFiles != 1 || len(d.Untracked) != 1 {
		t.Fatalf("diff = %+v, %v", d, err)
	}

	// Push from idle works (branch only; the untracked file is not pushed).
	if _, err := svc.Push(ctx, sess.ID); err != nil {
		t.Fatalf("push: %v", err)
	}
	if s, _ := svc.GetSession(ctx, sess.ID); !s.Artifact.Pushed {
		t.Fatal("artifact not marked pushed")
	}

	// Archive stops the run for good.
	if _, err := svc.Archive(ctx, sess.ID); err != nil {
		t.Fatal(err)
	}
	if launcher.get(sess.ID) != nil {
		t.Fatal("run still registered after archive")
	}
}

func TestLauncher_ProvisionFailureFailsSession(t *testing.T) {
	ctx := context.Background()
	factory := process.NewFakeFactory()
	agent := claude.NewAgentWithFactory(claude.Config{}, factory)
	_, stores := managedagent.NewMemStores()
	git := &gitrepo.Git{}
	launcher, _ := New(Config{Stores: stores, Agent: agent, Git: git})
	svc := managedagent.NewService(managedagent.Config{Stores: stores, Launcher: launcher, WorkspacesDir: t.TempDir()})
	_ = svc.EnsureDefaults(ctx)
	src, _ := svc.CreateSource(ctx, managedagent.SourceInput{URL: "https://127.0.0.1:1/nope/repo.git"})
	sess, err := svc.CreateSession(ctx, managedagent.CreateSessionInput{SourceID: src.ID, Prompt: "x"})
	if err != nil {
		t.Fatal(err)
	}
	waitFor(t, "failed", func() bool {
		s, _ := svc.GetSession(ctx, sess.ID)
		return s.Status == managedagent.SessionFailed
	})
	ws, _ := svc.GetWorkspace(ctx, sess.WorkspaceID)
	if ws.State != managedagent.WorkspaceFailed || ws.Error == "" {
		t.Fatalf("workspace = %+v", ws)
	}
	if len(factory.Starts()) != 0 {
		t.Fatal("agent must not start when provisioning fails")
	}
}

// stdinLog drains each fake process's stdin into a buffer for assertions.
type stdinLog struct {
	mu  sync.Mutex
	buf map[*process.FakeHandle]*bytes.Buffer
}

func (l *stdinLog) track(h *process.FakeHandle) {
	l.mu.Lock()
	l.buf[h] = &bytes.Buffer{}
	l.mu.Unlock()
	go func() {
		chunk := make([]byte, 4096)
		for {
			n, err := h.StdinR.Read(chunk)
			if n > 0 {
				l.mu.Lock()
				l.buf[h].Write(chunk[:n])
				l.mu.Unlock()
			}
			if err != nil {
				return
			}
		}
	}()
}

func (l *stdinLog) get(h *process.FakeHandle) string {
	l.mu.Lock()
	defer l.mu.Unlock()
	return l.buf[h].String()
}

func contains(list []string, s string) bool {
	for _, x := range list {
		if x == s {
			return true
		}
	}
	return false
}

var _ agentboot.Agent = (*claude.Agent)(nil)

// newLiveSession boots a service + launcher over a fake claude and returns
// the first turn's process handle once it is running.
func newLiveSession(t *testing.T) (*managedagent.Service, *Launcher, *managedagent.Session, *process.FakeHandle, *stdinLog) {
	t.Helper()
	if _, err := exec.LookPath("git"); err != nil {
		t.Skip("git not installed")
	}
	ctx := context.Background()
	origin := newOrigin(t)
	factory := process.NewFakeFactory()
	handles := make(chan *process.FakeHandle, 4)
	stdins := &stdinLog{buf: map[*process.FakeHandle]*bytes.Buffer{}}
	factory.OnStart = func(_ context.Context, _ process.LaunchSpec, h *process.FakeHandle) {
		stdins.track(h)
		handles <- h
	}
	_, stores := managedagent.NewMemStores()
	git := &gitrepo.Git{}
	launcher, err := New(Config{Stores: stores, Agent: claude.NewAgentWithFactory(claude.Config{}, factory), Git: git})
	if err != nil {
		t.Fatal(err)
	}
	svc := managedagent.NewService(managedagent.Config{Stores: stores, Launcher: launcher, Git: GitAdapter{git}, WorkspacesDir: t.TempDir()})
	_ = svc.EnsureDefaults(ctx)
	src, _ := svc.CreateSource(ctx, managedagent.SourceInput{URL: "file://" + origin})
	sess, err := svc.CreateSession(ctx, managedagent.CreateSessionInput{SourceID: src.ID, Prompt: "work"})
	if err != nil {
		t.Fatal(err)
	}
	var h *process.FakeHandle
	select {
	case h = <-handles:
	case <-time.After(10 * time.Second):
		t.Fatal("agent never started")
	}
	waitFor(t, "running", func() bool {
		s, _ := svc.GetSession(ctx, sess.ID)
		return s.Status == managedagent.SessionRunning
	})
	return svc, launcher, sess, h, stdins
}

func TestLauncher_InterruptLeavesSessionIdle(t *testing.T) {
	ctx := context.Background()
	svc, _, sess, h, _ := newLiveSession(t)
	if err := svc.Interrupt(ctx, sess.ID); err != nil {
		t.Fatal(err)
	}
	select {
	case <-h.Done():
	case <-time.After(5 * time.Second):
		t.Fatal("interrupt did not stop the process")
	}
	waitFor(t, "idle after interrupt", func() bool {
		s, _ := svc.GetSession(ctx, sess.ID)
		return s.Status == managedagent.SessionIdle && s.Error == ""
	})
	// Still resumable: a follow-up starts a new turn.
	if err := svc.SendMessage(ctx, sess.ID, "continue"); err != nil {
		t.Fatalf("send after interrupt: %v", err)
	}
	waitFor(t, "running again", func() bool {
		s, _ := svc.GetSession(ctx, sess.ID)
		return s.Status == managedagent.SessionRunning
	})
}

func TestLauncher_ArchiveWhileRunningStopsProcess(t *testing.T) {
	ctx := context.Background()
	svc, launcher, sess, h, _ := newLiveSession(t)
	archived, err := svc.Archive(ctx, sess.ID)
	if err != nil {
		t.Fatal(err)
	}
	if archived.Status != managedagent.SessionArchived {
		t.Fatalf("status = %s", archived.Status)
	}
	select {
	case <-h.Done():
	case <-time.After(5 * time.Second):
		t.Fatal("archive did not stop the process")
	}
	// The turn's finish must not flip an archived session back.
	time.Sleep(100 * time.Millisecond)
	if s, _ := svc.GetSession(ctx, sess.ID); s.Status != managedagent.SessionArchived {
		t.Fatalf("status after process exit = %s, want archived", s.Status)
	}
	if launcher.get(sess.ID) != nil {
		t.Fatal("run still registered")
	}
	if err := svc.SendMessage(ctx, sess.ID, "x"); err == nil {
		t.Fatal("steer after archive must fail")
	}
}

func TestLauncher_BypassPermissionsAutoApproves(t *testing.T) {
	if _, err := exec.LookPath("git"); err != nil {
		t.Skip("git not installed")
	}
	ctx := context.Background()
	origin := newOrigin(t)
	factory := process.NewFakeFactory()
	handles := make(chan *process.FakeHandle, 4)
	stdins := &stdinLog{buf: map[*process.FakeHandle]*bytes.Buffer{}}
	factory.OnStart = func(_ context.Context, _ process.LaunchSpec, h *process.FakeHandle) {
		stdins.track(h)
		handles <- h
	}
	_, stores := managedagent.NewMemStores()
	git := &gitrepo.Git{}
	launcher, _ := New(Config{Stores: stores, Agent: claude.NewAgentWithFactory(claude.Config{}, factory), Git: git})
	svc := managedagent.NewService(managedagent.Config{Stores: stores, Launcher: launcher, WorkspacesDir: t.TempDir()})
	_ = svc.EnsureDefaults(ctx)
	src, _ := svc.CreateSource(ctx, managedagent.SourceInput{URL: "file://" + origin})
	sess, err := svc.CreateSession(ctx, managedagent.CreateSessionInput{
		SourceID: src.ID, Prompt: "go", PermissionMode: managedagent.PermissionBypassPermissions,
	})
	if err != nil {
		t.Fatal(err)
	}
	var h *process.FakeHandle
	select {
	case h = <-handles:
	case <-time.After(10 * time.Second):
		t.Fatal("agent never started")
	}
	if !contains(h.Spec().Command, "bypassPermissions") {
		t.Fatalf("mode not passed to the CLI: %v", h.Spec().Command)
	}
	writeLine(t, h, map[string]any{"type": claude.SDKControlRequestMessage, "request_id": "req-9",
		"request": map[string]any{"subtype": claude.ControlRequestSubtypeCanUseTool, "tool_name": "Bash", "input": map[string]any{"command": "rm -rf build"}}})
	// The host answers; the session never waits.
	waitFor(t, "auto approval on stdin", func() bool { return strings.Contains(stdins.get(h), "req-9") })
	if s, _ := svc.GetSession(ctx, sess.ID); s.Status != managedagent.SessionRunning {
		t.Fatalf("status = %s, want running (no waiting_input under bypass)", s.Status)
	}
	events, _ := svc.ListEvents(ctx, sess.ID, 0, 0)
	var req, resp bool
	for _, e := range events {
		if e.Kind == managedagent.EventApprovalRequest && e.RequestID == "req-9" {
			req = true
		}
		if e.Kind == managedagent.EventApprovalResponse && e.RequestID == "req-9" && strings.Contains(e.Text, "bypassPermissions") {
			resp = true
		}
	}
	if !req || !resp {
		t.Fatalf("auto approval not logged: req=%v resp=%v", req, resp)
	}
	writeLine(t, h, map[string]any{"type": claude.SDKResultMessage, "subtype": claude.ResultSubtypeSuccess, "is_error": false})
	h.FinishOutput()
	h.SignalExit(nil)
	waitFor(t, "idle", func() bool {
		s, _ := svc.GetSession(ctx, sess.ID)
		return s.Status == managedagent.SessionIdle
	})
}

// TestLauncher_CLIStderrReachesSessionError runs a stand-in "claude" that
// rejects its arguments the way the real CLI does (message on stderr,
// non-zero exit) and checks the message lands on the session, where the
// user can act on it.
func TestLauncher_CLIStderrReachesSessionError(t *testing.T) {
	if _, err := exec.LookPath("sh"); err != nil {
		t.Skip("sh not available")
	}
	if _, err := exec.LookPath("git"); err != nil {
		t.Skip("git not installed")
	}
	ctx := context.Background()
	script := filepath.Join(t.TempDir(), "claude")
	// Discovery probes --version and expects the real banner; everything
	// else fails the way the CLI fails on an unknown mode.
	os.WriteFile(script, []byte("#!/bin/sh\ncase \"$1\" in --version) echo '2.0.0 (Claude Code)'; exit 0;; esac\necho \"error: option '--permission-mode <mode>' argument 'auto' is invalid. Allowed choices are acceptEdits, bypassPermissions, plan.\" >&2\nexit 1\n"), 0o755)

	_, stores := managedagent.NewMemStores()
	git := &gitrepo.Git{}
	cfg := claude.DefaultConfig()
	cfg.CLIPath = script
	launcher, err := New(Config{Stores: stores, Agent: claude.NewAgentWithConfig(cfg), Git: git})
	if err != nil {
		t.Fatal(err)
	}
	svc := managedagent.NewService(managedagent.Config{Stores: stores, Launcher: launcher, WorkspacesDir: t.TempDir()})
	_ = svc.EnsureDefaults(ctx)
	src, _ := svc.CreateSource(ctx, managedagent.SourceInput{URL: "file://" + newOrigin(t)})
	sess, err := svc.CreateSession(ctx, managedagent.CreateSessionInput{SourceID: src.ID, Prompt: "go", PermissionMode: managedagent.PermissionAuto})
	if err != nil {
		t.Fatal(err)
	}
	waitFor(t, "failed", func() bool {
		s, _ := svc.GetSession(ctx, sess.ID)
		return s.Status == managedagent.SessionFailed
	})
	s, _ := svc.GetSession(ctx, sess.ID)
	if !strings.Contains(s.Error, "argument 'auto' is invalid") {
		t.Fatalf("CLI stderr not surfaced: %q", s.Error)
	}
	// The user's way out: switch the mode and send again in the same checkout.
	if _, err := svc.SetPermissionMode(ctx, sess.ID, managedagent.PermissionAcceptEdits); err != nil {
		t.Fatalf("mode change after failure: %v", err)
	}
	if err := svc.SendMessage(ctx, sess.ID, "retry"); err != nil {
		t.Fatalf("retry after failure: %v", err)
	}
}

func TestLauncher_LocalDirectoryRunsInPlace(t *testing.T) {
	ctx := context.Background()
	factory := process.NewFakeFactory()
	handles := make(chan *process.FakeHandle, 2)
	stdins := &stdinLog{buf: map[*process.FakeHandle]*bytes.Buffer{}}
	factory.OnStart = func(_ context.Context, _ process.LaunchSpec, h *process.FakeHandle) {
		stdins.track(h)
		handles <- h
	}
	_, stores := managedagent.NewMemStores()
	git := &gitrepo.Git{MirrorsDir: filepath.Join(t.TempDir(), "mirrors")}
	launcher, _ := New(Config{Stores: stores, Agent: claude.NewAgentWithFactory(claude.Config{}, factory), Git: git})
	svc := managedagent.NewService(managedagent.Config{Stores: stores, Launcher: launcher, Git: GitAdapter{git}, WorkspacesDir: t.TempDir()})
	_ = svc.EnsureDefaults(ctx)
	dir := t.TempDir()
	src, err := svc.CreateSource(ctx, managedagent.SourceInput{URL: dir})
	if err != nil {
		t.Fatal(err)
	}
	sess, err := svc.CreateSession(ctx, managedagent.CreateSessionInput{SourceID: src.ID, Prompt: "hi"})
	if err != nil {
		t.Fatal(err)
	}
	var h *process.FakeHandle
	select {
	case h = <-handles:
	case <-time.After(10 * time.Second):
		t.Fatal("agent never started")
	}
	if h.Spec().WorkDir != dir {
		t.Fatalf("agent must run in the user's directory: %q", h.Spec().WorkDir)
	}
	if _, err := os.Stat(git.MirrorsDir); err == nil {
		t.Fatal("no mirror must be created for an in-place directory")
	}
	events, _ := svc.ListEvents(ctx, sess.ID, 0, 0)
	for _, e := range events {
		if e.Kind == managedagent.EventSystem && strings.Contains(e.Text, "provisioning") {
			t.Fatalf("no provisioning for an in-place directory: %+v", e)
		}
	}
	h.FinishOutput()
	h.SignalExit(nil)
}
