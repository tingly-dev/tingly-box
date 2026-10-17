package project

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"regexp"
	"strings"
	"sync"
	"time"

	"github.com/google/uuid"
	"github.com/sirupsen/logrus"
	"github.com/tingly-dev/tingly-box/agentboot"
	"github.com/tingly-dev/tingly-box/agentboot/engine"
	"github.com/tingly-dev/tingly-box/projects/internal/db"
)

// Errors returned to callers (mapped to HTTP statuses by the module).
var (
	ErrPaused   = errors.New("the project is paused; resume it first")
	ErrArchived = errors.New("the project is archived; unarchive it first")
	ErrResolved = errors.New("the thread is resolved; reopen it first")
	ErrNoPrompt = errors.New("no pending request with that id")
)

// MaxRunningPerProject caps concurrently running turns in one project. Turns
// beyond it wait their turn. Preferences such as "at most two at a time"
// are the coordinator's to follow; this is only the safety net.
const MaxRunningPerProject = 4

// MaxRunningSessions caps concurrently running folder sessions, which share
// one limit because they belong to no project. It matches the resident
// process pool.
const MaxRunningSessions = 10

// Default permission mode threads run in: edits are accepted, anything else
// that needs approval waits on the user in the thread.
const DefaultPermissionMode = "acceptEdits"

// ThreadEnv supplies what threads need from the global settings: the
// environment pointing Claude Code at the configured gateway, and the model
// to use when the project sets none.
type ThreadEnv interface {
	ThreadEnv(ctx context.Context) (env []string, defaultModel string)
}

// Report is what a finished turn tells the coordinator.
type Report struct {
	Thread db.Thread
	// Kind is finished, needs_input or failed.
	Kind string
	Text string
}

// Report kinds.
const (
	ReportFinished   = "finished"
	ReportNeedsInput = "needs_input"
	ReportFailed     = "failed"
)

// Threads runs and tracks every project's threads. Each thread is one
// Claude Code session, driven by agentboot's session engine; this type adds
// what makes it a thread: its queue, the per-project limit, approvals in the
// thread view, and the report to the coordinator after each turn.
type Threads struct {
	store  *db.Store
	layout Layout
	hub    *Hub
	engine *engine.Engine
	env    ThreadEnv
	// OnReport is called after each turn that was not stopped by the user.
	OnReport func(Report)

	mu      sync.Mutex
	active  map[string]*activeThread
	turns   map[string]*turnTally
	prompts map[string]*threadPrompter
	// The engine knows a thread by its Claude Code session id.
	sessionOfThread map[string]string
	threadOfSession map[string]string
	slots           map[string]chan struct{}
	threads         map[string]*sync.Mutex // serializes writes to one thread row
	// allowed holds tools the user allowed for the rest of a thread.
	allowed map[string]map[string]bool
}

// activeThread is a thread with a message in flight: waiting for a slot,
// or in a turn it started.
type activeThread struct {
	cancel  context.CancelFunc // gives up waiting for a slot
	current *queued            // the message of the turn in flight
	slot    chan struct{}      // held while current runs
	queue   []queued
	stopped bool
}

type queued struct {
	text string
	from string
}

// turnTally is what a turn's transcript entries add up to, for its report.
type turnTally struct {
	text         string // the main conversation's last text
	inputTokens  int64
	outputTokens int64
	model        string
	launchModel  string
}

// NewThreads builds the thread manager.
func NewThreads(store *db.Store, layout Layout, hub *Hub, agents Agents, env ThreadEnv) *Threads {
	m := &Threads{
		store: store, layout: layout, hub: hub, env: env,
		active: map[string]*activeThread{}, turns: map[string]*turnTally{}, prompts: map[string]*threadPrompter{},
		sessionOfThread: map[string]string{}, threadOfSession: map[string]string{},
		slots: map[string]chan struct{}{}, threads: map[string]*sync.Mutex{},
		allowed: map[string]map[string]bool{},
	}
	m.engine = engine.New(engine.Config{
		Agent:     agents.Service,
		Pool:      agents.Pool,
		Store:     threadStore{m},
		Prompter:  func(sid string) agentboot.Prompter { return m.prompter(m.threadOf(sid)) },
		OnTurnEnd: func(sid string, end engine.TurnEnd) { m.turnEnded(m.threadOf(sid), end) },
		PermissionMode: func(sid string) string {
			t, err := store.GetThread(m.threadOf(sid))
			if err != nil {
				return DefaultPermissionMode
			}
			if t.PermissionMode != "" {
				return t.PermissionMode
			}
			if p, err := store.GetProject(t.ProjectID); err == nil {
				return permissionMode(p)
			}
			return DefaultPermissionMode
		},
	})
	return m
}

