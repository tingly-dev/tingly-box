package managedagent

import (
	"context"
	"errors"
	"fmt"
	"os"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/tingly-dev/tingly-box/agentboot"
	"github.com/tingly-dev/tingly-box/remote/session"
)

// memStore is a minimal in-memory session.SessionStore, so session.Manager
// actually persists the transcript (AppendMessage/Messages) — a nil store
// silently drops it, which would make the transcript assertions below
// vacuous.
type memStore struct {
	mu       sync.Mutex
	sessions map[string]*session.Session
	messages map[string][]session.Message
}

func newMemStore() *memStore {
	return &memStore{sessions: map[string]*session.Session{}, messages: map[string][]session.Message{}}
}

func (s *memStore) Get(id string) (*session.Session, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	sess, ok := s.sessions[id]
	if !ok {
		return nil, fmt.Errorf("session %s: not found", id)
	}
	cp := *sess
	return &cp, nil
}

func (s *memStore) Set(id string, sess *session.Session) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	cp := *sess
	s.sessions[id] = &cp
	return nil
}

func (s *memStore) Delete(id string) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	delete(s.sessions, id)
	delete(s.messages, id)
	return nil
}

func (s *memStore) List() []*session.Session {
	s.mu.Lock()
	defer s.mu.Unlock()
	out := make([]*session.Session, 0, len(s.sessions))
	for _, sess := range s.sessions {
		cp := *sess
		out = append(out, &cp)
	}
	return out
}

func (s *memStore) FindByChatAgentProject(chatID, agent, project string) (*session.Session, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	for _, sess := range s.sessions {
		if sess.ChatID == chatID && sess.Agent == agent && sess.Project == project {
			cp := *sess
			return &cp, nil
		}
	}
	return nil, nil
}

func (s *memStore) ListByChat(chatID string) ([]*session.Session, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	var out []*session.Session
	for _, sess := range s.sessions {
		if sess.ChatID == chatID {
			cp := *sess
			out = append(out, &cp)
		}
	}
	return out, nil
}

func (s *memStore) AppendMessage(id string, msg session.Message) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.messages[id] = append(s.messages[id], msg)
	return nil
}

func (s *memStore) Messages(id string) ([]session.Message, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	out := make([]session.Message, len(s.messages[id]))
	copy(out, s.messages[id])
	return out, nil
}

func mustMkdir(t *testing.T, path string) {
	t.Helper()
	if err := os.MkdirAll(path, 0o755); err != nil {
		t.Fatalf("mkdir %s: %v", path, err)
	}
}

func mustWriteFile(t *testing.T, path string) {
	t.Helper()
	if err := os.WriteFile(path, []byte("x"), 0o644); err != nil {
		t.Fatalf("write %s: %v", path, err)
	}
}

// fakeAgent is a scripted agentboot.Agent: each Execute call builds one
// fakeHandle via the test-supplied script function, so a test controls
// exactly what a "turn" does without spawning a real Claude Code process.
type fakeAgent struct {
	script func(ctx context.Context, prompt string, opts agentboot.ExecutionOptions, h *fakeHandle)
}

func (a *fakeAgent) Type() agentboot.AgentType { return agentboot.AgentTypeClaude }
func (a *fakeAgent) IsAvailable() bool         { return true }

func (a *fakeAgent) Execute(ctx context.Context, prompt string, opts agentboot.ExecutionOptions) (agentboot.ExecutionHandle, error) {
	h := &fakeHandle{
		events: make(chan agentboot.StreamEvent, 16),
		resp:   make(chan respMsg, 1),
		done:   make(chan struct{}),
	}
	if opts.Store != nil {
		opts.Store.SetRunning(opts.SessionID)
	}
	go func() {
		defer close(h.done)
		defer close(h.events)
		a.script(ctx, prompt, opts, h)
		if h.err == nil {
			h.err = ctx.Err()
		}
		if opts.Store != nil {
			if h.err != nil {
				opts.Store.SetFailed(opts.SessionID, h.err.Error())
			} else {
				opts.Store.SetCompleted(opts.SessionID, h.result.TextOutput())
			}
		}
	}()
	return h, nil
}

type respMsg struct {
	id   string
	resp agentboot.ControlResponse
}

