package engine

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"errors"
	"os"
	"sort"
	"strings"
	"sync/atomic"
	"time"

	"github.com/tingly-dev/tingly-box/agentboot"
)

// turnTimeout bounds one turn. Coding tasks run far longer than a chat
// reply, so this is looser than a plain chat default.
const turnTimeout = 2 * time.Hour

// Start claims session id for one turn, records prompt as the user's
// message, marks the session running and runs the turn in the background in
// dir. resume continues the Claude session id (false only on its first
// turn). launch builds the turn's options once the turn starts.
//
// Returns false, recording nothing, if the session already has a turn in
// flight: the claim is atomic, so of two requests for one session arriving
// together exactly one wins.
func (e *Engine) Start(id, dir, prompt string, resume bool, launch Launch) bool {
	turnCtx, cancel := context.WithTimeout(context.Background(), turnTimeout)
	prompter := newWebPrompter(id, e.store)
	done := make(chan struct{})

	e.mu.Lock()
	if _, busy := e.runs[id]; busy {
		e.mu.Unlock()
		cancel()
		return false
	}
	e.runs[id] = &run{cancel: cancel, prompter: prompter, done: done}
	e.mu.Unlock()

	// Only the winner of the claim records the message, and the session
	// reads as running before this returns: the caller re-reads it right
	// away, and a status still showing the previous turn's outcome would
	// tell the page there is nothing to poll for.
	e.store.Append(id, Entry{Role: "user", Content: prompt, Timestamp: time.Now()})
	e.store.SetRunning(id)

	go e.runTurn(turnCtx, id, dir, prompt, resume, launch, prompter, cancel, done)
	return true
}

func (e *Engine) runTurn(ctx context.Context, sessionID, projectPath, prompt string, resume bool, launch Launch, webPrompt *webPrompter, cancel context.CancelFunc, done chan struct{}) {
	var end TurnEnd
	defer func() {
		e.mu.Lock()
		delete(e.runs, sessionID)
		e.mu.Unlock()
		cancel()
		close(done)
		e.turnEnded(sessionID, end)
	}()

	var opts agentboot.ExecutionOptions
	if launch != nil {
		opts = launch(ctx)
	}
	opts.SessionID = sessionID
	opts.Resume = resume
	opts.PermissionPromptTool = "stdio"

	prompter := e.prompterFor(sessionID, webPrompt, opts.PermissionMode)

	var werr error
	if e.pool != nil {
		var handled bool
		if werr, handled = e.runPersistentTurn(ctx, sessionID, projectPath, prompt, opts, prompter); handled {
			if errors.Is(werr, agentboot.ErrTurnInFlight) {
				end.NotSent, werr = true, nil
			}
			end.Err, end.Stopped = e.finishTurn(ctx, sessionID, werr, nil)
			return
		}
	}

	settled := &settleStore{LifecycleStore: e.store}
	opts.Store = settled
	conv := newConverter()
	sink := func(raw any) { e.record(sessionID, conv, raw) }
	_, werr = e.agent.Run(ctx, agentboot.RunRequest{ProjectPath: projectPath, Prompt: prompt, Opts: opts}, prompter, sink)
	// A one-shot process exits with its turn, and its background tasks
	// with it.
	e.clearTasks(sessionID)
	end.Err, end.Stopped = e.finishTurn(ctx, sessionID, werr, settled)
}

// settleStore notes whether the runner recorded a one-shot turn's outcome.
type settleStore struct {
	agentboot.LifecycleStore
	done atomic.Bool
}

func (s *settleStore) SetCompleted(id, response string) bool {
	s.done.Store(true)
	return s.LifecycleStore.SetCompleted(id, response)
}

func (s *settleStore) SetFailed(id, errMsg string) bool {
	s.done.Store(true)
	return s.LifecycleStore.SetFailed(id, errMsg)
}