// Shutdown stops every turn and closes every resident process.
func (m *Threads) Shutdown(ctx context.Context) { m.engine.Shutdown(ctx) }

// sessionOf is the session id the engine knows threadID by.
func (m *Threads) sessionOf(threadID string) string {
	m.mu.Lock()
	sid, ok := m.sessionOfThread[threadID]
	m.mu.Unlock()
	if ok {
		return sid
	}
	t, err := m.store.GetThread(threadID)
	if err != nil {
		return ""
	}
	m.mu.Lock()
	m.sessionOfThread[threadID], m.threadOfSession[t.SessionID] = t.SessionID, threadID
	m.mu.Unlock()
	return t.SessionID
}

// threadOf is the thread whose session the engine calls sid. Every session
// the engine runs was started through sessionOf, so it is known.
func (m *Threads) threadOf(sid string) string {
	m.mu.Lock()
	defer m.mu.Unlock()
	return m.threadOfSession[sid]
}

func (m *Threads) prompter(threadID string) *threadPrompter {
	m.mu.Lock()
	defer m.mu.Unlock()
	p, ok := m.prompts[threadID]
	if !ok {
		p = newThreadPrompter(m, threadID)
		m.prompts[threadID] = p
	}
	return p
}

// threadPermissionMode is the mode a thread's turns run in: its own, else
// its project's.
func threadPermissionMode(p *db.Project, t *db.Thread) string {
	if t.PermissionMode != "" {
		return t.PermissionMode
	}
	if p == nil {
		return DefaultPermissionMode
	}
	return permissionMode(p)
}

func queuedNote(p *db.Project) string {
	if p == nil {
		return fmt.Sprintf("Queued: %d sessions are already running.", MaxRunningSessions)
	}
	return fmt.Sprintf("Queued: %d threads are already running in this project.", MaxRunningPerProject)
}

func permissionMode(p *db.Project) string {
	if p.ThreadPermissionMode != "" {
		return p.ThreadPermissionMode
	}
	return DefaultPermissionMode
}

// slot is the running-turn limit of a project, or of all folder sessions
// together (projectID "").
func (m *Threads) slot(projectID string) chan struct{} {
	m.mu.Lock()
	defer m.mu.Unlock()
	s, ok := m.slots[projectID]
	if !ok {
		n := MaxRunningPerProject
		if projectID == "" {
			n = MaxRunningSessions
		}
		s = make(chan struct{}, n)
		m.slots[projectID] = s
	}
	return s
}

func (m *Threads) rowLock(threadID string) *sync.Mutex {
	m.mu.Lock()
	defer m.mu.Unlock()
	l, ok := m.threads[threadID]
	if !ok {
		l = &sync.Mutex{}
		m.threads[threadID] = l
	}
	return l
}

// update applies fn to the stored thread under its lock, saves and publishes.
func (m *Threads) update(threadID string, fn func(t *db.Thread)) (*db.Thread, error) {
	l := m.rowLock(threadID)
	l.Lock()
	defer l.Unlock()
	t, err := m.store.GetThread(threadID)
	if err != nil {
		return nil, err
	}
	fn(t)
	if err := m.store.SaveThread(t); err != nil {
		return nil, err
	}
	m.hub.Publish(Event{Type: EventThread, ProjectID: t.ProjectID, Data: t})
	return t, nil
}

func (m *Threads) addEvent(t *db.Thread, e *db.ThreadEvent) {
	e.ThreadID = t.ID
	if err := m.store.AddThreadEvent(e); err != nil {
		logrus.WithError(err).Warn("save thread event")
		return
	}
	m.hub.Publish(Event{Type: EventThreadEvent, ProjectID: t.ProjectID, Data: e})
}

func jsonString(v any) string {
	if v == nil {
		return ""
	}
	b, err := json.Marshal(v)
	if err != nil {
		return ""
	}
	return string(b)
}

// StartSpec describes a new thread.
type StartSpec struct {
	Title string
	Task  string
	// Repo is the path of one of the project's repos; empty picks the first,
	// "none" works without a repository.
	Repo   string
	Origin string
}

var slugRe = regexp.MustCompile(`[^a-z0-9]+`)

