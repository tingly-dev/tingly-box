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
	"sort"
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
	ErrPaused         = errors.New("the project is paused; resume it first")
	ErrArchived       = errors.New("the project is archived; unarchive it first")
	ErrThreadArchived = errors.New("the session is archived; restore it first")
	ErrResolved       = errors.New("the thread is resolved; reopen it first")
	ErrNoPrompt       = errors.New("no pending request with that id")
	// ErrNotProject is for starting a worker in a project that is a folder.
	ErrNotProject = errors.New("only a project's main thread starts threads in it")
	// ErrNoOwnBranch is for choosing a base or branch where the thread gets none.
	ErrNoOwnBranch = errors.New("base and branch need a repository and its own worktree: the thread uses no directory, or its directory is shared by every thread (a plain directory, or a Git one set to share)")
	// ErrMainThread is for running a main thread as an agent session.
	ErrMainThread = errors.New("a main thread is the project conversation; send to it through the coordinator")
	// ErrEmptyTitle is for renaming a thread to nothing.
	ErrEmptyTitle = errors.New("the title is empty")
	// ErrNoFailedTurn is for retrying a thread whose latest turn did not fail.
	ErrNoFailedTurn = errors.New("the thread has no failed turn to retry")
)

// MaxRunningThreadsLimit is the largest limit a project can set. There is
// no unlimited setting.
const MaxRunningThreadsLimit = 100

// RunningLimit is how many turns may run at once under p. A folder has no
// setting of its own: all folders share MaxRunningSessions. A project without
// a valid setting gets the default, never "no limit".
func RunningLimit(p *db.Project) int {
	if p.Type == db.ProjectFolder {
		return MaxRunningSessions
	}
	if p.MaxRunningThreads < 1 {
		return db.DefaultMaxRunningThreads
	}
	return p.MaxRunningThreads
}

// ValidMaxRunningThreads reports whether n is a value a project can be set to.
func ValidMaxRunningThreads(n int) bool { return n >= 1 && n <= MaxRunningThreadsLimit }

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
	// Kind is finished, needs_input, failed or stalled.
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
	// SessionToolsConfig provides the MCP configuration scoped to the current thread.
	SessionToolsConfig func(threadID string) (string, error)

	// StallAfter is how long a running thread may show no activity before
	// the coordinator is told; zero means DefaultStallAfter.
	StallAfter time.Duration
	// HoldGrace is how long a held report waits for the follow-up turn Claude
	// starts when a background task ends; zero means DefaultHoldGrace.
	HoldGrace time.Duration
	// HoldMax is the longest a report is held for background tasks that never
	// end; zero means DefaultHoldMax.
	HoldMax time.Duration
	// backgroundCount replaces the engine's count of a thread's running
	// background tasks; tests only.
	backgroundCount func(threadID string) int

	mu sync.Mutex
	// stalled holds the running threads already reported as quiet.
	stalled map[string]bool
	// held holds the finished report of a worker whose turn ended while it
	// still had background tasks (background.go).
	held map[string]*heldReport
	// folders caches which projects are folders (a project's type never changes).
	folders map[string]bool
	active  map[string]*activeThread
	turns   map[string]*turnTally
	prompts map[string]*threadPrompter
	// The engine knows a thread by its Claude Code session id.
	sessionOfThread map[string]string
	threadOfSession map[string]string
	slots           map[string]*limiter
	threads         map[string]*sync.Mutex // serializes writes to one thread row
	lifecycle       map[string]*sync.Mutex // serializes message admission and session rotation
	// allowed holds what the user allowed for the rest of a thread (allowKey).
	allowed map[string]map[string]bool
	// cards holds each thread's compaction card while one is running, so its
	// outcome updates it in place.
	cards map[string]*db.ThreadEvent
}

// activeThread is a thread with a message in flight: waiting for a slot,
// or in a turn it started.
type activeThread struct {
	cancel  context.CancelFunc // gives up waiting for a slot
	current *queued            // the message of the turn in flight
	slot    *limiter           // held while current runs
	queue   []queued
	stopped bool
	// stoppedBy is who stopped the turn, for the "Stopped" note.
	stoppedBy Source
}

type queued struct {
	text string
	src  Source
	atts []Attachment
	// relayed is a follow-up the coordinator sent with message_thread. Its
	// prompt says so; a thread's first task is its assignment and needs no mark.
	relayed bool
	// held is a message the user queued on purpose: it has no user entry in
	// the transcript yet, only the chip event eventID, until its turn comes.
	held    bool
	eventID string
	// compact makes the message a compaction request: its turn asks the agent
	// to compact instead of sending text (see Threads.Compact).
	compact *compactRequest
}

// compactRequest is a /compact waiting for its turn.
type compactRequest struct{ instructions string }

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

