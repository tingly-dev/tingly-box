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
	"fmt"
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
	KindTask             = "task"    // payload: TaskEvent
	KindUsage            = "usage"   // payload: TurnUsage
	KindCompact          = "compact" // payload: agentboot.CompactEvent
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
	// NotStarted means cancellation prevented even the first process launch.
	NotStarted bool
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
	// settling tracks callbacks after the engine has released the turn claim.
	settling   map[string]int
	settled    map[string]chan struct{}
	restarting map[string]bool
	// launch records what each resident process was started with (see
	// launchSignature), so a changed setting restarts it instead of being
	// silently ignored.
	launch map[string]launchState
	// lastTurn remembers each session's latest turn for Retry (turn.go).
	lastTurn map[string]lastTurn
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
	// compact collects the compaction events of a turn started by Compact.
	compact *compactCapture
	// end is how the turn ended; set before done closes.
	end TurnEnd
}

var (
	// ErrNoTurn: the session has no turn in flight.
	ErrNoTurn = errors.New("no turn in progress")
	// ErrBusy: the session has a turn in flight.
	ErrBusy = errors.New("a turn is in progress")
	// ErrNotSent: Compact's request never reached the agent, because it had
	// just started a turn of its own. The host's OnTurnEnd already saw this
	// (TurnEnd.NotSent); a host that re-queues on that must not do so again.
	ErrNotSent = errors.New("the agent started a turn of its own first")
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
		settling:       map[string]int{},
		settled:        map[string]chan struct{}{},
		restarting:     map[string]bool{},
		launch:         map[string]launchState{},
		lastTurn:       map[string]lastTurn{},
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
	return ok || e.restarting[id]
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

// ErrAgentReadsBetweenTurns is ErrNotInjectable for a turn that does run, on
// an agent that cannot take input while it works: the host can tell its user
// why the message waits for the next turn.
var ErrAgentReadsBetweenTurns = fmt.Errorf("%w: the agent reads messages only between turns", ErrNotInjectable)

// Inject adds prompt to the turn id has in flight, without ending it:
// Claude takes it up at its next step. The message is recorded as the
// user's. Returns ErrNotInjectable when there is no such turn, for the caller
// to Start one instead.
func (e *Engine) Inject(id, prompt string) error {
	e.mu.Lock()
	_, running := e.runs[id]
	restarting := e.restarting[id]
	e.mu.Unlock()
	res, ok := e.residentFor(id)
	if !running || restarting || !ok {
		return ErrNotInjectable
	}
	ctx, cancel := context.WithTimeout(context.Background(), liveControlTimeout)
	defer cancel()
	if err := res.c.Inject(ctx, prompt); err != nil {
		if errors.Is(err, agentboot.ErrInjectUnsupported) {
			return ErrAgentReadsBetweenTurns
		}
		if errors.Is(err, agentboot.ErrNoTurnInFlight) || errors.Is(err, agentboot.ErrSessionClosed) {
			return ErrNotInjectable
		}
		return err
	}
	e.store.Append(id, Entry{Role: "user", Content: prompt, Timestamp: time.Now()})
	return nil
}

// SetPermissionMode applies mode to id's live process now, with a
// set_permission_mode control request and no restart, so a change reaches the
// turn in flight instead of waiting for the next one (a turn can run for
// hours). It reports applied=true when the process has the mode; applied=false
// with a nil error means there was nothing to apply it to (no resident
// process, or the session is being restarted) and the next turn applies it as
// before. A refusal is returned: the mode stays what it was, and the next
// turn's applyLive tries again (with the auto fallback, if any).
//
// The mode is not remembered unless the process took it, so a failed push
// can never leave the engine believing the process runs a mode it refused.
func (e *Engine) SetPermissionMode(id, mode string) (bool, error) {
	if mode == "" {
		// The launch default has no name to send live (see applyLive).
		return false, nil
	}
	res, ok := e.residentFor(id)
	if !ok {
		return false, nil
	}
	e.mu.Lock()
	restarting := e.restarting[id]
	e.mu.Unlock()
	if restarting {
		return false, nil
	}
	return e.setModeLive(id, res.c, mode)
}

// setModeLive is SetPermissionMode for the process ctl controls.
func (e *Engine) setModeLive(id string, ctl liveController, mode string) (bool, error) {
	e.mu.Lock()
	have, known := e.launch[id]
	e.mu.Unlock()
	if !known {
		// A process the engine did not launch (nothing to remember it by).
		return false, nil
	}
	if have.mode == mode {
		return true, nil
	}
	ctx, cancel := context.WithTimeout(context.Background(), liveControlTimeout)
	defer cancel()
	if err := ctl.SetPermissionMode(ctx, mode); err != nil {
		// A mode this model cannot have is no reason to fail the change: the
		// process moves to the fallback mode and the transcript says why, the
		// same way applyLive does it.
		if modeUnavailable(err) && e.fallBack(id, ctl, have, launchState{mode: mode, model: have.model}) {
			return true, nil
		}
		return false, err
	}
	have.mode, have.refused = mode, ""
	e.setLaunch(id, have)
	return true, nil
}

// Injectable reports whether a message sent to id now would join the turn in
// flight: a turn is running on a live process whose agent reads input
// mid-turn. False means Inject would refuse it with ErrNotInjectable, and the
// message waits for the next turn.
func (e *Engine) Injectable(id string) bool {
	e.mu.Lock()
	_, running := e.runs[id]
	restarting := e.restarting[id]
	e.mu.Unlock()
	res, ok := e.residentFor(id)
	return running && !restarting && ok && res.c.CanInject()
}

// Release closes id's resident process so something else (a terminal) can
// take the Claude session over: two processes writing one session file
// corrupt it. A running turn, background task or unfinished turn callback is
// refused with ErrBusy rather than killed. The session is claimed while the
// process closes and its event reader drains, preventing a concurrent Start.
func (e *Engine) Release(id string) error {
	done := make(chan struct{})
	e.mu.Lock()
	if _, busy := e.runs[id]; busy || e.restarting[id] || len(e.live[id]) > 0 || e.settling[id] > 0 {
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
	if e.pool == nil {
		return nil
	}
	e.mu.Lock()
	resident := e.residents[id]
	e.mu.Unlock()
	ctx, cancel := context.WithTimeout(context.Background(), agentboot.SessionCloseTimeout)
	defer cancel()
	if err := e.pool.CloseAndRemove(ctx, id); err != nil {
		return err
	}
	if resident != nil {
		select {
		case <-resident.c.Done():
		case <-ctx.Done():
			return ctx.Err()
		}
	}
	e.mu.Lock()
	delete(e.launch, id)
	e.mu.Unlock()
	return nil
}

// Restart stops the current turn and closes its process. The next Start
// launches a fresh process and can resume the same conversation.
func (e *Engine) Restart(id string) error {
	e.mu.Lock()
	if e.restarting[id] {
		e.mu.Unlock()
		return ErrBusy
	}
	e.restarting[id] = true
	r, res := e.runs[id], e.residents[id]
	if r != nil {
		r.stopped.Store(true)
		r.cancel()
	}
	e.mu.Unlock()
	defer func() {
		e.mu.Lock()
		delete(e.restarting, id)
		e.mu.Unlock()
	}()

	ctx, cancel := context.WithTimeout(context.Background(), agentboot.SessionCloseTimeout)
	defer cancel()
	wait := func(done <-chan struct{}) error {
		select {
		case <-done:
			return nil
		case <-ctx.Done():
			return ctx.Err()
		}
	}
	if res != nil {
		res.cancel()
		// Close can terminate the process even when the agent is blocked
		// reading or writing a control message and cannot take an interrupt.
		if err := res.c.Session().Close(ctx); err != nil {
			return err
		}
		if err := wait(res.c.Done()); err != nil {
			return err
		}
	}
	if r != nil {
		if err := wait(r.done); err != nil {
			return err
		}
	}
	e.mu.Lock()
	ending := e.settled[id]
	e.mu.Unlock()
	if ending != nil {
		if err := wait(ending); err != nil {
			return err
		}
	}
	if e.pool != nil {
		e.pool.Remove(id)
	}
	e.mu.Lock()
	delete(e.launch, id)
	e.mu.Unlock()
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

// prompterFor is the prompter a turn of id answers with: the host's, or the
// engine's own wp, wrapped so that a bypassPermissions session approves tool
// use without asking. The wrapper reads the session's mode at each request
// (SetPermissionMode is live), falling back to mode — the turn's launch mode —
// when the host does not report one.
func (e *Engine) prompterFor(id string, wp *webPrompter, mode string) agentboot.Prompter {
	var p agentboot.Prompter = wp
	if e.hostPrompter != nil {
		p = e.hostPrompter(id)
	}
	live := func() string {
		if e.permissionMode != nil {
			return e.permissionMode(id)
		}
		return mode
	}
	return autoApprovePrompter{inner: p, mode: live}
}

func (e *Engine) turnEnded(id string, end TurnEnd) {
	if e.onTurnEnd != nil {
		e.onTurnEnd(id, end)
	}
}

// startSettling records a host callback before releasing the turn claim.
func (e *Engine) startSettling(id string) {
	if e.settling[id] == 0 {
		e.settled[id] = make(chan struct{})
	}
	e.settling[id]++
}

func (e *Engine) finishSettling(id string) {
	e.mu.Lock()
	defer e.mu.Unlock()
	e.settling[id]--
	if e.settling[id] == 0 {
		close(e.settled[id])
		delete(e.settled, id)
		delete(e.settling, id)
	}
}

// autoApproves reports whether the host should answer permission requests
// itself. Only bypassPermissions promises unconditional approval; every
// other mode keeps Claude Code's own deny/plan/classifier semantics (the
// same policy as @cc's own noApprovalModes).
func autoApproves(mode string) bool { return mode == "bypassPermissions" }