func slugify(s string) string {
	s = strings.Trim(slugRe.ReplaceAllString(strings.ToLower(s), "-"), "-")
	if len(s) > 32 {
		s = strings.Trim(s[:32], "-")
	}
	if s == "" {
		s = "thread"
	}
	return s
}

// Start creates a thread, prepares its workspace and runs its first turn.
func (m *Threads) Start(ctx context.Context, p *db.Project, spec StartSpec) (*db.Thread, error) {
	if p.ArchivedAt != nil {
		return nil, ErrArchived
	}
	if p.Paused {
		return nil, ErrPaused
	}
	title := strings.TrimSpace(spec.Title)
	if title == "" {
		title = firstLine(spec.Task, 60)
	}
	t := &db.Thread{
		ProjectID: p.ID,
		Title:     title,
		Task:      strings.TrimSpace(spec.Task),
		Status:    db.ThreadWorking,
		Origin:    spec.Origin,
		SessionID: uuid.NewString(),
	}
	repo, err := pickRepo(p, spec.Repo)
	if err != nil {
		return nil, err
	}
	if err := m.store.CreateThread(t); err != nil {
		return nil, err
	}
	t.Worktree = m.layout.WorktreeDir(p.ID, t.ID)
	if repo != nil {
		t.RepoPath, t.BaseBranch = repo.Path, repo.BaseBranch
		t.Branch = fmt.Sprintf("tp/%s-%s", slugify(title), t.ID[:6])
		if err := AddWorktree(ctx, repo.Path, t.Worktree, t.Branch, repo.BaseBranch); err != nil {
			t.Status, t.WaitReason = db.ThreadWaiting, "Could not create the worktree: "+err.Error()
			_ = m.store.SaveThread(t)
			m.hub.Publish(Event{Type: EventThread, ProjectID: p.ID, Data: t})
			return t, nil
		}
	} else if err := os.MkdirAll(t.Worktree, 0o755); err != nil {
		return nil, err
	}
	if err := m.writeSettings(p, t); err != nil {
		return nil, err
	}
	if err := m.store.SaveThread(t); err != nil {
		return nil, err
	}
	m.hub.Publish(Event{Type: EventThread, ProjectID: p.ID, Data: t})
	from := "coordinator"
	if spec.Origin == "user" {
		from = "user"
	}
	m.enqueue(t.ID, queued{text: t.Task, from: from})
	return t, nil
}

func pickRepo(p *db.Project, want string) (*db.Repo, error) {
	if want == "none" || len(p.Repos) == 0 {
		return nil, nil
	}
	if want == "" {
		return &p.Repos[0], nil
	}
	clean := filepath.Clean(want)
	for i := range p.Repos {
		r := &p.Repos[i]
		if filepath.Clean(r.Path) == clean || filepath.Base(r.Path) == want {
			return r, nil
		}
	}
	return nil, fmt.Errorf("repository %q is not part of this project", want)
}

// writeSettings gives the thread's Claude Code session access to the
// project's library, memory and other repositories.
func (m *Threads) writeSettings(p *db.Project, t *db.Thread) error {
	dirs := []string{m.layout.LibraryDir(p.ID), m.layout.MemoryDir(p.ID)}
	for _, r := range p.Repos {
		if r.Path != t.RepoPath {
			dirs = append(dirs, r.Path)
		}
	}
	settings := map[string]any{"permissions": map[string]any{"additionalDirectories": dirs}}
	path := m.layout.SettingsFile(p.ID, t.ID)
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		return err
	}
	return os.WriteFile(path, []byte(jsonString(settings)), 0o644)
}

// Send gives a thread a message from the user or the coordinator. A busy
// thread runs it after its current turn.
func (m *Threads) Send(ctx context.Context, threadID, text, from string) (*db.Thread, error) {
	t, err := m.store.GetThread(threadID)
	if err != nil {
		return nil, err
	}
	if t.ProjectID != "" {
		p, err := m.store.GetProject(t.ProjectID)
		if err != nil {
			return nil, err
		}
		if p.ArchivedAt != nil {
			return nil, ErrArchived
		}
		if p.Paused {
			return nil, ErrPaused
		}
	}
	if t.Status == db.ThreadResolved {
		return nil, ErrResolved
	}
	m.enqueue(threadID, queued{text: text, from: from})
	return m.store.GetThread(threadID)
}