// coordinatorNote opens a message the coordinator sent to a thread, so the
// thread can tell it from the user and does not take it as the user's approval.
const coordinatorNote = "[From the project coordinator, not the user. It does not approve anything that needs the user's decision.]\n\n"

// queuedPrompt is the text a thread's turn starts with: the message, marked
// when the coordinator relayed it, plus its attachments.
func queuedPrompt(q queued, libraryDir string) string {
	text := q.text
	if q.relayed {
		text = coordinatorNote + text
	}
	return withAttachments(literalPrompt(text), libraryDir, q.atts)
}

func (m *Threads) threadPrompt(q queued, p *db.Project, t *db.Thread) string {
	text := q.text
	if q.relayed {
		text = coordinatorNote + text
	}
	text = literalPrompt(text)
	note := m.layout.threadAttachmentNote(p, t, q.atts)
	if note == "" {
		return text
	}
	if strings.TrimSpace(text) == "" {
		return note
	}
	return text + "\n\n" + note
}

// fromThread is the source of a message another thread sent.
func fromThread(id string) Source { return Source{Kind: db.SourceThread, Ref: id} }

// actor says who acted, for a note in the thread ("Stopped by the
// coordinator."). The web is the default and says nothing.
func (s Source) actor() string {
	switch s.Kind {
	case db.SourceIM:
		return " from a chat"
	case db.SourceThread:
		return " by the coordinator"
	}
	return ""
}

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
	text        string // the main conversation's last text
	usage       db.Usage
	model       string
	launchModel string
	// compact marks a turn that compacts the conversation: it reports to
	// the chats that hear the thread, not to the coordinator.
	compact bool
}

// addUsage folds a "usage" entry in; main is false for a subagent's.
func (t *turnTally) addUsage(u engine.TurnUsage, main bool) {
	t.usage.Input += u.InputTokens + u.CacheReadTokens + u.CacheWriteTokens
	t.usage.Output += u.OutputTokens
	t.usage.CacheRead += u.CacheReadTokens
	t.usage.CacheWrite += u.CacheWriteTokens
	if main {
		if u.Model != "" {
			t.model = u.Model
		}
		t.usage.ContextTokens, t.usage.ContextWindow, t.usage.DurationMS = u.ContextTokens, u.ContextWindow, u.DurationMS
	}
}