// runPersistentTurn drives one turn through a long-lived Claude Code process
// kept in e.pool, mirroring remoteagent.ClaudeCodeExecutor's own
// runPersistentTurn (.design/claude-code-session.md §5.3) — same pool package, same
// Open-or-Acquire-then-Send shape, same fallback contract.
//
// handled=false means nothing was sent to any process yet (Open or the
// pool's own bookkeeping failed before a turn started), so the caller must
// fall back to a fresh one-shot AgentService.Run. handled=true means this
// function drove the turn to completion or failure itself — the caller must
// not retry it one-shot even on error, since side effects may already have
// happened.
//
// Unlike the one-shot path, ExecutionOptions.Store isn't consulted here (the
// persistent path has no Runner-owned lifecycle hook to attach it to), so
// session status transitions are driven directly.
func (e *Engine) runPersistentTurn(ctx context.Context, sessionID, projectPath, prompt string, opts agentboot.ExecutionOptions, prompter agentboot.Prompter) (werr error, handled bool) {
	sig := launchSignature(opts)
	want := launchState{sig: sig, model: opts.Model, mode: opts.PermissionMode}
	persistentSession, found := e.pool.Acquire(sessionID)
	res, conducted := e.residentFor(sessionID)
	if found && (!conducted || res.c.Session() != persistentSession) {
		// A pooled process nothing reads from can't be driven safely.
		e.Evict(sessionID)
		found = false
	}
	if found {
		e.mu.Lock()
		have := e.launch[sessionID]
		e.mu.Unlock()
		if have.sig != sig || !e.applyLive(sessionID, res.c, have, want) {
			// Gateway or profile settings changed since this process
			// started, or a model / permission mode change could not be
			// applied to it live; restart it (Open resumes the same Claude
			// session via opts.Resume).
			e.Evict(sessionID)
			found = false
		}
	}

	e.store.SetRunning(sessionID)
	if !found {
		opened, operr := e.agent.Open(ctx, "", projectPath, prompt, opts)
		if operr != nil {
			return nil, false
		}
		if perr := e.pool.Open(ctx, sessionID, opened); perr != nil {
			closeCtx, cancel := context.WithTimeout(context.Background(), agentboot.SessionCloseTimeout)
			_ = opened.Close(closeCtx)
			cancel()
			return nil, false
		}
		e.mu.Lock()
		e.launch[sessionID] = want
		e.mu.Unlock()
		res = e.newResident(sessionID, opened)
		_, werr = res.c.Await(ctx, prompter)
	} else {
		_, werr = res.c.RunTurn(ctx, prompt, prompter)
		switch {
		case errors.Is(werr, agentboot.ErrTurnInFlight):
			// Claude started a turn of its own (a background task just
			// finished) a moment before this message; that turn owns the
			// session now.
			// Not a failure of this session: the note says what happened,
			// and the status stays with the turn that is running.
			e.appendSystem(sessionID, "not sent: Claude is following up on a finished background task; send it again when that's done")
			return werr, true
		case errors.Is(werr, agentboot.ErrSessionClosed):
			// The process ended before the message reached it: nothing was
			// sent, so a one-shot turn can still take it.
			e.pool.Remove(sessionID)
			return nil, false
		}
	}

	if werr != nil && res.c.Session().Status() == agentboot.SessionStateTerminated {
		// A crash mid-turn: drop the pool's stale bookkeeping now rather
		// than waiting for the next Acquire to self-heal it.
		e.pool.Remove(sessionID)
	} else {
		e.pool.Touch(sessionID)
	}
	if werr == nil {
		e.store.SetCompleted(sessionID, "")
	} else {
		e.store.SetFailed(sessionID, werr.Error())
	}
	return werr, true
}

// finishTurn applies the two corrections both the one-shot and persistent
// paths need. First, a turn ended by our own Stop is not a failure. The CLI
// process (or, for a persistent session, the whole session —
// RunTurnWithPrompter closes it on ctx cancellation) was stopped from
// outside its own logic, and the session stays resumable. Whichever path
// just ran already recorded the "context canceled" failure, so it is
// corrected here rather than left to read as a real error. Second, a
// one-shot turn that failed before its process started is recorded as
// failed: settled says whether the runner recorded an outcome (nil on the
// persistent path, which always records its own). It returns the turn's
// failure, or stopped for a turn the user stopped.
func (e *Engine) finishTurn(ctx context.Context, sessionID string, werr error, settled *settleStore) (err error, stopped bool) {
	if werr == nil {
		return nil, false
	}
	if ctx.Err() == context.Canceled {
		e.store.SetInterrupted(sessionID)
		e.appendSystem(sessionID, "interrupted; send a message to resume")
		return nil, true
	}
	// The runner records the outcome of any turn whose process started.
	// Unrecorded here means it failed before that (CLI not found, launch
	// spec, transport), which nothing has recorded or shown yet.
	if settled != nil && !settled.done.Load() {
		e.store.SetFailed(sessionID, werr.Error())
		e.appendError(sessionID, werr.Error())
	}
	return werr, false
}

// launchState is what a resident process runs with: sig is what only a
// restart changes (launchSignature), model and mode are what a live control
// request changes.
type launchState struct {
	sig, model, mode string
}

// liveControlTimeout bounds applyLive; a process that does not answer in
// this time is restarted instead.
const liveControlTimeout = 5 * time.Second

// liveController is the part of a resident process applyLive drives.
type liveController interface {
	SetModel(ctx context.Context, model string) error
	SetPermissionMode(ctx context.Context, mode string) error
}

// applyLive brings res's model and permission mode to want without a
// restart. It reports false if a change could not be applied, in which case
// the caller restarts the process, which picks the settings up at launch.
func (e *Engine) applyLive(sessionID string, res liveController, have, want launchState) bool {
	if have.model == want.model && have.mode == want.mode {
		return true
	}
	ctx, cancel := context.WithTimeout(context.Background(), liveControlTimeout)
	defer cancel()
	if have.model != want.model {
		if err := res.SetModel(ctx, want.model); err != nil {
			return false
		}
		have.model = want.model
		e.setLaunch(sessionID, have)
	}
	if have.mode != want.mode {
		// An empty mode means the launch default, which a live process
		// cannot be told to go back to by name.
		if want.mode == "" {
			return false
		}
		if err := res.SetPermissionMode(ctx, want.mode); err != nil {
			return false
		}
		have.mode = want.mode
		e.setLaunch(sessionID, have)
	}
	return true
}

func (e *Engine) setLaunch(sessionID string, st launchState) {
	e.mu.Lock()
	e.launch[sessionID] = st
	e.mu.Unlock()
}

// launchSignature captures the launch-time settings a persistent process
// cannot change once started. Model and permission mode are not in it: they
// are changed live (applyLive). Env is sorted because hosts often build it
// from a map. A settings file keeps its path when its content is edited
// (a profile, say), so its content is part of the signature, not just the
// path.
func launchSignature(opts agentboot.ExecutionOptions) string {
	env := append([]string(nil), opts.Env...)
	sort.Strings(env)
	settings := opts.SettingsPath
	if settings != "" {
		if b, err := os.ReadFile(settings); err == nil {
			sum := sha256.Sum256(b)
			settings += "@" + hex.EncodeToString(sum[:])
		}
	}
	return opts.Entrypoint + "\x00" + settings + "\x00" + strings.Join(env, "\x00")
}
