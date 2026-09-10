package managedagent

import (
	"context"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"time"

	"github.com/sirupsen/logrus"
)

// DefaultWorkspaceTTL is how long a workspace with no active session is kept
// before its checkout is removed. The branch was pushed (or the user chose
// not to); the session log and index survive either way (§10 done ≠ locked).
const DefaultWorkspaceTTL = 7 * 24 * time.Hour

// recoveryListLimit bounds the startup scan; anything beyond it is not a
// realistic single-host session count.
const recoveryListLimit = 100000

// RecoverOnStart reconciles sessions that were live when the previous
// process died. Runs (and their Claude Code processes) do not survive a
// restart, so:
//
//   - running / waiting_input → idle, with a status event saying why. The
//     Claude Code session id is kept, so the next message resumes it.
//   - queued → started again through the Launcher (the prompt is already in
//     the log; provisioning restarts from scratch if it was cut short).
func (s *Service) RecoverOnStart(ctx context.Context) error {
	// Every active session, not the list page's default cap.
	active, err := s.stores.Sessions.ListSessions(ctx, SessionFilter{Active: true, Limit: recoveryListLimit})
	if err != nil {
		return err
	}
	var errs []error
	for i := range active {
		sess := &active[i]
		switch sess.Status {
		case SessionRunning, SessionWaitingInput:
			now := s.now()
			sess.Status, sess.LastActiveAt = SessionIdle, now
			if err := s.stores.Sessions.UpdateSession(ctx, sess); err != nil {
				errs = append(errs, err)
				continue
			}
			_ = s.stores.Events.AppendEvent(ctx, &Event{SessionID: sess.ID, Kind: EventStatus,
				Text: string(SessionIdle) + ": interrupted by restart; send a message to resume", At: now})
		case SessionQueued:
			if s.launcher == nil {
				continue
			}
			ws, err := s.stores.Workspaces.GetWorkspace(ctx, sess.WorkspaceID)
			if err != nil {
				errs = append(errs, err)
				continue
			}
			env, err := s.stores.Environments.GetEnvironment(ctx, ws.EnvironmentID)
			if err != nil {
				errs = append(errs, err)
				continue
			}
			src, err := s.stores.Sources.GetSource(ctx, ws.SourceID)
			if err != nil {
				errs = append(errs, err)
				continue
			}
			if err := s.launcher.Start(ctx, Run{Session: sess, Workspace: ws, Environment: env, Source: src}); err != nil {
				sess.Status, sess.Error = SessionFailed, "restart: "+err.Error()
				_ = s.stores.Sessions.UpdateSession(ctx, sess)
				errs = append(errs, err)
			}
		}
	}
	return errors.Join(errs...)
}

// ListWorkspaces lists checkouts, most recently active first.
func (s *Service) ListWorkspaces(ctx context.Context, f WorkspaceFilter) ([]Workspace, error) {
	return s.stores.Workspaces.ListWorkspaces(ctx, f)
}

// ReclaimWorkspace removes a checkout's directory and marks it reclaimed.
// It refuses while any session in it is still active; sessions are not
// touched (their log and index remain readable), only the directory goes.
func (s *Service) ReclaimWorkspace(ctx context.Context, id string) (*Workspace, error) {
	ws, err := s.stores.Workspaces.GetWorkspace(ctx, id)
	if err != nil {
		return nil, err
	}
	if ws.State == WorkspaceReclaimed {
		return ws, nil
	}
	live, err := s.stores.Sessions.ListSessions(ctx, SessionFilter{WorkspaceID: ws.ID, Active: true})
	if err != nil {
		return nil, err
	}
	if len(live) > 0 {
		return nil, conflict("workspace has %d active session(s); archive them first", len(live))
	}
	if !s.ownsPath(ws) {
		// A user's own directory (local source) is never deleted; the
		// workspace record is simply retired.
		if ws.Path != "" && ws.Path != filepath.Clean(s.workspacesDir) {
			logrus.WithField("workspace", ws.ID).Info("managed agent: retiring in-place workspace without deleting it")
		}
	} else if err := os.RemoveAll(ws.Path); err != nil {
		return nil, fmt.Errorf("remove checkout: %w", err)
	}
	ws.State, ws.LastActiveAt = WorkspaceReclaimed, s.now()
	if err := s.stores.Workspaces.UpdateWorkspace(ctx, ws); err != nil {
		return nil, err
	}
	return ws, nil
}

// ReclaimIdleWorkspaces reclaims every ready or failed workspace whose
// sessions are all inactive and whose last activity is older than ttl.
// Returns how many were reclaimed.
func (s *Service) ReclaimIdleWorkspaces(ctx context.Context, ttl time.Duration) (int, error) {
	all, err := s.stores.Workspaces.ListWorkspaces(ctx, WorkspaceFilter{})
	if err != nil {
		return 0, err
	}
	cutoff := s.now().Add(-ttl)
	n := 0
	for i := range all {
		ws := &all[i]
		if ws.State == WorkspaceReclaimed || ws.State == WorkspaceProvisioning || !s.ownsPath(ws) {
			continue
		}
		// A workspace's activity is its sessions' activity.
		sessions, err := s.stores.Sessions.ListSessions(ctx, SessionFilter{WorkspaceID: ws.ID})
		if err != nil {
			return n, err
		}
		last := ws.LastActiveAt
		active := false
		for _, sess := range sessions {
			if sess.Status.IsActive() {
				active = true
				break
			}
			if sess.LastActiveAt.After(last) {
				last = sess.LastActiveAt
			}
		}
		if active || !last.Before(cutoff) {
			continue
		}
		if _, err := s.ReclaimWorkspace(ctx, ws.ID); err != nil {
			logrus.WithError(err).WithField("workspace", ws.ID).Warn("managed agent: reclaim failed")
			continue
		}
		n++
	}
	return n, nil
}

// RunMaintenance recovers once, then sweeps idle workspaces every interval
// until ctx ends. Meant to run on its own goroutine from the server.
func (s *Service) RunMaintenance(ctx context.Context, ttl, interval time.Duration) {
	if err := s.RecoverOnStart(ctx); err != nil {
		logrus.WithError(err).Warn("managed agent: recovery finished with errors")
	}
	if interval <= 0 {
		return
	}
	ticker := time.NewTicker(interval)
	defer ticker.Stop()
	for {
		if n, err := s.ReclaimIdleWorkspaces(ctx, ttl); err != nil {
			logrus.WithError(err).Warn("managed agent: workspace sweep failed")
		} else if n > 0 {
			logrus.WithField("reclaimed", n).Info("managed agent: reclaimed idle workspaces")
		}
		select {
		case <-ctx.Done():
			return
		case <-ticker.C:
		}
	}
}

// Shutdown stops every live run so no agent process outlives the server.
// Sessions are left as they are; RecoverOnStart reconciles them on the
// next start (running → idle, resumable).
func (s *Service) Shutdown(ctx context.Context) {
	if s.launcher == nil {
		return
	}
	active, err := s.stores.Sessions.ListSessions(ctx, SessionFilter{Active: true, Limit: recoveryListLimit})
	if err != nil {
		logrus.WithError(err).Warn("managed agent: shutdown could not list sessions")
		return
	}
	for i := range active {
		if active[i].Status == SessionRunning || active[i].Status == SessionWaitingInput {
			_ = s.launcher.Stop(ctx, active[i].ID)
		}
	}
}
