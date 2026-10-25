// Package engine drives Claude Code sessions for a host that shows them on a
// page: it runs each turn (through a resident process when a pool is given,
// one-shot otherwise), turns what the CLI emits into transcript entries,
// holds approvals and questions until the user answers them, and tracks the
// background tasks a process runs between turns.
//
// It owns no storage. A host passes a Store, which receives every
// transcript entry and status change, and keeps its own session records and
// settings. Projects' threads and working-folder sessions are its host. The
// behaviour and the reasons for it are in tingly-box's .design/desk.md §3
// (written for its first host).
package engine

import (
	"context"
	"encoding/json"
	"errors"
	"sync"
	"sync/atomic"
	"time"

	"github.com/tingly-dev/tingly-box/agentboot"
	"github.com/tingly-dev/tingly-box/agentboot/pool"
)

// Entry is one transcript entry. Kind "" is a plain chat message (Role
// "user" or "assistant"); the other kinds are listed below.
type Entry struct {
	Role    string
	Content string
	// Kind is one of the Kind* constants, or "" for a chat message.
	Kind string
	// RequestID correlates a request with its response, and a tool call
	// with its result and its task events.
	RequestID string
	// Payload is kind-specific structured data.
	Payload json.RawMessage
	// Parent is the Agent tool call whose subagent produced this entry, ""
	// for the main conversation.
	Parent    string
	Timestamp time.Time
}

// Transcript entry kinds.
const (
	KindThinking         = "thinking"
	KindToolUse          = "tool_use"
	KindToolResult       = "tool_result"
	KindApprovalRequest  = "approval_request"
	KindApprovalResponse = "approval_response"
	KindAskRequest       = "ask_request"
	KindAskResponse      = "ask_response"
	KindTask             = "task"  // payload: TaskEvent
	KindUsage            = "usage" // payload: TurnUsage
	KindSystem           = "system"
	KindError            = "error"
)

// Store is where a host keeps its sessions. The engine writes to it and
// never reads from it.
type Store interface {
	// SetRunning, SetCompleted and SetFailed record a turn's status.
	agentboot.LifecycleStore
	// Append adds an entry to the session's transcript.
	Append(id string, e Entry)
	// SetInterrupted records a turn the user stopped: the session is idle
	// and resumable, with no error.
	SetInterrupted(id string)
}

// Launch builds a turn's launch options when the turn starts: permission
// mode, model, env and settings. The engine fills in the session id,
// resume and the permission prompt. It runs on the turn's goroutine, so it
// may resolve routing and append notes about it.
type Launch func(ctx context.Context) agentboot.ExecutionOptions

// Config wires an Engine to its dependencies.
type Config struct {
	Agent *agentboot.AgentService
	Store Store
	// Pool, if set, drives turns through a long-lived Claude Code process
	// per session instead of spawning one per message. A setup failure
	// always falls back to a one-shot turn; nil keeps every turn one-shot.
	Pool *pool.Pool
	// PermissionMode is the session's current permission mode. A turn
	// Claude Code starts by itself consults it to decide whether its
	// approvals are answered by the host. Optional.
	PermissionMode func(id string) string
	// Prompter, if set, answers a session's approvals and questions in
	// place of the engine's own (which records them in the transcript and
	// waits for Respond). Respond and AwaitingInput then have nothing to
	// act on: the host answers through its prompter. bypassPermissions
	// still approves tool use without asking it. Optional.
	Prompter func(id string) agentboot.Prompter
	// OnTurnEnd is called once after every turn, solicited or not, once
	// its status is recorded and the session is free for the next Start.
	// Optional.
	OnTurnEnd func(id string, end TurnEnd)
}

// TurnEnd is how a turn ended.
type TurnEnd struct {
	// Err is the turn's failure; nil when it completed or was stopped.
	Err error
	// Stopped: the user stopped it (Stop, Shutdown).
	Stopped bool
	// NotSent: the message never reached Claude, because Claude had just
	// started a turn of its own (a background task finished). The host
	// may send it again once that turn ends.
	NotSent bool
	// Unsolicited: Claude Code started this turn by itself.
	Unsolicited bool
}