// enqueue records the message and starts a turn unless the thread is busy,
// in which case it runs after the current one.
func (m *Threads) enqueue(threadID string, q queued) {
	t, err := m.store.GetThread(threadID)
	if err != nil {
		return
	}
	m.addEvent(t, &db.ThreadEvent{Kind: db.EvUser, Content: q.text, From: q.from})
	followingUp := m.engine.Busy(m.sessionOf(threadID))
	m.mu.Lock()
	if a, busy := m.active[threadID]; busy || followingUp {
		if !busy {
			// Claude is following up on a background task by itself; the
			// message goes after that turn.
			a = &activeThread{}
			m.active[threadID] = a
		}
		// A message sent after Stop is new work, not part of what was
		// stopped: it runs once the stopped turn ends.
		a.stopped = false
		a.queue = append(a.queue, q)
		m.mu.Unlock()
		if !busy {
			// That turn may have ended since Busy said otherwise, with
			// nothing left to pick this message up.
			m.next(threadID)
		}
		return
	}
	a := &activeThread{current: &q}
	m.active[threadID] = a
	m.mu.Unlock()
	go m.begin(threadID, a)
}

var needsInputRe = regexp.MustCompile(`(?m)^\s*NEEDS_INPUT:\s*(.+)$`)

// begin waits for a slot in the project, then starts a's current message
// as a turn.
func (m *Threads) begin(threadID string, a *activeThread) {
	t, err := m.store.GetThread(threadID)
	if err != nil {
		m.drop(threadID)
		return
	}
	// A folder session has no project: no instructions, library or limit
	// but the shared one.
	var p *db.Project
	if t.ProjectID != "" {
		if p, err = m.store.GetProject(t.ProjectID); err != nil {
			m.drop(threadID)
			return
		}
	}

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	m.mu.Lock()
	a.cancel = cancel
	m.mu.Unlock()
	slot := m.slot(t.ProjectID)
	select {
	case slot <- struct{}{}:
	default:
		m.addEvent(t, &db.ThreadEvent{Kind: db.EvSystem, Content: queuedNote(p)})
		select {
		case slot <- struct{}{}:
		case <-ctx.Done():
			m.giveUp(threadID, a)
			return
		}
	}

	m.mu.Lock()
	if a.stopped {
		m.mu.Unlock()
		<-slot
		m.giveUp(threadID, a)
		return
	}
	a.slot = slot
	prompt := a.current.text
	m.mu.Unlock()
	// Waiting for the slot can take a while: launch with what the owner
	// picked since (model, permission mode).
	if fresh, err := m.store.GetThread(threadID); err == nil {
		t = fresh
	}

	env, defModel := m.env.ThreadEnv(ctx)
	// A project thread runs the project's model. A session runs the one its
	// owner picked; a project thread's Model is only what the API reported,
	// and launching with it would restart the resident process each turn.
	model := t.Model
	if p != nil {
		model = p.ThreadModel
	}
	if model == "" {
		model = defModel
	}
	m.mu.Lock()
	m.turns[threadID] = &turnTally{launchModel: model}
	m.mu.Unlock()
	opts := agentboot.ExecutionOptions{Model: model, PermissionMode: threadPermissionMode(p, t), Env: env}
	if p != nil {
		opts.AppendSystemPrompt = threadSystemPrompt(m.layout, p, t)
		opts.SettingsPath = m.layout.SettingsFile(p.ID, t.ID)
	}
	started := m.engine.Start(m.sessionOf(threadID), t.Worktree, prompt, t.Turns > 0, func(context.Context) agentboot.ExecutionOptions { return opts })
	if !started {
		// Claude started a turn of its own in the meantime: this message
		// waits for it (turnEnded picks it up).
		m.mu.Lock()
		a.queue = append([]queued{*a.current}, a.queue...)
		a.current, a.slot = nil, nil
		m.mu.Unlock()
		<-slot
		m.next(threadID)
	}
}

// drop forgets a thread's in-flight state after it gave up before a turn.
func (m *Threads) drop(threadID string) {
	m.mu.Lock()
	delete(m.active, threadID)
	m.mu.Unlock()
}

// giveUp drops a's current message after Stop caught it before its turn,
// and moves on to anything sent since.
func (m *Threads) giveUp(threadID string, a *activeThread) {
	m.mu.Lock()
	a.current = nil
	m.mu.Unlock()
	m.next(threadID)
}

