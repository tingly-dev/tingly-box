package desk

import (
	"context"
	"errors"
	"fmt"
	"time"

	"github.com/tingly-dev/tingly-box/agentboot"
	"github.com/tingly-dev/tingly-box/remote/session"
)

// turnTimeout bounds one turn. Coding tasks run far longer than a chat
// reply, so this is looser than a plain chat default.
const turnTimeout = 2 * time.Hour

// startTurn claims the session for one turn and runs it in the background.
// Returns false (and starts nothing) if the session already has a turn in
// flight — the caller has already decided that should not happen, but the
// claim is atomic here to close the race between two requests for the same
// session arriving together.
// turnSettings are the per-session choices a turn launches with.
type turnSettings struct {
	PermissionMode string
	Profile        string
	Model          string
}

func settingsOf(sess session.Session) turnSettings {
	return turnSettings{PermissionMode: sess.PermissionMode, Profile: sess.Profile, Model: sess.Model}
}

func (s *Service) startTurn(sessionID, projectPath, prompt string, ts turnSettings, resume bool) bool {
	turnCtx, cancel := context.WithTimeout(context.Background(), turnTimeout)
	prompter := newWebPrompter(sessionID, s.sessions)
	done := make(chan struct{})

	s.mu.Lock()
	if _, busy := s.runs[sessionID]; busy {
		s.mu.Unlock()
		cancel()
		return false
	}
	s.runs[sessionID] = &run{cancel: cancel, prompter: prompter, done: done}
	s.mu.Unlock()

	// Only the winner of the claim records the message, and the session
	// reads as running before this returns: the caller re-reads it right
	// away, and a status still showing the previous turn's outcome would
	// tell the page there is nothing to poll for.
	s.appendUserMessage(sessionID, prompt)
	s.sessions.SetRunning(sessionID)

	go s.runTurn(turnCtx, sessionID, projectPath, prompt, ts, resume, prompter, cancel, done)
	return true
}

func (s *Service) runTurn(ctx context.Context, sessionID, projectPath, prompt string, ts turnSettings, resume bool, webPrompt *webPrompter, cancel context.CancelFunc, done chan struct{}) {
	permissionMode, profile := ts.PermissionMode, ts.Profile
	defer func() {
		s.mu.Lock()
		delete(s.runs, sessionID)
		s.mu.Unlock()
		cancel()
		close(done)
	}()

	// A profile's settings file carries its own gateway routing, so it
	// replaces the main scenario's env rather than adding to it (the same
	// either/or @cc's ClaudeCodeExecutor uses).
	var execEnv []string
	var settingsPath string
	if s.routing != nil {
		if profile != "" {
			if p, err := s.routing.GetClaudeCodeSettingsPathForProfile(ctx, profile); err == nil {
				settingsPath = p
			} else {
				s.appendSystem(sessionID, fmt.Sprintf("profile %s unavailable, running with the default routing: %v", profile, err))
			}
		}
		if settingsPath == "" {
			if env, err := s.routing.GetClaudeCodeEnv(ctx); err == nil {
				execEnv = env
			} else {
				s.appendSystem(sessionID, "gateway routing unavailable, running with host defaults: "+err.Error())
			}
		}
	}

	var prompter agentboot.Prompter = webPrompt
	if autoApproves(permissionMode) {
		prompter = autoApprovePrompter{inner: webPrompt}
	}

	opts := agentboot.ExecutionOptions{
		SessionID:            sessionID,
		Resume:               resume,
		PermissionPromptTool: "stdio",
		PermissionMode:       permissionMode,
		Model:                ts.Model,
		Env:                  execEnv,
		SettingsPath:         settingsPath,
	}

	var werr error
	if s.pool != nil {
		var handled bool
		if werr, handled = s.runPersistentTurn(ctx, sessionID, projectPath, prompt, opts, prompter); handled {
			s.finishTurn(ctx, sessionID, werr)
			return
		}
	}

	opts.Store = s.sessions
	conv := newConverter()
	sink := func(raw any) { s.record(sessionID, conv, raw) }
	_, werr = s.agent.Run(ctx, agentboot.RunRequest{ProjectPath: projectPath, Prompt: prompt, Opts: opts}, prompter, sink)
	// A one-shot process exits with its turn, and its background tasks
	// with it.
	s.clearTasks(sessionID)
	s.finishTurn(ctx, sessionID, werr)
}