// NewThreads builds the thread manager.
func NewThreads(store *db.Store, layout Layout, hub *Hub, agents Agents, env ThreadEnv) *Threads {
	m := &Threads{
		store: store, layout: layout, hub: hub, env: env,
		active: map[string]*activeThread{}, turns: map[string]*turnTally{}, prompts: map[string]*threadPrompter{},
		sessionOfThread: map[string]string{}, threadOfSession: map[string]string{},
		slots: map[string]*limiter{}, threads: map[string]*sync.Mutex{}, lifecycle: map[string]*sync.Mutex{},
		allowed: map[string]map[string]bool{}, folders: map[string]bool{}, held: map[string]*heldReport{}, cards: map[string]*db.ThreadEvent{},
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

// waitIdle waits until no thread has a turn in flight or queued, so nothing
// still writes its events, stats or files, or until ctx ends.
func (m *Threads) waitIdle(ctx context.Context) {
	tick := time.NewTicker(5 * time.Millisecond)
	defer tick.Stop()
	for {
		m.mu.Lock()
		idle := len(m.active) == 0
		m.mu.Unlock()
		if idle {
			return
		}
		select {
		case <-ctx.Done():
			return
		case <-tick.C:
		}
	}
}

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
	if t.SessionID == "" {
		// Not started yet: nothing to remember, and "" must not name a thread.
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
	n := RunningLimit(p)
	if p.Type == db.ProjectFolder {
		return fmt.Sprintf("Queued: %d folder threads are already running.", n)
	}
	return fmt.Sprintf("Queued: %d threads are already running in this project (its limit). It starts when one finishes.", n)
}

func permissionMode(p *db.Project) string {
	if p.ThreadPermissionMode != "" {
		return p.ThreadPermissionMode
	}
	return DefaultPermissionMode
}

// slot is the running-turn limiter of p's project, or of all folder threads
// together (key ""), set to p's current limit.
func (m *Threads) slot(p *db.Project) *limiter {
	key := p.ID
	if p.Type == db.ProjectFolder {
		key = ""
	}
	limit := RunningLimit(p)
	m.mu.Lock()
	l, ok := m.slots[key]
	if !ok {
		l = newLimiter(limit)
		m.slots[key] = l
	}
	m.mu.Unlock()
	if ok {
		l.setLimit(limit)
	}
	return l
}

// ApplyLimit makes a project's running limit take effect now: raising it
// starts waiting turns, lowering it leaves running turns alone.
func (m *Threads) ApplyLimit(p *db.Project) { m.slot(p) }

// setQueued marks whether t's next turn waits for a slot.
func (m *Threads) setQueued(threadID string, queued bool) {
	_, _ = m.update(threadID, func(t *db.Thread) { t.Queued = queued })
}

func (m *Threads) lifecycleLock(threadID string) *sync.Mutex {
	m.mu.Lock()
	defer m.mu.Unlock()
	if m.lifecycle[threadID] == nil {
		m.lifecycle[threadID] = &sync.Mutex{}
	}
	return m.lifecycle[threadID]
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
	t.RunningTasks = m.RunningTasks(t.ID)
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
	Repo string
	// Base is the local branch the thread's branch starts from; empty uses
	// the repository's base branch. Branch names the new branch; empty
	// generates one. Both need a worktree.
	Base, Branch string
	Origin       string
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
	if repo != nil {
		effective := p.EffectiveRepo(*repo)
		// Reject a directory whose type has changed since configuration rather than
		// silently running in a different workspace than the user selected.
		info, err := InspectDirectory(ctx, effective.Path)
		if err != nil {
			return nil, err
		}
		original, err := filepath.EvalSymlinks(effective.Path)
		if err != nil {
			return nil, err
		}
		if db.PathKey(info.Root) != db.PathKey(original) || info.Kind != effective.Kind {
			return nil, fmt.Errorf("directory %q has changed since it was configured", effective.Path)
		}
	}
	base, branch, err := chooseBranch(ctx, p, repo, spec)
	if err != nil {
		return nil, err
	}
	if err := m.store.CreateThread(t); err != nil {
		return nil, err
	}
	t.Cwd = m.layout.ThreadDir(p.ID, t.ID)
	var note string
	if repo != nil {
		t.RepoPath, t.BaseBranch = repo.Path, repo.BaseBranch
		if base != "" {
			t.BaseBranch = base
		}
		if p.EffectiveRepo(*repo).WorkspaceMode == db.WorkspaceShared {
			t.Cwd = repo.Path
		} else {
			t.Branch = branch
			if t.Branch == "" {
				t.Branch = fmt.Sprintf("tp/%s-%s", slugify(title), t.ID[:6])
			}
			// In the repository's own .claude/worktrees; if that cannot be used
			// (read-only repository, ...) fall back to the data directory, where
			// threads used to live, and say so.
			inRepo := RepoWorktreeDir(repo.Path, slugify(title), t.ID)
			err := excludeWorktrees(ctx, repo.Path)
			if err == nil {
				err = AddWorktree(ctx, repo.Path, inRepo, t.Branch, t.BaseBranch)
			}
			if err == nil {
				t.Cwd = inRepo
			} else {
				note = fmt.Sprintf("Could not create the worktree in %s (%v); using %s instead.", filepath.Join(repo.Path, ".claude", "worktrees"), err, t.Cwd)
				// A failed git worktree add can leave its branch behind. Use a
				// fresh name rather than deleting a branch we may not own.
				t.Branch += "-fallback-" + t.ID[6:12]
				if err := AddWorktree(ctx, repo.Path, t.Cwd, t.Branch, t.BaseBranch); err != nil {
					t.Status, t.WaitReason = db.ThreadWaiting, "Could not create the worktree: "+err.Error()
					_ = m.store.SaveThread(t)
					m.publish(t)
					return t, nil
				}
			}
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
	if note != "" {
		m.addEvent(t, &db.ThreadEvent{Kind: db.EvSystem, Content: note})
	}
	src := fromThread(p.MainThreadID)
	if spec.Origin == "user" {
		src = Source{}
	}
	m.enqueue(t.ID, queued{text: t.Task, src: src})
	return t, nil
}

// chooseBranch checks the base and branch name a caller asked for, before
// anything is created. Both empty means the defaults.
func chooseBranch(ctx context.Context, p *db.Project, repo *db.Repo, spec StartSpec) (base, branch string, err error) {
	base, branch = strings.TrimSpace(spec.Base), strings.TrimSpace(spec.Branch)
	if base == "" && branch == "" {
		return "", "", nil
	}
	if repo == nil || p.EffectiveRepo(*repo).WorkspaceMode == db.WorkspaceShared {
		return "", "", ErrNoOwnBranch
	}
	if base != "" && !BranchExists(ctx, repo.Path, base) {
		return "", "", fmt.Errorf("base %q is not a local branch of %s", base, repo.Path)
	}
	if branch != "" {
		if err := checkNewBranch(ctx, repo.Path, branch); err != nil {
			return "", "", err
		}
	}
	return base, branch, nil
}

// checkProposals refuses a proposed card Start would refuse later: the user
// cannot edit a card, so a bad base or a taken branch could never start. Two
// cards naming the same branch collide too, so the second is refused here.
func checkProposals(ctx context.Context, p *db.Project, threads []Suggestion) error {
	taken := map[string]bool{}
	for i, s := range threads {
		repo, err := pickRepo(p, s.Repo)
		if err != nil {
			return fmt.Errorf("thread %d: %w", i+1, err)
		}
		_, branch, err := chooseBranch(ctx, p, repo, StartSpec{Base: s.Base, Branch: s.Branch})
		if err != nil {
			return fmt.Errorf("thread %d: %w", i+1, err)
		}
		if branch != "" {
			key := repo.Path + "\x00" + branch
			if taken[key] {
				return fmt.Errorf("thread %d: branch %q is used by another proposed thread", i+1, branch)
			}
			taken[key] = true
		}
	}
	return nil
}

func pickRepo(p *db.Project, want string) (*db.Repo, error) {
	if want == "none" || len(p.Repos) == 0 {
		return nil, nil
	}
	if want == "" {
		return p.PrimaryRepo(), nil
	}
	clean := filepath.Clean(want)
	var match *db.Repo
	for i := range p.Repos {
		r := &p.Repos[i]
		if filepath.Clean(r.Path) == clean {
			return r, nil
		}
		if filepath.Base(r.Path) == want {
			if match != nil {
				return nil, fmt.Errorf("directory name %q is ambiguous; use its full path", want)
			}
			match = r
		}
	}
	if match != nil {
		return match, nil
	}
	return nil, fmt.Errorf("directory %q is not part of this project", want)
}

// writeSettings gives the thread's Claude Code session access to the
// project's library, memory and other repositories.
func (m *Threads) writeSettings(p *db.Project, t *db.Thread) error {
	// autoMemoryDirectory points Claude Code's own memory at the project's, so
	// a project has one memory whatever each thread's cwd is.
	settings := map[string]any{"autoMemoryDirectory": m.layout.MemoryDir(p.ID)}
	m.grantProjectAccess(settings, p, t)
	path := m.layout.SettingsFile(p.ID, t.ID)
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		return err
	}
	return writeFileIfChanged(path, []byte(jsonString(settings)), 0o644)
}

// writeFileIfChanged replaces path with data only when its content differs,
// through a rename, so a reader never sees it half written. The resident
// process reads the file only when it starts; permission rules that changed
// reach it as a control request instead (agentboot/engine applyLive), and
// never restart it.
func writeFileIfChanged(path string, data []byte, perm os.FileMode) error {
	if old, err := os.ReadFile(path); err == nil && string(old) == string(data) {
		return nil
	}
	tmp, err := os.CreateTemp(filepath.Dir(path), filepath.Base(path)+".tmp*")
	if err != nil {
		return err
	}
	_, err = tmp.Write(data)
	if cerr := tmp.Close(); err == nil {
		err = cerr
	}
	if err == nil {
		err = os.Chmod(tmp.Name(), perm)
	}
	if err == nil {
		err = os.Rename(tmp.Name(), path)
	}
	if err != nil {
		_ = os.Remove(tmp.Name())
	}
	return err
}

// grantProjectAccess adds the directories every project thread works with to
// doc: the library (the shared upload directory, with rules so it is writable
// in any permission mode), the memory and the project's other repositories.
func (m *Threads) grantProjectAccess(doc map[string]any, p *db.Project, t *db.Thread) {
	dirs := []string{m.layout.LibraryDir(p.ID), m.layout.MemoryDir(p.ID)}
	for _, r := range p.Repos {
		if r.Path != t.RepoPath {
			dirs = append(dirs, r.Path)
		}
	}
	addDirectories(doc, dirs...)
	grantLibrary(doc, m.layout.LibraryDir(p.ID))
	grantAutoGit(doc, p, t)
}

// Send gives a Claude Code thread a message; src says who sent it. A busy
// thread runs it after its current turn. A main thread is not run here: its
// messages go through Service.Send to the coordinator.
func (m *Threads) Send(ctx context.Context, threadID, text string, src Source, atts []Attachment) (*db.Thread, error) {
	lock := m.lifecycleLock(threadID)
	lock.Lock()
	defer lock.Unlock()
	if _, err := m.sendable(threadID); err != nil {
		return nil, err
	}
	m.enqueue(threadID, queued{text: text, src: src, atts: atts, relayed: src.Kind == db.SourceThread})
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
	if t.ArchivedAt != nil {
		return nil, ErrThreadArchived
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

// turnLaunch builds one turn's launch options: env and settings (worker
// profile included), model, effort, permission mode and session tools.
// Shared by begin (a queued user message) and Retry (a failed turn). An
// error means the turn cannot launch (a worker profile is unavailable); the
// thread has already been marked failed and noted.
func (m *Threads) turnLaunch(ctx context.Context, threadID string, t *db.Thread, p *db.Project, folder bool) (model string, opts agentboot.ExecutionOptions, err error) {
	env, defModel, settings := m.launchEnv(ctx, t, folder)
	if !folder {
		profile := workerProfile(p, t)
		if l, ok := m.env.(Launcher); ok && profile != "" {
			env, settings, err = l.LaunchEnv(ctx, profile)
			if err == nil {
				settings, err = m.workerSettings(p, t, settings)
			}
			if err != nil {
				m.addEvent(t, &db.ThreadEvent{Kind: db.EvSystem, Content: fmt.Sprintf("profile %s unavailable: %v", profile, err)})
				_, _ = m.update(t.ID, func(t *db.Thread) {
					t.Status, t.WaitReason = db.ThreadFailed, fmt.Sprintf("Failed: profile %s unavailable: %v", profile, err)
				})
				return "", opts, fmt.Errorf("profile %s unavailable: %w", profile, err)
			}
			defModel = ""
		} else if profile != "" {
			m.addEvent(t, &db.ThreadEvent{Kind: db.EvSystem, Content: "profile unavailable"})
			_, _ = m.update(t.ID, func(t *db.Thread) { t.Status, t.WaitReason = db.ThreadFailed, "Failed: profile unavailable" })
			return "", opts, errors.New("profile unavailable")
		}
	}
	// A worker's observed Model is history, not its launch choice. Only an
	// explicit override pins it instead of following the project's default.
	model = t.Model
	if !folder {
		model = t.ModelOverride
		if model == "" && t.ProfileOverride == nil {
			model = p.ThreadModel
		}
	}
	if model == "" {
		model = defModel
	}
	effort := t.EffortOverride
	if effort == "" && !folder {
		effort = p.ThreadEffort
	}
	env = m.withTmpEnv(env, p, t)
	opts = agentboot.ExecutionOptions{Model: model, Effort: effort, PermissionMode: threadPermissionMode(p, t), Env: env, SettingsPath: settings, Entrypoint: sessionEntrypoint}
	if m.SessionToolsConfig != nil {
		if config, err := m.SessionToolsConfig(threadID); err == nil {
			opts.MCPConfigPath = config
		} else {
			logrus.WithError(err).Warn("projects: cannot start session tools")
		}
	}
	if !folder {
		opts.AppendSystemPrompt = threadSystemPrompt(m.layout, p, t)
		if settings == "" {
			// The permission mode may have changed since Start wrote the file.
			if err := m.writeSettings(p, t); err != nil {
				logrus.WithError(err).WithField("thread_id", t.ID).Warn("projects: cannot refresh thread settings")
			}
			opts.SettingsPath = m.layout.SettingsFile(p.ID, t.ID)
		}
	} else if path, err := m.sessionSettings(p, t, settings); err == nil {
		opts.SettingsPath = path
	}
	return model, opts, nil
}

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
	slot := m.slot(p)
	waited := false
	err = slot.acquire(ctx, func() {
		waited = true
		m.addEvent(t, &db.ThreadEvent{Kind: db.EvSystem, Content: queuedNote(p)})
		m.setQueued(threadID, true)
	})
	if waited {
		m.setQueued(threadID, false)
	}
	if err != nil {
		m.giveUp(threadID, a)
		return
	}

	m.mu.Lock()
	if a.stopped {
		m.mu.Unlock()
		slot.release()
		m.giveUp(threadID, a)
		return
	}
	a.slot = slot
	prompt := m.threadPrompt(*a.current, p, t)
	m.mu.Unlock()
	// Waiting for the slot can take a while: launch with what the owner
	// picked since (profile, model, permission mode).
	if fresh, err := m.store.GetThread(threadID); err == nil {
		t = fresh
	}
	if fresh, err := m.store.GetProject(t.ProjectID); err == nil {
		p = fresh
	}

	model, opts, lerr := m.turnLaunch(ctx, threadID, t, p, folder)
	if lerr != nil {
		m.mu.Lock()
		a.slot = nil
		m.mu.Unlock()
		slot.release()
		m.giveUp(threadID, a)
		return
	}
	if req := a.current.compact; req != nil {
		m.mu.Lock()
		m.turns[threadID] = &turnTally{launchModel: model, compact: true}
		m.mu.Unlock()
		launch := func(context.Context) agentboot.ExecutionOptions { return opts }
		res, err := m.engine.Compact(ctx, m.sessionOf(threadID), t.Cwd, req.instructions, launch)
		m.compactReturned(t, a, slot, res, err)
		return
	}
	m.mu.Lock()
	m.turns[threadID] = &turnTally{launchModel: model}
	m.mu.Unlock()
	started := m.engine.Start(m.sessionOf(threadID), t.Cwd, prompt, t.Turns > 0, func(context.Context) agentboot.ExecutionOptions { return opts })
	if !started {
		// Claude started a turn of its own in the meantime: this message
		// waits for it (turnEnded picks it up).
		m.mu.Lock()
		a.queue = append([]queued{*a.current}, a.queue...)
		a.current, a.slot = nil, nil
		m.mu.Unlock()
		slot.release()
		m.next(threadID)
		return
	}
	// A Stop that landed while the turn was launching found nothing to
	// interrupt in the engine, and its cancel does not reach a turn the engine
	// owns. It left a.stopped for us: stop the turn now that it exists.
	m.mu.Lock()
	stopped := a.stopped
	m.mu.Unlock()
	if stopped {
		m.engine.Stop(m.sessionOf(threadID))
	}
}

// withTmpEnv points the thread's TMPDIR (and TMP, TEMP) at its scratch
// directory in the shared library and creates it, whatever the launch mode:
// project, profile or folder. On error the thread keeps the global temp dir.
func (m *Threads) withTmpEnv(env []string, p *db.Project, t *db.Thread) []string {
	dir := m.layout.TmpDir(UploadOwner(p, t), t.ID)
	if err := os.MkdirAll(dir, 0o755); err != nil {
		logrus.WithError(err).WithField("thread_id", t.ID).Warn("projects: cannot create the thread's temp dir")
		return env
	}
	return setEnv(env, "TMPDIR", dir, "TMP", dir, "TEMP", dir)
}

// cleanTmp removes a finished thread's scratch directory. Files it sent with
// send_file were copied into uploads/ and stay.
func (m *Threads) cleanTmp(t *db.Thread) {
	if t == nil {
		return
	}
	dir := m.layout.TmpDir(t.ProjectID, t.ID)
	if err := os.RemoveAll(dir); err != nil {
		logrus.WithError(err).WithField("thread_id", t.ID).Warn("projects: cannot remove the thread's temp dir")
	}
}

// workerSettings combines profile settings with the project's required access.
func (m *Threads) workerSettings(p *db.Project, t *db.Thread, host string) (string, error) {
	doc := map[string]any{}
	b, err := os.ReadFile(host)
	if err != nil {
		return "", err
	}
	if err := json.Unmarshal(b, &doc); err != nil {
		return "", err
	}
	m.grantProjectAccess(doc, p, t)
	doc["autoMemoryDirectory"] = m.layout.MemoryDir(p.ID)
	path := m.layout.SessionSettingsFile(t.ID)
	if err := os.MkdirAll(filepath.Dir(path), 0700); err != nil {
		return "", err
	}
	out, err := json.Marshal(doc)
	if err != nil {
		return "", err
	}
	return path, os.WriteFile(path, out, 0600)
}

// sessionSettings lets a folder thread read the folder's uploads, which live
// outside the folder: the host's settings (a profile's), if any, plus that
// extra directory (and the thread's own legacy upload area when it has one).
// On error the thread keeps the host's settings.
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
	library := m.layout.LibraryDir(UploadOwner(p, t))
	addDirectories(doc, library)
	grantLibrary(doc, library)
	// Uploads of a thread from before they were shared per folder.
	if legacy := m.layout.LibraryDir(SessionOwner(t.ID)); dirExists(legacy) {
		addDirectories(doc, legacy)
	}
	doc["autoMemoryDirectory"] = m.layout.MemoryDir(p.ID)
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
	var slot *limiter
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
		slot.release()
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
	m.dropHeldReport(threadID)
	t, err := m.store.GetThread(threadID)
	if err != nil {
		return
	}
	stats := BranchStats{}
	if t.Branch != "" {
		stats = Stats(context.Background(), t.Cwd, t.BaseBranch)
	}
	session := m.isFolder(t.ProjectID)
	head := ""
	if session {
		head = HeadBranch(context.Background(), t.Cwd)
	}
	report := Report{Kind: ReportFinished, Text: strings.TrimSpace(tally.text)}
	held := false
	t, _ = m.update(threadID, func(t *db.Thread) {
		// A compaction is not a conversation turn and makes no model call: it
		// leaves the turn count, the model that answered and the last turn's
		// figures as they were.
		if !tally.compact {
			t.Turns++
			t.ReportedModel = tally.model
			t.AddUsage(tally.usage, time.Now().UTC())
		}
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
		if t.Status == db.ThreadResolved || t.ArchivedAt != nil {
			// A finish or archive decision made while the turn ended stands.
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
			} else if m.hasBackground(threadID) {
				// The turn is over but its process still runs background tasks.
				// A worker's report waits for the follow-up turn Claude starts
				// when they end (background.go).
				t.Status = db.ThreadWorking
				held = !session
			} else {
				t.Status = restingStatus(stats.CommitsAhead)
			}
		}
		if report.Text != "" && !tally.compact {
			t.LastReport = report.Text
		}
	})
	if t == nil || t.Status == db.ThreadResolved || t.ArchivedAt != nil {
		return
	}
	switch {
	case report.Kind == "":
		m.mu.Lock()
		var by Source
		if a := m.active[threadID]; a != nil {
			by = a.stoppedBy
		}
		m.mu.Unlock()
		m.addEvent(t, &db.ThreadEvent{Kind: db.EvSystem, Content: "Stopped" + by.actor() + "."})
	case end.Err != nil:
		m.addEvent(t, &db.ThreadEvent{Kind: db.EvError, Content: end.Err.Error()})
	case session, tally.compact:
		// The report is for the coordinator; a session shows the turn itself,
		// and a compaction shows its own card.
	default:
		m.addEvent(t, &db.ThreadEvent{Kind: db.EvReport, Content: report.Text, Payload: jsonString(map[string]any{
			"status": t.Status, "commits_ahead": stats.CommitsAhead, "files_changed": stats.FilesChanged, "uncommitted": stats.Uncommitted,
		})})
	}
	if held {
		report.Thread = *t
		m.hold(report)
		return
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
	if tally.compact {
		return
	}
	if report.Kind != "" && m.OnReport != nil {
		report.Thread = *t
		m.OnReport(report)
	}
}

// Stop interrupts a thread's running turn and drops its queued messages.
// Its resident process, and the background work in it, survive. by is who
// asked, for the note in the thread; stopping a thread with nothing running
// does nothing.
func (m *Threads) Stop(threadID string, by Source) { m.stop(threadID, by) }

// Retry re-runs a failed turn without adding anything to the transcript.
// A turn that crashed mid-turn (with staged output) resumes the interrupted
// turn — the CLI replays it from its own transcript, so the user's message
// is never re-sent and interrupted tool calls are not re-run; a turn that
// failed before reaching Claude re-sends its prompt.
func (m *Threads) Retry(ctx context.Context, threadID string) (*db.Thread, error) {
	lock := m.lifecycleLock(threadID)
	lock.Lock()
	defer lock.Unlock()
	t, err := m.store.GetThread(threadID)
	if err != nil {
		return nil, err
	}
	if t.Status != db.ThreadFailed {
		return nil, ErrNoFailedTurn
	}
	p, err := m.store.GetProject(t.ProjectID)
	if err != nil {
		return nil, err
	}
	sid := m.sessionOf(threadID)
	if m.Running(threadID) {
		return nil, ErrBusy
	}
	model, opts, err := m.turnLaunch(ctx, threadID, t, p, p.Type == db.ProjectFolder)
	if err != nil {
		return nil, err
	}
	m.mu.Lock()
	m.turns[sid] = &turnTally{launchModel: model}
	m.mu.Unlock()
	if !m.engine.Retry(sid, t.Cwd, func(context.Context) agentboot.ExecutionOptions { return opts }) {
		return nil, ErrNoFailedTurn
	}
	return m.store.GetThread(threadID)
}

// stop is Stop, returning a channel closed once the stopped turn has
// exited (nil when no turn was running).
func (m *Threads) stop(threadID string, by Source) <-chan struct{} {
	m.mu.Lock()
	if a, ok := m.active[threadID]; ok {
		a.stopped, a.stoppedBy = true, by
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
		m.Stop(t.ID, Source{Kind: db.SourceSystem})
	}
}

// PendingRequest returns the transcript event of a request the thread is
// still waiting on; ok is false once it is answered or given up.
func (m *Threads) PendingRequest(threadID, requestID string) (db.ThreadEvent, bool) {
	return m.prompter(threadID).request(requestID)
}

// PendingRequests returns the transcript events of every approval and
// question the thread is still waiting on, oldest first: what a card must
// show to be answered without opening the thread.
func (m *Threads) PendingRequests(threadID string) []db.ThreadEvent {
	return m.prompter(threadID).requests()
}

// Respond answers a pending approval or question in a thread.
func (m *Threads) Respond(threadID, requestID string, ans Answer) error {
	return m.prompter(threadID).resolve(requestID, ans)
}

// Resolve marks a thread done (it stays readable and can be reopened).
func (m *Threads) Resolve(threadID string) (*db.Thread, error) {
	lock := m.lifecycleLock(threadID)
	lock.Lock()
	defer lock.Unlock()
	if done := m.stop(threadID, Source{}); done != nil {
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
		m.cleanTmp(t)
	}
	return t, err
}

// ArchiveSession stores an independent archive flag. A stopped waiting or
// working session becomes idle: there is no longer a live request to answer.
func (m *Threads) ArchiveSession(threadID string) (*db.Thread, error) {
	lock := m.lifecycleLock(threadID)
	lock.Lock()
	defer lock.Unlock()
	if done := m.stop(threadID, Source{}); done != nil {
		select {
		case <-done:
		case <-time.After(agentboot.SessionCloseTimeout):
		}
	}
	m.engine.Evict(m.sessionOf(threadID))
	return m.update(threadID, func(t *db.Thread) {
		if t.ArchivedAt == nil {
			now := time.Now()
			t.ArchivedAt = &now
		}
		if t.Status == db.ThreadWaiting || t.Status == db.ThreadWorking {
			t.Status, t.WaitReason = db.ThreadIdle, ""
		}
	})
}

// RestoreSession returns an archived session to the list in its stored state.
func (m *Threads) RestoreSession(threadID string) (*db.Thread, error) {
	return m.update(threadID, func(t *db.Thread) { t.ArchivedAt = nil })
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

// ErrNotWorker is for an action that only makes sense on a worker or session,
// asked of a project's main thread (its conversation with the coordinator).
var ErrNotWorker = errors.New("not available on a project's main thread")

// Reopen brings a resolved thread back; by is who asked. A thread that is not
// resolved is returned as it is: there is nothing to reopen, and its status
// belongs to the turn that may be running.
func (m *Threads) Reopen(threadID string, by Source) (*db.Thread, error) {
	t, err := m.store.GetThread(threadID)
	if err != nil {
		return nil, err
	}
	if t.Agent == db.AgentLLM {
		return nil, ErrNotWorker
	}
	if t.Status != db.ThreadResolved {
		return t, nil
	}
	// update reads the row again under its lock, so check again there: a
	// Reopen or Resolve may have got in between.
	reopened := false
	t, err = m.update(threadID, func(t *db.Thread) {
		if t.Status != db.ThreadResolved {
			return
		}
		reopened = true
		t.ResolvedAt = nil
		t.Status = db.ThreadIdle
		if t.CommitsAhead > 0 && t.MergedAt == nil {
			t.Status = db.ThreadReady
		}
	})
	if err == nil && reopened {
		m.addEvent(t, &db.ThreadEvent{Kind: db.EvSystem, Content: "Reopened" + by.actor() + "."})
	}
	return t, err
}

// MaxTitleLength caps a thread's title, like the column that stores it.
const MaxTitleLength = 300

// SetTitle renames a thread. Like every metadata edit it does not bump the
// thread's UpdatedAt, so it keeps its place in "recently updated".
func (m *Threads) SetTitle(threadID, title string) (*db.Thread, error) {
	title = strings.TrimSpace(title)
	if title == "" {
		return nil, ErrEmptyTitle
	}
	if runes := []rune(title); len(runes) > MaxTitleLength {
		title = string(runes[:MaxTitleLength])
	}
	l := m.rowLock(threadID)
	l.Lock()
	defer l.Unlock()
	t, err := m.store.GetThread(threadID)
	if err != nil {
		return nil, err
	}
	t.Title = title
	if err := m.store.SetThreadTitle(threadID, title); err != nil {
		return nil, err
	}
	m.publish(t)
	return t, nil
}

// Merge merges a thread's branch into its base in the main checkout and
// resolves the thread.
func (m *Threads) Merge(ctx context.Context, threadID string) (*db.Thread, error) {
	lock := m.lifecycleLock(threadID)
	lock.Lock()
	defer lock.Unlock()
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
		m.cleanTmp(t)
	}
	return t, err
}

// Diff returns a thread's changes for review.
func (m *Threads) Diff(ctx context.Context, threadID string) (Diff, error) {
	t, err := m.store.GetThread(threadID)
	if err != nil {
		return Diff{}, err
	}
	if t.Branch == "" {
		return Diff{}, errors.New("this thread has no worktree branch to review")
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
			t.Status, t.WaitReason, t.Queued = db.ThreadWaiting, "Interrupted when tingly-project stopped. Send a message to continue.", false
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
var AutoAnswerAfter = 5 * time.Minute

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

// allowKey is what "allow for this thread" remembers about a request: the tool,
// except for Bash, which is the exact command. Allowing one Bash command must
// not allow every other the thread runs. False means there is nothing safe to
// remember (a Bash call without a command), so the answer allows this call only.
func allowKey(tool string, input map[string]any) (string, bool) {
	if tool != "Bash" {
		return tool, true
	}
	cmd, _ := input["command"].(string)
	if cmd = strings.TrimSpace(cmd); cmd == "" {
		return "", false
	}
	return tool + "\x00" + cmd, true
}

func (m *Threads) isAllowed(threadID, key string) bool {
	m.mu.Lock()
	defer m.mu.Unlock()
	return m.allowed[threadID][key]
}

func (m *Threads) allow(threadID, key string) {
	m.mu.Lock()
	defer m.mu.Unlock()
	if m.allowed[threadID] == nil {
		m.allowed[threadID] = map[string]bool{}
	}
	m.allowed[threadID][key] = true
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
	key, scopable := allowKey(req.ToolName, req.Input)
	if scopable && p.m.isAllowed(p.threadID, key) {
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
	if ans.Allow && ans.Scope == "thread" && scopable {
		p.m.allow(p.threadID, key)
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

// requests returns the transcript events of every pending request, oldest first.
func (p *threadPrompter) requests() []db.ThreadEvent {
	p.mu.Lock()
	out := make([]db.ThreadEvent, 0, len(p.pending))
	for _, pd := range p.pending {
		out = append(out, pd.event)
	}
	p.mu.Unlock()
	sort.SliceStable(out, func(i, j int) bool { return out[i].CreatedAt.Before(out[j].CreatedAt) })
	return out
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
