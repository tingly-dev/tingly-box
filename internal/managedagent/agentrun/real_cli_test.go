package agentrun_test

// Real-path smoke test: the actual `claude` CLI, driven by the real Launcher,
// against an ephemeral tingly-box gateway whose upstream is the in-process
// virtual model (.design/harness-agent-testing.md). It is the managed agent
// counterpart of `harness agent claude --mock` and is what validates the
// assumptions the fake-process tests cannot: stream-json input over stdin,
// --settings routing, --session-id / --resume, and the event shapes the
// frontend will render.
//
// Opt-in (needs the claude binary on PATH and ~1 minute):
//
//	TB_MANAGED_AGENT_E2E=1 go test ./internal/managedagent/agentrun/ -run RealCLI -v

import (
	"context"
	"encoding/json"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/tingly-dev/tingly-box/agentboot/claude"
	internalagent "github.com/tingly-dev/tingly-box/internal/agent"
	"github.com/tingly-dev/tingly-box/internal/managedagent"
	"github.com/tingly-dev/tingly-box/internal/managedagent/agentrun"
	"github.com/tingly-dev/tingly-box/internal/managedagent/gitrepo"
	"github.com/tingly-dev/tingly-box/internal/protocoltest"
)

func TestRealCLI_SessionRoundTrip(t *testing.T) {
	if os.Getenv("TB_MANAGED_AGENT_E2E") == "" {
		t.Skip("set TB_MANAGED_AGENT_E2E=1 to run the real claude CLI smoke test")
	}
	if _, err := exec.LookPath("git"); err != nil {
		t.Skip("git not installed")
	}
	if _, err := claude.FindClaudeCLI(context.Background()); err != nil {
		t.Skipf("claude CLI not found: %v", err)
	}
	ctx := context.Background()

	// Gateway + virtual upstream, routed for the claude_code scenario.
	env, err := protocoltest.NewAgentTestEnv(protocoltest.AgentTypeClaudeCode)
	if err != nil {
		t.Fatal(err)
	}
	defer env.Close(false)
	if err := env.SetupAgent(protocoltest.AgentTypeClaudeCode, "virtual-claude", "tingly/cc"); err != nil {
		t.Fatal(err)
	}
	// Same settings shape the harness (and Quick Config) produce.
	claudeEnv, err := internalagent.DefaultClaudeCodePrefs(true).ToEnv(env.BaseURL(), env.ModelToken())
	if err != nil {
		t.Fatal(err)
	}
	settingsPath := filepath.Join(t.TempDir(), "settings.json")
	settingsJSON, _ := json.MarshalIndent(map[string]any{"env": claudeEnv}, "", "  ")
	if err := os.WriteFile(settingsPath, settingsJSON, 0o600); err != nil {
		t.Fatal(err)
	}

	// A local origin the agent is pointed at.
	origin := newOriginRepo(t)

	base := t.TempDir()
	eventLog, err := managedagent.NewEventLog(filepath.Join(base, "events"))
	if err != nil {
		t.Fatal(err)
	}
	_, stores := managedagent.NewMemStores()
	stores.Events = eventLog
	git := &gitrepo.Git{MirrorsDir: filepath.Join(base, "sources")}
	cfg := claude.DefaultConfig()
	cfg.DefaultExecutionTimeout = 3 * time.Minute
	launcher, err := agentrun.New(agentrun.Config{
		Stores: stores,
		Agent:  claude.NewAgentWithConfig(cfg),
		Git:    git,
		Routing: agentrun.RoutingFunc(func(context.Context, string) ([]string, string, error) {
			return nil, settingsPath, nil
		}),
	})
	if err != nil {
		t.Fatal(err)
	}
	svc := managedagent.NewService(managedagent.Config{
		Stores: stores, Launcher: launcher, Git: agentrun.GitAdapter{Git: git},
		WorkspacesDir: filepath.Join(base, "workspaces"),
	})
	if err := svc.EnsureDefaults(ctx); err != nil {
		t.Fatal(err)
	}
	src, err := svc.CreateSource(ctx, managedagent.SourceInput{URL: "file://" + origin})
	if err != nil {
		t.Fatal(err)
	}

	sess, err := svc.CreateSession(ctx, managedagent.CreateSessionInput{
		SourceID: src.ID, Prompt: "What is the capital of France?",
	})
	if err != nil {
		t.Fatal(err)
	}
	final := waitStatus(t, svc, sess.ID, 3*time.Minute, managedagent.SessionIdle, managedagent.SessionFailed)
	events, _ := svc.ListEvents(ctx, sess.ID, 0, 0)
	dump := eventsDump(events)
	if final.Status != managedagent.SessionIdle {
		t.Fatalf("session ended %s (%s)\n%s", final.Status, final.Error, dump)
	}
	if !strings.Contains(dump, protocoltest.VirtualMockAnswerMarker) {
		t.Fatalf("mock answer never reached the event log\n%s", dump)
	}
	if final.CCSessionID == "" {
		t.Fatalf("no cc session id recorded\n%s", dump)
	}
	// The marker alone is not proof: a real model answers "Paris" too. The
	// virtual upstream must have served the turn.
	if env.VirtualServer().CallCount() == 0 {
		t.Fatalf("the virtual upstream never saw a request; the CLI routed elsewhere\n%s", dump)
	}
	t.Logf("first turn ok: cc_session=%s usage=%+v\n%s", final.CCSessionID, final.Usage, dump)

	// Second turn resumes the same Claude Code session.
	before := len(events)
	if err := svc.SendMessage(ctx, sess.ID, "And Germany?"); err != nil {
		t.Fatal(err)
	}
	// The status is still idle from the first turn until the resumed turn
	// starts, so wait on the log instead: a new assistant message (or a
	// failure) after the steer.
	waitEvent(t, svc, sess.ID, int64(before), 3*time.Minute, managedagent.EventAssistantMessage, managedagent.EventError)
	final = waitStatus(t, svc, sess.ID, 3*time.Minute, managedagent.SessionIdle, managedagent.SessionFailed)
	events, _ = svc.ListEvents(ctx, sess.ID, 0, 0)
	dump = eventsDump(events[before:])
	// The virtual upstream answers in context ("Berlin."), so assert on the
	// shape rather than the France marker here.
	if final.Status != managedagent.SessionIdle || !strings.Contains(dump, string(managedagent.EventAssistantMessage)+": ") {
		t.Fatalf("resumed turn: status=%s err=%q\n%s", final.Status, final.Error, dump)
	}
	if strings.Count(eventsDump(events), "claude code session "+final.CCSessionID) != 2 {
		t.Fatalf("resumed turn did not report the same claude code session\n%s", eventsDump(events))
	}
	t.Logf("resumed turn ok\n%s", dump)

	if _, err := svc.Archive(ctx, sess.ID); err != nil {
		t.Fatal(err)
	}
}

