package engine

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"os"
	"sort"
	"strings"
	"sync/atomic"
	"time"

	"github.com/sirupsen/logrus"
	"github.com/tingly-dev/tingly-box/agentboot"
	"github.com/tingly-dev/tingly-box/agentboot/claude"
)

// turnTimeout bounds one turn. Coding tasks run far longer than a chat
// reply, so this is looser than a plain chat default.
const turnTimeout = 2 * time.Hour

// maxPreOutputRetries bounds the automatic relaunch of a first turn that
// failed strictly pre-output; preOutputPause is the fixed wait between
// attempts. Pre-output failures are launch flakes and startup crashes, not
// sustained outages, so a short fixed pause beats a backoff curve.
const (
	maxPreOutputRetries = 2
	preOutputPause      = 500 * time.Millisecond
)

// retryPause waits preOutputPause unless ctx ends first. It reports false
// when the context was canceled — the caller should give up rather than
// start another attempt.
func retryPause(ctx context.Context) bool {
	timer := time.NewTimer(preOutputPause)
	defer timer.Stop()
	select {
	case <-ctx.Done():
		return false
	case <-timer.C:
		return true
	}
}

// Start claims session id for one turn, records prompt as the user's
// message, marks the session running and runs the turn in the background in
// dir. resume continues the Claude session id (false only on its first
// turn). launch builds the turn's options once the turn starts.
//
// Returns false, recording nothing, if the session already has a turn in
// flight: the claim is atomic, so of two requests for one session arriving
// together exactly one wins.
func (e *Engine) Start(id, dir, prompt string, resume bool, launch Launch) bool {
	_, ok := e.begin(id, dir, prompt, resume, launch, nil)
	return ok
}

// begin claims the session and starts its turn. A non-nil capture makes it a
// compaction (Compact): the prompt is the backend's compaction request, which
// is not recorded as the user's message and is not offered to Retry.
func (e *Engine) begin(id, dir, prompt string, resume bool, launch Launch, capture *compactCapture) (*run, bool) {
	turnCtx, cancel := context.WithTimeout(context.Background(), turnTimeout)
	prompter := newWebPrompter(id, e.store)
	done := make(chan struct{})

	e.mu.Lock()
	if _, busy := e.runs[id]; busy {
		e.mu.Unlock()
		cancel()
		return nil, false
	}
	r := &run{cancel: cancel, prompter: prompter, done: done, compact: capture}
	e.runs[id] = r
	e.mu.Unlock()

	// Only the winner of the claim records the message, and the session
	// reads as running before this returns: the caller re-reads it right
	// away, and a status still showing the previous turn's outcome would
	// tell the page there is nothing to poll for.
	if capture == nil {
		e.store.Append(id, Entry{Role: "user", Content: prompt, Timestamp: time.Now()})
	}
	e.store.SetRunning(id)
	if capture == nil {
		e.mu.Lock()
		e.lastTurn[id] = lastTurn{prompt: prompt}
		e.mu.Unlock()
	}

	go e.runTurn(turnCtx, id, dir, prompt, resume, launch, prompter, cancel, done)
	return r, true
}

func (e *Engine) runTurn(ctx context.Context, sessionID, projectPath, prompt string, resume bool, launch Launch, webPrompt *webPrompter, cancel context.CancelFunc, done chan struct{}) {
	var end TurnEnd
	defer func() {
		e.mu.Lock()
		if r := e.runs[sessionID]; r != nil && r.done == done {
			r.end = end
		}
		delete(e.runs, sessionID)
		e.settling[sessionID]++
		e.mu.Unlock()
		cancel()
		close(done)
		e.turnEnded(sessionID, end)
		e.mu.Lock()
		e.settling[sessionID]--
		if e.settling[sessionID] == 0 {
			delete(e.settling, sessionID)
		}
		e.mu.Unlock()
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
			e.settleTurn(sessionID, prompt, end.Err)
			return
		}
	}

	settled := &settleStore{LifecycleStore: e.store}
	opts.Store = settled
	conv := newConverter()
	sink := func(raw any) { e.record(sessionID, conv, raw) }
	res, werr := e.agent.Run(ctx, agentboot.RunRequest{ProjectPath: projectPath, Prompt: prompt, Opts: opts}, prompter, sink)
	// A one-shot process exits with its turn, and its background tasks
	// with it.
	e.clearTasks(sessionID)
	end.Err, end.Stopped = e.finishTurn(ctx, sessionID, werr, settled)
	e.settleTurn(sessionID, prompt, end.Err)
	if end.Err != nil {
		// The host may retry this turn: whether output was seen decides
		// between resuming the interrupted turn and re-sending the prompt.
		e.markTurnOutput(sessionID, res != nil && len(res.Events) > 0)
	}
}