// turnEnded settles a finished turn: the thread's status and counters, its
// report to the coordinator, and the next queued message.
func (m *Threads) turnEnded(threadID string, end engine.TurnEnd) {
	m.prompter(threadID).cancelAll()

	m.mu.Lock()
	a := m.active[threadID]
	var slot chan struct{}
	if a != nil && !end.Unsolicited {
		slot, a.slot = a.slot, nil
		if end.NotSent && a.current != nil {
			// Claude took the turn first; the message goes after it.
			a.queue = append([]queued{*a.current}, a.queue...)
		}
		a.current = nil
	}
	tally := m.turns[threadID]
	delete(m.turns, threadID)
	m.mu.Unlock()
	if slot != nil {
		<-slot
	}

	if !end.NotSent {
		m.settle(threadID, end, tally)
	}

	m.next(threadID)
}

// next starts the thread's next queued message, unless a turn is in
// flight (its end calls next again), or forgets the thread when nothing is
// left.
func (m *Threads) next(threadID string) {
	m.mu.Lock()
	a := m.active[threadID]
	if a == nil || a.current != nil {
		m.mu.Unlock()
		return
	}
	if a.stopped || len(a.queue) == 0 {
		delete(m.active, threadID)
		m.mu.Unlock()
		return
	}
	m.mu.Unlock()
	if m.engine.Busy(m.sessionOf(threadID)) {
		return
	}
	m.mu.Lock()
	if a.current != nil || len(a.queue) == 0 {
		m.mu.Unlock()
		return
	}
	q := a.queue[0]
	a.queue = a.queue[1:]
	a.current = &q
	m.mu.Unlock()
	go m.begin(threadID, a)
}

// settle records a finished turn on the thread and reports it.
func (m *Threads) settle(threadID string, end engine.TurnEnd, tally *turnTally) {
	if tally == nil {
		tally = &turnTally{}
	}
	t, err := m.store.GetThread(threadID)
	if err != nil {
		return
	}
	stats := BranchStats{}
	if t.RepoPath != "" {
		stats = Stats(context.Background(), t.Worktree, t.BaseBranch)
	}
	session := t.ProjectID == ""
	report := Report{Kind: ReportFinished, Text: strings.TrimSpace(tally.text)}
	t, _ = m.update(threadID, func(t *db.Thread) {
		t.Turns++
		t.InputTokens += tally.inputTokens
		t.OutputTokens += tally.outputTokens
		// A session's Model is the owner's pick; a project thread shows the
		// model that answered.
		switch {
		case session:
		case tally.model != "":
			t.Model = tally.model
		case t.Model == "":
			t.Model = tally.launchModel
		}
		t.CommitsAhead, t.FilesChanged, t.Uncommitted = stats.CommitsAhead, stats.FilesChanged, stats.Uncommitted
		if t.Status == db.ThreadResolved {
			// Resolved while the turn was ending: that decision stands.
			return
		}
		t.WaitReason = ""
		switch {
		case end.Stopped:
			t.Status = db.ThreadIdle
			report.Kind = ""
		case end.Err != nil:
			t.Status, t.WaitReason = db.ThreadWaiting, "Failed: "+end.Err.Error()
			report.Kind, report.Text = ReportFailed, end.Err.Error()
		default:
			if mm := needsInputRe.FindStringSubmatch(report.Text); mm != nil && !session {
				t.Status, t.WaitReason = db.ThreadWaiting, strings.TrimSpace(mm[1])
				report.Kind = ReportNeedsInput
			} else if stats.CommitsAhead > 0 {
				t.Status = db.ThreadReady
			} else {
				t.Status = db.ThreadIdle
			}
		}
		if report.Text != "" {
			t.LastReport = report.Text
		}
	})
	if t == nil || t.Status == db.ThreadResolved {
		return
	}
	switch {
	case report.Kind == "":
		m.addEvent(t, &db.ThreadEvent{Kind: db.EvSystem, Content: "Stopped."})
	case end.Err != nil:
		m.addEvent(t, &db.ThreadEvent{Kind: db.EvError, Content: end.Err.Error()})
	case session:
		// The report is for the coordinator; a session shows the turn itself.
	default:
		m.addEvent(t, &db.ThreadEvent{Kind: db.EvReport, Content: report.Text, Payload: jsonString(map[string]any{
			"status": t.Status, "commits_ahead": stats.CommitsAhead, "files_changed": stats.FilesChanged, "uncommitted": stats.Uncommitted,
		})})
	}
	if session {
		// Nobody to report to; the session page shows the turn itself.
		return
	}
	_ = m.store.TouchProject(t.ProjectID)
	if report.Kind != "" && m.OnReport != nil {
		report.Thread = *t
		m.OnReport(report)
	}
}