// fakeHandle is the ExecutionHandle a fakeAgent hands back. A test's script
// sends events on send(), optionally waits for a Respond via awaitRespond(),
// and sets result/err before returning.
type fakeHandle struct {
	events chan agentboot.StreamEvent
	resp   chan respMsg
	done   chan struct{}
	result *agentboot.Result
	err    error
}

func (h *fakeHandle) Events() <-chan agentboot.StreamEvent { return h.events }

func (h *fakeHandle) Respond(reqID string, resp agentboot.ControlResponse) error {
	select {
	case h.resp <- respMsg{id: reqID, resp: resp}:
		return nil
	default:
		return agentboot.ErrUnknownRequestID
	}
}

func (h *fakeHandle) Wait() (*agentboot.Result, error) {
	<-h.done
	if h.result == nil {
		h.result = &agentboot.Result{Format: agentboot.OutputFormatText}
	}
	return h.result, h.err
}

func (h *fakeHandle) Cancel() {}

// send pushes an event and returns immediately; the script goroutine owns
// h.events so this is only ever called from within a script.
func (h *fakeHandle) send(ev agentboot.StreamEvent) { h.events <- ev }

// awaitRespond blocks for the test's Respond call or ctx cancellation.
func (h *fakeHandle) awaitRespond(ctx context.Context) (respMsg, bool) {
	select {
	case r := <-h.resp:
		return r, true
	case <-ctx.Done():
		return respMsg{}, false
	}
}

// ---------- fixtures ----------

func newTestService(t *testing.T, script func(ctx context.Context, prompt string, opts agentboot.ExecutionOptions, h *fakeHandle)) (*Service, *session.Manager) {
	t.Helper()
	mgr := session.NewManager(session.Config{Timeout: time.Hour, MessageRetention: time.Hour}, newMemStore())
	t.Cleanup(mgr.Stop)

	agentSvc, err := agentboot.NewAgentService(agentboot.DefaultConfig())
	if err != nil {
		t.Fatalf("NewAgentService: %v", err)
	}
	agentSvc.RegisterAgent(agentboot.AgentTypeClaude, &fakeAgent{script: script})

	return NewService(Config{Sessions: mgr, Agent: agentSvc}), mgr
}

// completingScript finishes a turn immediately with one assistant-ish
// message and no error — the common "happy path" turn.
func completingScript(ctx context.Context, prompt string, opts agentboot.ExecutionOptions, h *fakeHandle) {
	h.send(agentboot.MessageEvent{Raw: "ok: " + prompt})
	h.result = &agentboot.Result{Format: agentboot.OutputFormatText, Output: "done"}
}

// blockingScript never finishes on its own; it only ends when ctx is
// cancelled, simulating a turn that Interrupt has to stop.
func blockingScript(ctx context.Context, prompt string, opts agentboot.ExecutionOptions, h *fakeHandle) {
	<-ctx.Done()
	h.err = ctx.Err()
}

// approvalScript asks for one tool approval and waits for Respond before
// finishing, so tests can exercise the full Respond -> webPrompter round trip.
// (Its own reply, "approved"/"denied", is what the approval_response
// transcript entry carries — webPrompter appends that directly, so this
// script does not need to construct a claude message type to prove it.)
func approvalScript(ctx context.Context, prompt string, opts agentboot.ExecutionOptions, h *fakeHandle) {
	h.send(agentboot.ApprovalRequestEvent{ID: "req-1", ToolName: "Bash", Input: map[string]any{"command": "ls"}})
	if _, ok := h.awaitRespond(ctx); !ok {
		h.err = ctx.Err()
		return
	}
	h.result = &agentboot.Result{Format: agentboot.OutputFormatText, Output: "done"}
}

func waitStatus(t *testing.T, svc *Service, id string, want session.Status, timeout time.Duration) *session.Session {
	t.Helper()
	deadline := time.Now().Add(timeout)
	for {
		sess, err := svc.GetSession(id)
		if err != nil {
			t.Fatalf("GetSession(%s): %v", id, err)
		}
		if sess.Status == want {
			return sess
		}
		if time.Now().After(deadline) {
			t.Fatalf("session %s: want status %s, got %s (after %s)", id, want, sess.Status, timeout)
		}
		time.Sleep(5 * time.Millisecond)
	}
}

// ---------- folders / fs ----------