// lastTurn remembers a session's most recent turn for Retry: the prompt it
// ran ("" for a recovery launch), whether it ended in failure, and whether
// the agent produced any output before failing.
type lastTurn struct {
	prompt     string
	failed     bool
	outputSeen bool
}

// settleTurn records the outcome of a turn: a failure arms Retry, a clean
// turn clears the record — there is nothing to retry.
func (e *Engine) settleTurn(id, prompt string, err error) {
	e.mu.Lock()
	defer e.mu.Unlock()
	if err == nil {
		delete(e.lastTurn, id)
		return
	}
	if lt, ok := e.lastTurn[id]; ok {
		lt.prompt, lt.failed = prompt, true
		e.lastTurn[id] = lt
	}
}

// markTurnOutput notes whether the failed turn produced any output, the
// gate that decides how Retry relaunches.
func (e *Engine) markTurnOutput(id string, seen bool) {
	e.mu.Lock()
	if lt, ok := e.lastTurn[id]; ok {
		lt.outputSeen = seen
		e.lastTurn[id] = lt
	}
	e.mu.Unlock()
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
	want := launchState{sig: sig, model: opts.Model, mode: opts.PermissionMode, live: liveSettings(opts)}
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
		// A first turn that failed strictly pre-output (launch flake,
		// startup crash, network error before the agent produced anything)
		// is safe to relaunch: no output was streamed, no tool ran, and
		// with no output there is no Claude session id to resume — each
		// attempt is a fresh launch of the same prompt. Bounded retries
		// with a short fixed pause. A retry that also fails returns the
		// final error with handled=true (the turn ran on a persistent
		// process) — never falling through to a one-shot re-run; see
		// .design/claude-code-session.md §5.4.
		for attempt := 0; ; attempt++ {
			res, werr = e.openPersistentTurn(ctx, sessionID, projectPath, prompt, opts, want, prompter)
			if werr == nil || attempt >= maxPreOutputRetries ||
				!agentboot.PreOutputFailure(werr) || (res != nil && res.c.TurnOutputSeen()) {
				break
			}
			logrus.WithError(werr).Warnf("engine: pre-output failure on session %s's first turn (attempt %d), relaunching", sessionID, attempt+1)
			e.Evict(sessionID)
			e.store.SetRunning(sessionID)
			if !retryPause(ctx) {
				break
			}
		}
		if res == nil {
			// No process ever started (or the pool refused it): the
			// existing fallback contract applies — nothing was sent, so a
			// one-shot turn can still take the message.
			return nil, false
		}
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

		// The crash happened with staged work on disk (any output seen
		// means the session reached the transcript): recover it naturally
		// instead of failing the turn. --resume replays the interrupted
		// turn from the transcript — the user's message is never re-sent
		// — and interrupted tool calls are not re-run (CLI-guaranteed).
		// One attempt only: a recovery that also fails surfaces the error.
		if agentboot.PreOutputFailure(werr) && res.c.TurnOutputSeen() {
			logrus.WithError(werr).Warnf("engine: session %s crashed mid-turn, resuming the interrupted turn", sessionID)
			recRes, recErr := e.recoverInterruptedTurn(ctx, sessionID, projectPath, opts, want, prompter)
			if recErr == nil {
				werr = nil
				res = recRes
			} else {
				werr = recErr
			}
		}
	} else if werr == nil || res.c.Session().Status() != agentboot.SessionStateTerminated {
		// Not a mid-turn crash (clean turn, or a ResultError on a live
		// session): the session stays pooled — count the activity.
		e.pool.Touch(sessionID)
	}
	if werr == nil {
		e.store.SetCompleted(sessionID, "")
	} else {
		e.store.SetFailed(sessionID, werr.Error())
		// The host may retry this turn: whether output was seen decides
		// between resuming the interrupted turn and re-sending the prompt.
		e.markTurnOutput(sessionID, res.c.TurnOutputSeen())
	}
	return werr, true
}