// Engine runs the turns of any number of sessions, keyed by session id.
type Engine struct {
	agent          *agentboot.AgentService
	store          Store
	pool           *pool.Pool
	permissionMode func(id string) string
	hostPrompter   func(id string) agentboot.Prompter
	onTurnEnd      func(id string, end TurnEnd)

	mu   sync.Mutex
	runs map[string]*run // session id -> live turn, while one is in flight
	// launch records what each resident process was started with (see
	// launchSignature), so a changed setting restarts it instead of being
	// silently ignored.
	launch map[string]launchState
	// residents holds the Conductor of each pooled process (resident.go).
	residents map[string]*resident
	// live is each session's running background tasks (tasks.go).
	live          map[string][]TaskBrief
	stopKeepAlive chan struct{}
	shutdownOnce  sync.Once
}

// run is what Stop and Respond need for a session with a turn in flight.
// It is removed once the turn ends.
type run struct {
	cancel   context.CancelFunc
	prompter *webPrompter
	done     chan struct{} // closed once the turn goroutine has fully exited
	// unsolicited marks a turn Claude Code started by itself (resident.go):
	// no goroutine of ours waits on it, so stopping it means interrupting
	// the process directly.
	unsolicited bool
	stopped     atomic.Bool // the user stopped it, so its error result is expected
}

var (
	// ErrNoTurn: the session has no turn in flight.
	ErrNoTurn = errors.New("no turn in progress")
	// ErrBusy: the session has a turn in flight.
	ErrBusy = errors.New("a turn is in progress")
	// ErrUnknownRequest: no pending approval or question has that id.
	ErrUnknownRequest = errors.New("no such pending request")
	// ErrTaskNotRunning: the session has no live background task by that id.
	ErrTaskNotRunning = errors.New("task is not running")
)

// New builds an Engine. Call Shutdown when done with it.
func New(cfg Config) *Engine {
	e := &Engine{
		agent:          cfg.Agent,
		store:          cfg.Store,
		pool:           cfg.Pool,
		permissionMode: cfg.PermissionMode,
		hostPrompter:   cfg.Prompter,
		onTurnEnd:      cfg.OnTurnEnd,
		runs:           map[string]*run{},
		launch:         map[string]launchState{},
		residents:      map[string]*resident{},
		live:           map[string][]TaskBrief{},
		stopKeepAlive:  make(chan struct{}),
	}
	if e.pool != nil {
		go e.keepAlive(e.stopKeepAlive)
	}
	return e
}

// Busy reports whether id has a turn in flight.
func (e *Engine) Busy(id string) bool {
	e.mu.Lock()
	defer e.mu.Unlock()
	_, ok := e.runs[id]
	return ok
}

// AwaitingInput reports whether id's live turn is blocked on the user: an
// approval or a question nobody has answered yet.
func (e *Engine) AwaitingInput(id string) bool {
	e.mu.Lock()
	r, ok := e.runs[id]
	e.mu.Unlock()
	return ok && r.prompter.hasPending()
}

// Respond answers a pending approval or question of id's live turn.
func (e *Engine) Respond(id, requestID string, approved bool, answer string) error {
	e.mu.Lock()
	r, ok := e.runs[id]
	e.mu.Unlock()
	if !ok {
		return ErrNoTurn
	}
	if !r.prompter.resolve(requestID, approved, answer) {
		return ErrUnknownRequest
	}
	return nil
}

// Stop cancels id's in-flight turn, if any, and returns a channel closed
// once it has exited. The session stays resumable. Returns false if there
// was no turn to stop.
func (e *Engine) Stop(id string) (<-chan struct{}, bool) {
	e.mu.Lock()
	r, ok := e.runs[id]
	e.mu.Unlock()
	if !ok {
		return nil, false
	}
	r.stopped.Store(true)
	r.cancel()
	if r.unsolicited {
		if res, ok := e.residentFor(id); ok {
			ctx, cancel := context.WithTimeout(context.Background(), agentboot.SessionCloseTimeout)
			defer cancel()
			_ = res.c.Interrupt(ctx)
		}
	}
	return r.done, true
}