func TestRecentFolders_EmptyInitially(t *testing.T) {
	svc, _ := newTestService(t, completingScript)
	if got := svc.RecentFolders(context.Background(), 0); len(got) != 0 {
		t.Fatalf("want no recent folders, got %v", got)
	}
}

func TestRecentFolders_ReflectsCompletedSession(t *testing.T) {
	svc, _ := newTestService(t, completingScript)
	dir := t.TempDir()

	sess, err := svc.CreateSession(context.Background(), CreateSessionInput{Path: dir, Prompt: "hi"})
	if err != nil {
		t.Fatalf("CreateSession: %v", err)
	}
	waitStatus(t, svc, sess.ID, session.StatusCompleted, time.Second)

	folders := svc.RecentFolders(context.Background(), 0)
	if len(folders) != 1 || folders[0].Path != dir {
		t.Fatalf("want one recent folder %q, got %v", dir, folders)
	}
}

func TestListDirs_RejectsRelativeAndMissingPaths(t *testing.T) {
	svc, _ := newTestService(t, completingScript)

	if _, _, err := svc.ListDirs(context.Background(), "relative/path"); !errors.Is(err, ErrValidation) {
		t.Fatalf("relative path: want ErrValidation, got %v", err)
	}
	if _, _, err := svc.ListDirs(context.Background(), "/definitely/does/not/exist/xyz"); !errors.Is(err, ErrValidation) {
		t.Fatalf("missing path: want ErrValidation, got %v", err)
	}
}

func TestListDirs_ListsSubdirectoriesOnly(t *testing.T) {
	svc, _ := newTestService(t, completingScript)
	base := t.TempDir()
	mustMkdir(t, base+"/child")
	mustWriteFile(t, base+"/afile.txt")

	path, entries, err := svc.ListDirs(context.Background(), base)
	if err != nil {
		t.Fatalf("ListDirs: %v", err)
	}
	if path != base {
		t.Fatalf("path = %q, want %q", path, base)
	}
	if len(entries) != 1 || entries[0].Name != "child" {
		t.Fatalf("entries = %v, want just [child]", entries)
	}
}

// ---------- session lifecycle ----------

func TestCreateSession_Validation(t *testing.T) {
	svc, _ := newTestService(t, completingScript)
	dir := t.TempDir()

	cases := []CreateSessionInput{
		{Path: dir, Prompt: ""},
		{Path: "", Prompt: "hi"},
		{Path: "relative", Prompt: "hi"},
		{Path: dir, Prompt: "hi", PermissionMode: "not-a-mode"},
	}
	for i, in := range cases {
		if _, err := svc.CreateSession(context.Background(), in); !errors.Is(err, ErrValidation) {
			t.Fatalf("case %d: want ErrValidation, got %v", i, err)
		}
	}
}

func TestCreateSession_HappyPath(t *testing.T) {
	svc, _ := newTestService(t, completingScript)
	dir := t.TempDir()

	sess, err := svc.CreateSession(context.Background(), CreateSessionInput{Path: dir, Prompt: "hello"})
	if err != nil {
		t.Fatalf("CreateSession: %v", err)
	}
	if sess.Project != dir {
		t.Fatalf("Project = %q, want %q", sess.Project, dir)
	}
	waitStatus(t, svc, sess.ID, session.StatusCompleted, time.Second)

	msgs, err := svc.Messages(sess.ID)
	if err != nil {
		t.Fatalf("Messages: %v", err)
	}
	if len(msgs) == 0 || msgs[0].Role != "user" || msgs[0].Content != "hello" {
		t.Fatalf("first message = %+v, want the user's prompt", msgs)
	}

	list := svc.ListSessions(false)
	if len(list) != 1 || list[0].ID != sess.ID {
		t.Fatalf("ListSessions = %v, want just the created session", list)
	}
}