// recoverInterruptedTurn relaunches a persistent process that crashed
// mid-turn, recovering the interrupted turn instead of failing it. The
// relaunch resumes the same Claude session (the engine pins the Claude
// session id to the engine's own at first launch, so --resume <sessionID>
// finds it) with CLAUDE_CODE_RESUME_INTERRUPTED_TURN set: the CLI replays
// the interrupted turn's prompt from its transcript — the host's prompt is
// empty, injecting no message — and continues it without re-running
// interrupted tool calls.
func (e *Engine) recoverInterruptedTurn(ctx context.Context, sessionID, projectPath string, opts agentboot.ExecutionOptions, want launchState, prompter agentboot.Prompter) (*resident, error) {
	e.Evict(sessionID)
	e.store.SetRunning(sessionID)

	recOpts := opts
	recOpts.Resume = true
	recOpts.Env = append(append([]string(nil), opts.Env...), claude.EnvResumeInterruptedTurn+"=1")
	recWant := launchState{sig: launchSignature(recOpts), model: opts.Model, mode: opts.PermissionMode, live: liveSettings(recOpts)}
	return e.openPersistentTurn(ctx, sessionID, projectPath, "", recOpts, recWant, prompter)
}

// Retry re-runs a failed turn without adding anything to the transcript.
// How it relaunches depends on the failed turn (settleTurn/markTurnOutput):
// output seen means staged work reached the CLI's transcript, so the
// relaunch resumes the same Claude session with
// CLAUDE_CODE_RESUME_INTERRUPTED_TURN and an empty prompt — the CLI replays
// the interrupted turn itself, exactly like the automatic mid-turn recovery.
// No output means the prompt never reached Claude, so the recorded prompt is
// sent again as a fresh turn. It returns false when the session is busy or
// has no failed turn to retry; like Start, the claim is atomic.
func (e *Engine) Retry(id, dir string, launch Launch) bool {
	e.mu.Lock()
	if _, busy := e.runs[id]; busy {
		e.mu.Unlock()
		return false
	}
	failed, ok := e.lastTurn[id]
	if !ok || !failed.failed {
		e.mu.Unlock()
		return false
	}
	delete(e.lastTurn, id)
	turnCtx, cancel := context.WithTimeout(context.Background(), turnTimeout)
	prompter := newWebPrompter(id, e.store)
	done := make(chan struct{})
	e.runs[id] = &run{cancel: cancel, prompter: prompter, done: done}
	e.mu.Unlock()

	prompt, resume := "", true
	if !failed.outputSeen {
		prompt, resume = failed.prompt, false
	} else if launch != nil {
		base := launch
		launch = func(ctx context.Context) agentboot.ExecutionOptions {
			opts := base(ctx)
			opts.Env = append(opts.Env, claude.EnvResumeInterruptedTurn+"=1")
			return opts
		}
	}
	e.store.SetRunning(id)

	go e.runTurn(turnCtx, id, dir, prompt, resume, launch, prompter, cancel, done)
	return true
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

// openPersistentTurn starts a fresh persistent process for sessionID's first
// turn (prompt submitted via Open's InitialInput), registers it with the pool
// and the launch bookkeeping, and awaits the turn. A nil resident means
// nothing was sent — the caller falls back to a one-shot turn.
func (e *Engine) openPersistentTurn(ctx context.Context, sessionID, projectPath, prompt string, opts agentboot.ExecutionOptions, want launchState, prompter agentboot.Prompter) (*resident, error) {
	opened, operr := e.agent.Open(ctx, "", projectPath, prompt, opts)
	if operr != nil {
		return nil, operr
	}
	if perr := e.pool.Open(ctx, sessionID, opened); perr != nil {
		closeCtx, cancel := context.WithTimeout(context.Background(), agentboot.SessionCloseTimeout)
		_ = opened.Close(closeCtx)
		cancel()
		return nil, perr
	}
	e.mu.Lock()
	e.launch[sessionID] = want
	e.mu.Unlock()
	res := e.newResident(sessionID, opened)
	_, werr := res.c.Await(ctx, prompter)
	return res, werr
}

// launchState is what a resident process runs with: sig is what only a
// restart changes (launchSignature), model, mode and live are what a live
// control request changes.
type launchState struct {
	sig, model, mode string
	// live is the permission rules of the settings file (liveSettings) the
	// process has been given: read at its start, then pushed by applyLive.
	live string
	// refused is the mode and model ("mode\x00model") Claude Code said it
	// cannot run, whose fallback mode the process is in meanwhile. It is
	// asked for again only when the model changes.
	refused string
}

// liveControlTimeout bounds applyLive; a process that does not answer in
// this time is restarted instead.
const liveControlTimeout = 5 * time.Second

// liveController is the part of a resident process applyLive drives.
type liveController interface {
	SetModel(ctx context.Context, model string) error
	SetPermissionMode(ctx context.Context, mode string) error
	ApplySettings(ctx context.Context, settings map[string]any) error
}

// applyLive brings res's model and permission mode to want without a
// restart. It reports false if a model or mode change could not be applied,
// in which case the caller restarts the process, which picks them up at
// launch. Permission rules never restart it: a change the process refuses
// waits for its next start, and is tried again with the next turn.
func (e *Engine) applyLive(sessionID string, res liveController, have, want launchState) bool {
	if have.model == want.model && have.mode == want.mode && have.live == want.live {
		return true
	}
	ctx, cancel := context.WithTimeout(context.Background(), liveControlTimeout)
	defer cancel()
	if have.live != want.live {
		if err := res.ApplySettings(ctx, liveDelta(have.live, want.live)); err != nil {
			logrus.WithError(err).WithField("session_id", sessionID).Warn("engine: permission rules not applied to the running process; they take effect when it next starts")
		} else {
			have.live = want.live
			e.setLaunch(sessionID, have)
		}
	}
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
		if have.refused == refusal(want) {
			return true
		}
		if err := res.SetPermissionMode(ctx, want.mode); err != nil {
			// A mode this model cannot have is no reason to restart: the
			// restart would come up in default mode, and end the process's
			// background tasks.
			if !modeUnavailable(err) || !e.fallBack(sessionID, res, have, want) {
				return false
			}
			return true
		}
		have.mode = want.mode
		e.setLaunch(sessionID, have)
	}
	return true
}

