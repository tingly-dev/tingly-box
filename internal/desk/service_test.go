package desk

import (
	"context"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	"github.com/tingly-dev/tingly-box/agentboot"
	"github.com/tingly-dev/tingly-box/agentboot/pool"
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

// fakeAgent is a scripted agentboot.Agent: each Execute call builds one
// fakeHandle via the test-supplied script function, so a test controls
// exactly what a "turn" does without spawning a real Claude Code process.
// It also implements agentboot.PersistentAgent's Open — openFn is nil in
// tests that only exercise the one-shot path, which makes every persistent
// attempt fail (simulating an agent that can't Open), same as a real launch
// failure would: runPersistentTurn falls back to one-shot either way.
type fakeAgent struct {
	script    func(ctx context.Context, prompt string, opts agentboot.ExecutionOptions, h *fakeHandle)
	openFn    func(ctx context.Context, prompt string, opts agentboot.ExecutionOptions) (agentboot.PersistentSession, error)
	openCalls atomic.Int32
	// executeErr, if set, fails Execute before any process "starts", the
	// way a missing CLI or a bad launch spec does.
	executeErr error
}

func (a *fakeAgent) Type() agentboot.AgentType { return agentboot.AgentTypeClaude }
func (a *fakeAgent) IsAvailable() bool         { return true }

func (a *fakeAgent) Open(ctx context.Context, prompt string, opts agentboot.ExecutionOptions) (agentboot.PersistentSession, error) {
	a.openCalls.Add(1)
	if a.openFn == nil {
		return nil, errors.New("fakeAgent: Open not configured")
	}
	return a.openFn(ctx, prompt, opts)
}

func (a *fakeAgent) Execute(ctx context.Context, prompt string, opts agentboot.ExecutionOptions) (agentboot.ExecutionHandle, error) {
	if a.executeErr != nil {
		return nil, a.executeErr
	}
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

// newPersistentTestService is newTestService plus a *pool.Pool, so Service
// drives turns through openFn's PersistentSession first, falling back to
// oneShotScript only if openFn returns an error (or is nil).
func newPersistentTestService(t *testing.T, oneShotScript func(ctx context.Context, prompt string, opts agentboot.ExecutionOptions, h *fakeHandle), openFn func(ctx context.Context, prompt string, opts agentboot.ExecutionOptions) (agentboot.PersistentSession, error)) (*Service, *fakeAgent) {
	t.Helper()
	mgr := session.NewManager(session.Config{Timeout: time.Hour, MessageRetention: time.Hour}, newMemStore())
	t.Cleanup(mgr.Stop)

	agentSvc, err := agentboot.NewAgentService(agentboot.DefaultConfig())
	if err != nil {
		t.Fatalf("NewAgentService: %v", err)
	}
	fa := &fakeAgent{script: oneShotScript, openFn: openFn}
	agentSvc.RegisterAgent(agentboot.AgentTypeClaude, fa)

	p := pool.New(pool.Config{MaxSessions: 10, IdleTimeout: time.Hour})
	t.Cleanup(func() { p.Shutdown(context.Background()) })

	return NewService(Config{Sessions: mgr, Agent: agentSvc, Pool: p}), fa
}

// fakePersistentSession is a controllable agentboot.PersistentSession: a
// script decides what each Send (including the implicit one from Open)
// does, via the emit/completeTurn/crash helpers below.
type fakePersistentSession struct {
	mu     sync.Mutex
	status agentboot.SessionState
	events chan agentboot.StreamEvent

	script func(ctx context.Context, prompt string, s *fakePersistentSession)
}

// newFakePersistentSession builds a session already Running its first turn
// (prompt), matching real Open's contract — the caller (an openFn) has
// exactly the ctx/prompt Open itself received, so it kicks the script off
// here rather than leaving turn 1 unrun until some later Send.
func newFakePersistentSession(ctx context.Context, prompt string, script func(ctx context.Context, prompt string, s *fakePersistentSession)) *fakePersistentSession {
	s := &fakePersistentSession{status: agentboot.SessionStateRunning, events: make(chan agentboot.StreamEvent, 16), script: script}
	go script(ctx, prompt, s)
	return s
}

func (s *fakePersistentSession) Send(ctx context.Context, prompt string) error {
	s.mu.Lock()
	if s.status != agentboot.SessionStateIdle {
		s.mu.Unlock()
		return errors.New("fakePersistentSession: turn already in flight")
	}
	s.status = agentboot.SessionStateRunning
	s.mu.Unlock()
	go s.script(ctx, prompt, s)
	return nil
}

func (s *fakePersistentSession) Events() <-chan agentboot.StreamEvent { return s.events }

func (s *fakePersistentSession) Respond(reqID string, resp agentboot.ControlResponse) error {
	return agentboot.ErrUnknownRequestID
}

func (s *fakePersistentSession) Status() agentboot.SessionState {
	s.mu.Lock()
	defer s.mu.Unlock()
	return s.status
}

func (s *fakePersistentSession) Close(ctx context.Context) error {
	s.mu.Lock()
	if s.status == agentboot.SessionStateTerminated {
		s.mu.Unlock()
		return nil
	}
	s.status = agentboot.SessionStateTerminated
	s.mu.Unlock()
	select {
	case s.events <- agentboot.SessionStateEvent{State: agentboot.SessionStateTerminated, Reason: "closed"}:
	default:
	}
	return nil
}

func (s *fakePersistentSession) emit(ev agentboot.StreamEvent) { s.events <- ev }

func (s *fakePersistentSession) completeTurn(result *agentboot.Result) {
	s.mu.Lock()
	s.status = agentboot.SessionStateIdle
	s.mu.Unlock()
	s.events <- agentboot.TurnCompleteEvent{Result: result}
}

func (s *fakePersistentSession) crash(reason string) {
	s.mu.Lock()
	s.status = agentboot.SessionStateTerminated
	s.mu.Unlock()
	s.events <- agentboot.SessionStateEvent{State: agentboot.SessionStateTerminated, Reason: reason}
}

// completingPersistentScript finishes a turn immediately, the persistent
// counterpart of completingScript.
func completingPersistentScript(ctx context.Context, prompt string, s *fakePersistentSession) {
	s.emit(agentboot.MessageEvent{Raw: "ok: " + prompt})
	s.completeTurn(&agentboot.Result{Format: agentboot.OutputFormatText, Output: "done"})
}

// crashingPersistentScript simulates the process dying mid-turn.
func crashingPersistentScript(ctx context.Context, prompt string, s *fakePersistentSession) {
	s.crash("boom")
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

// ---------- folders ----------

func TestRecentFolders_EmptyInitially(t *testing.T) {
	svc, _ := newTestService(t, completingScript)
	if got := svc.RecentFolders(0); len(got) != 0 {
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

	folders := svc.RecentFolders(0)
	if len(folders) != 1 || folders[0].Path != dir {
		t.Fatalf("want one recent folder %q, got %v", dir, folders)
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

// ---------- persistent sessions (agentboot/pool) ----------

func TestPersistentTurn_ReusesSessionAcrossMessages(t *testing.T) {
	svc, fa := newPersistentTestService(t, completingScript, func(ctx context.Context, prompt string, opts agentboot.ExecutionOptions) (agentboot.PersistentSession, error) {
		return newFakePersistentSession(ctx, prompt, completingPersistentScript), nil
	})
	dir := t.TempDir()

	sess, err := svc.CreateSession(context.Background(), CreateSessionInput{Path: dir, Prompt: "hi"})
	if err != nil {
		t.Fatalf("CreateSession: %v", err)
	}
	waitStatus(t, svc, sess.ID, session.StatusCompleted, time.Second)

	if err := svc.SendMessage(context.Background(), sess.ID, "again"); err != nil {
		t.Fatalf("SendMessage: %v", err)
	}
	waitStatus(t, svc, sess.ID, session.StatusCompleted, time.Second)

	if got := fa.openCalls.Load(); got != 1 {
		t.Fatalf("Open called %d times, want 1 (the second message should Acquire+Send the pooled session, not Open a new one)", got)
	}
}

func TestPersistentTurn_FallsBackToOneShotOnOpenFailure(t *testing.T) {
	svc, fa := newPersistentTestService(t, completingScript, func(ctx context.Context, prompt string, opts agentboot.ExecutionOptions) (agentboot.PersistentSession, error) {
		return nil, errors.New("boom: launch failed")
	})
	dir := t.TempDir()

	sess, err := svc.CreateSession(context.Background(), CreateSessionInput{Path: dir, Prompt: "hi"})
	if err != nil {
		t.Fatalf("CreateSession: %v", err)
	}
	// completingScript (one-shot) still completes the session normally.
	waitStatus(t, svc, sess.ID, session.StatusCompleted, time.Second)
	if got := fa.openCalls.Load(); got != 1 {
		t.Fatalf("Open called %d times, want 1", got)
	}
}

func TestPersistentTurn_CrashMidTurnDropsFromPoolAndFails(t *testing.T) {
	svc, fa := newPersistentTestService(t, completingScript, func(ctx context.Context, prompt string, opts agentboot.ExecutionOptions) (agentboot.PersistentSession, error) {
		return newFakePersistentSession(ctx, prompt, crashingPersistentScript), nil
	})
	dir := t.TempDir()

	sess, err := svc.CreateSession(context.Background(), CreateSessionInput{Path: dir, Prompt: "hi"})
	if err != nil {
		t.Fatalf("CreateSession: %v", err)
	}
	// A mid-turn crash is a real failure, not something to silently retry
	// one-shot (runPersistentTurn returns handled=true even on error).
	waitStatus(t, svc, sess.ID, session.StatusFailed, time.Second)

	// The crashed session must have been dropped from the pool, so a
	// follow-up message Opens a fresh one rather than reusing the dead one.
	if err := svc.SendMessage(context.Background(), sess.ID, "again"); err != nil {
		t.Fatalf("SendMessage: %v", err)
	}
	// The session's status is already Failed from turn 1, so waitStatus on
	// StatusFailed alone could return before turn 2 even starts. Wait for the
	// second Open call directly, then confirm the status it lands on.
	deadline := time.Now().Add(time.Second)
	for fa.openCalls.Load() < 2 {
		if time.Now().After(deadline) {
			t.Fatalf("Open called %d times, want 2 (the crashed session must not be reused)", fa.openCalls.Load())
		}
		time.Sleep(5 * time.Millisecond)
	}
	waitStatus(t, svc, sess.ID, session.StatusFailed, time.Second)
}

func TestArchive_EvictsResidentPersistentSession(t *testing.T) {
	svc, _ := newPersistentTestService(t, completingScript, func(ctx context.Context, prompt string, opts agentboot.ExecutionOptions) (agentboot.PersistentSession, error) {
		return newFakePersistentSession(ctx, prompt, completingPersistentScript), nil
	})
	dir := t.TempDir()

	sess, err := svc.CreateSession(context.Background(), CreateSessionInput{Path: dir, Prompt: "hi"})
	if err != nil {
		t.Fatalf("CreateSession: %v", err)
	}
	waitStatus(t, svc, sess.ID, session.StatusCompleted, time.Second)

	if _, ok := svc.pool.Acquire(sess.ID); !ok {
		t.Fatalf("session %s not resident in the pool after its first turn completed", sess.ID)
	}

	if _, err := svc.Archive(sess.ID); err != nil {
		t.Fatalf("Archive: %v", err)
	}

	if _, ok := svc.pool.Acquire(sess.ID); ok {
		t.Fatalf("pool still holds a session for %s after Archive — the on-disk session file would stay locked", sess.ID)
	}
}

func TestPersistentTurn_PermissionModeChangeRestartsProcess(t *testing.T) {
	var mu sync.Mutex
	var opened []agentboot.ExecutionOptions
	var sessions []*fakePersistentSession
	svc, fa := newPersistentTestService(t, completingScript, func(ctx context.Context, prompt string, opts agentboot.ExecutionOptions) (agentboot.PersistentSession, error) {
		ps := newFakePersistentSession(ctx, prompt, completingPersistentScript)
		mu.Lock()
		opened = append(opened, opts)
		sessions = append(sessions, ps)
		mu.Unlock()
		return ps, nil
	})
	dir := t.TempDir()

	sess, err := svc.CreateSession(context.Background(), CreateSessionInput{Path: dir, Prompt: "hi"})
	if err != nil {
		t.Fatalf("CreateSession: %v", err)
	}
	waitStatus(t, svc, sess.ID, session.StatusCompleted, time.Second)

	if _, err := svc.SetPermissionMode(sess.ID, PermissionAcceptEdits); err != nil {
		t.Fatalf("SetPermissionMode: %v", err)
	}
	if err := svc.SendMessage(context.Background(), sess.ID, "again"); err != nil {
		t.Fatalf("SendMessage: %v", err)
	}
	deadline := time.Now().Add(time.Second)
	for fa.openCalls.Load() < 2 {
		if time.Now().After(deadline) {
			t.Fatalf("Open called %d times, want 2: a resident process launched with the old mode must not serve the new one", fa.openCalls.Load())
		}
		time.Sleep(5 * time.Millisecond)
	}

	mu.Lock()
	defer mu.Unlock()
	if got := opened[1].PermissionMode; got != PermissionAcceptEdits {
		t.Fatalf("restarted process PermissionMode = %q, want %q", got, PermissionAcceptEdits)
	}
	if !opened[1].Resume || opened[1].SessionID != sess.ID {
		t.Fatalf("restarted process must resume the same Claude session, got Resume=%v SessionID=%q", opened[1].Resume, opened[1].SessionID)
	}
	if st := sessions[0].Status(); st != agentboot.SessionStateTerminated {
		t.Fatalf("old process state = %v, want terminated", st)
	}
}

func TestNewService_RecoversSessionsInterruptedByRestart(t *testing.T) {
	store := newMemStore()
	now := time.Now()
	for _, s := range []*session.Session{
		{ID: "web-running", ChatID: webChatID, Agent: agentType, Status: session.StatusRunning, LastActivity: now},
		{ID: "web-pending", ChatID: webChatID, Agent: agentType, Status: session.StatusPending, LastActivity: now},
		{ID: "web-done", ChatID: webChatID, Agent: agentType, Status: session.StatusFailed, Error: "real failure", LastActivity: now},
		{ID: "im-running", ChatID: "some-im-chat", Agent: agentType, Status: session.StatusRunning, LastActivity: now},
	} {
		_ = store.Set(s.ID, s)
	}
	mgr := session.NewManager(session.Config{Timeout: time.Hour, MessageRetention: time.Hour}, store)
	t.Cleanup(mgr.Stop)

	svc := NewService(Config{Sessions: mgr})

	for id, want := range map[string]session.Status{
		"web-running": session.StatusCompleted,
		"web-pending": session.StatusCompleted,
		"web-done":    session.StatusFailed,  // a real outcome, left alone
		"im-running":  session.StatusRunning, // @cc's session, not ours to touch
	} {
		snap, ok := mgr.Snapshot(id)
		if !ok {
			t.Fatalf("%s: missing", id)
		}
		if snap.Status != want {
			t.Errorf("%s: status %s, want %s", id, snap.Status, want)
		}
	}
	msgs, _ := svc.Messages("web-running")
	if len(msgs) == 0 || !strings.Contains(msgs[len(msgs)-1].Content, "restart") {
		t.Errorf("web-running: want a system note explaining the restart, got %+v", msgs)
	}
}

func TestShutdown_ClosesResidentPersistentSessions(t *testing.T) {
	var resident *fakePersistentSession
	svc, _ := newPersistentTestService(t, completingScript, func(ctx context.Context, prompt string, opts agentboot.ExecutionOptions) (agentboot.PersistentSession, error) {
		resident = newFakePersistentSession(ctx, prompt, completingPersistentScript)
		return resident, nil
	})
	dir := t.TempDir()

	sess, err := svc.CreateSession(context.Background(), CreateSessionInput{Path: dir, Prompt: "hi"})
	if err != nil {
		t.Fatalf("CreateSession: %v", err)
	}
	waitStatus(t, svc, sess.ID, session.StatusCompleted, time.Second)

	svc.Shutdown(context.Background())

	if st := resident.Status(); st != agentboot.SessionStateTerminated {
		t.Fatalf("resident process state after Shutdown = %v, want terminated", st)
	}
}

func TestCreateSession_DoesNotExpire(t *testing.T) {
	svc, _ := newTestService(t, completingScript)
	sess, err := svc.CreateSession(context.Background(), CreateSessionInput{Path: t.TempDir(), Prompt: "hi"})
	if err != nil {
		t.Fatalf("CreateSession: %v", err)
	}
	snap, _ := svc.sessions.Snapshot(sess.ID)
	if !snap.ExpiresAt.IsZero() {
		t.Fatalf("ExpiresAt = %v, want zero: the manager's expiry sweep would delete the session and its transcript", snap.ExpiresAt)
	}
}

func TestTurn_FailureBeforeProcessStartIsRecorded(t *testing.T) {
	// No openFn: the persistent Open fails, the turn falls back to one-shot,
	// and Execute then fails before any process starts.
	svc, fa := newPersistentTestService(t, completingScript, nil)
	fa.executeErr = errors.New("agent CLI not available")

	sess, err := svc.CreateSession(context.Background(), CreateSessionInput{Path: t.TempDir(), Prompt: "hi"})
	if err != nil {
		t.Fatalf("CreateSession: %v", err)
	}
	got := waitStatus(t, svc, sess.ID, session.StatusFailed, time.Second)
	if !strings.Contains(got.Error, "not available") {
		t.Errorf("session error = %q, want the launch failure", got.Error)
	}
	msgs, _ := svc.Messages(sess.ID)
	var shown bool
	for _, m := range msgs {
		if m.Kind == "error" && strings.Contains(m.Content, "not available") {
			shown = true
		}
	}
	if !shown {
		t.Errorf("transcript does not show the launch failure: %+v", msgs)
	}
}

// lingeringBlockingScript is blockingScript whose process takes a moment to
// exit after cancellation, like a CLI shutting down gracefully.
func lingeringBlockingScript(ctx context.Context, prompt string, opts agentboot.ExecutionOptions, h *fakeHandle) {
	<-ctx.Done()
	time.Sleep(50 * time.Millisecond)
	h.err = ctx.Err()
}

func TestArchive_DuringTurnStaysClosed(t *testing.T) {
	svc, _ := newTestService(t, lingeringBlockingScript)
	sess, err := svc.CreateSession(context.Background(), CreateSessionInput{Path: t.TempDir(), Prompt: "hi"})
	if err != nil {
		t.Fatalf("CreateSession: %v", err)
	}
	waitStatus(t, svc, sess.ID, session.StatusRunning, time.Second)

	if _, err := svc.Archive(sess.ID); err != nil {
		t.Fatalf("Archive: %v", err)
	}
	// The page re-reads the session right after archiving, which reloads it
	// into the manager; a turn still exiting must not be able to revive it.
	if got, _ := svc.GetSession(sess.ID); got.Status != session.StatusClosed {
		t.Fatalf("status right after Archive = %s, want closed", got.Status)
	}
	time.Sleep(150 * time.Millisecond)
	if got, _ := svc.GetSession(sess.ID); got.Status != session.StatusClosed {
		t.Fatalf("status after the cancelled turn exited = %s, want closed", got.Status)
	}
}

func TestMessages_ReadableAfterArchive(t *testing.T) {
	svc, _ := newTestService(t, completingScript)
	sess, err := svc.CreateSession(context.Background(), CreateSessionInput{Path: t.TempDir(), Prompt: "hi"})
	if err != nil {
		t.Fatalf("CreateSession: %v", err)
	}
	waitStatus(t, svc, sess.ID, session.StatusCompleted, time.Second)
	if _, err := svc.Archive(sess.ID); err != nil {
		t.Fatalf("Archive: %v", err)
	}
	msgs, err := svc.Messages(sess.ID)
	if err != nil {
		t.Fatalf("Messages after Archive: %v (the transcript is meant to stay readable)", err)
	}
	if len(msgs) == 0 {
		t.Fatal("Messages after Archive returned an empty transcript")
	}
}

func TestStartTurn_LosingClaimRecordsNothing(t *testing.T) {
	svc, _ := newTestService(t, completingScript)
	sess, err := svc.CreateSession(context.Background(), CreateSessionInput{Path: t.TempDir(), Prompt: "hi"})
	if err != nil {
		t.Fatalf("CreateSession: %v", err)
	}
	waitStatus(t, svc, sess.ID, session.StatusCompleted, time.Second)
	before, _ := svc.Messages(sess.ID)

	// Simulate a concurrent send that already won the claim.
	svc.mu.Lock()
	svc.runs[sess.ID] = &run{cancel: func() {}, done: make(chan struct{})}
	svc.mu.Unlock()

	if svc.startTurn(sess.ID, sess.Project, "second", "", "", true) {
		t.Fatal("startTurn succeeded while another turn held the claim")
	}
	after, _ := svc.Messages(sess.ID)
	if len(after) != len(before) {
		t.Fatalf("losing startTurn changed the transcript: %d -> %d messages", len(before), len(after))
	}
}

// fakeRouting knows one profile, "p1".
type fakeRouting struct{}

func (fakeRouting) GetClaudeCodeEnv(context.Context) ([]string, error) {
	return []string{"ANTHROPIC_BASE_URL=http://gateway"}, nil
}

func (fakeRouting) GetClaudeCodeSettingsPathForProfile(_ context.Context, id string) (string, error) {
	if id != "p1" {
		return "", fmt.Errorf("claude code profile %q not found", id)
	}
	return "/profiles/p1/settings.json", nil
}

func newRoutedTestService(t *testing.T, fa *fakeAgent, p *pool.Pool) *Service {
	t.Helper()
	mgr := session.NewManager(session.Config{Timeout: time.Hour, MessageRetention: time.Hour}, newMemStore())
	t.Cleanup(mgr.Stop)
	agentSvc, err := agentboot.NewAgentService(agentboot.DefaultConfig())
	if err != nil {
		t.Fatalf("NewAgentService: %v", err)
	}
	agentSvc.RegisterAgent(agentboot.AgentTypeClaude, fa)
	return NewService(Config{Sessions: mgr, Agent: agentSvc, Routing: fakeRouting{}, Pool: p})
}

func TestProfile_TurnLaunchesWithTheProfileSettingsInsteadOfEnv(t *testing.T) {
	got := make(chan agentboot.ExecutionOptions, 1)
	fa := &fakeAgent{script: func(ctx context.Context, prompt string, opts agentboot.ExecutionOptions, h *fakeHandle) {
		got <- opts
		completingScript(ctx, prompt, opts, h)
	}}
	svc := newRoutedTestService(t, fa, nil)

	sess, err := svc.CreateSession(context.Background(), CreateSessionInput{Path: t.TempDir(), Prompt: "hi", Profile: "p1"})
	if err != nil {
		t.Fatalf("CreateSession: %v", err)
	}
	opts := <-got
	if opts.SettingsPath != "/profiles/p1/settings.json" || len(opts.Env) != 0 {
		t.Fatalf("launch got SettingsPath=%q Env=%v; want the profile's settings and no main-scenario env", opts.SettingsPath, opts.Env)
	}
	if snap, _ := svc.sessions.Snapshot(sess.ID); snap.Profile != "p1" {
		t.Fatalf("session profile = %q, want p1", snap.Profile)
	}
}

func TestProfile_UnknownProfileIsRejectedUpFront(t *testing.T) {
	svc := newRoutedTestService(t, &fakeAgent{script: completingScript}, nil)
	_, err := svc.CreateSession(context.Background(), CreateSessionInput{Path: t.TempDir(), Prompt: "hi", Profile: "nope"})
	if !errors.Is(err, ErrValidation) {
		t.Fatalf("CreateSession with unknown profile: err = %v, want ErrValidation", err)
	}
}

func TestProfile_SwitchingRestartsTheResidentProcess(t *testing.T) {
	var mu sync.Mutex
	var opened []agentboot.ExecutionOptions
	fa := &fakeAgent{script: completingScript, openFn: func(ctx context.Context, prompt string, opts agentboot.ExecutionOptions) (agentboot.PersistentSession, error) {
		mu.Lock()
		opened = append(opened, opts)
		mu.Unlock()
		return newFakePersistentSession(ctx, prompt, completingPersistentScript), nil
	}}
	p := pool.New(pool.Config{MaxSessions: 10, IdleTimeout: time.Hour})
	t.Cleanup(func() { p.Shutdown(context.Background()) })
	svc := newRoutedTestService(t, fa, p)

	sess, err := svc.CreateSession(context.Background(), CreateSessionInput{Path: t.TempDir(), Prompt: "hi"})
	if err != nil {
		t.Fatalf("CreateSession: %v", err)
	}
	waitStatus(t, svc, sess.ID, session.StatusCompleted, time.Second)

	if _, err := svc.SetProfile(context.Background(), sess.ID, "p1"); err != nil {
		t.Fatalf("SetProfile: %v", err)
	}
	if err := svc.SendMessage(context.Background(), sess.ID, "again"); err != nil {
		t.Fatalf("SendMessage: %v", err)
	}
	deadline := time.Now().Add(time.Second)
	for fa.openCalls.Load() < 2 {
		if time.Now().After(deadline) {
			t.Fatalf("Open called %d times, want 2: a process launched without the profile must not serve it", fa.openCalls.Load())
		}
		time.Sleep(5 * time.Millisecond)
	}
	mu.Lock()
	defer mu.Unlock()
	if opened[1].SettingsPath != "/profiles/p1/settings.json" || !opened[1].Resume {
		t.Fatalf("restarted with SettingsPath=%q Resume=%v; want the profile, resuming the same session", opened[1].SettingsPath, opened[1].Resume)
	}
}

func TestAwaitingInput_TracksAnOpenApproval(t *testing.T) {
	svc, _ := newTestService(t, approvalScript)
	sess, err := svc.CreateSession(context.Background(), CreateSessionInput{Path: t.TempDir(), Prompt: "hi"})
	if err != nil {
		t.Fatalf("CreateSession: %v", err)
	}
	deadline := time.Now().Add(time.Second)
	for !svc.AwaitingInput(sess.ID) {
		if time.Now().After(deadline) {
			t.Fatal("AwaitingInput never became true while the approval was open")
		}
		time.Sleep(5 * time.Millisecond)
	}
	if err := svc.Respond(sess.ID, "req-1", true, ""); err != nil {
		t.Fatalf("Respond: %v", err)
	}
	waitStatus(t, svc, sess.ID, session.StatusCompleted, time.Second)
	if svc.AwaitingInput(sess.ID) {
		t.Fatal("AwaitingInput still true after the approval was answered and the turn ended")
	}
}

func TestHandoff_RefusesDuringATurn(t *testing.T) {
	svc, _ := newTestService(t, blockingScript)
	sess, err := svc.CreateSession(context.Background(), CreateSessionInput{Path: t.TempDir(), Prompt: "hi"})
	if err != nil {
		t.Fatalf("CreateSession: %v", err)
	}
	if _, err := svc.Handoff(context.Background(), sess.ID); !errors.Is(err, ErrConflict) {
		t.Fatalf("Handoff during a turn: err = %v, want ErrConflict", err)
	}
	_ = svc.Interrupt(sess.ID)
}

func TestHandoff_ReleasesTheResidentProcessAndBuildsTheCommand(t *testing.T) {
	var resident *fakePersistentSession
	fa := &fakeAgent{script: completingScript, openFn: func(ctx context.Context, prompt string, opts agentboot.ExecutionOptions) (agentboot.PersistentSession, error) {
		resident = newFakePersistentSession(ctx, prompt, completingPersistentScript)
		return resident, nil
	}}
	p := pool.New(pool.Config{MaxSessions: 10, IdleTimeout: time.Hour})
	t.Cleanup(func() { p.Shutdown(context.Background()) })
	svc := newRoutedTestService(t, fa, p)
	dir := filepath.Join(t.TempDir(), "it's here")
	if err := os.Mkdir(dir, 0o755); err != nil {
		t.Fatal(err)
	}

	sess, err := svc.CreateSession(context.Background(), CreateSessionInput{Path: dir, Prompt: "hi", Profile: "p1"})
	if err != nil {
		t.Fatalf("CreateSession: %v", err)
	}
	waitStatus(t, svc, sess.ID, session.StatusCompleted, time.Second)

	cmd, err := svc.Handoff(context.Background(), sess.ID)
	if err != nil {
		t.Fatalf("Handoff: %v", err)
	}
	if st := resident.Status(); st != agentboot.SessionStateTerminated {
		t.Fatalf("resident process state after Handoff = %v, want terminated: it would share the session file with the terminal", st)
	}
	want := `cd '` + strings.ReplaceAll(dir, `'`, `'\''`) + `' && claude --resume '` + sess.ID + `' --settings '/profiles/p1/settings.json'`
	if cmd != want {
		t.Fatalf("command:\n got %s\nwant %s", cmd, want)
	}
}