func TestSendMessage_NotFoundAndConflict(t *testing.T) {
	blocked := make(chan struct{})
	svc, _ := newTestService(t, func(ctx context.Context, prompt string, opts agentboot.ExecutionOptions, h *fakeHandle) {
		close(blocked)
		<-ctx.Done()
		h.err = ctx.Err()
	})

	if err := svc.SendMessage(context.Background(), "missing", "hi"); !errors.Is(err, ErrNotFound) {
		t.Fatalf("missing session: want ErrNotFound, got %v", err)
	}

	dir := t.TempDir()
	sess, err := svc.CreateSession(context.Background(), CreateSessionInput{Path: dir, Prompt: "hi"})
	if err != nil {
		t.Fatalf("CreateSession: %v", err)
	}
	<-blocked // the turn is now in flight

	if err := svc.SendMessage(context.Background(), sess.ID, "again"); !errors.Is(err, ErrConflict) {
		t.Fatalf("busy session: want ErrConflict, got %v", err)
	}

	if err := svc.Interrupt(sess.ID); err != nil {
		t.Fatalf("Interrupt: %v", err)
	}
	waitStatus(t, svc, sess.ID, session.StatusCompleted, time.Second)
}

func TestRespond_ApprovalRoundTrip(t *testing.T) {
	svc, _ := newTestService(t, approvalScript)
	dir := t.TempDir()

	sess, err := svc.CreateSession(context.Background(), CreateSessionInput{Path: dir, Prompt: "run ls"})
	if err != nil {
		t.Fatalf("CreateSession: %v", err)
	}

	// Wait for the approval request to land in the transcript.
	var found bool
	deadline := time.Now().Add(time.Second)
	for time.Now().Before(deadline) {
		msgs, _ := svc.Messages(sess.ID)
		for _, m := range msgs {
			if m.Kind == "approval_request" && m.RequestID == "req-1" {
				found = true
			}
		}
		if found {
			break
		}
		time.Sleep(5 * time.Millisecond)
	}
	if !found {
		t.Fatalf("approval_request never appeared in transcript")
	}

	if err := svc.Respond(sess.ID, "req-1", true, ""); err != nil {
		t.Fatalf("Respond: %v", err)
	}
	waitStatus(t, svc, sess.ID, session.StatusCompleted, time.Second)

	msgs, _ := svc.Messages(sess.ID)
	var sawResponse bool
	for _, m := range msgs {
		if m.Kind == "approval_response" && m.RequestID == "req-1" && m.Content == "approved" {
			sawResponse = true
		}
	}
	if !sawResponse {
		t.Fatalf("messages = %+v, want an approval_response entry", msgs)
	}

	if err := svc.Respond(sess.ID, "req-1", true, ""); !errors.Is(err, ErrConflict) {
		t.Fatalf("Respond with no turn in flight: want ErrConflict, got %v", err)
	}
}

func TestRespond_UnknownRequestID(t *testing.T) {
	svc, _ := newTestService(t, approvalScript)
	dir := t.TempDir()
	sess, err := svc.CreateSession(context.Background(), CreateSessionInput{Path: dir, Prompt: "run ls"})
	if err != nil {
		t.Fatalf("CreateSession: %v", err)
	}
	if err := svc.Respond(sess.ID, "not-the-real-id", true, ""); !errors.Is(err, ErrNotFound) {
		t.Fatalf("unknown request id: want ErrNotFound, got %v", err)
	}
	_ = svc.Interrupt(sess.ID)
}

func TestInterrupt_NoTurnInFlight(t *testing.T) {
	svc, _ := newTestService(t, completingScript)
	dir := t.TempDir()
	sess, err := svc.CreateSession(context.Background(), CreateSessionInput{Path: dir, Prompt: "hi"})
	if err != nil {
		t.Fatalf("CreateSession: %v", err)
	}
	waitStatus(t, svc, sess.ID, session.StatusCompleted, time.Second)

	if err := svc.Interrupt(sess.ID); !errors.Is(err, ErrConflict) {
		t.Fatalf("Interrupt with no turn: want ErrConflict, got %v", err)
	}
}

func TestInterrupt_MarksSessionResumable(t *testing.T) {
	svc, _ := newTestService(t, blockingScript)
	dir := t.TempDir()
	sess, err := svc.CreateSession(context.Background(), CreateSessionInput{Path: dir, Prompt: "hi"})
	if err != nil {
		t.Fatalf("CreateSession: %v", err)
	}
	waitStatus(t, svc, sess.ID, session.StatusRunning, time.Second)

	if err := svc.Interrupt(sess.ID); err != nil {
		t.Fatalf("Interrupt: %v", err)
	}
	final := waitStatus(t, svc, sess.ID, session.StatusCompleted, time.Second)
	if final.Error != "" {
		t.Fatalf("Error = %q, want cleared after interrupt", final.Error)
	}

	msgs, _ := svc.Messages(sess.ID)
	var sawInterrupted bool
	for _, m := range msgs {
		if strings.Contains(m.Content, "interrupted") {
			sawInterrupted = true
		}
	}
	if !sawInterrupted {
		t.Fatalf("messages = %+v, want an 'interrupted' system message", msgs)
	}
}