// modeFallback is the mode a thread runs in when the one it asked for cannot
// be had. Claude Code's own answer to an auto mode the model does not have
// is default mode, which asks for every edit; acceptEdits is the closest
// mode that does not.
var modeFallback = map[string]string{"auto": "acceptEdits"}

func refusal(st launchState) string { return st.mode + "\x00" + st.model }

// modeUnavailable reports whether err is Claude Code saying the mode cannot
// be had on this process ("auto mode unavailable for this model").
func modeUnavailable(err error) bool {
	var rejected *agentboot.ControlRejectedError
	return errors.As(err, &rejected) && strings.Contains(rejected.Message, "unavailable")
}

// fallBack moves res from the mode want asked for to its fallback and
// remembers that, so the thread's transcript says why it prompts more than
// its setting suggests. It reports false if there is no fallback or it could
// not be set.
func (e *Engine) fallBack(sessionID string, res liveController, have, want launchState) bool {
	fb, ok := modeFallback[want.mode]
	if !ok {
		return false
	}
	ctx, cancel := context.WithTimeout(context.Background(), liveControlTimeout)
	defer cancel()
	if err := res.SetPermissionMode(ctx, fb); err != nil {
		return false
	}
	have.mode, have.refused = fb, refusal(want)
	e.setLaunch(sessionID, have)
	note := want.mode + " mode is not available here (the model does not support it, or settings turn it off)"
	if want.model != "" {
		note = want.mode + " mode is not available here (model " + want.model + " does not support it, or settings turn it off)"
	}
	e.appendSystem(sessionID, note+": running in "+fb+" instead")
	return true
}