// Stop interrupts a thread's running turn and drops its queued messages.
// Its resident process, and the background work in it, survive.
func (m *Threads) Stop(threadID string) { m.stop(threadID) }

// stop is Stop, returning a channel closed once the stopped turn has
// exited (nil when no turn was running).
func (m *Threads) stop(threadID string) <-chan struct{} {
	m.mu.Lock()
	if a, ok := m.active[threadID]; ok {
		a.stopped = true
		a.queue = nil
		if a.cancel != nil {
			a.cancel()
		}
	}
	m.mu.Unlock()
	done, _ := m.engine.Stop(m.sessionOf(threadID))
	return done
}

// Running reports whether a thread has a turn in flight or waiting.
func (m *Threads) Running(threadID string) bool {
	m.mu.Lock()
	_, ok := m.active[threadID]
	m.mu.Unlock()
	return ok || m.engine.Busy(m.sessionOf(threadID))
}

// StopProject interrupts every running thread of a project (pause, archive,
// delete).
func (m *Threads) StopProject(projectID string) {
	threads, err := m.store.ListThreads(projectID)
	if err != nil {
		return
	}
	for _, t := range threads {
		m.Stop(t.ID)
	}
}

// Respond answers a pending approval or question in a thread.
func (m *Threads) Respond(threadID, requestID string, ans Answer) error {
	return m.prompter(threadID).resolve(requestID, ans)
}

// Resolve marks a thread done (it stays readable and can be reopened).
func (m *Threads) Resolve(threadID string) (*db.Thread, error) {
	if done := m.stop(threadID); done != nil {
		// Let the turn take its interrupt before its process is closed.
		select {
		case <-done:
		case <-time.After(agentboot.SessionCloseTimeout):
		}
	}
	// A resolved thread's process has nothing left to do.
	m.engine.Evict(m.sessionOf(threadID))
	return m.update(threadID, func(t *db.Thread) {
		now := time.Now()
		t.Status, t.WaitReason, t.ResolvedAt = db.ThreadResolved, "", &now
	})
}

// Reopen brings a resolved thread back.
func (m *Threads) Reopen(threadID string) (*db.Thread, error) {
	return m.update(threadID, func(t *db.Thread) {
		t.ResolvedAt = nil
		t.Status = db.ThreadIdle
		if t.CommitsAhead > 0 && t.MergedAt == nil {
			t.Status = db.ThreadReady
		}
	})
}

// Merge merges a thread's branch into its base in the main checkout and
// resolves the thread.
func (m *Threads) Merge(ctx context.Context, threadID string) (*db.Thread, error) {
	t, err := m.store.GetThread(threadID)
	if err != nil {
		return nil, err
	}
	if t.RepoPath == "" || t.Branch == "" {
		return nil, errors.New("this thread has no branch to merge")
	}
	if m.Running(threadID) {
		return nil, errors.New("the thread is still working; wait for it to finish or stop it")
	}
	if err := MergeBranch(ctx, t.RepoPath, t.BaseBranch, t.Branch, fmt.Sprintf("Merge %s: %s", t.Branch, t.Title)); err != nil {
		return nil, err
	}
	t, err = m.update(threadID, func(t *db.Thread) {
		now := time.Now()
		t.MergedAt, t.ResolvedAt, t.Status, t.WaitReason = &now, &now, db.ThreadResolved, ""
	})
	if err == nil {
		m.addEvent(t, &db.ThreadEvent{Kind: db.EvSystem, Content: fmt.Sprintf("Merged %s into %s.", t.Branch, t.BaseBranch)})
	}
	return t, err
}

// Diff returns a thread's changes for review.
func (m *Threads) Diff(ctx context.Context, threadID string) (Diff, error) {
	t, err := m.store.GetThread(threadID)
	if err != nil {
		return Diff{}, err
	}
	if t.RepoPath == "" {
		return Diff{}, errors.New("this thread works without a repository")
	}
	return BranchDiff(ctx, t.Worktree, t.BaseBranch, t.Branch)
}

// RemoveWorktrees deletes a project's worktrees (branches are kept).
func (m *Threads) RemoveWorktrees(ctx context.Context, projectID string) {
	threads, _ := m.store.ListThreads(projectID)
	for _, t := range threads {
		if t.RepoPath != "" {
			_ = RemoveWorktree(ctx, t.RepoPath, t.Worktree)
		}
	}
}