func TestSetPermissionMode(t *testing.T) {
	svc, _ := newTestService(t, completingScript)
	dir := t.TempDir()
	sess, err := svc.CreateSession(context.Background(), CreateSessionInput{Path: dir, Prompt: "hi"})
	if err != nil {
		t.Fatalf("CreateSession: %v", err)
	}
	waitStatus(t, svc, sess.ID, session.StatusCompleted, time.Second)

	if _, err := svc.SetPermissionMode(sess.ID, "not-a-mode"); !errors.Is(err, ErrValidation) {
		t.Fatalf("bad mode: want ErrValidation, got %v", err)
	}
	updated, err := svc.SetPermissionMode(sess.ID, PermissionAcceptEdits)
	if err != nil {
		t.Fatalf("SetPermissionMode: %v", err)
	}
	if updated.PermissionMode != PermissionAcceptEdits {
		t.Fatalf("PermissionMode = %q, want %q", updated.PermissionMode, PermissionAcceptEdits)
	}
	if _, err := svc.SetPermissionMode("missing", PermissionDefault); !errors.Is(err, ErrNotFound) {
		t.Fatalf("missing session: want ErrNotFound, got %v", err)
	}
}

func TestArchive_ClosesAndCancelsRunningTurn(t *testing.T) {
	svc, _ := newTestService(t, blockingScript)
	dir := t.TempDir()
	sess, err := svc.CreateSession(context.Background(), CreateSessionInput{Path: dir, Prompt: "hi"})
	if err != nil {
		t.Fatalf("CreateSession: %v", err)
	}
	waitStatus(t, svc, sess.ID, session.StatusRunning, time.Second)

	archived, err := svc.Archive(sess.ID)
	if err != nil {
		t.Fatalf("Archive: %v", err)
	}
	if archived.Status != session.StatusClosed {
		t.Fatalf("Status = %s, want closed", archived.Status)
	}

	// Archive again is a no-op that still returns the closed session.
	archived2, err := svc.Archive(sess.ID)
	if err != nil {
		t.Fatalf("Archive (again): %v", err)
	}
	if archived2.Status != session.StatusClosed {
		t.Fatalf("Status = %s, want closed", archived2.Status)
	}

	if _, err := svc.Archive("missing"); !errors.Is(err, ErrNotFound) {
		t.Fatalf("missing session: want ErrNotFound, got %v", err)
	}
}

// TestArchive_ConcurrentWithRunsMap exercises the s.runs map from both
// Archive and a turn finishing at the same time; run with -race to catch a
// regression of the unguarded map read this once had.
func TestArchive_ConcurrentWithRunsMap(t *testing.T) {
	svc, _ := newTestService(t, completingScript)
	dir := t.TempDir()

	var wg sync.WaitGroup
	for i := 0; i < 5; i++ {
		sess, err := svc.CreateSession(context.Background(), CreateSessionInput{Path: dir, Prompt: "hi"})
		if err != nil {
			t.Fatalf("CreateSession: %v", err)
		}
		wg.Add(1)
		go func(id string) {
			defer wg.Done()
			_, _ = svc.Archive(id)
		}(sess.ID)
	}
	wg.Wait()
}

func TestDiff_NoGitConfigured(t *testing.T) {
	svc, _ := newTestService(t, completingScript)
	dir := t.TempDir()
	sess, err := svc.CreateSession(context.Background(), CreateSessionInput{Path: dir, Prompt: "hi"})
	if err != nil {
		t.Fatalf("CreateSession: %v", err)
	}
	waitStatus(t, svc, sess.ID, session.StatusCompleted, time.Second)

	diff, err := svc.Diff(context.Background(), sess.ID)
	if err != nil {
		t.Fatalf("Diff: %v", err)
	}
	if diff.ChangedFiles != 0 || diff.Patch != "" {
		t.Fatalf("diff = %+v, want empty (no git configured)", diff)
	}
}