// ErrNotInjectable: Inject has nothing to join (no turn in flight on a live
// process), so the message is a new turn for Start.
var ErrNotInjectable = errors.New("no running turn to add the message to")

// Inject adds prompt to the turn id has in flight, without ending it:
// Claude takes it up at its next step. The message is recorded as the
// user's. Returns ErrNotInjectable when there is no such turn, for the caller
// to Start one instead.
func (e *Engine) Inject(id, prompt string) error {
	e.mu.Lock()
	_, running := e.runs[id]
	e.mu.Unlock()
	res, ok := e.residentFor(id)
	if !running || !ok {
		return ErrNotInjectable
	}
	ctx, cancel := context.WithTimeout(context.Background(), liveControlTimeout)
	defer cancel()
	if err := res.c.Inject(ctx, prompt); err != nil {
		if errors.Is(err, agentboot.ErrNoTurnInFlight) || errors.Is(err, agentboot.ErrSessionClosed) {
			return ErrNotInjectable
		}
		return err
	}
	e.store.Append(id, Entry{Role: "user", Content: prompt, Timestamp: time.Now()})
	return nil
}

// Release closes id's resident process so something else (a terminal) can
// take the Claude session over: two processes writing one session file
// corrupt it. A turn in flight is refused with ErrBusy rather than killed,
// and the session is claimed while the process closes, so no turn can
// start in between and have its process closed under it.
func (e *Engine) Release(id string) error {
	done := make(chan struct{})
	e.mu.Lock()
	if _, busy := e.runs[id]; busy {
		e.mu.Unlock()
		return ErrBusy
	}
	e.runs[id] = &run{cancel: func() {}, prompter: newWebPrompter(id, e.store), done: done}
	e.mu.Unlock()
	defer func() {
		e.mu.Lock()
		delete(e.runs, id)
		e.mu.Unlock()
		close(done)
	}()
	e.Evict(id)
	return nil
}

// Evict closes and forgets id's resident process, if any. A no-op without
// a pool or when the session only ever ran one-shot turns.
func (e *Engine) Evict(id string) {
	if e.pool == nil {
		return
	}
	ctx, cancel := context.WithTimeout(context.Background(), agentboot.SessionCloseTimeout)
	defer cancel()
	_ = e.pool.CloseAndRemove(ctx, id)
	e.mu.Lock()
	delete(e.launch, id)
	e.mu.Unlock()
}

// Shutdown stops every in-flight turn and closes every resident process,
// so none outlives the host holding its Claude session file open.
func (e *Engine) Shutdown(ctx context.Context) {
	e.shutdownOnce.Do(func() { close(e.stopKeepAlive) })
	e.mu.Lock()
	for _, r := range e.runs {
		r.cancel()
	}
	e.mu.Unlock()
	if e.pool != nil {
		e.pool.Shutdown(ctx)
	}
}

func (e *Engine) appendSystem(id, text string) {
	e.store.Append(id, Entry{Kind: KindSystem, Content: text, Timestamp: time.Now()})
}

func (e *Engine) appendError(id, text string) {
	e.store.Append(id, Entry{Kind: KindError, Content: text, Timestamp: time.Now()})
}

// prompterFor is the prompter a turn of id answers with, before the
// bypassPermissions wrapper: the host's, or the engine's own wp.
func (e *Engine) prompterFor(id string, wp *webPrompter, mode string) agentboot.Prompter {
	var p agentboot.Prompter = wp
	if e.hostPrompter != nil {
		p = e.hostPrompter(id)
	}
	if autoApproves(mode) {
		p = autoApprovePrompter{inner: p}
	}
	return p
}

func (e *Engine) turnEnded(id string, end TurnEnd) {
	if e.onTurnEnd != nil {
		e.onTurnEnd(id, end)
	}
}

// autoApproves reports whether the host should answer permission requests
// itself. Only bypassPermissions promises unconditional approval; every
// other mode keeps Claude Code's own deny/plan/classifier semantics (the
// same policy as @cc's own noApprovalModes).
func autoApproves(mode string) bool { return mode == "bypassPermissions" }
