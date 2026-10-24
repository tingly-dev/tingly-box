package project

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"maps"
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
	// ErrNotProject is for starting a worker in a project that is a folder.
	ErrNotProject = errors.New("only a project's main thread starts threads in it")
	// ErrMainThread is for running a main thread as an agent session.
	ErrMainThread = errors.New("a main thread is the project conversation; send to it through the coordinator")
)

// MaxRunningPerProject caps concurrently running turns in one project. Turns
// beyond it wait their turn. Preferences such as "at most two at a time"
// are the coordinator's to follow; this is only the safety net.
const MaxRunningPerProject = 4

// MaxRunningSessions caps concurrently running folder threads, which share
// one limit across all folders. It matches the resident process pool.
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
	// OnReply is called after each turn that was not stopped by the user, for
	// every kind of thread (a folder thread included), with the turn's final
	// text or error. The chats bound to that thread hear it. It must not block.
	OnReply func(Report)
	// OnWait is called when a thread stops to ask the user for an approval
	// or an answer, with the request's transcript event. It must not block.
	OnWait func(t db.Thread, request db.ThreadEvent)
	// OnSettled is called when such a request is answered (by the user or
	// its suggestion) or given up, with the event as it ends. It must not
	// block.
	OnSettled func(t db.Thread, request db.ThreadEvent)
	// OnStep is called with each text and tool call of a thread's own turn
	// (not a subagent's), as it is recorded. It must not block.
	OnStep func(t db.Thread, e db.ThreadEvent)

	mu sync.Mutex
	// folders caches which projects are folders (a project's type never changes).
	folders map[string]bool
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
	src  Source
	atts []Attachment
	// held is a message the user queued on purpose: it has no user entry in
	// the transcript yet, only the chip event eventID, until its turn comes.
	held    bool
	eventID string
}

// Source says where a message to a thread came from. The zero value is the
// web. It is stored on the user event the message becomes.
type Source struct {
	// Kind is db.SourceIM, db.SourceThread or db.SourceSystem; empty for the web.
	Kind string
	// Ref is the sending thread's id for db.SourceThread.
	Ref string
	// Platform and ChatID name the IM chat for db.SourceIM.
	Platform string
	ChatID   string
}

// fromThread is the source of a message another thread sent.
func fromThread(id string) Source { return Source{Kind: db.SourceThread, Ref: id} }

func (s Source) stamp(e *db.ThreadEvent) {
	e.Source, e.SourceRef, e.SourcePlatform, e.SourceChatID = s.Kind, s.Ref, s.Platform, s.ChatID
}

// isFolder reports whether a project is a folder.
func (m *Threads) isFolder(projectID string) bool {
	m.mu.Lock()
	f, ok := m.folders[projectID]
	m.mu.Unlock()
	if ok {
		return f
	}
	p, err := m.store.GetProject(projectID)
	if err != nil {
		return false
	}
	m.mu.Lock()
	m.folders[projectID] = p.Type == db.ProjectFolder
	m.mu.Unlock()
	return p.Type == db.ProjectFolder
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
		allowed: map[string]map[string]bool{}, folders: map[string]bool{},
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
	return permissionMode(p)
}

func queuedNote(p *db.Project) string {
	if p.Type == db.ProjectFolder {
		return fmt.Sprintf("Queued: %d folder threads are already running.", MaxRunningSessions)
	}
	return fmt.Sprintf("Queued: %d threads are already running in this project.", MaxRunningPerProject)
}

func permissionMode(p *db.Project) string {
	if p.ThreadPermissionMode != "" {
		return p.ThreadPermissionMode
	}
	return DefaultPermissionMode
}