// Recover runs at startup: turns that were running when the process stopped
// are gone, so their threads wait on the user to continue them.
func (m *Threads) Recover() {
	threads, err := m.store.ListThreadsByStatus(db.ThreadWorking)
	if err != nil {
		return
	}
	for _, t := range threads {
		_, _ = m.update(t.ID, func(t *db.Thread) {
			t.Status, t.WaitReason = db.ThreadWaiting, "Interrupted when tingly-project stopped. Send a message to continue."
		})
	}
	// Approvals pending at shutdown can no longer be answered.
	waiting, _ := m.store.ListThreadsByStatus(db.ThreadWaiting)
	for _, t := range waiting {
		events, _ := m.store.ListThreadEvents(t.ID)
		for i := range events {
			e := &events[i]
			if (e.Kind == db.EvApproval || e.Kind == db.EvAsk) && strings.Contains(e.Payload, `"state":"pending"`) {
				e.Payload = strings.Replace(e.Payload, `"state":"pending"`, `"state":"expired"`, 1)
				_ = m.store.UpdateThreadEvent(e)
			}
		}
	}
}

// AutoResolve resolves threads that have been idle for a week, as the
// official Overview does.
func (m *Threads) AutoResolve(maxIdle time.Duration) {
	threads, err := m.store.ListThreadsByStatus(db.ThreadIdle)
	if err != nil {
		return
	}
	for _, t := range threads {
		// Folder sessions are archived by hand, as Desk's were.
		if t.ProjectID != "" && time.Since(t.UpdatedAt) > maxIdle {
			_, _ = m.Resolve(t.ID)
		}
	}
}

func firstLine(s string, n int) string {
	s = strings.TrimSpace(s)
	if i := strings.IndexByte(s, '\n'); i >= 0 {
		s = s[:i]
	}
	r := []rune(s)
	if len(r) > n {
		return strings.TrimSpace(string(r[:n])) + "…"
	}
	return s
}

// ---- prompter ----

// Answer is the user's response to an approval or question.
type Answer struct {
	Allow bool
	// Scope "thread" also allows later calls of the same tool in this thread.
	Scope  string
	Answer string
}

type pending struct {
	event *db.ThreadEvent
	tool  string
	ch    chan Answer
}

// threadPrompter routes a turn's approvals and questions to the thread view:
// the request becomes a transcript entry, the thread waits on the user, and
// the turn blocks until they answer in the thread (no timeout, as in the
// official Projects).
type threadPrompter struct {
	m        *Threads
	threadID string

	mu      sync.Mutex
	pending map[string]*pending
}

func newThreadPrompter(m *Threads, threadID string) *threadPrompter {
	return &threadPrompter{m: m, threadID: threadID, pending: map[string]*pending{}}
}

func (m *Threads) isAllowed(threadID, tool string) bool {
	m.mu.Lock()
	defer m.mu.Unlock()
	return m.allowed[threadID][tool]
}

func (m *Threads) allow(threadID, tool string) {
	m.mu.Lock()
	defer m.mu.Unlock()
	if m.allowed[threadID] == nil {
		m.allowed[threadID] = map[string]bool{}
	}
	m.allowed[threadID][tool] = true
}

type requestPayload struct {
	RequestID string         `json:"request_id"`
	Tool      string         `json:"tool,omitempty"`
	Input     map[string]any `json:"input,omitempty"`
	Reason    string         `json:"reason,omitempty"`
	// State is pending, allowed, denied, answered or expired.
	State  string `json:"state"`
	Scope  string `json:"scope,omitempty"`
	Answer string `json:"answer,omitempty"`
}

func (p *threadPrompter) wait(ctx context.Context, kind, tool, question string, payload requestPayload) (Answer, error) {
	t, err := p.m.store.GetThread(p.threadID)
	if err != nil {
		return Answer{}, err
	}
	ev := &db.ThreadEvent{Kind: kind, Content: question, Payload: jsonString(payload)}
	p.m.addEvent(t, ev)
	pd := &pending{event: ev, tool: tool, ch: make(chan Answer, 1)}
	p.mu.Lock()
	p.pending[payload.RequestID] = pd
	p.mu.Unlock()
	reason := question
	if kind == db.EvApproval {
		reason = "Approve: " + question
	}
	_, _ = p.m.update(p.threadID, func(t *db.Thread) { t.Status, t.WaitReason = db.ThreadWaiting, reason })

	var ans Answer
	select {
	case ans = <-pd.ch:
	case <-ctx.Done():
		p.mu.Lock()
		delete(p.pending, payload.RequestID)
		p.mu.Unlock()
		payload.State = "expired"
		ev.Payload = jsonString(payload)
		_ = p.m.store.UpdateThreadEvent(ev)
		p.m.hub.Publish(Event{Type: EventThreadEvent, ProjectID: t.ProjectID, Data: ev})
		return Answer{}, ctx.Err()
	}
	switch {
	case kind == db.EvAsk:
		payload.State = "answered"
	case ans.Allow:
		payload.State = "allowed"
	default:
		payload.State = "denied"
	}
	payload.Scope, payload.Answer = ans.Scope, ans.Answer
	ev.Payload = jsonString(payload)
	_ = p.m.store.UpdateThreadEvent(ev)
	p.m.hub.Publish(Event{Type: EventThreadEvent, ProjectID: t.ProjectID, Data: ev})
	_, _ = p.m.update(p.threadID, func(t *db.Thread) { t.Status, t.WaitReason = db.ThreadWorking, "" })
	return ans, nil
}