func waitStatus(t *testing.T, svc *managedagent.Service, id string, timeout time.Duration, want ...managedagent.SessionStatus) *managedagent.Session {
	t.Helper()
	deadline := time.Now().Add(timeout)
	for time.Now().Before(deadline) {
		s, err := svc.GetSession(context.Background(), id)
		if err != nil {
			t.Fatal(err)
		}
		for _, w := range want {
			if s.Status == w {
				return s
			}
		}
		time.Sleep(200 * time.Millisecond)
	}
	s, _ := svc.GetSession(context.Background(), id)
	events, _ := svc.ListEvents(context.Background(), id, 0, 0)
	t.Fatalf("timed out waiting for %v; status=%s\n%s", want, s.Status, eventsDump(events))
	return nil
}

// waitEvent blocks until an event of one of the kinds appears after seq.
func waitEvent(t *testing.T, svc *managedagent.Service, id string, after int64, timeout time.Duration, kinds ...managedagent.EventKind) {
	t.Helper()
	deadline := time.Now().Add(timeout)
	for time.Now().Before(deadline) {
		events, _ := svc.ListEvents(context.Background(), id, after, 0)
		for _, e := range events {
			for _, k := range kinds {
				if e.Kind == k {
					return
				}
			}
		}
		time.Sleep(200 * time.Millisecond)
	}
	events, _ := svc.ListEvents(context.Background(), id, 0, 0)
	t.Fatalf("timed out waiting for %v after seq %d\n%s", kinds, after, eventsDump(events))
}

func eventsDump(events []managedagent.Event) string {
	var b strings.Builder
	for _, e := range events {
		txt := e.Text
		if len(txt) > 200 {
			txt = txt[:200] + "…"
		}
		b.WriteString(string(e.Kind) + ": " + strings.ReplaceAll(txt, "\n", " "))
		b.WriteString("\n")
	}
	return b.String()
}

func newOriginRepo(t *testing.T) string {
	t.Helper()
	root := t.TempDir()
	work := filepath.Join(root, "work")
	run := func(dir string, args ...string) {
		cmd := exec.Command("git", args...)
		cmd.Dir = dir
		cmd.Env = append(os.Environ(), "GIT_AUTHOR_NAME=t", "GIT_AUTHOR_EMAIL=t@x", "GIT_COMMITTER_NAME=t", "GIT_COMMITTER_EMAIL=t@x")
		if out, err := cmd.CombinedOutput(); err != nil {
			t.Fatalf("git %v: %v\n%s", args, err, out)
		}
	}
	run(root, "init", "-q", "-b", "main", work)
	os.WriteFile(filepath.Join(work, "README.md"), []byte("hi\n"), 0o644)
	run(work, "add", ".")
	run(work, "commit", "-q", "-m", "init")
	bare := filepath.Join(root, "origin.git")
	run(root, "clone", "-q", "--bare", work, bare)
	return bare
}