// checkInitMode compares the mode a turn's init message reports with the one
// the process was asked for. Claude Code starts an auto mode it cannot have
// in default mode without a word; it is moved to the fallback mode.
func (e *Engine) checkInitMode(sessionID string, raw any) {
	m, ok := raw.(*claude.SystemMessage)
	if !ok || m.SubType != claude.SystemSubtypeInit {
		return
	}
	got, _ := m.Raw["permissionMode"].(string)
	e.mu.Lock()
	have, r := e.launch[sessionID], e.residents[sessionID]
	e.mu.Unlock()
	if r == nil || got != "default" || have.refused != "" || modeFallback[have.mode] == "" {
		return
	}
	// A control request waits for the reader this is called from.
	go e.fallBack(sessionID, r.c, have, have)
}

func (e *Engine) setLaunch(sessionID string, st launchState) {
	e.mu.Lock()
	e.launch[sessionID] = st
	e.mu.Unlock()
}

// liveSettingsKeys are the top-level settings keys a running process takes
// without a restart, through an apply_flag_settings control request: the
// permission rules. The rest of a settings file (env, model, hooks, ...) is
// read once when the process starts.
var liveSettingsKeys = []string{"permissions", "autoMode"}

// splitSettings reads the settings file at path and separates the permission
// rules a running process takes live from the rest. rest is what only a
// restart changes: the file's other keys as canonical JSON, or its raw bytes
// when it is not a JSON object (nil if it cannot be read). live is the rules as
// canonical JSON, "" if there are none.
func splitSettings(path string) (rest []byte, live string) {
	if path == "" {
		return nil, ""
	}
	raw, err := os.ReadFile(path)
	if err != nil {
		return nil, ""
	}
	var doc map[string]json.RawMessage
	if json.Unmarshal(raw, &doc) != nil {
		return raw, ""
	}
	rules := map[string]json.RawMessage{}
	for _, k := range liveSettingsKeys {
		if v, ok := doc[k]; ok {
			rules[k] = v
			delete(doc, k)
		}
	}
	rest, _ = json.Marshal(doc)
	if len(rules) > 0 {
		b, _ := json.Marshal(rules)
		live = string(b)
	}
	return rest, live
}

// liveSettings is the permission rules of opts' settings file, as a running
// process takes them live ("" if there are none). The process reads the file
// only at its start, so this is what it has been given once it started, and
// what applyLive pushes when it changes.
func liveSettings(opts agentboot.ExecutionOptions) string {
	_, live := splitSettings(opts.SettingsPath)
	return live
}

// liveDelta is the settings to send to take a process from have to want
// (both from liveSettings). A key replaces what was sent for it before, and a
// key that is gone is sent empty. The process keeps the rules it started
// with underneath, so a rule that was in its file at start cannot be taken
// back before it restarts.
func liveDelta(have, want string) map[string]any {
	parse := func(s string) map[string]json.RawMessage {
		m := map[string]json.RawMessage{}
		_ = json.Unmarshal([]byte(s), &m)
		return m
	}
	h, w := parse(have), parse(want)
	out := map[string]any{}
	for k, v := range w {
		out[k] = v
	}
	for k := range h {
		if _, still := w[k]; !still {
			out[k] = map[string]any{}
		}
	}
	return out
}

// launchSignature captures the launch-time settings a persistent process
// cannot change once started. Model, permission mode and the settings file's
// permission rules are not in it: they are changed live (applyLive), and a
// change to them must never restart a process that may be running background
// tasks. Env is sorted because hosts often build it from a map. A settings
// file keeps its path when its content is edited (a profile, say), so the
// rest of its content is part of the signature, not just the path.
func launchSignature(opts agentboot.ExecutionOptions) string {
	env := append([]string(nil), opts.Env...)
	sort.Strings(env)
	settings := opts.SettingsPath
	if rest, _ := splitSettings(settings); rest != nil {
		sum := sha256.Sum256(rest)
		settings += "@" + hex.EncodeToString(sum[:])
	}
	mcpConfig, _ := json.Marshal(opts.MCPServers)
	if opts.MCPConfigPath != "" {
		if b, err := os.ReadFile(opts.MCPConfigPath); err == nil {
			mcpConfig = append(mcpConfig, b...)
		}
	}
	mcpHash := sha256.Sum256(mcpConfig)
	return opts.Entrypoint + "\x00" + settings + "\x00" + opts.Effort + "\x00" + strings.Join(env, "\x00") + "\x00" + hex.EncodeToString(mcpHash[:])
}