// slot is the running-turn limit of a project, or of all folder threads
// together (key "").
func (m *Threads) slot(key string) chan struct{} {
	m.mu.Lock()
	defer m.mu.Unlock()
	s, ok := m.slots[key]
	if !ok {
		n := MaxRunningPerProject
		if key == "" {
			n = MaxRunningSessions
		}
		s = make(chan struct{}, n)
		m.slots[key] = s
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

// publish tells open pages a thread changed, with its project's fresh counts.
func (m *Threads) publish(t *db.Thread) {
	ev := Event{Type: EventThread, ProjectID: t.ProjectID, Data: t}
	if c, err := m.store.ProjectThreadCounts(t.ProjectID); err == nil {
		ev.Threads = &c
	}
	m.hub.Publish(ev)
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
	m.publish(t)
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
	if p.Type != db.ProjectProject || p.MainThreadID == "" {
		return nil, ErrNotProject
	}
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
		ParentID:  p.MainThreadID,
		Agent:     db.AgentClaudeCode,
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
	t.Cwd = m.layout.ThreadDir(p.ID, t.ID)
	if repo != nil {
		t.RepoPath, t.BaseBranch = repo.Path, repo.BaseBranch
		t.Branch = fmt.Sprintf("tp/%s-%s", slugify(title), t.ID[:6])
		if err := AddWorktree(ctx, repo.Path, t.Cwd, t.Branch, repo.BaseBranch); err != nil {
			t.Status, t.WaitReason = db.ThreadWaiting, "Could not create the worktree: "+err.Error()
			_ = m.store.SaveThread(t)
			m.publish(t)
			return t, nil
		}
	} else if err := os.MkdirAll(t.Cwd, 0o755); err != nil {
		return nil, err
	}
	if err := m.writeSettings(p, t); err != nil {
		return nil, err
	}
	if err := m.store.SaveThread(t); err != nil {
		return nil, err
	}
	m.publish(t)
	src := fromThread(p.MainThreadID)
	if spec.Origin == "user" {
		src = Source{}
	}
	m.enqueue(t.ID, queued{text: t.Task, src: src})
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

// Send gives a Claude Code thread a message; src says who sent it. A busy
// thread runs it after its current turn. A main thread is not run here: its
// messages go through Service.Send to the coordinator.
func (m *Threads) Send(ctx context.Context, threadID, text string, src Source, atts []Attachment) (*db.Thread, error) {
	if _, err := m.sendable(threadID); err != nil {
		return nil, err
	}
	m.enqueue(threadID, queued{text: text, src: src, atts: atts})
	return m.store.GetThread(threadID)
}

// sendable is the thread if a message may be sent to it now.
func (m *Threads) sendable(threadID string) (*db.Thread, error) {
	t, err := m.store.GetThread(threadID)
	if err != nil {
		return nil, err
	}
	if t.Agent == db.AgentLLM {
		return nil, ErrMainThread
	}
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
	if t.Status == db.ThreadResolved {
		return nil, ErrResolved
	}
	return t, nil
}

// enqueue records the message and starts a turn unless the thread is busy,
// in which case it runs after the current one.
func (m *Threads) enqueue(threadID string, q queued) {
	t, err := m.store.GetThread(threadID)
	if err != nil {
		return
	}
	if q.held && m.Running(threadID) {
		q.eventID = m.holdMessage(t, q)
	} else {
		q.held = false
		ev := &db.ThreadEvent{Kind: db.EvUser, Content: q.text, Payload: AttachmentsPayload(q.atts)}
		q.src.stamp(ev)
		m.addEvent(t, ev)
	}
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
	// The turn ended while the message was being held: it goes now.
	m.releaseHeld(t, &q)
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
	// A folder has no instructions, library or limit but the shared one.
	p, err := m.store.GetProject(t.ProjectID)
	if err != nil {
		m.drop(threadID)
		return
	}
	folder := p.Type == db.ProjectFolder

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	m.mu.Lock()
	a.cancel = cancel
	m.mu.Unlock()
	slotKey := p.ID
	if folder {
		slotKey = ""
	}
	slot := m.slot(slotKey)
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
	prompt := withAttachments(a.current.text, m.layout.LibraryDir(UploadOwner(p, t)), a.current.atts)
	m.mu.Unlock()
	// Waiting for the slot can take a while: launch with what the owner
	// picked since (profile, model, permission mode).
	if fresh, err := m.store.GetThread(threadID); err == nil {
		t = fresh
	}

	env, defModel, settings := m.launchEnv(ctx, t, folder)
	// A project thread runs the project's model. A folder thread runs the one
	// its owner picked; a project thread's Model is only what the API reported,
	// and launching with it would restart the resident process each turn.
	model := t.Model
	if !folder {
		model = p.ThreadModel
	}
	if model == "" {
		model = defModel
	}
	m.mu.Lock()
	m.turns[threadID] = &turnTally{launchModel: model}
	m.mu.Unlock()
	opts := agentboot.ExecutionOptions{Model: model, PermissionMode: threadPermissionMode(p, t), Env: env, SettingsPath: settings, Entrypoint: sessionEntrypoint}
	if !folder {
		opts.AppendSystemPrompt = threadSystemPrompt(m.layout, p, t)
		opts.SettingsPath = m.layout.SettingsFile(p.ID, t.ID)
	} else if path, err := m.sessionSettings(p, t, settings); err == nil {
		opts.SettingsPath = path
	}
	started := m.engine.Start(m.sessionOf(threadID), t.Cwd, prompt, t.Turns > 0, func(context.Context) agentboot.ExecutionOptions { return opts })
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

// sessionSettings lets a folder thread read its own uploads, which live
// outside the folder: the host's settings (a profile's), if any, plus that one
// extra directory. On error the thread keeps the host's settings.
func (m *Threads) sessionSettings(p *db.Project, t *db.Thread, host string) (string, error) {
	doc := map[string]any{}
	if host != "" {
		b, err := os.ReadFile(host)
		if err != nil {
			return host, err
		}
		if err := json.Unmarshal(b, &doc); err != nil {
			return host, err
		}
	}
	perms, _ := doc["permissions"].(map[string]any)
	if perms == nil {
		perms = map[string]any{}
	}
	dirs, _ := perms["additionalDirectories"].([]any)
	perms["additionalDirectories"] = append(dirs, m.layout.LibraryDir(UploadOwner(p, t)))
	doc["permissions"] = perms
	path := m.layout.SessionSettingsFile(t.ID)
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		return host, err
	}
	out, err := json.Marshal(doc)
	if err != nil {
		return host, err
	}
	return path, os.WriteFile(path, out, 0o644)
}

// sessionEntrypoint is what Claude Code records as the way a session was
// started. A headless launch defaults to "sdk-cli", which the interactive
// /resume picker hides; this keeps a thread's session resumable from a
// terminal opened in its folder.
const sessionEntrypoint = "tingly-box"

// launchEnv is how t's turn reaches a model. A folder thread on a host
// with launch profiles runs under its profile, and an unset model is left to
// that profile; everything else runs in the host's thread environment.
func (m *Threads) launchEnv(ctx context.Context, t *db.Thread, folder bool) (env []string, defModel, settings string) {
	if l, ok := m.env.(Launcher); ok && folder {
		env, settings, err := l.LaunchEnv(ctx, t.Profile)
		if err == nil {
			return env, "", settings
		}
		m.addEvent(t, &db.ThreadEvent{Kind: db.EvSystem, Content: fmt.Sprintf("profile %s unavailable, running with the default routing: %v", orDefault(t.Profile), err)})
	}
	env, defModel = m.env.ThreadEnv(ctx)
	return env, defModel, ""
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
	if t, err := m.store.GetThread(threadID); err == nil {
		m.releaseHeld(t, &q)
	}
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
		stats = Stats(context.Background(), t.Cwd, t.BaseBranch)
	}
	session := m.isFolder(t.ProjectID)
	head := ""
	if session {
		head = HeadBranch(context.Background(), t.Cwd)
	}
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
		if session {
			t.HeadBranch = head
		}
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
			t.Status, t.WaitReason = db.ThreadFailed, "Failed: "+end.Err.Error()
			report.Kind, report.Text = ReportFailed, end.Err.Error()
		default:
			if mm := needsInputRe.FindStringSubmatch(report.Text); mm != nil && !session {
				t.Status, t.WaitReason = db.ThreadWaiting, strings.TrimSpace(mm[1])
				report.Kind = ReportNeedsInput
			} else if session && len(m.engine.BackgroundTasks(m.sessionOf(threadID))) > 0 {
				// The turn is over but its process still runs background tasks.
				t.Status = db.ThreadWorking
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
	if report.Kind != "" && m.OnReply != nil {
		report.Thread = *t
		m.OnReply(report)
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
		m.dropHeld(threadID, a.queue)
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

// PendingRequest returns the transcript event of a request the thread is
// still waiting on; ok is false once it is answered or given up.
func (m *Threads) PendingRequest(threadID, requestID string) (db.ThreadEvent, bool) {
	return m.prompter(threadID).request(requestID)
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
	t, err := m.update(threadID, func(t *db.Thread) {
		now := time.Now()
		t.Status, t.WaitReason, t.ResolvedAt = db.ThreadResolved, "", &now
	})
	if err == nil {
		m.releaseChats(t)
	}
	return t, err
}

// releaseChats hands the chats focused on a finished thread back to its
// parent, so they do not keep writing to something that is done. A folder
// thread has no parent: its chat starts a new session instead (PostFromChannel).
func (m *Threads) releaseChats(t *db.Thread) {
	if t.ParentID == "" {
		return
	}
	if _, err := m.store.MoveChannelBindings(t.ID, t.ParentID); err != nil {
		logrus.WithError(err).WithField("thread_id", t.ID).Warn("move chats of a resolved thread to its parent")
	}
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
		m.releaseChats(t)
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
	return BranchDiff(ctx, t.Cwd, t.BaseBranch, t.Branch)
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
	// Failures were recorded as waiting before failed was a status.
	waiting, _ := m.store.ListThreadsByStatus(db.ThreadWaiting)
	for _, t := range waiting {
		if strings.HasPrefix(t.WaitReason, "Failed: ") {
			_, _ = m.update(t.ID, func(t *db.Thread) { t.Status = db.ThreadFailed })
		}
	}
	// Approvals pending at shutdown can no longer be answered.
	waiting, _ = m.store.ListThreadsByStatus(db.ThreadWaiting)
	for _, t := range waiting {
		events, _ := m.store.ListThreadEvents(t.ID)
		for i := range events {
			e := &events[i]
			if e.Kind == db.EvQueued && strings.Contains(e.Payload, `"state":"held"`) {
				e.Payload = strings.Replace(e.Payload, `"state":"held"`, `"state":"removed"`, 1)
				_ = m.store.UpdateThreadEvent(e)
			}
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
		// Folder threads are archived by hand.
		if !m.isFolder(t.ProjectID) && time.Since(t.UpdatedAt) > maxIdle {
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
	// Answers maps each question of an AskUserQuestion call to its answer
	// (an option's label, several joined by ", ", or the user's own words).
	Answers map[string]string
	// Via is where the answer came from: db.SourceIM for a chat, empty for
	// the web.
	Via string
}

// Auto-answer settings: when a question with a suggested answer is answered
// without the user.
const (
	AutoAnswerOff   = "off"
	AutoAnswerDelay = "delay"
	AutoAnswerNow   = "now"
)

// AutoAnswerAfter is how long a question waits under AutoAnswerDelay.
var AutoAnswerAfter = 10 * time.Minute

// ValidAutoAnswer reports whether mode is a setting; empty is the default.
func ValidAutoAnswer(mode string) bool {
	switch mode {
	case "", AutoAnswerOff, AutoAnswerDelay, AutoAnswerNow:
		return true
	}
	return false
}

// autoAnswerMode is the setting a thread's questions follow: its own, else
// delay for a project thread (its owner is usually away) and off for a
// folder session (its owner is usually watching).
func autoAnswerMode(t *db.Thread, folder bool) string {
	if t.AutoAnswer != "" {
		return t.AutoAnswer
	}
	if folder {
		return AutoAnswerOff
	}
	return AutoAnswerDelay
}

var recommendedRe = regexp.MustCompile(`(?i)\s*\((recommended|推荐)\)\s*$`)

// suggestedAnswers answers every question of an AskUserQuestion input with
// the option Claude Code marked "(Recommended)". ok is false when a question
// has no such option or takes several picks: those are left to the user.
func suggestedAnswers(input map[string]any) (map[string]string, bool) {
	qs, _ := input["questions"].([]any)
	if len(qs) == 0 {
		return nil, false
	}
	out := map[string]string{}
	for _, q := range qs {
		m, _ := q.(map[string]any)
		question, _ := m["question"].(string)
		if question == "" {
			return nil, false
		}
		if multi, _ := m["multiSelect"].(bool); multi {
			return nil, false
		}
		opts, _ := m["options"].([]any)
		for _, o := range opts {
			om, _ := o.(map[string]any)
			if label, _ := om["label"].(string); recommendedRe.MatchString(label) {
				out[question] = label
				break
			}
		}
		if _, found := out[question]; !found {
			return nil, false
		}
	}
	return out, true
}

type pending struct {
	// event is the request as it was asked; the waiting turn keeps updating
	// its own copy.
	event db.ThreadEvent
	tool  string
	// retime wakes a waiting question when the auto-answer setting changed.
	retime chan struct{}
	ch     chan Answer
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
	State   string            `json:"state"`
	Scope   string            `json:"scope,omitempty"`
	Answer  string            `json:"answer,omitempty"`
	Answers map[string]string `json:"answers,omitempty"`
	// AutoAt is when a pending question is answered with its suggestion.
	AutoAt *time.Time `json:"auto_at,omitempty"`
	// Auto says the suggestion answered it, not the user.
	Auto bool `json:"auto,omitempty"`
	// Via is where the answer came from (db.SourceIM), empty for the web.
	Via string `json:"via,omitempty"`
}

// retime makes every waiting question read the auto-answer setting again.
func (p *threadPrompter) retime() {
	p.mu.Lock()
	defer p.mu.Unlock()
	for _, pd := range p.pending {
		select {
		case pd.retime <- struct{}{}:
		default:
		}
	}
}

// autoDeadline is when a question asked at asked is answered with its
// suggestion under the thread's setting; nil when it waits for the user.
func (p *threadPrompter) autoDeadline(asked time.Time, suggested bool) *time.Time {
	if !suggested {
		return nil
	}
	t, err := p.m.store.GetThread(p.threadID)
	if err != nil {
		return nil
	}
	var at time.Time
	switch autoAnswerMode(t, p.m.isFolder(t.ProjectID)) {
	case AutoAnswerNow:
		at = time.Now()
	case AutoAnswerDelay:
		at = asked.Add(AutoAnswerAfter)
	default:
		return nil
	}
	return &at
}

func (p *threadPrompter) wait(ctx context.Context, kind, tool, question string, payload requestPayload) (Answer, error) {
	t, err := p.m.store.GetThread(p.threadID)
	if err != nil {
		return Answer{}, err
	}
	// A question whose every part has a suggestion can answer itself.
	var suggested map[string]string
	if kind == db.EvAsk {
		suggested, _ = suggestedAnswers(payload.Input)
	}
	asked := time.Now()
	payload.AutoAt = p.autoDeadline(asked, suggested != nil)
	ev := &db.ThreadEvent{Kind: kind, Content: question, Payload: jsonString(payload)}
	p.m.addEvent(t, ev)
	pd := &pending{event: *ev, tool: tool, ch: make(chan Answer, 1), retime: make(chan struct{}, 1)}
	p.mu.Lock()
	p.pending[payload.RequestID] = pd
	p.mu.Unlock()
	reason := question
	if kind == db.EvApproval {
		reason = "Approve: " + question
	}
	waiting, _ := p.m.update(p.threadID, func(t *db.Thread) { t.Status, t.WaitReason = db.ThreadWaiting, reason })
	if waiting != nil && p.m.OnWait != nil {
		p.m.OnWait(*waiting, *ev)
	}
	settled := func() {
		if p.m.OnSettled != nil {
			cur := *t
			if latest, err := p.m.store.GetThread(p.threadID); err == nil {
				cur = *latest
			}
			p.m.OnSettled(cur, *ev)
		}
	}

	var ans Answer
	timer := time.NewTimer(time.Hour)
	defer timer.Stop()
	arm := func() <-chan time.Time {
		if !timer.Stop() {
			select {
			case <-timer.C:
			default:
			}
		}
		if payload.AutoAt == nil {
			return nil
		}
		timer.Reset(time.Until(*payload.AutoAt))
		return timer.C
	}
	fire := arm()
wait:
	for {
		select {
		case ans = <-pd.ch:
			break wait
		case <-pd.retime:
			payload.AutoAt = p.autoDeadline(asked, suggested != nil)
			ev.Payload = jsonString(payload)
			_ = p.m.store.UpdateThreadEvent(ev)
			p.m.hub.Publish(Event{Type: EventThreadEvent, ProjectID: t.ProjectID, Data: ev})
			fire = arm()
		case <-fire:
			p.mu.Lock()
			_, still := p.pending[payload.RequestID]
			delete(p.pending, payload.RequestID)
			p.mu.Unlock()
			if !still {
				// Answered at the same moment (theirs wins), or the turn is
				// being cancelled: wait for whichever lands.
				fire = nil
				continue
			}
			ans = Answer{Allow: true, Answers: suggested}
			payload.Auto = true
			break wait
		case <-ctx.Done():
			p.mu.Lock()
			delete(p.pending, payload.RequestID)
			p.mu.Unlock()
			payload.State, payload.AutoAt = "expired", nil
			ev.Payload = jsonString(payload)
			_ = p.m.store.UpdateThreadEvent(ev)
			p.m.hub.Publish(Event{Type: EventThreadEvent, ProjectID: t.ProjectID, Data: ev})
			settled()
			return Answer{}, ctx.Err()
		}
	}
	payload.AutoAt = nil
	if kind == db.EvAsk {
		ans = settleAsk(question, payload.Input, ans)
	}
	switch {
	case kind == db.EvAsk:
		payload.State = "answered"
	case ans.Allow:
		payload.State = "allowed"
	default:
		payload.State = "denied"
	}
	payload.Scope, payload.Answer, payload.Answers, payload.Via = ans.Scope, ans.Answer, ans.Answers, ans.Via
	ev.Payload = jsonString(payload)
	_ = p.m.store.UpdateThreadEvent(ev)
	p.m.hub.Publish(Event{Type: EventThreadEvent, ProjectID: t.ProjectID, Data: ev})
	_, _ = p.m.update(p.threadID, func(t *db.Thread) { t.Status, t.WaitReason = db.ThreadWorking, "" })
	settled()
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
	if len(ans.Answers) == 0 {
		return agentboot.AskResponse{Approved: false, Reason: "The user declined to answer."}, nil
	}
	// Claude Code reads the answers from the tool's input, keyed by
	// question; without them the call returns with nothing chosen.
	updated := make(map[string]any, len(req.Input)+1)
	maps.Copy(updated, req.Input)
	updated["answers"] = ans.Answers
	return agentboot.AskResponse{Approved: true, UpdatedInput: updated, Response: ans.Answer}, nil
}

// askQuestions lists the question texts of an AskUserQuestion input, in
// order; fallback stands in when the input carries none.
func askQuestions(input map[string]any, fallback string) []string {
	var out []string
	qs, _ := input["questions"].([]any)
	for _, q := range qs {
		if m, ok := q.(map[string]any); ok {
			if s, ok := m["question"].(string); ok && s != "" {
				out = append(out, s)
			}
		}
	}
	if len(out) == 0 && fallback != "" {
		out = []string{fallback}
	}
	return out
}

// settleAsk completes an answer to a question: a typed answer with no
// per-question answers goes to the first question, answers to questions
// that were not asked are dropped, and Answer becomes what the transcript
// shows (the answer itself, or one "question: answer" line each).
func settleAsk(question string, input map[string]any, ans Answer) Answer {
	questions := askQuestions(input, question)
	answers := map[string]string{}
	for _, q := range questions {
		if a := strings.TrimSpace(ans.Answers[q]); a != "" {
			answers[q] = a
		}
	}
	if len(answers) == 0 && len(questions) > 0 {
		if a := strings.TrimSpace(ans.Answer); a != "" {
			answers[questions[0]] = a
		}
	}
	if !ans.Allow && len(ans.Answers) == 0 {
		// Declined: nothing is chosen, whatever the box held.
		answers = map[string]string{}
	}
	var text []string
	for _, q := range questions {
		a, ok := answers[q]
		if !ok {
			continue
		}
		if len(questions) == 1 {
			text = append(text, a)
		} else {
			text = append(text, q+": "+a)
		}
	}
	ans.Answers, ans.Answer = answers, strings.Join(text, "\n")
	if len(answers) == 0 {
		ans.Answers = nil
	}
	return ans
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

// request returns the transcript event of a pending request.
func (p *threadPrompter) request(requestID string) (db.ThreadEvent, bool) {
	p.mu.Lock()
	defer p.mu.Unlock()
	pd, ok := p.pending[requestID]
	if !ok {
		return db.ThreadEvent{}, false
	}
	return pd.event, true
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