// runPersistentTurn drives one turn through a long-lived Claude Code process
// kept in s.pool, mirroring remoteagent.ClaudeCodeExecutor's own
// runPersistentTurn (.design/claude-code.md §5.3) — same pool package, same
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
func (s *Service) runPersistentTurn(ctx context.Context, sessionID, projectPath, prompt string, opts agentboot.ExecutionOptions, prompter agentboot.Prompter) (werr error, handled bool) {
	sig := launchSignature(opts)
	persistentSession, found := s.pool.Acquire(sessionID)
	res, conducted := s.residentFor(sessionID)
	if found && (!conducted || res.c.Session() != persistentSession) {
		// A pooled process nothing reads from can't be driven safely.
		s.evictPersistent(sessionID)
		found = false
	}
	if found {
		s.mu.Lock()
		stale := s.launch[sessionID] != sig
		s.mu.Unlock()
		if stale {
			// Permission mode, model or gateway settings changed since this
			// process started; none can be changed on a live process, so
			// restart it (Open resumes the same Claude session via
			// opts.Resume).
			s.evictPersistent(sessionID)
			found = false
		}
	}

	s.sessions.SetRunning(sessionID)
	if !found {
		opened, operr := s.agent.Open(ctx, "", projectPath, prompt, opts)
		if operr != nil {
			return nil, false
		}
		if perr := s.pool.Open(ctx, sessionID, opened); perr != nil {
			closeCtx, cancel := context.WithTimeout(context.Background(), agentboot.SessionCloseTimeout)
			_ = opened.Close(closeCtx)
			cancel()
			return nil, false
		}
		s.mu.Lock()
		s.launch[sessionID] = sig
		s.mu.Unlock()
		res = s.newResident(sessionID, opened)
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
			s.appendSystem(sessionID, "not sent: Claude is following up on a finished background task; send it again when that's done")
			return nil, true
		case errors.Is(werr, agentboot.ErrSessionClosed):
			// The process ended before the message reached it: nothing was
			// sent, so a one-shot turn can still take it.
			s.pool.Remove(sessionID)
			return nil, false
		}
	}

	if werr != nil && res.c.Session().Status() == agentboot.SessionStateTerminated {
		// A crash mid-turn: drop the pool's stale bookkeeping now rather
		// than waiting for the next Acquire to self-heal it.
		s.pool.Remove(sessionID)
	} else {
		s.pool.Touch(sessionID)
	}
	if werr == nil {
		s.sessions.SetCompleted(sessionID, "")
	} else {
		s.sessions.SetFailed(sessionID, werr.Error())
	}
	return werr, true
}

// finishTurn applies the two corrections both the one-shot and persistent
// paths need. First, a turn ended by our own Interrupt is not a failure. The CLI
// process (or, for a persistent session, the whole session —
// RunTurnWithPrompter closes it on ctx cancellation) was stopped from
// outside its own logic, and the session stays resumable. Whichever path
// just ran already called SetFailed with the "context canceled" text, so it
// is corrected here rather than left to read as a real error. Second, a
// turn that failed before its process started is recorded as failed.
func (s *Service) finishTurn(ctx context.Context, sessionID string, werr error) {
	if werr == nil {
		return
	}
	if ctx.Err() == context.Canceled {
		s.sessions.Update(sessionID, func(sess *session.Session) {
			sess.Status, sess.Error = session.StatusCompleted, ""
		})
		s.appendSystem(sessionID, "interrupted; send a message to resume")
		return
	}
	// Both paths record the outcome of any turn whose process started (the
	// runner inside Wait, the persistent path itself). Still running here
	// means it failed before that (CLI not found, launch spec, transport),
	// which nothing has recorded or shown yet.
	if snap, ok := s.sessions.Snapshot(sessionID); ok && snap.Status == session.StatusRunning {
		s.sessions.SetFailed(sessionID, werr.Error())
		s.sessions.AppendMessage(sessionID, session.Message{Kind: "error", Content: werr.Error(), Timestamp: time.Now()})
	}
}