func (p *threadPrompter) OnApproval(ctx context.Context, req agentboot.ApprovalRequestEvent) (agentboot.ApprovalResponse, error) {
	if p.m.isAllowed(p.threadID, req.ToolName) {
		return agentboot.ApprovalResponse{Approved: true}, nil
	}
	id := req.ID
	if id == "" {
		id = uuid.NewString()
	}
	ans, err := p.wait(ctx, db.EvApproval, req.ToolName, describeTool(req.ToolName, req.Input), requestPayload{
		RequestID: id, Tool: req.ToolName, Input: req.Input, Reason: req.Reason, State: "pending",
	})
	if err != nil {
		return agentboot.ApprovalResponse{Approved: false, Reason: "not answered"}, nil
	}
	if ans.Allow && ans.Scope == "thread" {
		p.m.allow(p.threadID, req.ToolName)
	}
	reason := ""
	if !ans.Allow {
		reason = "The user denied this action."
		if ans.Answer != "" {
			reason += " " + ans.Answer
		}
	}
	return agentboot.ApprovalResponse{Approved: ans.Allow, Reason: reason}, nil
}

func (p *threadPrompter) OnAsk(ctx context.Context, req agentboot.AskRequestEvent) (agentboot.AskResponse, error) {
	id := req.ID
	if id == "" {
		id = uuid.NewString()
	}
	question := req.Message
	if question == "" {
		question = describeQuestion(req.Input)
	}
	ans, err := p.wait(ctx, db.EvAsk, req.ToolName, question, requestPayload{RequestID: id, Tool: req.ToolName, Input: req.Input, State: "pending"})
	if err != nil {
		return agentboot.AskResponse{Approved: false, Reason: "not answered"}, nil
	}
	return agentboot.AskResponse{Approved: true, Response: ans.Answer}, nil
}

func (p *threadPrompter) resolve(requestID string, ans Answer) error {
	p.mu.Lock()
	pd, ok := p.pending[requestID]
	if ok {
		delete(p.pending, requestID)
	}
	p.mu.Unlock()
	if !ok {
		return ErrNoPrompt
	}
	pd.ch <- ans
	return nil
}

func (p *threadPrompter) cancelAll() {
	p.mu.Lock()
	defer p.mu.Unlock()
	for id := range p.pending {
		delete(p.pending, id)
	}
}

// describeTool renders a permission request as the concrete thing the
// user is approving (the command, the file), not just the tool name.
func describeTool(tool string, input map[string]any) string {
	pick := func(keys ...string) string {
		for _, k := range keys {
			if v, ok := input[k].(string); ok && v != "" {
				return v
			}
		}
		return ""
	}
	switch tool {
	case "Bash":
		if c := pick("command"); c != "" {
			return "Run `" + firstLine(c, 200) + "`"
		}
	case "Write", "Edit", "MultiEdit", "NotebookEdit":
		if f := pick("file_path", "notebook_path"); f != "" {
			return tool + " " + f
		}
	case "WebFetch":
		if u := pick("url"); u != "" {
			return "Fetch " + u
		}
	}
	if v := pick("command", "file_path", "path", "url", "pattern", "query"); v != "" {
		return tool + ": " + firstLine(v, 200)
	}
	return "Use " + tool
}

func describeQuestion(input map[string]any) string {
	if qs, ok := input["questions"].([]any); ok && len(qs) > 0 {
		if q, ok := qs[0].(map[string]any); ok {
			if s, ok := q["question"].(string); ok {
				return s
			}
		}
	}
	return "The thread has a question for you."
}
