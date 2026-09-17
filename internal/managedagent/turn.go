package managedagent

import (
	"context"
	"time"

	"github.com/tingly-dev/tingly-box/agentboot"
	"github.com/tingly-dev/tingly-box/remote/session"
)

// turnTimeout bounds one turn. Coding tasks run far longer than a chat
// reply, so this is looser than a plain chat default (matches @cc's own
// managed-agent-flavoured executions).
const turnTimeout = 2 * time.Hour

// startTurn claims the session for one turn and runs it in the background.
// Returns false (and starts nothing) if the session already has a turn in
// flight — the caller has already decided that should not happen, but the
// claim is atomic here to close the race between two requests for the same
// session arriving together.
func (s *Service) startTurn(sessionID, projectPath, prompt, permissionMode string, resume bool) bool {
	turnCtx, cancel := context.WithTimeout(context.Background(), turnTimeout)
	prompter := newWebPrompter(sessionID, s.sessions)

	s.mu.Lock()
	if _, busy := s.runs[sessionID]; busy {
		s.mu.Unlock()
		cancel()
		return false
	}
	s.runs[sessionID] = &run{cancel: cancel, prompter: prompter}
	s.mu.Unlock()

	go s.runTurn(turnCtx, sessionID, projectPath, prompt, permissionMode, resume, prompter, cancel)
	return true
}

func (s *Service) runTurn(ctx context.Context, sessionID, projectPath, prompt, permissionMode string, resume bool, webPrompt *webPrompter, cancel context.CancelFunc) {
	defer func() {
		s.mu.Lock()
		delete(s.runs, sessionID)
		s.mu.Unlock()
		cancel()
	}()

	var execEnv []string
	if s.routing != nil {
		if env, err := s.routing.GetClaudeCodeEnv(ctx); err == nil {
			execEnv = env
		} else {
			s.appendSystem(sessionID, "gateway routing unavailable, running with host defaults: "+err.Error())
		}
	}

	var prompter agentboot.Prompter = webPrompt
	if autoApproves(permissionMode) {
		prompter = autoApprovePrompter{inner: webPrompt}
	}

	conv := newConverter()
	sink := func(raw any) {
		if ev, ok := raw.(agentboot.ErrorEvent); ok {
			if ev.Err != nil {
				s.sessions.AppendMessage(sessionID, session.Message{Kind: "error", Content: ev.Err.Error(), Timestamp: time.Now()})
			}
			return
		}
		for _, m := range conv.messages(raw) {
			s.sessions.AppendMessage(sessionID, m)
		}
	}

	_, werr := s.agent.Run(ctx, agentboot.RunRequest{
		ProjectPath: projectPath,
		Prompt:      prompt,
		Opts: agentboot.ExecutionOptions{
			SessionID:            sessionID,
			Resume:               resume,
			PermissionPromptTool: "stdio",
			PermissionMode:       permissionMode,
			Env:                  execEnv,
			Store:                s.sessions,
		},
	}, prompter, sink)

	// A turn ended by our own Interrupt is not a failure: the CLI process
	// was cancelled from outside its own logic, and the session stays
	// resumable. AgentService.Run's Store hook already called SetFailed
	// with the "context canceled" text by the time we see this, so it is
	// corrected here rather than left to read as a real error.
	if werr != nil && ctx.Err() == context.Canceled {
		s.sessions.Update(sessionID, func(sess *session.Session) {
			sess.Status, sess.Error = session.StatusCompleted, ""
		})
		s.appendSystem(sessionID, "interrupted; send a message to resume")
	}
}
