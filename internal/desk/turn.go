package desk

import (
	"context"
	"fmt"

	"github.com/tingly-dev/tingly-box/agentboot"
	"github.com/tingly-dev/tingly-box/agentboot/engine"
	"github.com/tingly-dev/tingly-box/remote/session"
)

// turnSettings are the per-session choices a turn launches with.
type turnSettings struct {
	PermissionMode string
	Profile        string
	Model          string
}

func settingsOf(sess session.Session) turnSettings {
	return turnSettings{PermissionMode: sess.PermissionMode, Profile: sess.Profile, Model: sess.Model}
}

// startTurn claims the session for one turn and runs it in the background
// (engine.Start). Returns false, and starts nothing, if the session already
// has a turn in flight.
func (s *Service) startTurn(sessionID, projectPath, prompt string, ts turnSettings, resume bool) bool {
	return s.engine.Start(sessionID, projectPath, prompt, resume, func(ctx context.Context) agentboot.ExecutionOptions {
		return s.launchOptions(ctx, sessionID, ts)
	})
}

// launchOptions resolves a turn's gateway routing. A profile's settings
// file carries its own routing, so it replaces the main scenario's env
// rather than adding to it (the same either/or @cc's ClaudeCodeExecutor
// uses).
func (s *Service) launchOptions(ctx context.Context, sessionID string, ts turnSettings) agentboot.ExecutionOptions {
	var execEnv []string
	var settingsPath string
	if s.routing != nil {
		if ts.Profile != "" {
			if p, err := s.routing.GetClaudeCodeSettingsPathForProfile(ctx, ts.Profile); err == nil {
				settingsPath = p
			} else {
				s.appendSystem(sessionID, fmt.Sprintf("profile %s unavailable, running with the default routing: %v", ts.Profile, err))
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
	return agentboot.ExecutionOptions{
		PermissionMode: ts.PermissionMode,
		Model:          ts.Model,
		Env:            execEnv,
		SettingsPath:   settingsPath,
	}
}

// deskStore is the engine's view of the session store @cc shares.
type deskStore struct{ s *Service }

func (d deskStore) SetRunning(id string) bool { return d.s.sessions.SetRunning(id) }
func (d deskStore) SetCompleted(id, response string) bool {
	return d.s.sessions.SetCompleted(id, response)
}
func (d deskStore) SetFailed(id, errMsg string) bool { return d.s.sessions.SetFailed(id, errMsg) }

func (d deskStore) SetInterrupted(id string) {
	d.s.sessions.Update(id, func(sess *session.Session) {
		sess.Status, sess.Error = session.StatusCompleted, ""
	})
}

func (d deskStore) Append(id string, e engine.Entry) {
	m := session.Message{Role: e.Role, Content: e.Content, Kind: e.Kind, RequestID: e.RequestID, Payload: e.Payload, Parent: e.Parent, Timestamp: e.Timestamp}
	d.s.sessions.AppendMessage(id, m)
	d.s.noteUsage(id, m)
}
