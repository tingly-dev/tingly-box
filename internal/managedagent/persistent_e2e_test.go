package managedagent

import (
	"context"
	"encoding/json"
	"strings"
	"testing"
	"time"

	"github.com/tingly-dev/tingly-box/agentboot"
	"github.com/tingly-dev/tingly-box/agentboot/claude"
	"github.com/tingly-dev/tingly-box/agentboot/pool"
	"github.com/tingly-dev/tingly-box/agentboot/process"
	"github.com/tingly-dev/tingly-box/remote/session"
)

// twoTurnPersistentFactory scripts a single fake `claude` process that
// answers len(replies) sequential turns on the same stdin without exiting
// between them — the persistent-CLI behavior remoteagent's own
// TestPersistentSession_E2E_ReusesOneProcessAcrossTurns
// (remote/control/remoteagent/persistent_session_e2e_test.go) exercises for
// @cc. This is the same fixture, reused here to prove the managedagent
// front door drives the real claude.Agent/Driver/pool stack the same way,
// not just the package-local fakes in service_test.go.
func twoTurnPersistentFactory(replies ...string) *process.FakeFactory {
	factory := process.NewFakeFactory()
	factory.OnStart = func(_ context.Context, _ process.LaunchSpec, h *process.FakeHandle) {
		go func() {
			dec := json.NewDecoder(h.StdinR)
			for _, reply := range replies {
				var msg map[string]any
				if err := dec.Decode(&msg); err != nil {
					h.FinishOutput()
					h.SignalExit(err)
					return
				}
				assistant, _ := json.Marshal(map[string]any{
					"type": claude.SDKAssistantMessage,
					"message": map[string]any{
						"role": "assistant",
						"content": []any{
							map[string]any{"type": "text", "text": reply},
						},
					},
				})
				_, _ = h.WriteOutput(append(assistant, '\n'))
				result, _ := json.Marshal(map[string]any{
					"type":     claude.SDKResultMessage,
					"subtype":  claude.ResultSubtypeSuccess,
					"is_error": false,
				})
				_, _ = h.WriteOutput(append(result, '\n'))
			}
			// Wait for stdin to close (the pool's Close call on
			// eviction/shutdown) before exiting cleanly, exactly like the
			// real CLI.
			var next map[string]any
			_ = dec.Decode(&next)
			h.FinishOutput()
			h.SignalExit(nil)
		}()
	}
	return factory
}

// TestPersistentSession_E2E_ReusesOneProcessAcrossTurns drives two web
// messages through the real Service.CreateSession/SendMessage ->
// runPersistentTurn -> agentboot.AgentService/pool.Pool -> claude.Agent/
// Driver pipeline (a fake process factory stands in for the actual `claude`
// binary), and asserts:
//  1. both replies land in the transcript, and
//  2. only one underlying process was ever spawned — proving the second
//     message reused the first turn's live session (Acquire+Send) instead
//     of going through the one-shot Run path.
func TestPersistentSession_E2E_ReusesOneProcessAcrossTurns(t *testing.T) {
	mgr := session.NewManager(session.Config{Timeout: time.Hour, MessageRetention: time.Hour}, newMemStore())
	t.Cleanup(mgr.Stop)

	agentSvc, err := agentboot.NewAgentService(agentboot.DefaultConfig())
	if err != nil {
		t.Fatalf("NewAgentService: %v", err)
	}
	factory := twoTurnPersistentFactory("first reply", "second reply")
	agentSvc.RegisterAgent(agentboot.AgentTypeClaude, claude.NewAgentWithFactory(claude.Config{}, factory))
	if err := agentSvc.SetDefaultAgent(agentboot.AgentTypeClaude); err != nil {
		t.Fatalf("SetDefaultAgent: %v", err)
	}

	sessionPool := pool.New(pool.Config{})
	t.Cleanup(func() { sessionPool.Shutdown(context.Background()) })

	svc := NewService(Config{Sessions: mgr, Agent: agentSvc, Pool: sessionPool})
	dir := t.TempDir()

	sess, err := svc.CreateSession(context.Background(), CreateSessionInput{Path: dir, Prompt: "hello one"})
	if err != nil {
		t.Fatalf("CreateSession: %v", err)
	}
	waitStatus(t, svc, sess.ID, session.StatusCompleted, 3*time.Second)
	assertTranscriptContains(t, svc, sess.ID, "first reply")

	if err := svc.SendMessage(context.Background(), sess.ID, "hello two"); err != nil {
		t.Fatalf("SendMessage: %v", err)
	}
	// The session's status is already Completed from turn 1, so waiting on
	// StatusCompleted alone could return before turn 2 even starts (the same
	// race TestPersistentTurn_CrashMidTurnDropsFromPoolAndFails hit in
	// service_test.go). Wait for the second reply to actually land instead.
	waitTranscriptContains(t, svc, sess.ID, "second reply", 3*time.Second)

	if got := len(factory.Starts()); got != 1 {
		t.Fatalf("persistent mode must reuse one process across both turns, got %d process starts", got)
	}
}

func transcriptContains(t *testing.T, svc *Service, id, want string) bool {
	t.Helper()
	msgs, err := svc.Messages(id)
	if err != nil {
		t.Fatalf("Messages(%s): %v", id, err)
	}
	for _, m := range msgs {
		if strings.Contains(m.Content, want) {
			return true
		}
	}
	return false
}

func assertTranscriptContains(t *testing.T, svc *Service, id, want string) {
	t.Helper()
	if !transcriptContains(t, svc, id, want) {
		t.Fatalf("session %s: transcript does not contain %q", id, want)
	}
}

func waitTranscriptContains(t *testing.T, svc *Service, id, want string, timeout time.Duration) {
	t.Helper()
	deadline := time.Now().Add(timeout)
	for {
		if transcriptContains(t, svc, id, want) {
			return
		}
		if time.Now().After(deadline) {
			t.Fatalf("session %s: transcript never contained %q (after %s)", id, want, timeout)
		}
		time.Sleep(5 * time.Millisecond)
	}
}
